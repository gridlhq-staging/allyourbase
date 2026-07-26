package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// internalTablePrefixes lists legacy public-table prefixes to skip.
var internalTablePrefixes = []string{
	"_supabase_",
	"_realtime_",
	"_analytics_",
	"_pgsodium_",
	"_prisma_",
}

// internalTableNames lists exact public table names known to be managed by Supabase.
var internalTableNames = map[string]struct{}{
	"schema_migrations":       {},
	"supabase_migrations":     {},
	"buckets_vectors":         {},
	"vector_indexes":          {},
	"storage.buckets_vectors": {},
	"storage.vector_indexes":  {},
}

// isInternalTable returns true if the table name belongs to a Supabase internal system.
func isInternalTable(name string) bool {
	if _, ok := internalTableNames[name]; ok {
		return true
	}
	for _, prefix := range internalTablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isAYBTable returns true if the table name belongs to AYB's internal tables.
func isAYBTable(name string) bool {
	return strings.HasPrefix(name, "_ayb_")
}

func isAdmittedUserTable(schema, name string) bool {
	if !isAdmittedUserSchema(schema) {
		return false
	}
	if schema != "public" {
		return true
	}
	return !isInternalTable(name) && !isAYBTable(name)
}

// introspectTables queries information_schema for user-owned schema tables,
// skipping Supabase internals and AYB system tables.
func introspectTables(ctx context.Context, db *sql.DB) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		ORDER BY table_schema, table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tables: %w", err)
	}
	defer rows.Close()

	type tableRef struct {
		schema string
		name   string
	}
	var tableNames []tableRef
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, fmt.Errorf("scanning table name: %w", err)
		}
		if !isAdmittedUserTable(schema, name) {
			continue
		}
		tableNames = append(tableNames, tableRef{schema: schema, name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var tables []TableInfo
	for _, table := range tableNames {
		ti, err := introspectTable(ctx, db, table.schema, table.name)
		if err != nil {
			return nil, fmt.Errorf("introspecting table %s: %w", schemaQualifiedName(table.schema, table.name), err)
		}
		tables = append(tables, *ti)
	}

	return tables, nil
}

// introspectTable gets detailed column/constraint info for a single table.
func introspectTable(ctx context.Context, db *sql.DB, schemaName, tableName string) (*TableInfo, error) {
	ti := &TableInfo{SchemaName: schemaName, Name: tableName}

	// Columns.
	colRows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, ''), ordinal_position
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, schemaName, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying columns: %w", err)
	}
	defer colRows.Close()

	for colRows.Next() {
		var c ColumnInfo
		var nullable string
		if err := colRows.Scan(&c.Name, &c.DataType, &nullable, &c.DefaultValue, &c.OrdinalPos); err != nil {
			return nil, fmt.Errorf("scanning column: %w", err)
		}
		c.IsNullable = nullable == "YES"
		ti.Columns = append(ti.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	// Primary key. Composite keys are intentionally left empty because the
	// DDL generator only supports recreating a single-column primary key.
	err = db.QueryRowContext(ctx, `
		SELECT CASE WHEN COUNT(*) = 1 THEN MIN(a.attname) ELSE '' END
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
	`, ti.quotedName()).Scan(&ti.PrimaryKey)
	if err != nil {
		return nil, fmt.Errorf("querying primary key: %w", err)
	}

	// Foreign keys.
	fkRows, err := db.QueryContext(ctx, `
		SELECT tc.constraint_name, kcu.column_name,
		       ccu.table_schema AS ref_schema,
		       ccu.table_name AS ref_table, ccu.column_name AS ref_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.constraint_schema = kcu.constraint_schema
		 AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		 AND tc.constraint_schema = ccu.constraint_schema
		WHERE tc.table_schema = $1
		  AND tc.table_name = $2
		  AND tc.constraint_type = 'FOREIGN KEY'
		ORDER BY tc.constraint_name
	`, schemaName, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var fk ForeignKeyInfo
		if err := fkRows.Scan(&fk.ConstraintName, &fk.ColumnName, &fk.RefSchemaName, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, fmt.Errorf("scanning foreign key: %w", err)
		}
		ti.ForeignKeys = append(ti.ForeignKeys, fk)
	}
	if err := fkRows.Err(); err != nil {
		return nil, err
	}

	sequences, err := introspectTableSequences(ctx, db, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	ti.Sequences = sequences

	// Row count (approximate is fine for pre-flight; exact for small tables).
	err = db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, ti.quotedName())).Scan(&ti.RowCount)
	if err != nil {
		return nil, fmt.Errorf("counting rows: %w", err)
	}

	return ti, nil
}

