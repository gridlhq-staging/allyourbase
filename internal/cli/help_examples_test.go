package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const expectedOwnedTopLevelCommands = 41

func TestEveryCommandHasExample(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	commands := topLevelCommands(rootCmd)
	if len(commands) != expectedOwnedTopLevelCommands {
		t.Fatalf("top-level command count = %d, want %d", len(commands), expectedOwnedTopLevelCommands)
	}

	var missing []string
	for _, cmd := range commands {
		if !renderedHelpHasExamples(cmd) {
			missing = append(missing, cmd.Name())
		}
	}

	covered := len(commands) - len(missing)
	if len(missing) > 0 {
		t.Fatalf("rendered top-level examples coverage = %d/%d; missing EXAMPLES for: %s",
			covered, len(commands), strings.Join(missing, ", "))
	}
	t.Logf("rendered top-level examples coverage = %d/%d", covered, len(commands))
}

func TestSchemaHelpLongShowsDetailSentenceOnce(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	output := renderHelpOutput(schemaCmd)
	const detailSentence = "With a table name, shows full detail: columns, types, foreign keys, indexes."
	if count := strings.Count(output, detailSentence); count != 1 {
		t.Fatalf("schema help detail sentence count = %d, want 1", count)
	}
	if count := countStandaloneHeading(output, "EXAMPLES"); count != 1 {
		t.Fatalf("schema help EXAMPLES heading count = %d, want 1", count)
	}
}

func TestMCPHelpIncludesClientConfigSnippet(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	output := renderHelpOutput(mcpCmd)
	if count := countStandaloneHeading(output, "EXAMPLES"); count != 1 {
		t.Fatalf("mcp help EXAMPLES heading count = %d, want 1", count)
	}
	if !strings.Contains(output, "Configuration in Claude Desktop (claude_desktop_config.json):") {
		t.Fatalf("mcp help missing Claude Desktop config snippet")
	}
	if !strings.Contains(output, `"mcpServers"`) {
		t.Fatalf("mcp help missing mcpServers config JSON")
	}
	if !strings.Contains(output, `"args": ["mcp", "--admin-token", "YOUR_TOKEN"]`) {
		t.Fatalf("mcp help config snippet missing admin-token wiring")
	}
}

// TestExamplesRenderBeforeUsage generalizes the launch-critical ordering
// contract in TestLaunchCriticalHelpContainsExampleBeforeUsage to every
// top-level command. Examples used to live in Long, which renders ahead of the
// USAGE heading, so moving them into Example must not push them below it.
func TestExamplesRenderBeforeUsage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	for _, cmd := range topLevelCommands(rootCmd) {
		output := renderHelpOutput(cmd)
		examplesIndex := standaloneHeadingIndex(output, "EXAMPLES")
		usageIndex := standaloneHeadingIndex(output, "USAGE")
		if examplesIndex < 0 {
			t.Errorf("%s help missing EXAMPLES heading", cmd.Name())
			continue
		}
		if usageIndex < 0 {
			t.Errorf("%s help missing USAGE heading", cmd.Name())
			continue
		}
		if examplesIndex > usageIndex {
			t.Errorf("%s help renders EXAMPLES at line %d, after USAGE at line %d; want EXAMPLES first",
				cmd.Name(), examplesIndex, usageIndex)
		}
	}
}

