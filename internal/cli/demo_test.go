package cli

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/vector"
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
		"kanban":     5173,
		"live-polls": 5175,
		"movies":     5177,
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

func TestEmbeddedDemoFSContainsSchemas(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "movies"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
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

func TestEmbeddedKanbanAttachmentSchema(t *testing.T) {
	data, err := fs.ReadFile(examples.FS, "kanban/schema.sql")
	if err != nil {
		t.Fatalf("reading embedded kanban/schema.sql: %v", err)
	}
	content := string(data)
	if got := strings.Count(content, "CREATE TABLE IF NOT EXISTS attachments"); got != 1 {
		t.Fatalf("attachments table owner count = %d, want 1", got)
	}

	tableStart := strings.Index(content, "CREATE TABLE IF NOT EXISTS attachments")
	if tableStart < 0 {
		t.Fatal("kanban/schema.sql missing attachments table")
	}
	tableEnd := strings.Index(content[tableStart:], "\n);")
	if tableEnd < 0 {
		t.Fatal("kanban/schema.sql missing attachments table terminator")
	}
	attachmentsTable := content[tableStart : tableStart+tableEnd]

	requiredTableClauses := []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE",
		"bucket TEXT NOT NULL",
		"object_name TEXT NOT NULL",
		"file_name TEXT NOT NULL",
		"content_type TEXT NOT NULL",
		"size BIGINT NOT NULL",
		"user_id UUID NOT NULL REFERENCES _ayb_users(id)",
		"created_at TIMESTAMPTZ DEFAULT now()",
	}
	for _, clause := range requiredTableClauses {
		if !strings.Contains(attachmentsTable, clause) {
			t.Errorf("kanban/schema.sql attachments table missing clause %q", clause)
		}
	}

	requiredSchemaClauses := []string{
		"DROP POLICY IF EXISTS attachments_select ON attachments",
		"DROP POLICY IF EXISTS attachments_insert ON attachments",
		"DROP POLICY IF EXISTS attachments_update ON attachments",
		"DROP POLICY IF EXISTS attachments_delete ON attachments",
		"ALTER TABLE attachments ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY attachments_select ON attachments FOR SELECT USING (true)",
		"CREATE POLICY attachments_insert ON attachments FOR INSERT WITH CHECK (\n  user_id::text = current_setting('ayb.user_id', true)\n)",
		"CREATE POLICY attachments_delete ON attachments FOR DELETE USING (true)",
	}
	for _, clause := range requiredSchemaClauses {
		if !strings.Contains(content, clause) {
			t.Errorf("kanban/schema.sql missing attachment clause %q", clause)
		}
	}
	if strings.Contains(content, "CREATE POLICY attachments_update") {
		t.Error("kanban/schema.sql must not allow attachment metadata updates")
	}
}

func TestEmbeddedDemoFSContainsMoviesSeed(t *testing.T) {
	data, err := fs.ReadFile(examples.FS, "movies/seed.sql")
	if err != nil {
		t.Fatalf("reading embedded movies/seed.sql: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded movies/seed.sql is empty")
	}
	content := string(data)
	if !strings.Contains(content, "INSERT INTO movies") {
		t.Fatal("movies/seed.sql should insert into movies table")
	}
	if !strings.Contains(content, "ON CONFLICT") {
		t.Fatal("movies/seed.sql should be upsert-safe")
	}
}

func TestEmbeddedDemoFSContainsMoviesEmbeddingsArtifact(t *testing.T) {
	seed, err := fs.ReadFile(examples.FS, "movies/seed.sql")
	if err != nil {
		t.Fatalf("reading embedded movies/seed.sql: %v", err)
	}
	artifact, err := fs.ReadFile(examples.FS, "movies/embeddings.json")
	if err != nil {
		t.Fatalf("reading embedded movies/embeddings.json: %v", err)
	}
	decoded, err := vector.LoadCommittedMoviesEmbeddingArtifact(seed, artifact)
	if err != nil {
		t.Fatalf("embedded movies artifact should decode and match embedded seed checksum: %v", err)
	}
	if len(decoded.Records) == 0 {
		t.Fatal("embedded movies artifact has no records")
	}
}

