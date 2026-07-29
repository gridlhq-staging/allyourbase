package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

type deferredSchemaTable struct {
	table   TableInfo
	lastErr error
}

type schemaPhaseItems struct {
	tables []TableInfo
	views  []ViewInfo
}

func (m *Migrator) prepareSchemaMigration(
	ctx context.Context,
	tx *sql.Tx,
	functions []functionCatalogEntry,
) (*schemaPhaseItems, error) {
	tables, views, err := m.loadSchemaPhaseItems(ctx)
	if err != nil {
		return nil, err
	}
	schemas := userSchemasForMigration(tables, views, functionSchemasForMigration(functions))
	if err := createUserSchemas(ctx, tx, schemas); err != nil {
		return nil, err
	}
	return &schemaPhaseItems{tables: tables, views: views}, nil
}

func (m *Migrator) migrateSchema(
	ctx context.Context,
	tx *sql.Tx,
	items *schemaPhaseItems,
	phase migrate.Phase,
) error {
	totalItems := len(items.tables) + len(items.views)
	start := m.startSchemaPhase(phase, totalItems)

	deferred, err := m.createInitialSchemaTables(ctx, tx, phase, totalItems, items.tables)
	if err != nil {
		return err
	}
	if err := m.retryDeferredSchemaTables(ctx, tx, deferred); err != nil {
		return err
	}
	if err := m.createSchemaViews(ctx, tx, items.views); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, totalItems, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d tables, %d views created\n", m.stats.Tables, m.stats.Views)
	return nil
}

func (m *Migrator) loadSchemaPhaseItems(ctx context.Context) ([]TableInfo, []ViewInfo, error) {
	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return nil, nil, fmt.Errorf("introspecting tables: %w", err)
	}
	views, err := introspectViews(ctx, m.source)
	if err != nil {
		return nil, nil, fmt.Errorf("introspecting views: %w", err)
	}
	return tables, views, nil
}

func userSchemasForMigration(tables []TableInfo, views []ViewInfo, functionSchemas []string) []string {
	seen := map[string]struct{}{}
	for _, table := range tables {
		addNonPublicSchema(seen, table.SchemaName)
	}
	for _, view := range views {
		addNonPublicSchema(seen, view.SchemaName)
	}
	for _, schema := range functionSchemas {
		addNonPublicSchema(seen, schema)
	}

	schemas := make([]string, 0, len(seen))
	for schema := range seen {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)
	return schemas
}

func addNonPublicSchema(seen map[string]struct{}, schema string) {
	if schema == "" || schema == "public" {
		return
	}
	if isAdmittedUserSchema(schema) {
		seen[schema] = struct{}{}
	}
}

func createUserSchemas(ctx context.Context, tx *sql.Tx, schemas []string) error {
	for _, schema := range schemas {
		if err := createUserSchema(ctx, tx, schema); err != nil {
			return err
		}
	}
	return nil
}

func createUserSchema(ctx context.Context, tx *sql.Tx, schema string) error {
	quotedSchema := sqlutil.QuoteIdent(schema)
	if _, err := tx.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+quotedSchema); err != nil {
		return fmt.Errorf("creating schema %s: %w", schema, err)
	}
	return nil
}

func (m *Migrator) startSchemaPhase(phase migrate.Phase, totalItems int) time.Time {
	m.progress.StartPhase(phase, totalItems)
	fmt.Fprintln(m.output, "Creating schema...")
	return time.Now()
}

func (m *Migrator) createInitialSchemaTables(
	ctx context.Context,
	tx *sql.Tx,
	phase migrate.Phase,
	totalItems int,
	tables []TableInfo,
) ([]deferredSchemaTable, error) {
	deferred := make([]deferredSchemaTable, 0)
	for idx, table := range tables {
		savepoint := fmt.Sprintf("ayb_schema_table_%d", idx)
		if err := createTableWithSavepoint(ctx, tx, table, savepoint); err != nil {
			if isSkippableSchemaTableError(err) {
				deferred = append(deferred, deferredSchemaTable{table: table, lastErr: err})
				continue
			}
			return nil, fmt.Errorf("creating table %s: %w", table.QualifiedName(), err)
		}

		m.stats.Tables++
		m.markSchemaTableCreated(table)
		m.progress.Progress(phase, idx+1, totalItems)
		m.logSchemaTableCreated(table)
	}

	return deferred, nil
}

