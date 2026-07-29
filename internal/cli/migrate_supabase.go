// Package cli Implements the CLI command for migrating data tables, auth users, OAuth identities, and RLS policies from Supabase to AYB, with support for dry-run, skip options, and Supabase storage file migration.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/spf13/cobra"
)

type supabaseMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*sbmigrate.MigrationStats, error)
	Close() error
}

var newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
	return sbmigrate.NewMigrator(opts)
}

var buildSupabaseValidationSummary = sbmigrate.BuildValidationSummary

var migrateSupabaseCmd = &cobra.Command{
	Use:   "supabase",
	Short: "Migrate data, auth users, and RLS policies from a Supabase database",
	Long: `Migrate data tables, auth users, OAuth identities, and RLS policies from a Supabase PostgreSQL database
(Supabase Cloud or self-hosted Supabase).

This command connects directly to the Supabase PostgreSQL database and migrates:
- Public schema tables → recreated in AYB with data streamed in batches
- auth.users → _ayb_users (preserves UUIDs, bcrypt passwords work with AYB auth)
- auth.identities → _ayb_oauth_accounts (Google, GitHub, etc.)
- RLS policies → rewritten from auth.uid() to AYB session variables

Example:
  ayb migrate supabase \
    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
    --database-url postgres://localhost:5432/myapp

Example (with a local storage export):
  ayb migrate supabase \
    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
    --database-url postgres://localhost:5432/myapp \
    --storage-export ./supabase-storage-export \
    --storage-path ./ayb_storage

Example (pull storage bytes directly from S3):
  ayb migrate supabase \
    --source-url postgres://postgres:pass@db.xxx.supabase.co:5432/postgres \
    --database-url postgres://localhost:5432/myapp \
    --storage-s3-endpoint s3.example.com \
    --storage-s3-region us-east-1 \
    --storage-s3-access-key ACCESS_KEY \
    --storage-s3-secret-key SECRET_KEY \
    --storage-path ./ayb_storage

For direct S3 pulls, source bucket names come from the Supabase PostgreSQL inventory;
there is no bucket-name flag. --storage-export and --storage-s3-* cannot be combined.
Use --storage-s3-use-ssl=false only for HTTP endpoints such as local MinIO.

Example (self-hosted Supabase):
  ayb migrate supabase \
    --source-url postgres://postgres:pass@supabase-db.internal:5432/postgres \
    --database-url postgres://localhost:5432/myapp

Use the direct database connection (port 5432), not the connection pooler (port 6543).
Use --skip-data to migrate only auth and RLS (no data tables).
Use --skip-functions to skip database functions and dependent triggers.
Use --skip-storage to skip file migration.
Use -y/--yes to skip confirmation prompts and --json for machine-readable output.

Database migration runs in a transaction. Storage files and metadata are migrated afterward;
successfully migrated storage objects remain if a later storage object fails.
Use --dry-run to preview what would be migrated without changing the database or storage.`,
	RunE: runMigrateSupabase,
}

func init() {
	migrateCmd.AddCommand(migrateSupabaseCmd)

	migrateSupabaseCmd.Flags().String("source-url", "", "Supabase PostgreSQL connection URL (source)")
	migrateSupabaseCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateSupabaseCmd.Flags().String("storage-export", "", "Path to exported Supabase storage directory")
	migrateSupabaseCmd.Flags().String("storage-path", "", "Destination directory for AYB storage files (default: ./ayb_storage)")
	migrateSupabaseCmd.Flags().String("storage-s3-endpoint", "", "S3-compatible source endpoint")
	migrateSupabaseCmd.Flags().String("storage-s3-region", "", "S3-compatible source region")
	migrateSupabaseCmd.Flags().String("storage-s3-access-key", "", "S3-compatible source access key")
	migrateSupabaseCmd.Flags().String("storage-s3-secret-key", "", "S3-compatible source secret key")
	migrateSupabaseCmd.Flags().Bool("storage-s3-use-ssl", true, "Use HTTPS for S3 source endpoints without a scheme")
	migrateSupabaseCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateSupabaseCmd.Flags().Bool("force", false, "Allow migration when _ayb_users is not empty")
	migrateSupabaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateSupabaseCmd.Flags().Bool("skip-rls", false, "Skip RLS policy rewriting")
	migrateSupabaseCmd.Flags().Bool("skip-oauth", false, "Skip OAuth identity migration")
	migrateSupabaseCmd.Flags().Bool("skip-data", false, "Skip data table migration (auth and RLS only)")
	migrateSupabaseCmd.Flags().Bool("skip-functions", false, "Skip database function and trigger migration")
	migrateSupabaseCmd.Flags().Bool("skip-storage", false, "Skip storage file migration")
	migrateSupabaseCmd.Flags().Bool("include-anonymous", false, "Include anonymous Supabase users")
	migrateSupabaseCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	migrateSupabaseCmd.Flags().Bool("json", false, "Output migration stats as JSON")

	migrateSupabaseCmd.MarkFlagRequired("source-url")
	migrateSupabaseCmd.MarkFlagRequired("database-url")
}

