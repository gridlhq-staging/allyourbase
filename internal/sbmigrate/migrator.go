// Package sbmigrate handles migration from Supabase to AYB, with streaming data copy, dependency resolution, and support for auth users, OAuth identities, and RLS policies.
package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Migrator handles migration from Supabase to AYB.
type Migrator struct {
	source   *sql.DB
	target   *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
	// sourceColumnCache memoizes source schema column existence checks.
	sourceColumnCache map[string]bool
	// skippedTables tracks source tables intentionally skipped due schema incompatibilities.
	skippedTables    map[string]string
	schemaTables     tableIdentitySet
	skippedFunctions map[string]string
	skippedTriggers  map[string]string
}

// NewMigrator creates a migrator that connects to both the source (Supabase)
// and target (AYB) PostgreSQL databases.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if opts.SourceURL == "" {
		return nil, fmt.Errorf("source database URL is required")
	}
	if opts.TargetURL == "" {
		return nil, fmt.Errorf("target database URL is required")
	}
	if err := opts.validateStorageSourceOptions(); err != nil {
		return nil, err
	}

	source, err := sql.Open("pgx", opts.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to source database: %w", err)
	}
	if err := source.PingContext(context.Background()); err != nil {
		source.Close()
		return nil, fmt.Errorf("pinging source database: %w", err)
	}

	target, err := sql.Open("pgx", opts.TargetURL)
	if err != nil {
		source.Close()
		return nil, fmt.Errorf("connecting to target database: %w", err)
	}
	if err := target.PingContext(context.Background()); err != nil {
		source.Close()
		target.Close()
		return nil, fmt.Errorf("pinging target database: %w", err)
	}

	// Verify source is a Supabase database.
	var exists bool
	err = source.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'auth' AND table_name = 'users'
		)
	`).Scan(&exists)
	if err != nil || !exists {
		source.Close()
		target.Close()
		return nil, fmt.Errorf("source database does not appear to be a Supabase database (auth.users table not found)")
	}

	output := io.Writer(os.Stdout)
	if opts.DryRun && !opts.Verbose {
		output = io.Discard
	}

	progress := opts.Progress
	if progress == nil {
		progress = migrate.NopReporter{}
	}

	return &Migrator{
		source:   source,
		target:   target,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}, nil
}

// Close releases both database connections.
func (m *Migrator) Close() error {
	var errs []string
	if m.source != nil {
		if err := m.source.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if m.target != nil {
		if err := m.target.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing connections: %s", strings.Join(errs, "; "))
	}
	return nil
}

// phaseCount returns the total number of migration phases based on options.
func (m *Migrator) phaseCount() int {
	n := len(supabaseTransactionPhaseNames(m.opts))
	if !m.opts.SkipStorage && m.opts.storageSourceConfigured() {
		n++
	}
	return n
}

func (m *Migrator) skipFunctions() bool {
	return skipFunctionsForOptions(m.opts)
}

func skipFunctionsForOptions(opts MigrationOptions) bool {
	return opts.SkipFunctions || opts.SkipData
}

func supabaseTransactionPhaseNames(opts MigrationOptions) []string {
	names := make([]string, 0, 7)
	if !opts.SkipData {
		if !skipFunctionsForOptions(opts) {
			names = append(names, "Functions")
		}
		names = append(names, "Schema", "Data")
	}
	names = append(names, "Auth users")
	if !opts.SkipOAuth {
		names = append(names, "OAuth")
	}
	if !opts.SkipRLS {
		names = append(names, "RLS policies")
	}
	if !skipFunctionsForOptions(opts) {
		names = append(names, "Triggers")
	}
	return names
}

func (m *Migrator) migrateSchemaAndFunctions(
	ctx context.Context,
	tx *sql.Tx,
	phaseIdx int,
	totalPhases int,
) (int, migratedFunctionSet, error) {
	migratedFunctions := make(migratedFunctionSet)
	if m.opts.SkipData {
		return phaseIdx, migratedFunctions, nil
	}

	var functions []functionCatalogEntry
	if !m.skipFunctions() {
		var err error
		functions, err = loadFunctionCatalog(ctx, m.source)
		if err != nil {
			return phaseIdx, migratedFunctions, fmt.Errorf("function inventory: %w", err)
		}
	}
	schemaItems, err := m.prepareSchemaMigration(ctx, tx, functions)
	if err != nil {
		return phaseIdx, migratedFunctions, fmt.Errorf("preparing schema migration: %w", err)
	}

	// Namespaces must exist before functions, while table DDL follows so its
	// defaults and generated expressions can reference migrated functions.
	if !m.skipFunctions() {
		phaseIdx++
		phase := migrate.Phase{Name: "Functions", Index: phaseIdx, Total: totalPhases}
		migratedFunctions, err = m.migrateFunctions(ctx, tx, functions, phase)
		if err != nil {
			return phaseIdx, migratedFunctions, fmt.Errorf("function migration: %w", err)
		}
	}

	phaseIdx++
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}
	if err := m.migrateSchema(ctx, tx, schemaItems, phase); err != nil {
		return phaseIdx, migratedFunctions, fmt.Errorf("schema migration: %w", err)
	}
	return phaseIdx, migratedFunctions, nil
}

// Migrate runs the full Supabase → AYB migration in a single transaction.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	fmt.Fprintln(m.output, "Starting Supabase migration...")

	// Begin target transaction.
	tx, err := m.target.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := m.validateMigrationTarget(ctx, tx); err != nil {
		return nil, err
	}

	totalPhases := m.phaseCount()
	phaseIdx := 0

	var migratedFunctions migratedFunctionSet
	phaseIdx, migratedFunctions, err = m.migrateSchemaAndFunctions(ctx, tx, phaseIdx, totalPhases)
	if err != nil {
		return nil, err
	}

	// Phase: Data (stream rows from source to target).
	if !m.opts.SkipData {
		phaseIdx++
		if err := m.migrateData(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("data migration: %w", err)
		}
	}

	// Phase: Auth users.
	phaseIdx++
	if err := m.migrateAuthUsers(ctx, tx, phaseIdx, totalPhases); err != nil {
		return nil, fmt.Errorf("auth user migration: %w", err)
	}

	// Phase: OAuth identities.
	if !m.opts.SkipOAuth {
		phaseIdx++
		if err := m.migrateOAuthIdentities(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("OAuth identity migration: %w", err)
		}
	}

	// Phase: RLS policies.
	if !m.opts.SkipRLS {
		phaseIdx++
		if err := m.migrateRLSPolicies(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("RLS policy migration: %w", err)
		}
	}

	if !m.skipFunctions() {
		triggers, err := loadTriggerCatalog(ctx, m.source)
		if err != nil {
			return nil, fmt.Errorf("trigger inventory: %w", err)
		}
		triggerCloneStates, err := loadPartitionTriggerCloneStates(ctx, m.source)
		if err != nil {
			return nil, fmt.Errorf("partition trigger clone state inventory: %w", err)
		}
		phaseIdx++
		phase := migrate.Phase{Name: "Triggers", Index: phaseIdx, Total: totalPhases}
		if err := m.migrateTriggers(ctx, tx, triggers, triggerCloneStates, migratedFunctions, phase); err != nil {
			return nil, fmt.Errorf("trigger migration: %w", err)
		}
	}

	// Commit DB transaction before file operations.
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	// Phase: Storage files (outside transaction — filesystem operations).
	if !m.opts.SkipStorage && m.opts.storageSourceConfigured() && !m.opts.DryRun {
		phaseIdx++
		if err := m.migrateStorage(ctx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("storage migration: %w", err)
		}
	}

	fmt.Fprintln(m.output, "\nMigration complete!")
	m.printStats()

	return &m.stats, nil
}

func (m *Migrator) validateMigrationTarget(ctx context.Context, tx *sql.Tx) error {
	var tableExists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&tableExists)
	if err != nil || !tableExists {
		return fmt.Errorf("_ayb_users table not found — run 'ayb start' or 'ayb migrate up' first")
	}

	// Check for existing users unless --force.
	if !m.opts.Force {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM _ayb_users`).Scan(&count); err != nil {
			return fmt.Errorf("checking existing users: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("_ayb_users table is not empty (%d users) — use --force to proceed", count)
		}
	}
	return nil
}
