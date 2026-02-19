package cli

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetVersion is called from main to inject build-time version info.
func SetVersion(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
}

var rootCmd = &cobra.Command{
	Use:   "ayb",
	Short: "Allyourbase — Backend-as-a-Service for PostgreSQL",
	Long: `Allyourbase (AYB) connects to PostgreSQL, introspects the schema,
and auto-generates a REST API with an admin dashboard. Single binary. One config file.

Get started (managed PostgreSQL, zero config):
  ayb start

Or with an external database:
  ayb start --database-url postgresql://user:pass@localhost:5432/mydb`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "Output in JSON format (shorthand for --output json)")
	rootCmd.PersistentFlags().String("output", "table", "Output format: table, json, or csv")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(adminCmd)
	rootCmd.AddCommand(typesCmd)
	rootCmd.AddCommand(sqlCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(webhooksCmd)
	rootCmd.AddCommand(usersCmd)
	rootCmd.AddCommand(storageCmd)
	rootCmd.AddCommand(schemaCmd)
	rootCmd.AddCommand(rpcCmd)
	rootCmd.AddCommand(apikeysCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(secretsCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(demoCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// outputFormat returns the resolved output format from flags.
// --json is a shorthand for --output json.
func outputFormat(cmd *cobra.Command) string {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return "json"
	}
	out, _ := cmd.Flags().GetString("output")
	if out == "" {
		return "table"
	}
	return out
}

// writeCSV writes rows as CSV to the given writer.
// cols is the list of column headers; rows is a slice of string slices.
func writeCSV(w io.Writer, cols []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("writing CSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// writeCSVStdout is a convenience wrapper that writes CSV to os.Stdout.
func writeCSVStdout(cols []string, rows [][]string) error {
	return writeCSV(os.Stdout, cols, rows)
}