func introspectTableSequences(ctx context.Context, db *sql.DB, schemaName, tableName string) ([]SequenceInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT seq_ns.nspname, seq.relname, tbl_ns.nspname, tbl.relname, col.attname
		FROM pg_class seq
		JOIN pg_namespace seq_ns ON seq_ns.oid = seq.relnamespace
		JOIN pg_depend dep ON dep.objid = seq.oid
		JOIN pg_class tbl ON tbl.oid = dep.refobjid
		JOIN pg_namespace tbl_ns ON tbl_ns.oid = tbl.relnamespace
		JOIN pg_attribute col ON col.attrelid = tbl.oid AND col.attnum = dep.refobjsubid
		WHERE seq.relkind = 'S'
		  AND dep.deptype IN ('a', 'i')
		  AND tbl_ns.nspname = $1
		  AND tbl.relname = $2
		ORDER BY seq_ns.nspname, seq.relname
	`, schemaName, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying table sequences: %w", err)
	}
	defer rows.Close()

	var sequences []SequenceInfo
	for rows.Next() {
		var seq SequenceInfo
		if err := rows.Scan(
			&seq.SchemaName,
			&seq.Name,
			&seq.TableSchemaName,
			&seq.TableName,
			&seq.ColumnName,
		); err != nil {
			return nil, fmt.Errorf("scanning table sequence: %w", err)
		}
		sequences = append(sequences, seq)
	}
	return sequences, rows.Err()
}

// introspectViews gets user-defined view definitions from user-owned schemas.
func introspectViews(ctx context.Context, db *sql.DB) ([]ViewInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT schemaname, viewname, definition
		FROM pg_views
		ORDER BY schemaname, viewname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying views: %w", err)
	}
	defer rows.Close()

	var views []ViewInfo
	for rows.Next() {
		var v ViewInfo
		if err := rows.Scan(&v.SchemaName, &v.Name, &v.Definition); err != nil {
			return nil, fmt.Errorf("scanning view: %w", err)
		}
		if !isAdmittedUserSchema(v.SchemaName) {
			continue
		}
		views = append(views, v)
	}
	return views, rows.Err()
}

// pgTypeName maps information_schema data types to PostgreSQL DDL type names.
// information_schema uses verbose names like "character varying" while DDL uses "varchar".
func pgTypeName(infoSchemaType string) string {
	switch infoSchemaType {
	case "character varying":
		return "varchar"
	case "character":
		return "char"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	case "time without time zone":
		return "time"
	case "time with time zone":
		return "timetz"
	case "double precision":
		return "float8"
	case "boolean":
		return "bool"
	case "ARRAY":
		return "jsonb" // fallback: arrays become jsonb in the target
	case "USER-DEFINED":
		return "text" // fallback: enums, PostGIS geometry, etc. become text
	default:
		return infoSchemaType
	}
}

// createTableSQL generates a CREATE TABLE DDL statement from a TableInfo.
// This is a pure function with no DB dependencies, easy to unit test.
func createTableSQL(table TableInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS %s (\n", table.quotedName())

	for i, col := range table.Columns {
		typeName := pgTypeName(col.DataType)
		fmt.Fprintf(&sb, "  %s %s", sqlutil.QuoteIdent(col.Name), typeName)
		if !col.IsNullable {
			sb.WriteString(" NOT NULL")
		}
		if col.DefaultValue != "" {
			fmt.Fprintf(&sb, " DEFAULT %s", col.DefaultValue)
		}
		if i < len(table.Columns)-1 || table.PrimaryKey != "" || len(table.ForeignKeys) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	if table.PrimaryKey != "" {
		hasFKs := len(table.ForeignKeys) > 0
		fmt.Fprintf(&sb, "  PRIMARY KEY (%s)", sqlutil.QuoteIdent(table.PrimaryKey))
		if hasFKs {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	for i, fk := range table.ForeignKeys {
		refName := sqlutil.QuoteIdent(fk.RefTable)
		if fk.RefSchemaName != "" {
			refName = sqlutil.QuoteQualifiedName(fk.RefSchemaName, fk.RefTable)
		}
		fmt.Fprintf(&sb, "  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
			sqlutil.QuoteIdent(fk.ConstraintName), sqlutil.QuoteIdent(fk.ColumnName), refName, sqlutil.QuoteIdent(fk.RefColumn))
		if i < len(table.ForeignKeys)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(");")
	return sb.String()
}

// createViewSQL generates a CREATE OR REPLACE VIEW statement.
func createViewSQL(view ViewInfo) string {
	return fmt.Sprintf("CREATE OR REPLACE VIEW %s AS %s", view.quotedName(), view.Definition)
}

func copyTableSelectSQL(table TableInfo) string {
	colList := tableColumnList(table)
	return fmt.Sprintf("SELECT %s FROM %s ORDER BY 1", colList, table.quotedName())
}

func copyTableInsertSQL(table TableInfo) string {
	colList := tableColumnList(table)
	placeholders := make([]string, len(table.Columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		table.quotedName(), colList, strings.Join(placeholders, ", "))
}

func tableColumnList(table TableInfo) string {
	colNames := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		colNames[i] = sqlutil.QuoteIdent(c.Name)
	}
	return strings.Join(colNames, ", ")
}

func createSequenceSQLs(table TableInfo) []string {
	statements := make([]string, 0, len(table.Sequences))
	for _, seq := range table.Sequences {
		statements = append(statements, "CREATE SEQUENCE IF NOT EXISTS "+quotedSequenceName(seq))
	}
	return statements
}

func ownSequenceSQLs(table TableInfo) []string {
	statements := make([]string, 0, len(table.Sequences))
	for _, seq := range table.Sequences {
		ownedTable := TableInfo{SchemaName: seq.TableSchemaName, Name: seq.TableName}
		statements = append(statements, fmt.Sprintf(
			"ALTER SEQUENCE %s OWNED BY %s.%s",
			quotedSequenceName(seq), ownedTable.quotedName(), sqlutil.QuoteIdent(seq.ColumnName),
		))
	}
	return statements
}

func quotedSequenceName(seq SequenceInfo) string {
	return sqlutil.QuoteOptionallyQualifiedName(seq.SchemaName, seq.Name)
}

// copyTableData streams rows from source to target in batches.
// progressFn is called after each batch with the cumulative count.
func copyTableData(ctx context.Context, source *sql.DB, tx *sql.Tx, table TableInfo, progressFn func(int)) (int, error) {
	if len(table.Columns) == 0 {
		return 0, nil
	}

	// SELECT from source.
	selectSQL := copyTableSelectSQL(table)
	rows, err := source.QueryContext(ctx, selectSQL)
	if err != nil {
		return 0, fmt.Errorf("selecting from %s: %w", table.QualifiedName(), err)
	}
	defer rows.Close()

	// Prepare INSERT statement with placeholders.
	insertSQL := copyTableInsertSQL(table)

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, fmt.Errorf("preparing insert for %s: %w", table.QualifiedName(), err)
	}
	defer stmt.Close()

	total := 0
	const batchSize = 1000

	for rows.Next() {
		// Scan into []any.
		vals := make([]any, len(table.Columns))
		ptrs := make([]any, len(table.Columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return total, fmt.Errorf("scanning row from %s: %w", table.QualifiedName(), err)
		}

		result, err := stmt.ExecContext(ctx, vals...)
		if err != nil {
			return total, fmt.Errorf("inserting row into %s: %w", table.QualifiedName(), err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			total++
		}

		if total%batchSize == 0 && progressFn != nil {
			progressFn(total)
		}
	}
	if err := rows.Err(); err != nil {
		return total, err
	}

	// Final progress callback.
	if progressFn != nil {
		progressFn(total)
	}

	return total, nil
}

// resetSequences advances every owned sequence past the copied column values.
func resetSequences(ctx context.Context, tx *sql.Tx, tables []TableInfo) (int, error) {
	count := 0
	for _, table := range tables {
		for _, sequence := range table.Sequences {
			ownedTable := TableInfo{SchemaName: sequence.TableSchemaName, Name: sequence.TableName}
			resetSQL := resetSequenceSQL(ownedTable, sequence.ColumnName)
			if _, err := tx.ExecContext(ctx, resetSQL); err != nil {
				return count, fmt.Errorf(
					"resetting sequence for %s.%s: %w",
					ownedTable.QualifiedName(),
					sequence.ColumnName,
					err,
				)
			}
			count++
		}
	}
	return count, nil
}

func resetSequenceSQL(table TableInfo, columnName string) string {
	return fmt.Sprintf(
		`SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE(MAX(%s), 1), MAX(%s) IS NOT NULL) FROM %s`,
		quoteLiteral(table.quotedName()), quoteLiteral(columnName), sqlutil.QuoteIdent(columnName), sqlutil.QuoteIdent(columnName), table.quotedName(),
	)
}

// quoteLiteral escapes a string for use as a SQL string literal (single-quoted).
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