type supabaseCommandOptions struct {
	migration               sbmigrate.MigrationOptions
	storageSourceConfigured bool
	yes                     bool
	jsonOut                 bool
}

func parseSupabaseCommandOptions(cmd *cobra.Command) (supabaseCommandOptions, error) {
	sourceURL, _ := cmd.Flags().GetString("source-url")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	verbose, _ := cmd.Flags().GetBool("verbose")
	storageExport, _ := cmd.Flags().GetString("storage-export")
	storagePath, _ := cmd.Flags().GetString("storage-path")
	storageS3Endpoint, _ := cmd.Flags().GetString("storage-s3-endpoint")
	storageS3Region, _ := cmd.Flags().GetString("storage-s3-region")
	storageS3AccessKey, _ := cmd.Flags().GetString("storage-s3-access-key")
	storageS3SecretKey, _ := cmd.Flags().GetString("storage-s3-secret-key")
	storageS3UseSSL, _ := cmd.Flags().GetBool("storage-s3-use-ssl")
	skipRLS, _ := cmd.Flags().GetBool("skip-rls")
	skipOAuth, _ := cmd.Flags().GetBool("skip-oauth")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	skipFunctions, _ := cmd.Flags().GetBool("skip-functions")
	skipStorage, _ := cmd.Flags().GetBool("skip-storage")
	includeAnon, _ := cmd.Flags().GetBool("include-anonymous")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")

	s3FlagSet := supabaseS3FlagSet(cmd)
	if err := validateSupabaseStorageSource(storageExport, s3FlagSet, storageS3Endpoint, storageS3Region, storageS3AccessKey, storageS3SecretKey); err != nil {
		return supabaseCommandOptions{}, err
	}

	if skipData {
		skipFunctions = true
	}

	return supabaseCommandOptions{
		migration: sbmigrate.MigrationOptions{
			SourceURL:          sourceURL,
			TargetURL:          databaseURL,
			StorageExportPath:  storageExport,
			StoragePath:        storagePath,
			StorageS3Endpoint:  storageS3Endpoint,
			StorageS3Region:    storageS3Region,
			StorageS3AccessKey: storageS3AccessKey,
			StorageS3SecretKey: storageS3SecretKey,
			StorageS3UseSSL:    storageS3UseSSL,
			DryRun:             dryRun,
			Force:              force,
			Verbose:            verbose,
			SkipRLS:            skipRLS,
			SkipOAuth:          skipOAuth,
			SkipData:           skipData,
			SkipFunctions:      skipFunctions,
			SkipStorage:        skipStorage,
			IncludeAnonymous:   includeAnon,
			Progress:           newSupabaseProgressReporter(jsonOut),
		},
		storageSourceConfigured: strings.TrimSpace(storageExport) != "" || s3FlagSet,
		yes:                     yes,
		jsonOut:                 jsonOut,
	}, nil
}

