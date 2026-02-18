package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print AYB version",
	Run: func(cmd *cobra.Command, args []string) {
		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{
				"version": buildVersion,
				"commit":  buildCommit,
				"date":    buildDate,
			})
			return
		}
		fmt.Printf("%s ayb %s (commit: %s, built: %s)\n", ui.BrandEmoji, buildVersion, buildCommit, buildDate)
	},
}
