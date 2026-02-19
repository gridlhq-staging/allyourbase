package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

type demoInfo struct {
	Name        string
	Title       string
	Description string
	Port        int
	TrySteps    []string
}

var demoRegistry = map[string]demoInfo{
	"kanban": {
		Name:        "kanban",
		Title:       "Kanban Board",
		Description: "Trello-lite with drag-and-drop, auth, and realtime sync",
		Port:        5173,
		TrySteps: []string{
			"Open http://localhost:5173",
			"Register an account",
			"Create a board and add some cards",
			"Open a second browser tab to see realtime sync",
		},
	},
	"pixel-canvas": {
		Name:        "pixel-canvas",
		Title:       "Pixel Canvas",
		Description: "r/place clone — collaborative pixel art with realtime SSE",
		Port:        5174,
		TrySteps: []string{
			"Open http://localhost:5174",
			"Register an account",
			"Pick a color and click to place pixels",
			"Open a second browser to see pixels appear in realtime",
		},
	},
	"live-polls": {
		Name:        "live-polls",
		Title:       "Live Polls",
		Description: "Slido-lite — real-time polling with voting and bar charts",
		Port:        5175,
		TrySteps: []string{
			"Open http://localhost:5175",
			"Register an account",
			"Create a poll with a few options",
			"Open a second browser, register, and vote — watch results update live",
		},
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo <name>",
	Short: "Run a demo app (one command, batteries included)",
	Long: `Download and run one of the AYB demo applications.

Available demos:
  kanban        Trello-lite Kanban board with drag-and-drop    (port 5173)
  pixel-canvas  r/place clone collaborative pixel canvas       (port 5174)
  live-polls    Slido-lite real-time polling app                (port 5175)

The command handles everything:
  - Starts the AYB server if not already running
  - Applies the database schema
  - Installs npm dependencies
  - Starts the Vite dev server

Examples:
  ayb demo kanban
  ayb demo pixel-canvas
  ayb demo live-polls`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"kanban", "pixel-canvas", "live-polls"},
	RunE:      runDemo,
}

func init() {
	demoCmd.Flags().String("dir", ".", "Parent directory for demo files")
}

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

	dir, _ := cmd.Flags().GetString("dir")
	useColor := colorEnabled()
	isTTY := ui.StderrIsTTY()
	sp := ui.NewStepSpinner(os.Stderr, !isTTY)

	// Header
	fmt.Fprintf(os.Stderr, "\n  %s %s\n\n",
		ui.BrandEmoji,
		boldCyan(fmt.Sprintf("Allyourbase Demo: %s", demo.Title), useColor))

	// Step 1: Check prerequisites
	sp.Start("Checking prerequisites...")
	if err := checkDemoPrerequisites(); err != nil {
		sp.Fail()
		return err
	}
	sp.Done()

	// Step 2: Ensure AYB server is running
	sp.Start("Connecting to AYB server...")
	baseURL, serverCmd, err := ensureDemoServer()
	if err != nil {
		sp.Fail()
		return err
	}
	sp.Done()

	// Clean up server on exit if we started it
	if serverCmd != nil {
		defer func() {
			if serverCmd.Process != nil {
				serverCmd.Process.Signal(syscall.SIGTERM)
				serverCmd.Wait()
			}
		}()
	}

	// Check if auth is enabled (warn but don't block)
	checkDemoAuth(baseURL, useColor)

	// Step 3: Extract demo files
	sp.Start("Extracting demo files...")
	demoDir := filepath.Join(dir, name)
	extracted, err := extractDemoFiles(name, demoDir)
	if err != nil {
		sp.Fail()
		return fmt.Errorf("extracting demo files: %w", err)
	}
	sp.Done()
	if !extracted {
		fmt.Fprintf(os.Stderr, "  %s\n", dim(fmt.Sprintf("Using existing files in %s/", name), useColor))
	}

	// Step 4: Apply schema
	sp.Start("Applying database schema...")
	schemaResult, err := applyDemoSchema(baseURL, demoDir)
	if err != nil {
		sp.Fail()
		return fmt.Errorf("applying schema: %w", err)
	}
	sp.Done()
	if schemaResult == "exists" {
		fmt.Fprintf(os.Stderr, "  %s\n", dim("Schema already applied (tables exist)", useColor))
	}

	// Step 5: npm install
	if _, err := os.Stat(filepath.Join(demoDir, "node_modules")); os.IsNotExist(err) {
		sp.Start("Installing npm dependencies...")
		if err := npmInstallDemo(demoDir); err != nil {
			sp.Fail()
			return fmt.Errorf("npm install: %w", err)
		}
		sp.Done()
	} else {
		fmt.Fprintf(os.Stderr, "  %s %s\n", dim("Dependencies already installed", useColor), ui.SymbolCheck)
	}

	// Step 6: Print banner
	fmt.Fprintln(os.Stderr)
	padLabel := func(label string, width int) string {
		return bold(fmt.Sprintf("%-*s", width, label), useColor)
	}
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("Demo:", 8), demo.Description)
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("App:", 8), cyan(fmt.Sprintf("http://localhost:%d", demo.Port), useColor))
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("API:", 8), cyan(baseURL+"/api", useColor))
	fmt.Fprintf(os.Stderr, "  %s %s\n", padLabel("Admin:", 8), cyan(baseURL+"/admin", useColor))

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %s\n", dim("Try:", useColor))
	for i, step := range demo.TrySteps {
		fmt.Fprintf(os.Stderr, "  %s %s\n", dim(fmt.Sprintf("%d.", i+1), useColor), step)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Press Ctrl+C to stop.", useColor))

	// Step 7: Run vite dev server in foreground
	return runViteDev(demoDir)
}

