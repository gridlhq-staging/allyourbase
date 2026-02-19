package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/examples"
)

func TestDemoCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "demo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'demo' subcommand to be registered")
	}
}

func TestDemoRegistryComplete(t *testing.T) {
	expected := map[string]int{
		"kanban":       5173,
		"pixel-canvas": 5174,
		"live-polls":   5175,
	}
	for name, port := range expected {
		demo, ok := demoRegistry[name]
		if !ok {
			t.Errorf("demo %q not found in registry", name)
			continue
		}
		if demo.Port != port {
			t.Errorf("demo %q: expected port %d, got %d", name, port, demo.Port)
		}
		if demo.Title == "" {
			t.Errorf("demo %q: title is empty", name)
		}
		if len(demo.TrySteps) == 0 {
			t.Errorf("demo %q: no try steps", name)
		}
	}
	if len(demoRegistry) != len(expected) {
		t.Errorf("expected %d demos, got %d", len(expected), len(demoRegistry))
	}
}

func TestDemoUnknownName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"demo", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown demo name")
	}
	if !strings.Contains(err.Error(), "unknown demo") {
		t.Fatalf("expected 'unknown demo' error, got %q", err.Error())
	}
}

func TestDemoRequiresName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"demo"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing demo name")
	}
}

func TestExtractDemoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		demoDir := filepath.Join(tmpDir, name)
		extracted, err := extractDemoFiles(name, demoDir)
		if err != nil {
			t.Fatalf("extractDemoFiles(%q): %v", name, err)
		}
		if !extracted {
			t.Fatalf("expected files to be extracted for %q", name)
		}

		// Verify key files exist
		for _, f := range []string{"package.json", "schema.sql", "index.html", "vite.config.ts"} {
			path := filepath.Join(demoDir, f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("demo %q: expected file %s to exist", name, f)
			}
		}

		// Verify src directory exists
		srcDir := filepath.Join(demoDir, "src")
		if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
			t.Errorf("demo %q: expected src/ directory to exist", name)
		}
	}
}

func TestExtractDemoFilesSkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	demoDir := filepath.Join(tmpDir, "kanban")

	// Create directory with all required files to simulate existing extraction
	if err := os.MkdirAll(demoDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"package.json", "schema.sql", "vite.config.ts"} {
		if err := os.WriteFile(filepath.Join(demoDir, f), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	extracted, err := extractDemoFiles("kanban", demoDir)
	if err != nil {
		t.Fatalf("extractDemoFiles: %v", err)
	}
	if extracted {
		t.Fatal("expected extraction to be skipped when all required files exist")
	}
}

func TestEmbeddedDemoFSContainsSchemas(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		data, err := examples.FS.ReadFile(name + "/schema.sql")
		if err != nil {
			t.Errorf("reading embedded %s/schema.sql: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded %s/schema.sql is empty", name)
		}
		if !strings.Contains(string(data), "CREATE TABLE") {
			t.Errorf("embedded %s/schema.sql doesn't contain CREATE TABLE", name)
		}
	}
}

func TestDemoCommandHasDirFlag(t *testing.T) {
	flag := demoCmd.Flags().Lookup("dir")
	if flag == nil {
		t.Fatal("expected --dir flag on demo command")
	}
	if flag.DefValue != "." {
		t.Errorf("expected --dir default to be '.', got %q", flag.DefValue)
	}
}

func TestDemoRegistryNameConsistency(t *testing.T) {
	for key, demo := range demoRegistry {
		if key != demo.Name {
			t.Errorf("registry key %q != demo.Name %q", key, demo.Name)
		}
	}
}

func TestDemoRegistryDescriptionNonEmpty(t *testing.T) {
	for name, demo := range demoRegistry {
		if demo.Description == "" {
			t.Errorf("demo %q: Description is empty", name)
		}
	}
}

func TestEmbeddedDemoFSContainsAllConfigFiles(t *testing.T) {
	requiredFiles := []string{
		"index.html", "package.json", "schema.sql",
		"tsconfig.json", "vite.config.ts",
		"tailwind.config.js", "postcss.config.js",
	}
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		for _, f := range requiredFiles {
			data, err := examples.FS.ReadFile(name + "/" + f)
			if err != nil {
				t.Errorf("embedded %s/%s: %v", name, f, err)
				continue
			}
			if len(data) == 0 {
				t.Errorf("embedded %s/%s is empty", name, f)
			}
		}
	}
}

func TestEmbeddedPackageJSONHasDevScript(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		data, err := examples.FS.ReadFile(name + "/package.json")
		if err != nil {
			t.Errorf("reading embedded %s/package.json: %v", name, err)
			continue
		}
		content := string(data)
		if !strings.Contains(content, `"dev"`) {
			t.Errorf("embedded %s/package.json has no \"dev\" script", name)
		}
	}
}

func TestExtractDemoFilesContentIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	name := "kanban"
	demoDir := filepath.Join(tmpDir, name)

	extracted, err := extractDemoFiles(name, demoDir)
	if err != nil {
		t.Fatalf("extractDemoFiles(%q): %v", name, err)
	}
	if !extracted {
		t.Fatal("expected files to be extracted")
	}

	// Verify extracted content matches embedded content for key files
	for _, f := range []string{"package.json", "schema.sql", "vite.config.ts"} {
		embeddedData, err := examples.FS.ReadFile(name + "/" + f)
		if err != nil {
			t.Fatalf("reading embedded %s/%s: %v", name, f, err)
		}

		diskData, err := os.ReadFile(filepath.Join(demoDir, f))
		if err != nil {
			t.Fatalf("reading extracted %s/%s: %v", name, f, err)
		}

		if string(embeddedData) != string(diskData) {
			t.Errorf("%s/%s: extracted content does not match embedded content", name, f)
		}
	}
}

func TestExtractDemoFilesPartialRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	demoDir := filepath.Join(tmpDir, "kanban")

	// Simulate partial extraction: only package.json exists (missing schema.sql, vite.config.ts)
	if err := os.MkdirAll(demoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(demoDir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should re-extract because schema.sql and vite.config.ts are missing
	extracted, err := extractDemoFiles("kanban", demoDir)
	if err != nil {
		t.Fatalf("extractDemoFiles: %v", err)
	}
	if !extracted {
		t.Fatal("expected re-extraction when only package.json exists (partial extraction)")
	}

	// Verify all files now exist
	for _, f := range []string{"package.json", "schema.sql", "vite.config.ts", "index.html"} {
		if _, err := os.Stat(filepath.Join(demoDir, f)); os.IsNotExist(err) {
			t.Errorf("expected %s to exist after re-extraction", f)
		}
	}
}

func TestDemoValidArgsMatchRegistry(t *testing.T) {
	validArgs := demoCmd.ValidArgs
	if len(validArgs) != len(demoRegistry) {
		t.Fatalf("ValidArgs has %d entries but registry has %d", len(validArgs), len(demoRegistry))
	}
	for _, arg := range validArgs {
		if _, ok := demoRegistry[arg]; !ok {
			t.Errorf("ValidArg %q not found in demoRegistry", arg)
		}
	}
}

// TestEmbeddedSchemasUseCorrectRLSKey verifies all demo schemas reference
// the 'ayb.user_id' session variable that the AYB server actually sets
// (via SET LOCAL in internal/auth/rls.go). Any reference to 'request.jwt.sub'
// would silently break RLS policies and RPC functions at runtime.
func TestEmbeddedSchemasUseCorrectRLSKey(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		data, err := examples.FS.ReadFile(name + "/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)

		// Must use the key the server sets
		if !strings.Contains(content, "ayb.user_id") {
			t.Errorf("%s/schema.sql: does not reference 'ayb.user_id' — RLS policies won't work", name)
		}

		// Must NOT use the wrong key
		if strings.Contains(content, "request.jwt.sub") {
			t.Errorf("%s/schema.sql: contains 'request.jwt.sub' instead of 'ayb.user_id' — server sets ayb.user_id", name)
		}
	}
}

// TestEmbeddedSchemasHaveRLS verifies every demo schema enables row-level security.
func TestEmbeddedSchemasHaveRLS(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		data, err := examples.FS.ReadFile(name + "/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)
		if !strings.Contains(content, "ENABLE ROW LEVEL SECURITY") {
			t.Errorf("%s/schema.sql: does not enable RLS", name)
		}
		if !strings.Contains(content, "CREATE POLICY") {
			t.Errorf("%s/schema.sql: does not create any RLS policies", name)
		}
	}
}

// TestEmbeddedViteConfigPortsMatchRegistry verifies that each demo's vite.config.ts
// configures the port declared in the demo registry, preventing port mismatches.
func TestEmbeddedViteConfigPortsMatchRegistry(t *testing.T) {
	for name, demo := range demoRegistry {
		data, err := examples.FS.ReadFile(name + "/vite.config.ts")
		if err != nil {
			t.Fatalf("reading embedded %s/vite.config.ts: %v", name, err)
		}
		content := string(data)
		portStr := fmt.Sprintf("port: %d", demo.Port)
		if !strings.Contains(content, portStr) {
			t.Errorf("%s/vite.config.ts: does not contain %q (registry declares port %d)", name, portStr, demo.Port)
		}
	}
}

// TestEmbeddedViteConfigProxiesAPI verifies each demo proxies /api to the AYB server.
func TestEmbeddedViteConfigProxiesAPI(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "pixel-canvas"} {
		data, err := examples.FS.ReadFile(name + "/vite.config.ts")
		if err != nil {
			t.Fatalf("reading embedded %s/vite.config.ts: %v", name, err)
		}
		content := string(data)
		if !strings.Contains(content, `"/api"`) {
			t.Errorf("%s/vite.config.ts: does not proxy /api", name)
		}
		if !strings.Contains(content, "localhost:8090") {
			t.Errorf("%s/vite.config.ts: does not proxy to localhost:8090", name)
		}
	}
}
