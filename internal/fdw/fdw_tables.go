package fdw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// ListServers returns all tracked FDW servers with their configuration retrieved from the _ayb_fdw_servers table.
func (s *Service) ListServers(ctx context.Context) ([]ForeignServer, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT name, fdw_type, options, created_at
		FROM _ayb_fdw_servers
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query fdw servers: %w", err)
	}
	defer rows.Close()

	servers := make([]ForeignServer, 0)
	for rows.Next() {
		var server ForeignServer
		var optionsJSON []byte
		if err := rows.Scan(&server.Name, &server.FDWType, &optionsJSON, &server.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan fdw server row: %w", err)
		}
		server.Options = map[string]string{}
		if len(optionsJSON) > 0 {
			if err := json.Unmarshal(optionsJSON, &server.Options); err != nil {
				return nil, fmt.Errorf("decode options for server %q: %w", server.Name, err)
			}
		}
		servers = append(servers, server)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fdw servers: %w", err)
	}
	return servers, nil
}

// ImportTables executes IMPORT FOREIGN SCHEMA and returns the imported foreign tables.
func (s *Service) ImportTables(ctx context.Context, serverName string, opts ImportOpts) ([]ForeignTable, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	if err := ValidateIdentifier(serverName); err != nil {
		return nil, err
	}

	remoteSchema, localSchema, err := normalizeImportSchemas(opts)
	if err != nil {
		return nil, err
	}
	if err := validateImportTableNames(opts.TableNames); err != nil {
		return nil, err
	}

	importSQL := buildImportSchemaSQL(serverName, remoteSchema, localSchema, opts.TableNames)
	if _, err := s.db.Exec(ctx, importSQL); err != nil {
		return nil, fmt.Errorf("import foreign schema from server %q: %w", serverName, err)
	}

	return s.listForeignTablesQuery(ctx, serverName, localSchema, opts.TableNames)
}

func normalizeImportSchemas(opts ImportOpts) (string, string, error) {
	remoteSchema := opts.RemoteSchema
	if strings.TrimSpace(remoteSchema) == "" {
		remoteSchema = "public"
	}
	localSchema := opts.LocalSchema
	if strings.TrimSpace(localSchema) == "" {
		localSchema = "public"
	}

	if err := ValidateIdentifier(remoteSchema); err != nil {
		return "", "", fmt.Errorf("invalid remote schema: %w", err)
	}
	if err := ValidateIdentifier(localSchema); err != nil {
		return "", "", fmt.Errorf("invalid local schema: %w", err)
	}
	return remoteSchema, localSchema, nil
}

func validateImportTableNames(tableNames []string) error {
	for _, table := range tableNames {
		if err := ValidateIdentifier(table); err != nil {
			return fmt.Errorf("invalid table name in filter %q: %w", table, err)
		}
	}
	return nil
}

// buildImportSchemaSQL constructs an IMPORT FOREIGN SCHEMA SQL statement, optionally limiting the import to specified table names.
func buildImportSchemaSQL(serverName, remoteSchema, localSchema string, tableNames []string) string {
	var limitClause string
	if len(tableNames) > 0 {
		quotedTables := make([]string, 0, len(tableNames))
		for _, table := range tableNames {
			quotedTables = append(quotedTables, sqlutil.QuoteIdent(table))
		}
		limitClause = " LIMIT TO (" + strings.Join(quotedTables, ", ") + ")"
	}
	return fmt.Sprintf(
		`IMPORT FOREIGN SCHEMA %s%s FROM SERVER %s INTO %s`,
		sqlutil.QuoteIdent(remoteSchema),
		limitClause,
		sqlutil.QuoteIdent(serverName),
		sqlutil.QuoteIdent(localSchema),
	)
}

func (s *Service) ListForeignTables(ctx context.Context) ([]ForeignTable, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	return s.listForeignTablesQuery(ctx, "", "", nil)
}

// listForeignTablesQuery queries the information schema to retrieve foreign tables.
func (s *Service) listForeignTablesQuery(ctx context.Context, serverName, schemaName string, onlyTables []string) ([]ForeignTable, error) {
	query := `
		SELECT
			ft.foreign_table_schema,
			ft.foreign_table_name,
			ft.foreign_server_name,
			c.column_name,
			c.udt_name
		FROM information_schema.foreign_tables ft
		JOIN information_schema.columns c
			ON c.table_schema = ft.foreign_table_schema
			AND c.table_name = ft.foreign_table_name
		WHERE ($1 = '' OR ft.foreign_server_name = $1)
			AND ($2 = '' OR ft.foreign_table_schema = $2)
		ORDER BY ft.foreign_table_schema, ft.foreign_table_name, c.ordinal_position
	`
	rows, err := s.db.Query(ctx, query, serverName, schemaName)
	if err != nil {
		return nil, fmt.Errorf("query foreign tables: %w", err)
	}
	defer rows.Close()

	filter := make(map[string]struct{}, len(onlyTables))
	for _, name := range onlyTables {
		filter[name] = struct{}{}
	}

	out := make([]ForeignTable, 0)
	indexByKey := make(map[string]int)
	for rows.Next() {
		var schema string
		var table string
		var server string
		var col string
		var typ string
		if err := rows.Scan(&schema, &table, &server, &col, &typ); err != nil {
			return nil, fmt.Errorf("scan foreign table row: %w", err)
		}

		if len(filter) > 0 {
			if _, ok := filter[table]; !ok {
				continue
			}
		}

		key := schema + "." + table
		idx, exists := indexByKey[key]
		if !exists {
			idx = len(out)
			indexByKey[key] = idx
			out = append(out, ForeignTable{
				Schema:     schema,
				Name:       table,
				ServerName: server,
				Columns:    []ForeignColumn{},
				Options:    map[string]string{},
			})
		}
		out[idx].Columns = append(out[idx].Columns, ForeignColumn{
			Name: col,
			Type: typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign table rows: %w", err)
	}
	return out, nil
}

// DropForeignTable drops a foreign table by schema and name.
func (s *Service) DropForeignTable(ctx context.Context, schemaName, tableName string) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	if err := ValidateIdentifier(schemaName); err != nil {
		return fmt.Errorf("invalid schema name: %w", err)
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	sql := fmt.Sprintf(`DROP FOREIGN TABLE IF EXISTS %s`, sqlutil.QuoteQualifiedName(schemaName, tableName))
	if _, err := s.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("drop foreign table %s.%s: %w", schemaName, tableName, err)
	}
	return nil
}