// checkDemoPrerequisites verifies node and npm are available.
func checkDemoPrerequisites() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("%s", ui.FormatError(
			"Node.js is required but not found in PATH",
			"brew install node",
			"or visit https://nodejs.org",
		))
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("%s", ui.FormatError(
			"npm is required but not found in PATH",
			"brew install node",
			"or visit https://nodejs.org",
		))
	}
	return nil
}

// ensureDemoServer checks if the AYB server is running. If not, starts it.
// Returns the base URL, the server command (if we started it), and any error.
func ensureDemoServer() (string, *exec.Cmd, error) {
	base := serverURL()
	client := &http.Client{Timeout: 2 * time.Second}

	// Check if already running
	resp, err := client.Get(base + "/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return base, nil, nil
		}
	}

	// Not running — auto-start
	aybBin, err := os.Executable()
	if err != nil {
		aybBin = os.Args[0]
	}

	cmd := exec.Command(aybBin, "start")
	cmd.Env = append(os.Environ(), "AYB_AUTH_ENABLED=true")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start AYB server: %w", err)
	}

	// Poll until healthy (up to 30 seconds)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		resp, err := client.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base, cmd, nil
			}
		}
	}

	// Timed out — kill the process
	if cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	}
	return "", nil, fmt.Errorf("AYB server did not become healthy within 30 seconds")
}

// checkDemoAuth probes the server to warn if auth is disabled.
func checkDemoAuth(baseURL string, useColor bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/auth/me")
	if err != nil {
		return
	}
	resp.Body.Close()

	// 401 = auth is enabled (expected). 404 = auth endpoint not found (disabled).
	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "\n  %s %s\n\n",
			yellow("⚠", useColor),
			yellow("Auth appears to be disabled. Demos require auth for registration/login.", useColor))
		fmt.Fprintf(os.Stderr, "  %s\n", dim("Add to ayb.toml:", useColor))
		fmt.Fprintf(os.Stderr, "    [auth]\n    enabled = true\n\n")
		fmt.Fprintf(os.Stderr, "  %s\n\n", dim("Then restart: ayb stop && ayb start", useColor))
	}
}

