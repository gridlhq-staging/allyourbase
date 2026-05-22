// Package cli demo.go implements the ayb demo command, which runs bundled demo applications with built-in server startup, schema application, and user account seeding.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

type seedAccount struct {
	Email    string
	Password string
}

// demoSeedUsers are pre-created accounts so users can log in instantly.
var demoSeedUsers = []seedAccount{
	{Email: "alice@demo.test", Password: "password123"},
	{Email: "bob@demo.test", Password: "password123"},
	{Email: "charlie@demo.test", Password: "password123"},
}

type demoInfo struct {
	Name           string
	Title          string
	Description    string
	Port           int
	TrySteps       []string
	NeedsAdminAuth bool
}

var demoRegistry = map[string]demoInfo{
	"kanban": {
		Name:        "kanban",
		Title:       "Kanban Board",
		Description: "Trello-lite with drag-and-drop, auth, and realtime sync",
		Port:        5173,
		TrySteps: []string{
			"Open http://localhost:5173",
			"Sign in with a demo account (shown on the login page)",
			"Create a board and add some cards",
			"Open a second browser tab to see realtime sync",
		},
	},
	"live-polls": {
		Name:        "live-polls",
		Title:       "Live Polls",
		Description: "Slido-lite — real-time polling with voting and bar charts",
		Port:        5175,
		TrySteps: []string{
			"Open http://localhost:5175",
			"Sign in with a demo account (shown on the login page)",
			"Create a poll with a few options",
			"Open a second browser, sign in as another user, and vote — watch results update live",
		},
	},
	"movies": {
		Name:           "movies",
		Title:          "Movies",
		Description:    "Semantic movie search with notes, chat, and bring-your-own-key",
		Port:           5177,
		NeedsAdminAuth: true,
		TrySteps: []string{
			"Open http://localhost:5177",
			"Sign in with a demo account (shown on the login page)",
			"Search for a movie by theme or plot (e.g. 'time travel')",
			"Select a result, add a note, then ask a question in the chat panel",
		},
	},
}

const demoDefaultServerPort = "8090"

var demoCmd = &cobra.Command{
	Use:   "demo <name>",
	Short: "Run a demo app (one command, batteries included)",
	Long: `Run one of the bundled AYB demo applications.

Available demos:
  kanban        Trello-lite Kanban board with drag-and-drop    (port 5173)
  live-polls    Slido-lite real-time polling app                (port 5175)
  movies        Semantic movie search with chat and BYOK        (port 5177)

The command handles everything:
  - Starts the AYB server if not already running
  - Applies the database schema
  - Serves the pre-built demo app (no Node.js required)

Examples:
  ayb demo kanban
  ayb demo live-polls
  ayb demo movies`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"kanban", "live-polls", "movies"},
	RunE:      runDemo,
}

// runDemo runs a bundled demo application end-to-end: ensures the server is running, applies the schema, seeds users, and serves the pre-built app with a reverse proxy to the API.
func runDemo(cmd *cobra.Command, args []string) error {
	name := args[0]
	demo, ok := demoRegistry[name]
	if !ok {
		names := make([]string, 0, len(demoRegistry))
		for k := range demoRegistry {
			names = append(names, k)
		}
		return fmt.Errorf("unknown demo %q (available: %s)", name, strings.Join(names, ", "))
	}

	useColor := colorEnabled()
	isTTY := ui.StderrIsTTY()
	sp := ui.NewStepSpinner(os.Stderr, !isTTY)

	// Header
	fmt.Fprintf(os.Stderr, "\n  %s %s\n\n",
		ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase Demo: %s", demo.Title), useColor))

	// Step 1: Ensure AYB server is running
	sp.Start("Connecting to AYB server...")
	baseURL, weStarted, err := ensureDemoServer()
	if err != nil {
		sp.Fail()
		return err
	}
	sp.Done()

	// Clean up server on exit if we started it.
	if weStarted {
		aybBin, _ := os.Executable()
		defer exec.Command(aybBin, "stop").Run() //nolint:errcheck
	}

	// Demos depend on the public auth routes for registration and login.
	// If a user already has an auth-disabled server running, fail before we
	// mutate schema or attempt seed-user creation.
	if err := requireDemoAuthEnabled(baseURL, useColor); err != nil {
		return err
	}

	// Step 2: Apply schema
	sp.Start("Applying database schema...")
	schemaResult, err := applyDemoSchema(baseURL, name)
	if err != nil {
		sp.Fail()
		return fmt.Errorf("applying schema: %w", err)
	}
	sp.Done()
	if schemaResult == "exists" {
		fmt.Fprintf(os.Stderr, "  %s\n", dim("Schema already applied (tables exist)", useColor))
	}

	// Step 3: Seed demo users
	sp.Start("Creating demo accounts...")
	if err := seedDemoUsers(baseURL); err != nil {
		sp.Fail()
		return fmt.Errorf("seeding demo users: %w", err)
	}
	sp.Done()

	// Step 4: Print banner
	printDemoBanner(os.Stderr, demo, baseURL, useColor)

	// Step 5: Resolve admin token for demos that need admin API access.
	var adminToken string
	if demo.NeedsAdminAuth {
		sp.Start("Resolving admin credentials...")
		adminToken, err = resolveDemoAdminToken(baseURL)
		if err != nil {
			sp.Fail()
			return fmt.Errorf("resolving admin token: %w", err)
		}
		sp.Done()
	}

	// Step 6: Serve the pre-built demo app
	return serveDemoApp(name, demo.Port, baseURL, adminToken)
}

func printDemoBanner(w *os.File, demo demoInfo, baseURL string, useColor bool) {
	padLabel := func(label string, width int) string {
		return bold(fmt.Sprintf("%-*s", width, label), useColor)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", padLabel("Demo:", 10), demo.Description)
	fmt.Fprintf(w, "  %s %s\n", padLabel("App:", 10), cyan(fmt.Sprintf("http://localhost:%d", demo.Port), useColor))
	fmt.Fprintf(w, "  %s %s\n", padLabel("API:", 10), cyan(baseURL+"/api", useColor))
	fmt.Fprintf(w, "  %s %s\n", padLabel("Admin:", 10), cyan(baseURL+"/admin", useColor))

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", bold("Accounts:", useColor))
	for _, u := range demoSeedUsers {
		fmt.Fprintf(w, "    %s  %s %s\n",
			cyan(fmt.Sprintf("%-22s", u.Email), useColor),
			dim("/", useColor),
			green(u.Password, useColor))
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", dim("Try:", useColor))
	for i, step := range demo.TrySteps {
		fmt.Fprintf(w, "  %s %s\n", dim(fmt.Sprintf("%d.", i+1), useColor), step)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", dim("Press Ctrl+C to stop.", useColor))
}

// formatDemoHints returns styled lines for each registered demo, sorted by
// name. Used by help.go and start_banner.go to derive demo listings from the
// canonical demoRegistry instead of hardcoding per-demo strings.
func formatDemoHints(useColor bool) []string {
	names := make([]string, 0, len(demoRegistry))
	for k := range demoRegistry {
		names = append(names, k)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		demo := demoRegistry[name]
		cmd := fmt.Sprintf("ayb demo %-11s", name)
		comment := fmt.Sprintf("# %s  (port %d)", demo.Description, demo.Port)
		lines = append(lines, fmt.Sprintf("  %s  %s", green(cmd, useColor), dim(comment, useColor)))
	}
	return lines
}
