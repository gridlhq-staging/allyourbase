package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/spf13/cobra"
)

var migrateSupabaseCmd = &cobra.Command{
	Use:   "supabase",
	Short: "Migrate data, auth users, and RLS policies from a Supabase database",
	Long: `Migrate data tables, auth users, OAuth identities, and RLS policies from a Supabase PostgreSQL database.

This command connects directly to the Supabase PostgreSQL database and migrates:
- Public schema tables → recreated in AYB with data streamed in batches
- auth.users → _ayb_users (preserves UUIDs, bcrypt passwords work with AYB auth)
- auth.identities → _ayb_oauth_accounts (Google, GitHub, etc.)
- RLS policies → rewritten from auth.uid() to AYB session variables

Example:
  ayb migrate supabase \
    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
    --database-url postgres://localhost:5432/myapp

Use the direct database connection (port 5432), not the connection pooler (port 6543).
Use --skip-data to migrate only auth and RLS (no data tables).

The migration runs in a single transaction, so either everything succeeds or
nothing is changed. Use --dry-run to preview what would be migrated.`,
	RunE: runMigrateSupabase,
}

func init() {
	migrateCmd.AddCommand(migrateSupabaseCmd)

	migrateSupabaseCmd.Flags().String("source-url", "", "Supabase PostgreSQL connection URL (source)")
	migrateSupabaseCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateSupabaseCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateSupabaseCmd.Flags().Bool("force", false, "Allow migration when _ayb_users is not empty")
	migrateSupabaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateSupabaseCmd.Flags().Bool("skip-rls", false, "Skip RLS policy rewriting")
	migrateSupabaseCmd.Flags().Bool("skip-oauth", false, "Skip OAuth identity migration")
	migrateSupabaseCmd.Flags().Bool("skip-data", false, "Skip data table migration (auth and RLS only)")
	migrateSupabaseCmd.Flags().Bool("include-anonymous", false, "Include anonymous Supabase users")

	migrateSupabaseCmd.MarkFlagRequired("source-url")
	migrateSupabaseCmd.MarkFlagRequired("database-url")
}

func runMigrateSupabase(cmd *cobra.Command, args []string) error {
	sourceURL, _ := cmd.Flags().GetString("source-url")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	verbose, _ := cmd.Flags().GetBool("verbose")
	skipRLS, _ := cmd.Flags().GetBool("skip-rls")
	skipOAuth, _ := cmd.Flags().GetBool("skip-oauth")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	includeAnon, _ := cmd.Flags().GetBool("include-anonymous")

	migrator, err := sbmigrate.NewMigrator(sbmigrate.MigrationOptions{
		SourceURL:        sourceURL,
		TargetURL:        databaseURL,
		DryRun:           dryRun,
		Force:            force,
		Verbose:          verbose,
		SkipRLS:          skipRLS,
		SkipOAuth:        skipOAuth,
		SkipData:         skipData,
		IncludeAnonymous: includeAnon,
		Progress:         migrate.NewCLIReporter(os.Stderr),
	})
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	return nil
}