func (m *Migrator) retryDeferredSchemaTables(
	ctx context.Context,
	tx *sql.Tx,
	deferred []deferredSchemaTable,
) error {
	if len(deferred) == 0 {
		return nil
	}

	for pass := 1; pass <= len(deferred); pass++ {
		if len(deferred) == 0 {
			break
		}

		next := make([]deferredSchemaTable, 0, len(deferred))
		progressed := false
		for idx, item := range deferred {
			savepoint := fmt.Sprintf("ayb_schema_table_retry_%d_%d", pass, idx)
			if err := createTableWithSavepoint(ctx, tx, item.table, savepoint); err != nil {
				if isSkippableSchemaTableError(err) {
					item.lastErr = err
					next = append(next, item)
					continue
				}
				return fmt.Errorf("creating table %s: %w", item.table.QualifiedName(), err)
			}

			progressed = true
			m.stats.Tables++
			m.markSchemaTableCreated(item.table)
			m.logSchemaTableCreated(item.table)
		}
		if !progressed {
			m.skipDeferredSchemaTables(next)
			break
		}
		deferred = next
	}
	return nil
}

func (m *Migrator) skipDeferredSchemaTables(deferred []deferredSchemaTable) {
	for _, item := range deferred {
		m.markSkippedTable(item.table, item.lastErr)
		m.stats.Skipped++
		m.progress.Warn(fmt.Sprintf("skipping table %s due source/target schema incompatibility: %v", item.table.QualifiedName(), item.lastErr))
	}
}

func (m *Migrator) createSchemaViews(ctx context.Context, tx *sql.Tx, views []ViewInfo) error {
	for idx, view := range views {
		savepoint := fmt.Sprintf("ayb_schema_view_%d", idx)
		if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("creating savepoint for view %s: %w", view.QualifiedName(), err)
		}
		if _, err := tx.ExecContext(ctx, createViewSQL(view)); err != nil {
			if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
				return fmt.Errorf("rolling back savepoint for view %s after error %v: %w", view.QualifiedName(), err, rbErr)
			}
			if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
				return fmt.Errorf("releasing savepoint for view %s after rollback: %w", view.QualifiedName(), relErr)
			}
			// Views may depend on tables that don't exist in the target yet.
			// Log a warning instead of failing.
			m.progress.Warn(fmt.Sprintf("skipping view %s: %v", view.QualifiedName(), err))
			continue
		}
		if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return fmt.Errorf("releasing savepoint for view %s: %w", view.QualifiedName(), err)
		}
		m.stats.Views++
		m.logSchemaViewCreated(view)
	}
	return nil
}

func (m *Migrator) logSchemaTableCreated(table TableInfo) {
	if m.verbose {
		fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", table.QualifiedName(), len(table.Columns))
	}
}

func (m *Migrator) logSchemaViewCreated(view ViewInfo) {
	if m.verbose {
		fmt.Fprintf(m.output, "  CREATE VIEW %s\n", view.QualifiedName())
	}
}

// createTableWithSavepoint creates a table within a database savepoint, rolling back and releasing the savepoint if the creation fails.
func createTableWithSavepoint(ctx context.Context, tx *sql.Tx, table TableInfo, savepoint string) error {
	ddl := createTableSQL(table)
	if err := execSavepointCommand(ctx, tx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("creating savepoint for table %s: %w", table.QualifiedName(), err)
	}
	for _, stmt := range createSequenceSQLs(table) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return rollbackTableSavepoint(ctx, tx, table, savepoint, err)
		}
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return rollbackTableSavepoint(ctx, tx, table, savepoint, err)
	}
	for _, stmt := range ownSequenceSQLs(table) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return rollbackTableSavepoint(ctx, tx, table, savepoint, err)
		}
	}
	if err := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("releasing savepoint for table %s: %w", table.QualifiedName(), err)
	}
	return nil
}

func rollbackTableSavepoint(ctx context.Context, tx *sql.Tx, table TableInfo, savepoint string, err error) error {
	if rbErr := execSavepointCommand(ctx, tx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
		return fmt.Errorf("rolling back savepoint for table %s after error %v: %w", table.QualifiedName(), err, rbErr)
	}
	if relErr := execSavepointCommand(ctx, tx, "RELEASE SAVEPOINT "+savepoint); relErr != nil {
		return fmt.Errorf("releasing savepoint for table %s after rollback: %w", table.QualifiedName(), relErr)
	}
	return err
}