func TestEmbeddedMoviesConfigWiresLocalOllamaAI(t *testing.T) {
	data, err := fs.ReadFile(examples.FS, "movies/ayb.toml")
	if err != nil {
		t.Fatalf("reading embedded movies/ayb.toml: %v", err)
	}
	cfg, err := config.ParseTOML(data)
	if err != nil {
		t.Fatalf("parsing embedded movies/ayb.toml: %v", err)
	}
	if cfg.AI.DefaultProvider != "ollama" {
		t.Fatalf("movies default AI provider = %q, want ollama", cfg.AI.DefaultProvider)
	}
	if cfg.AI.EmbeddingProvider != "ollama" {
		t.Fatalf("movies embedding AI provider = %q, want ollama", cfg.AI.EmbeddingProvider)
	}
	ollama, ok := cfg.AI.Providers["ollama"]
	if !ok {
		t.Fatal("movies ayb.toml must configure an ollama provider")
	}
	if ollama.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("movies ollama base_url = %q, want http://127.0.0.1:11434", ollama.BaseURL)
	}
	if ollama.DefaultModel == "" {
		t.Fatal("movies ollama provider must set a default chat model")
	}
	if cfg.AI.EmbeddingModel == "" {
		t.Fatal("movies ayb.toml must set an embedding model")
	}
}