// runMigrateSupabase handles the supabase migration CLI command, parsing flags, analyzing the source Supabase database, prompting for confirmation, executing the migration in a transaction, and outputting results as JSON or a human-readable summary.
func runMigrateSupabase(cmd *cobra.Command, args []string) error {
	options, err := parseSupabaseCommandOptions(cmd)
	if err != nil {
		return err
	}

	migrator, err := newSupabaseMigrator(options.migration)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if !options.jsonOut {
		appendSupabaseMissingStorageSourceWarning(report, options.storageSourceConfigured, options.migration.SkipStorage)
		report.PrintReport(os.Stderr)

		if !options.yes && !options.migration.DryRun {
			fmt.Fprint(os.Stderr, "  Proceed? [Y/n] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "" && answer != "y" && answer != "yes" {
				fmt.Fprintln(os.Stderr, "  Migration cancelled.")
				return nil
			}
		}

		fmt.Fprintln(os.Stderr)
	}

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if !options.jsonOut && !options.migration.DryRun {
		summaryReport := normalizeSupabaseSummaryReport(
			report,
			options.migration.SkipData,
			options.migration.SkipFunctions,
			options.migration.SkipOAuth,
			options.migration.SkipRLS,
			options.migration.SkipStorage,
			options.storageSourceConfigured,
		)
		summary := buildSupabaseValidationSummary(summaryReport, stats)
		summary.PrintSummary(os.Stderr)
	}

	if options.jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	return nil
}

func newSupabaseProgressReporter(jsonOut bool) migrate.ProgressReporter {
	if jsonOut {
		return migrate.NopReporter{}
	}
	return migrate.NewCLIReporter(os.Stderr)
}

func supabaseS3FlagSet(cmd *cobra.Command) bool {
	for _, name := range []string{
		"storage-s3-endpoint",
		"storage-s3-region",
		"storage-s3-access-key",
		"storage-s3-secret-key",
		"storage-s3-use-ssl",
	} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func validateSupabaseStorageSource(
	storageExport string,
	s3FlagSet bool,
	s3Endpoint string,
	s3Region string,
	s3AccessKey string,
	s3SecretKey string,
) error {
	if strings.TrimSpace(storageExport) != "" && s3FlagSet {
		return fmt.Errorf("--storage-export cannot be combined with --storage-s3-*")
	}
	if !s3FlagSet {
		return nil
	}
	for _, required := range []struct {
		flag  string
		value string
	}{
		{flag: "storage-s3-endpoint", value: s3Endpoint},
		{flag: "storage-s3-region", value: s3Region},
		{flag: "storage-s3-access-key", value: s3AccessKey},
		{flag: "storage-s3-secret-key", value: s3SecretKey},
	} {
		if strings.TrimSpace(required.value) == "" {
			return fmt.Errorf("--%s is required when using --storage-s3-*", required.flag)
		}
	}
	return nil
}

func appendSupabaseMissingStorageSourceWarning(
	report *migrate.AnalysisReport,
	storageSourceConfigured bool,
	skipStorage bool,
) {
	if report == nil || report.Files == 0 || storageSourceConfigured || skipStorage {
		return
	}
	report.Warnings = append(report.Warnings, fmt.Sprintf(
		"%d analyzed storage files will not migrate; supply --storage-export or --storage-s3-* credentials, or intentionally choose --skip-storage.",
		report.Files,
	))
}

// normalizeSupabaseSummaryReport returns a copy of the analysis report with fields zeroed out based on which migration components were skipped, used to produce accurate validation summaries that reflect what was actually migrated.
func normalizeSupabaseSummaryReport(
	report *migrate.AnalysisReport,
	skipData bool,
	skipFunctions bool,
	skipOAuth bool,
	skipRLS bool,
	skipStorage bool,
	storageSourceConfigured bool,
) *migrate.AnalysisReport {
	if report == nil {
		return nil
	}

	normalized := *report
	if skipData {
		normalized.Tables = 0
		normalized.Views = 0
		normalized.Records = 0
	}
	if skipData || skipFunctions {
		normalized.Functions = 0
		normalized.Triggers = 0
	}
	if skipOAuth {
		normalized.OAuthLinks = 0
	}
	if skipRLS {
		normalized.RLSPolicies = 0
	}
	if skipStorage || !storageSourceConfigured {
		normalized.Files = 0
		normalized.FileSizeBytes = 0
	}

	return &normalized
}
