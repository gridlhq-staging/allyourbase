package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebbieHooksRemoveIgnoredDemoRuntimeArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	debbieConfig := filepath.Join(repoRoot, ".debbie.toml")
	if _, err := os.Stat(debbieConfig); os.IsNotExist(err) {
		t.Skip(".debbie.toml and its source-only hooks are intentionally omitted from public mirrors")
	} else if err != nil {
		t.Fatalf("inspect Debbie configuration: %v", err)
	}

	debbieRoot := filepath.Join(repoRoot, ".debbie")
	targetRoot := t.TempDir()
	runtimePaths := []string{
		filepath.Join("examples", "instantsearch_demo", "playwright-report", "results.json"),
		filepath.Join("examples", "live-polls", "test-results", "trace.zip"),
	}
	for _, relativePath := range runtimePaths {
		path := filepath.Join(targetRoot, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create runtime artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("runtime artifact"), 0o644); err != nil {
			t.Fatalf("write runtime artifact: %v", err)
		}
	}
	keptPath := filepath.Join(targetRoot, "examples", "live-polls", "src", "main.ts")
	if err := os.MkdirAll(filepath.Dir(keptPath), 0o755); err != nil {
		t.Fatalf("create retained source directory: %v", err)
	}
	if err := os.WriteFile(keptPath, []byte("export {}"), 0o644); err != nil {
		t.Fatalf("write retained source: %v", err)
	}

	cleanupScript := filepath.Join(debbieRoot, "remove_test_artifacts.sh")
	for run := 1; run <= 2; run++ {
		if output, err := exec.Command("bash", cleanupScript, targetRoot).CombinedOutput(); err != nil {
			t.Fatalf("cleanup run %d failed: %v\n%s", run, err, output)
		}
	}
	for _, relativePath := range runtimePaths {
		if _, err := os.Stat(filepath.Join(targetRoot, relativePath)); !os.IsNotExist(err) {
			t.Fatalf("runtime artifact %s survived cleanup: %v", relativePath, err)
		}
	}
	if content, err := os.ReadFile(keptPath); err != nil || string(content) != "export {}" {
		t.Fatalf("cleanup changed retained source: content=%q err=%v", content, err)
	}

	requiredInvocation := `bash "$DEV_ROOT/.debbie/remove_test_artifacts.sh" "$TARGET_ROOT"`
	for _, hookName := range []string{"post-sync-staging.sh", "post-sync-prod.sh"} {
		hook := readRepoText(t, filepath.Join(debbieRoot, hookName))
		if !strings.Contains(hook, requiredInvocation) {
			t.Fatalf("%s must invoke shared runtime-artifact cleanup", hookName)
		}
	}
}

func TestDebbieHooksRehydratePublicScreenSpecsAfterDocsCleanup(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	debbieConfigPath := filepath.Join(repoRoot, ".debbie.toml")
	if _, err := os.Stat(debbieConfigPath); os.IsNotExist(err) {
		t.Skip(".debbie.toml and its source-only hooks are intentionally omitted from public mirrors")
	} else if err != nil {
		t.Fatalf("inspect Debbie configuration: %v", err)
	}

	config := readRepoText(t, debbieConfigPath)
	if !strings.Contains(config, `path = "docs/reference/screen_specs/"`) {
		t.Fatal(".debbie.toml must include the screen-spec corpus consumed by the public coverage gate")
	}

	debbieRoot := filepath.Join(repoRoot, ".debbie")
	docsCleanup := `"$TARGET_ROOT/docs" \`
	screenSpecSource := `"$DEV_ROOT/docs/reference/screen_specs/"`
	screenSpecTarget := `"$TARGET_ROOT/docs/reference/screen_specs/"`
	for _, hookName := range []string{"post-sync-staging.sh", "post-sync-prod.sh"} {
		hook := readRepoText(t, filepath.Join(debbieRoot, hookName))
		cleanupIndex := strings.Index(hook, docsCleanup)
		sourceIndex := strings.Index(hook, screenSpecSource)
		targetIndex := strings.Index(hook, screenSpecTarget)
		if cleanupIndex < 0 || sourceIndex < 0 || targetIndex < 0 {
			t.Fatalf(
				"%s must remove the private docs tree and rehydrate the public screen-spec corpus",
				hookName,
			)
		}
		if sourceIndex < cleanupIndex || targetIndex < cleanupIndex {
			t.Fatalf("%s rehydrates screen specs before the docs cleanup deletes them", hookName)
		}
	}
}