// extractDemoFiles writes the embedded demo files to demoDir.
// Returns true if files were extracted, false if reusing existing.
func extractDemoFiles(name, demoDir string) (bool, error) {
	// Skip if all critical files already exist (handles partial extraction)
	requiredFiles := []string{"package.json", "schema.sql", "vite.config.ts"}
	allExist := true
	for _, f := range requiredFiles {
		if _, err := os.Stat(filepath.Join(demoDir, f)); err != nil {
			allExist = false
			break
		}
	}
	if allExist {
		return false, nil
	}

	// Walk the embedded FS and write files
	prefix := name
	err := fs.WalkDir(examples.FS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path from the demo name prefix
		relPath := strings.TrimPrefix(path, prefix)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			return nil
		}

		target := filepath.Join(demoDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		// Read from embed and write to disk
		data, err := examples.FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		return os.WriteFile(target, data, 0644)
	})

	if err != nil {
		return false, err
	}
	return true, nil
}

// applyDemoSchema reads schema.sql and sends it to the running server.
// Returns "applied", "exists", or an error.
func applyDemoSchema(baseURL, demoDir string) (string, error) {
	schemaPath := filepath.Join(demoDir, "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", fmt.Errorf("reading schema.sql: %w", err)
	}

	token, err := resolveDemoAdminToken(baseURL)
	if err != nil {
		return "", fmt.Errorf("authenticating with server: %w", err)
	}

	body, err := json.Marshal(map[string]string{"query": string(schemaSQL)})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequest("POST", baseURL+"/api/admin/sql/", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending schema to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading server response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		// "already exists" is fine — schema was previously applied
		if strings.Contains(bodyStr, "already exists") {
			return "exists", nil
		}
		// Parse error message if possible
		var errResp map[string]any
		if json.Unmarshal(respBody, &errResp) == nil {
			if msg, ok := errResp["message"].(string); ok {
				if strings.Contains(msg, "already exists") {
					return "exists", nil
				}
				return "", fmt.Errorf("SQL error: %s", msg)
			}
		}
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, bodyStr)
	}

	return "applied", nil
}

// resolveDemoAdminToken finds an admin token for the running server.
// Reuses the same logic as `ayb sql`.
func resolveDemoAdminToken(baseURL string) (string, error) {
	// Check env var first
	if token := os.Getenv("AYB_ADMIN_TOKEN"); token != "" {
		return token, nil
	}

	// Try saved admin password from ~/.ayb/admin-token
	tokenPath, pathErr := aybAdminTokenPath()
	if pathErr != nil {
		return "", fmt.Errorf("no admin token: could not resolve home directory: %w", pathErr)
	}

	data, readErr := os.ReadFile(tokenPath)
	if readErr != nil {
		return "", fmt.Errorf("no admin token: %s not found (visit /admin to set a password first)", tokenPath)
	}

	password := strings.TrimSpace(string(data))
	t, loginErr := adminLogin(baseURL, password)
	if loginErr != nil {
		return "", fmt.Errorf("admin login failed (saved password may be stale): %w", loginErr)
	}
	return t, nil
}

// npmInstallDemo runs npm install in the demo directory.
func npmInstallDemo(dir string) error {
	cmd := exec.Command("npm", "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runViteDev starts `npm run dev` as a foreground process with signal forwarding.
// Server cleanup is handled by the deferred function in runDemo, not here.
func runViteDev(dir string) error {
	cmd := exec.Command("npm", "run", "dev")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting dev server: %w", err)
	}

	// Forward signals to the vite process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh) // unblock goroutine waiting on sigCh
	}()

	go func() {
		sig, ok := <-sigCh
		if !ok {
			return
		}
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}
	}()

	err := cmd.Wait()

	// Ignore exit code from Ctrl+C (expected shutdown)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 || exitErr.ExitCode() == 130 {
				return nil
			}
		}
	}
	return err
}