func TestDemoServerStartCommandMoviesUsesEmbeddedConfig(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	cmd, cleanup, err := demoServerStartCommand("ayb", "movies")
	if err != nil {
		t.Fatalf("building movies start command: %v", err)
	}
	defer cleanup()
	if len(cmd.Args) != 4 {
		t.Fatalf("movies start args = %#v, want ayb start --config <path>", cmd.Args)
	}
	if cmd.Args[0] != "ayb" || cmd.Args[1] != "start" || cmd.Args[2] != "--config" {
		t.Fatalf("movies start args = %#v, want ayb start --config <path>", cmd.Args)
	}
	data, err := os.ReadFile(cmd.Args[3])
	if err != nil {
		t.Fatalf("reading materialized movies config: %v", err)
	}
	cfg, err := config.ParseTOML(data)
	if err != nil {
		t.Fatalf("parsing materialized movies config: %v", err)
	}
	if cfg.AI.DefaultProvider != "ollama" {
		t.Fatalf("materialized movies default AI provider = %q, want ollama", cfg.AI.DefaultProvider)
	}
	if _, ok := cfg.AI.Providers["ollama"]; !ok {
		t.Fatal("materialized movies config must include ollama provider")
	}
	info, err := os.Stat(cmd.Args[3])
	if err != nil {
		t.Fatalf("stat materialized movies config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("materialized movies config mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestDemoServerStartCommandOtherDemosUseDefaultConfigDiscovery(t *testing.T) {
	cmd, cleanup, err := demoServerStartCommand("ayb", "kanban")
	if err != nil {
		t.Fatalf("building kanban start command: %v", err)
	}
	defer cleanup()
	if strings.Join(cmd.Args, " ") != "ayb start" {
		t.Fatalf("kanban start args = %#v, want ayb start", cmd.Args)
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

func TestDemoLongLinksSearchGuides(t *testing.T) {
	expected := []string{
		"https://allyourbase.io/guide/search",
		"https://allyourbase.io/guide/migrating-from-algolia",
	}
	for _, text := range expected {
		if !strings.Contains(demoCmd.Long, text) {
			t.Errorf("demo long help should link %q", text)
		}
	}
}

// TestEmbeddedSchemasUseCorrectRLSKey verifies all demo schemas reference
// the 'ayb.user_id' session variable that the AYB server actually sets
// (via SET LOCAL in internal/auth/rls.go). Any reference to 'request.jwt.sub'
// would silently break RLS policies and RPC functions at runtime.
func TestEmbeddedSchemasUseCorrectRLSKey(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
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
	for _, name := range []string{"kanban", "live-polls"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
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

// TestDemoDistContainsIndexHTML verifies each demo's dist/ has an index.html.
func TestDemoDistContainsIndexHTML(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "movies"} {
		distFS, err := examples.DemoDist(name)
		if err != nil {
			t.Fatalf("DemoDist(%q): %v", name, err)
		}
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			t.Errorf("demo %q: dist/index.html not found: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("demo %q: dist/index.html is empty", name)
		}
	}
}

// TestDemoDistContainsAssets verifies each demo's dist/ has at least one JS and CSS file.
func TestDemoDistContainsAssets(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "movies"} {
		distFS, err := examples.DemoDist(name)
		if err != nil {
			t.Fatalf("DemoDist(%q): %v", name, err)
		}

		var hasJS, hasCSS bool
		fs.WalkDir(distFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".js") {
				hasJS = true
			}
			if strings.HasSuffix(path, ".css") {
				hasCSS = true
			}
			return nil
		})

		if !hasJS {
			t.Errorf("demo %q: dist/ has no .js files", name)
		}
		if !hasCSS {
			t.Errorf("demo %q: dist/ has no .css files", name)
		}
	}
}

// TestEmbeddedSchemasHaveCHECKConstraints verifies every demo schema has CHECK
// constraints on critical columns to prevent invalid data at the database level.
// Since there is no manual QA, CHECK constraints are the last line of defense.
func TestEmbeddedSchemasHaveCHECKConstraints(t *testing.T) {
	checks := map[string][]string{
		"live-polls": {"CHECK (length(question) > 0)", "CHECK (length(label) > 0)", "CHECK (position >= 0)"},
		"kanban":     {"CHECK (length(title) > 0)", "CHECK (position >= 0)"},
	}
	for name, expected := range checks {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)
		for _, check := range expected {
			if !strings.Contains(content, check) {
				t.Errorf("%s/schema.sql: missing CHECK constraint %q", name, check)
			}
		}
	}
}

// TestDemoTryStepsContainCorrectPort verifies that TrySteps URLs match the registered port.
func TestDemoTryStepsContainCorrectPort(t *testing.T) {
	for name, demo := range demoRegistry {
		portStr := fmt.Sprintf("localhost:%d", demo.Port)
		found := false
		for _, step := range demo.TrySteps {
			if strings.Contains(step, portStr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("demo %q: TrySteps don't reference port %d", name, demo.Port)
		}
	}
}

// TestResolveDemoAdminTokenMissingFile verifies that when the admin-token file
// doesn't exist, the error message gives actionable instructions (not a cryptic
// "admin-token not found"). This was a bug fix in session 186.
func TestResolveDemoAdminTokenMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AYB_ADMIN_TOKEN", "") // ensure env var doesn't short-circuit

	_, err := resolveDemoAdminToken("http://127.0.0.1:8090")
	if err == nil {
		t.Fatal("expected error when admin-token file missing")
	}
	msg := err.Error()
	// Should contain actionable instructions, not just "file not found".
	if !strings.Contains(msg, "ayb stop") {
		t.Errorf("error message should suggest 'ayb stop', got: %s", msg)
	}
	if !strings.Contains(msg, "ayb demo") {
		t.Errorf("error message should suggest 'ayb demo', got: %s", msg)
	}
}

// TestResolveDemoAdminTokenFromEnv verifies the AYB_ADMIN_TOKEN env var
// short-circuits file lookup.
func TestResolveDemoAdminTokenFromEnv(t *testing.T) {
	t.Setenv("AYB_ADMIN_TOKEN", "test-token-from-env")

	token, err := resolveDemoAdminToken("http://127.0.0.1:8090")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token-from-env" {
		t.Errorf("expected token from env, got %q", token)
	}
}

func TestDemoAuthEnabledReportsEnabledWhenRouteExists(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/me" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "missing token", http.StatusUnauthorized)
	}))
	defer ts.Close()

	enabled, err := demoAuthEnabled(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected auth to be reported as enabled")
	}
}

func TestDemoAuthEnabledReportsDisabledWhenRouteMissing(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	enabled, err := demoAuthEnabled(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected auth to be reported as disabled")
	}
}