// TestExampleAnnotationsPreserveHelpContext guards the explanatory annotations
// that rendered help printed before the runnable lines moved from Long into
// Example. Moving a runnable line must carry its explanation with it, otherwise
// the migration silently deletes user-facing help content.
func TestExampleAnnotationsPreserveHelpContext(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	tests := []struct {
		name        string
		cmd         *cobra.Command
		annotations []string
	}{
		{
			name: "logs",
			cmd:  logsCmd,
			annotations: []string{
				"ayb logs                   # Show last 100 log lines",
				"ayb logs -n 50             # Show last 50 log lines",
				"ayb logs --follow          # Stream logs in real-time",
				"ayb logs --level error     # Filter by log level",
			},
		},
		{
			name: "stats",
			cmd:  statsCmd,
			annotations: []string{
				"ayb stats             # Show stats in table format",
				"ayb stats --json      # Show stats as JSON",
			},
		},
		{
			name: "init",
			cmd:  initCmd,
			annotations: []string{
				"ayb init my-app                         # React (default)",
				"ayb init my-app --template next         # Next.js",
				"ayb init my-app --template express      # Express/Node backend",
				"ayb init my-app --template plain        # Minimal TypeScript",
			},
		},
		{
			name: "start",
			cmd:  startCmd,
			annotations: []string{
				"ayb start --database-url postgresql://user:pass@localhost:5432/mydb  # External database",
				"ayb start --from ./pb_data  # Migrate and start from PocketBase",
				"ayb start --from postgres://db.xxx.supabase.co:5432/postgres  # Migrate and start from Supabase",
			},
		},
		{
			name: "mcp",
			cmd:  mcpCmd,
			annotations: []string{
				"ayb mcp  # Stdio mode for Claude Desktop / Claude Code",
				"ayb mcp --url http://localhost:8090  # Explicit server URL",
				"ayb mcp --admin-token YOUR_TOKEN  # Enable SQL access",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := renderHelpOutput(tc.cmd)
			for _, annotation := range tc.annotations {
				if !strings.Contains(output, annotation) {
					t.Errorf("%s help missing annotated example line %q", tc.name, annotation)
				}
			}
		})
	}
}

func TestRenderedHelpHasExamplesPreservesOutInheritance(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	startCmd.SetOut(nil)
	rootCmd.SetOut(nil)
	t.Cleanup(func() {
		startCmd.SetOut(nil)
		rootCmd.SetOut(nil)
	})

	if !renderedHelpHasExamples(startCmd) {
		t.Fatalf("start help should render EXAMPLES")
	}

	var output bytes.Buffer
	rootCmd.SetOut(&output)
	startCmd.HelpFunc()(startCmd, nil)
	if !strings.Contains(output.String(), "Start the Allyourbase server") {
		t.Fatalf("start help did not inherit root output writer after renderedHelpHasExamples")
	}
}

// generatedCompletionCommandName is cobra's auto-added completion command. It
// is registered lazily on the first rootCmd.Execute(), so whether it is present
// depends on which other tests in this package already ran. It has no
// declaration owner in this repo and therefore no Example to populate, so it is
// excluded to keep the coverage denominator deterministic at
// expectedOwnedTopLevelCommands.
const generatedCompletionCommandName = "completion"

func topLevelCommands(root *cobra.Command) []*cobra.Command {
	// Register the generated completion command up front (idempotent) so the
	// measured set is the same whether or not an earlier test in this package
	// already triggered it via Execute. Without this the exclusion below is
	// only exercised in some run orders.
	root.InitDefaultCompletionCmd()

	var commands []*cobra.Command
	for _, cmd := range root.Commands() {
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			continue
		}
		if cmd.Name() == generatedCompletionCommandName {
			continue
		}
		commands = append(commands, cmd)
	}
	return commands
}

func renderedHelpHasExamples(cmd *cobra.Command) bool {
	return hasStandaloneHeading(renderHelpOutput(cmd), "EXAMPLES")
}

func renderHelpOutput(cmd *cobra.Command) string {
	var output bytes.Buffer
	cmd.SetOut(&output)
	defer cmd.SetOut(nil)

	cmd.HelpFunc()(cmd, nil)
	return output.String()
}

func hasStandaloneHeading(output, heading string) bool {
	return countStandaloneHeading(output, heading) > 0
}

// standaloneHeadingIndex returns the zero-based line number of the first
// standalone heading line, or -1 when the heading is absent.
func standaloneHeadingIndex(output, heading string) int {
	for i, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == heading {
			return i
		}
	}
	return -1
}

func countStandaloneHeading(output, heading string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == heading {
			count++
		}
	}
	return count
}
