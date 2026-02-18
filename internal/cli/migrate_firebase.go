package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/spf13/cobra"
)

var migrateFirebaseCmd = &cobra.Command{
	Use:   "firebase",
	Short: "Migrate auth users and Firestore data from Firebase",
	Long: `Migrate auth users, OAuth identities, and Firestore collections from a Firebase project.

This command reads offline Firebase export files (no GCP SDK required) and migrates:
- Auth users → _ayb_users (Firebase scrypt hashes preserved, progressive re-hash on login)
- OAuth providers → _ayb_oauth_accounts (Google, GitHub, etc.)
- Firestore collections → PostgreSQL tables with JSONB data + GIN indexes

Export your Firebase auth data:
  firebase auth:export auth-export.json --format=json

Export your Firestore data (use a tool like firestore-export-import or custom script).

Example:
  ayb migrate firebase \
    --auth-export auth-export.json \
    --firestore-export ./firestore-data/ \
    --database-url postgres://localhost:5432/myapp

At least one of --auth-export or --firestore-export is required.
Use --dry-run to preview what would be migrated.`,
	RunE: runMigrateFirebase,
}

func init() {
	migrateCmd.AddCommand(migrateFirebaseCmd)

	migrateFirebaseCmd.Flags().String("auth-export", "", "Path to Firebase auth export JSON file")
	migrateFirebaseCmd.Flags().String("firestore-export", "", "Path to Firestore export directory")
	migrateFirebaseCmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	migrateFirebaseCmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	migrateFirebaseCmd.Flags().Bool("verbose", false, "Show detailed progress")
	migrateFirebaseCmd.Flags().Bool("json", false, "Output stats as JSON")

	migrateFirebaseCmd.MarkFlagRequired("database-url")
}

func runMigrateFirebase(cmd *cobra.Command, args []string) error {
	authExport, _ := cmd.Flags().GetString("auth-export")
	firestoreExport, _ := cmd.Flags().GetString("firestore-export")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")

	if authExport == "" && firestoreExport == "" {
		return fmt.Errorf("at least one of --auth-export or --firestore-export is required")
	}

	migrator, err := fbmigrate.NewMigrator(fbmigrate.MigrationOptions{
		AuthExportPath:      authExport,
		FirestoreExportPath: firestoreExport,
		DatabaseURL:         databaseURL,
		DryRun:              dryRun,
		Verbose:             verbose,
		Progress:            migrate.NewCLIReporter(os.Stderr),
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