func TestRequireDemoAuthEnabledReturnsActionableError(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	err := requireDemoAuthEnabled(ts.URL, false)
	if err == nil {
		t.Fatal("expected auth-disabled server to be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "auth disabled") {
		t.Fatalf("expected auth-disabled guidance, got: %s", msg)
	}
	if !strings.Contains(msg, "ayb stop && ayb demo <name>") {
		t.Fatalf("expected actionable restart instructions, got: %s", msg)
	}
}

// TestDemoAppPortDefaultsToRegistry proves that without the override env,
// effectiveDemoPort returns each demo's documented default so `ayb demo <name>`
// keeps its user-facing port.
func TestDemoAppPortDefaultsToRegistry(t *testing.T) {
	t.Setenv(demoAppPortOverrideEnv, "")
	for name, demo := range demoRegistry {
		if got := effectiveDemoPort(demo); got != demo.Port {
			t.Errorf("demo %q: expected default port %d, got %d", name, demo.Port, got)
		}
	}
}

// TestDemoAppPortOverrideFromEnv proves the release-gate smoke test can serve a
// demo on an isolated port via AYB_DEMO_APP_PORT, so the gate need not require
// the universal Vite defaults (5173/5175/5177) to be globally free.
func TestDemoAppPortOverrideFromEnv(t *testing.T) {
	demo := demoRegistry["kanban"]
	t.Setenv(demoAppPortOverrideEnv, "48173")
	if got := effectiveDemoPort(demo); got != 48173 {
		t.Fatalf("expected override port 48173, got %d", got)
	}
}

// TestDemoAppPortOverrideIgnoresInvalid proves a malformed or out-of-range
// override is ignored and the documented default is used, so a bad env value
// never silently breaks the user-facing demo port.
func TestDemoAppPortOverrideIgnoresInvalid(t *testing.T) {
	demo := demoRegistry["kanban"]
	for _, bad := range []string{"0", "-1", "70000", "abc", "  "} {
		t.Setenv(demoAppPortOverrideEnv, bad)
		if got := effectiveDemoPort(demo); got != demo.Port {
			t.Errorf("override %q: expected fallback to default %d, got %d", bad, demo.Port, got)
		}
	}
}

// TestDemoServerStartEnvUsesOverridePort proves the demo-started AYB server
// advertises the overridden app origin as its site URL, so WebAuthn origin
// checks stay consistent with the isolated port the demo actually serves on.
func TestDemoServerStartEnvUsesOverridePort(t *testing.T) {
	t.Setenv(demoAppPortOverrideEnv, "49173")
	env := demoServerStartEnv("secret", "kanban", "8090")
	want := "AYB_SERVER_SITE_URL=http://localhost:49173"
	found := false
	for _, kv := range env {
		if kv == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected env to contain %q, got %v", want, env)
	}
}

func TestDemoServerStartEnvStorageOverride(t *testing.T) {
	t.Setenv("AYB_STORAGE_ENABLED", "false")
	tests := []struct {
		demoName  string
		wantTrue  int
		wantFalse int
	}{
		{demoName: "kanban", wantTrue: 1, wantFalse: 0},
		{demoName: "live-polls", wantTrue: 0, wantFalse: 1},
		{demoName: "movies", wantTrue: 0, wantFalse: 1},
	}
	for _, tt := range tests {
		t.Run(tt.demoName, func(t *testing.T) {
			env := demoServerStartEnv("secret", tt.demoName, "8090")
			var trueCount, falseCount int
			for _, entry := range env {
				switch entry {
				case "AYB_STORAGE_ENABLED=true":
					trueCount++
				case "AYB_STORAGE_ENABLED=false":
					falseCount++
				}
			}
			if trueCount != tt.wantTrue {
				t.Errorf("AYB_STORAGE_ENABLED=true count = %d, want %d in %v", trueCount, tt.wantTrue, env)
			}
			if falseCount != tt.wantFalse {
				t.Errorf("AYB_STORAGE_ENABLED=false count = %d, want %d in %v", falseCount, tt.wantFalse, env)
			}
		})
	}
}
