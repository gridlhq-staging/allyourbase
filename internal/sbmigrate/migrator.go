package sbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

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
	n := 1 // auth is always run
	if !m.opts.SkipData {
		n += 2 // schema + data
	}
	if !m.opts.SkipOAuth {
		n++
	}
	if !m.opts.SkipRLS {
		n++
	}
	if !m.opts.SkipStorage && m.opts.StorageExportPath != "" {
		n++
	}
	return n
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

	// Verify _ayb_users table exists.
	var tableExists bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&tableExists)
	if err != nil || !tableExists {
		return nil, fmt.Errorf("_ayb_users table not found — run 'ayb start' or 'ayb migrate up' first")
	}

	// Check for existing users unless --force.
	if !m.opts.Force {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM _ayb_users`).Scan(&count); err != nil {
			return nil, fmt.Errorf("checking existing users: %w", err)
		}
		if count > 0 {
			return nil, fmt.Errorf("_ayb_users table is not empty (%d users) — use --force to proceed", count)
		}
	}

	totalPhases := m.phaseCount()
	phaseIdx := 0

	// Phase: Schema (create tables + views in target).
	if !m.opts.SkipData {
		phaseIdx++
		if err := m.migrateSchema(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("schema migration: %w", err)
		}
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

	// Commit DB transaction before file operations.
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	// Phase: Storage files (outside transaction — filesystem operations).
	if !m.opts.SkipStorage && m.opts.StorageExportPath != "" && !m.opts.DryRun {
		phaseIdx++
		if err := m.migrateStorage(ctx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("storage migration: %w", err)
		}
	}

	fmt.Fprintln(m.output, "\nMigration complete!")
	m.printStats()

	return &m.stats, nil
}

// Analyze performs a pre-flight analysis of the source database.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	report := &migrate.AnalysisReport{
		SourceType: "Supabase",
		SourceInfo: redactURL(m.opts.SourceURL),
	}

	// Count auth users (excluding anonymous, matching Migrate behavior).
	authQuery := `SELECT COUNT(*) FROM auth.users WHERE deleted_at IS NULL`
	if !m.opts.IncludeAnonymous {
		authQuery += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	err := m.source.QueryRowContext(ctx, authQuery).Scan(&report.AuthUsers)
	if err != nil {
		return nil, fmt.Errorf("counting auth users: %w", err)
	}

	// Count OAuth identities (excluding 'email' provider).
	err = m.source.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM auth.identities
		WHERE provider != 'email'
	`).Scan(&report.OAuthLinks)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count OAuth identities: %v", err))
	}

	// Count RLS policies.
	err = m.source.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_policy pol
		JOIN pg_class c ON c.oid = pol.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
	`).Scan(&report.RLSPolicies)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count RLS policies: %v", err))
	}

	// Count public tables and total rows.
	if !m.opts.SkipData {
		tables, err := introspectTables(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect tables: %v", err))
		} else {
			report.Tables = len(tables)
			for _, t := range tables {
				report.Records += int(t.RowCount)
			}
		}

		views, err := introspectViews(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect views: %v", err))
		} else {
			report.Views = len(views)
		}
	}

	// Count storage objects.
	if !m.opts.SkipStorage {
		buckets, err := m.listStorageBuckets(ctx)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not list storage buckets: %v", err))
		} else {
			for _, b := range buckets {
				objects, err := m.listStorageObjects(ctx, b.ID)
				if err != nil {
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("could not list objects in bucket %s: %v", b.Name, err))
					continue
				}
				report.Files += len(objects)
				for _, o := range objects {
					report.FileSizeBytes += o.Size
				}
			}
		}
	}

	return report, nil
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Supabase (source)",
		TargetLabel: "AYB (target)",
	}

	if report.Tables > 0 || stats.Tables > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables,
		})
	}
	if report.Views > 0 || stats.Views > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Views", SourceCount: report.Views, TargetCount: stats.Views,
		})
	}
	if report.Records > 0 || stats.Records > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Records", SourceCount: report.Records, TargetCount: stats.Records,
		})
	}
	summary.Rows = append(summary.Rows, migrate.ValidationRow{
		Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: stats.Users,
	})
	if report.OAuthLinks > 0 || stats.OAuthLinks > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "OAuth links", SourceCount: report.OAuthLinks, TargetCount: stats.OAuthLinks,
		})
	}
	if report.RLSPolicies > 0 || stats.Policies > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies,
		})
	}
	if report.Files > 0 || stats.StorageFiles > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Storage files", SourceCount: report.Files, TargetCount: stats.StorageFiles,
		})
	}

	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d items skipped during migration", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during migration", len(stats.Errors)))
	}

	return summary
}

func (m *Migrator) migrateSchema(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Schema", Index: phaseIdx, Total: totalPhases}

	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables: %w", err)
	}

	views, err := introspectViews(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting views: %w", err)
	}

	totalItems := len(tables) + len(views)
	m.progress.StartPhase(phase, totalItems)
	start := time.Now()

	fmt.Fprintln(m.output, "Creating schema...")

	for i, t := range tables {
		ddl := createTableSQL(t)
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating table %s: %w", t.Name, err)
		}
		m.stats.Tables++
		m.progress.Progress(phase, i+1, totalItems)
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE TABLE %s (%d columns)\n", t.Name, len(t.Columns))
		}
	}

	for _, v := range views {
		ddl := createViewSQL(v)
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			// Views may depend on tables that don't exist in the target yet.
			// Log a warning instead of failing.
			m.progress.Warn(fmt.Sprintf("skipping view %s: %v", v.Name, err))
			continue
		}
		m.stats.Views++
		if m.verbose {
			fmt.Fprintf(m.output, "  CREATE VIEW %s\n", v.Name)
		}
	}

	m.progress.CompletePhase(phase, totalItems, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d tables, %d views created\n", m.stats.Tables, m.stats.Views)
	return nil
}

func (m *Migrator) migrateData(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Data", Index: phaseIdx, Total: totalPhases}

	// Re-introspect to get tables that now exist in the target.
	tables, err := introspectTables(ctx, m.source)
	if err != nil {
		return fmt.Errorf("introspecting tables for data copy: %w", err)
	}

	var totalRows int64
	for _, t := range tables {
		totalRows += t.RowCount
	}

	m.progress.StartPhase(phase, int(totalRows))
	start := time.Now()

	fmt.Fprintln(m.output, "Copying data...")

	copied := 0
	for _, t := range tables {
		count, err := copyTableData(ctx, m.source, tx, t, func(n int) {
			m.progress.Progress(phase, copied+n, int(totalRows))
		})
		if err != nil {
			return fmt.Errorf("copying data for %s: %w", t.Name, err)
		}
		copied += count
		m.stats.Records += count
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d rows\n", t.Name, count)
		}
	}

	// Reset sequences.
	seqCount, err := resetSequences(ctx, tx, tables)
	if err != nil {
		m.progress.Warn(fmt.Sprintf("sequence reset: %v", err))
	}
	m.stats.Sequences = seqCount

	m.progress.CompletePhase(phase, int(totalRows), time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d records copied across %d tables\n", m.stats.Records, len(tables))
	return nil
}

func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0) // unknown count until we query
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating auth users...")

	query := `
		SELECT id, COALESCE(email, ''), COALESCE(encrypted_password, ''),
		       email_confirmed_at, created_at, updated_at,
		       COALESCE(is_anonymous, false)
		FROM auth.users
		WHERE deleted_at IS NULL`
	if !m.opts.IncludeAnonymous {
		query += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	query += " ORDER BY created_at"

	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying auth.users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var u SupabaseUser
		var emailConfAt sql.NullTime

		if err := rows.Scan(
			&u.ID, &u.Email, &u.EncryptedPassword,
			&emailConfAt, &u.CreatedAt, &u.UpdatedAt,
			&u.IsAnonymous,
		); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}

		// Skip users without email (phone-only, anonymous).
		if u.Email == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (no email)\n", u.ID)
			}
			continue
		}

		// Skip users without a password hash (OAuth-only users get empty string).
		// They'll still be importable via OAuth identity linking.
		if u.EncryptedPassword == "" {
			// Insert with a placeholder — they can only auth via OAuth or password reset.
			u.EncryptedPassword = "$none$"
		}

		emailVerified := emailConfAt.Valid

		if m.verbose {
			fmt.Fprintf(m.output, "  %s (%s) verified=%v\n", u.Email, u.ID, emailVerified)
		}

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_users (id, email, password_hash, email_verified, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			u.ID, strings.ToLower(u.Email), u.EncryptedPassword,
			emailVerified, u.CreatedAt, u.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting user %s: %w", u.Email, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.Users++
		}
		m.progress.Progress(phase, m.stats.Users, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.Users, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d users migrated (%d skipped)\n", m.stats.Users, m.stats.Skipped)
	return nil
}

func (m *Migrator) migrateOAuthIdentities(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "OAuth", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating OAuth identities...")

	rows, err := m.source.QueryContext(ctx, `
		SELECT i.user_id, i.provider, i.identity_data, i.created_at
		FROM auth.identities i
		JOIN auth.users u ON u.id = i.user_id
		WHERE u.deleted_at IS NULL
		ORDER BY i.created_at
	`)
	if err != nil {
		return fmt.Errorf("querying auth.identities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, provider, identityDataJSON string
		var createdAt sql.NullTime

		if err := rows.Scan(&userID, &provider, &identityDataJSON, &createdAt); err != nil {
			return fmt.Errorf("scanning identity: %w", err)
		}

		// Skip the "email" provider — that's password auth, not OAuth.
		if provider == "email" {
			continue
		}

		var identityData map[string]any
		if err := json.Unmarshal([]byte(identityDataJSON), &identityData); err != nil {
			m.stats.Errors = append(m.stats.Errors,
				fmt.Sprintf("parsing identity_data for user %s: %v", userID, err))
			continue
		}

		providerUserID := extractString(identityData, "sub", "provider_id")
		email := extractString(identityData, "email")
		name := extractString(identityData, "name", "full_name")

		if providerUserID == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped identity for user %s (no provider_user_id)\n", userID)
			}
			continue
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s → %s (%s)\n", provider, email, providerUserID)
		}

		created := time.Now()
		if createdAt.Valid {
			created = createdAt.Time
		}

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
			userID, provider, providerUserID, email, name, created,
		)
		if err != nil {
			return fmt.Errorf("inserting OAuth account for user %s: %w", userID, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.OAuthLinks++
		}
		m.progress.Progress(phase, m.stats.OAuthLinks, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.OAuthLinks, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d OAuth identities migrated\n", m.stats.OAuthLinks)
	return nil
}

func (m *Migrator) migrateRLSPolicies(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "RLS policies", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating RLS policies...")

	policies, err := ReadRLSPolicies(ctx, m.source)
	if err != nil {
		return err
	}

	if len(policies) == 0 {
		fmt.Fprintln(m.output, "  no RLS policies found in public schema")
		m.progress.CompletePhase(phase, 0, time.Since(start))
		return nil
	}

	for _, p := range policies {
		if m.verbose {
			fmt.Fprintf(m.output, "  %s.%s: %s\n", p.TableName, p.PolicyName, p.Command)
		}

		// Drop existing policy on target (idempotent).
		dropSQL := fmt.Sprintf("DROP POLICY IF EXISTS %q ON %q.%q",
			p.PolicyName, p.SchemaName, p.TableName)
		if _, err := tx.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("dropping policy %s on %s: %w", p.PolicyName, p.TableName, err)
		}

		// Enable RLS on the table.
		enableSQL := fmt.Sprintf("ALTER TABLE %q.%q ENABLE ROW LEVEL SECURITY",
			p.SchemaName, p.TableName)
		if _, err := tx.ExecContext(ctx, enableSQL); err != nil {
			return fmt.Errorf("enabling RLS on %s: %w", p.TableName, err)
		}

		// Create rewritten policy.
		rewrittenSQL := GenerateRewrittenPolicy(p)
		if _, err := tx.ExecContext(ctx, rewrittenSQL); err != nil {
			return fmt.Errorf("creating policy %s on %s: %w", p.PolicyName, p.TableName, err)
		}
		m.stats.Policies++
		m.progress.Progress(phase, m.stats.Policies, len(policies))
	}

	m.progress.CompletePhase(phase, m.stats.Policies, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d RLS policies rewritten\n", m.stats.Policies)
	return nil
}

func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "\nSummary:\n")
	if m.stats.Tables > 0 {
		fmt.Fprintf(m.output, "  Tables:     %d\n", m.stats.Tables)
	}
	if m.stats.Views > 0 {
		fmt.Fprintf(m.output, "  Views:      %d\n", m.stats.Views)
	}
	if m.stats.Records > 0 {
		fmt.Fprintf(m.output, "  Records:    %d\n", m.stats.Records)
	}
	if m.stats.Sequences > 0 {
		fmt.Fprintf(m.output, "  Sequences:  %d\n", m.stats.Sequences)
	}
	fmt.Fprintf(m.output, "  Users:      %d\n", m.stats.Users)
	fmt.Fprintf(m.output, "  OAuth:      %d\n", m.stats.OAuthLinks)
	fmt.Fprintf(m.output, "  RLS:        %d\n", m.stats.Policies)
	if m.stats.StorageFiles > 0 {
		fmt.Fprintf(m.output, "  Files:      %d (%s)\n", m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	}
	fmt.Fprintf(m.output, "  Skipped:    %d\n", m.stats.Skipped)
	if len(m.stats.Errors) > 0 {
		fmt.Fprintf(m.output, "  Errors:     %d\n", len(m.stats.Errors))
		for _, e := range m.stats.Errors {
			fmt.Fprintf(m.output, "    - %s\n", e)
		}
	}
}

// redactURL strips credentials from a PostgreSQL connection URL for safe display.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// extractString tries multiple keys in a map and returns the first non-empty string value.
func extractString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
