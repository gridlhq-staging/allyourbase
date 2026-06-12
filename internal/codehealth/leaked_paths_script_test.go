package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stripLeakedPathsScript = "scripts/strip-leaked-paths.sh"

func TestStripLeakedPathsScriptRewritesSupportedSurfaces(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	tempRoot := t.TempDir()
	paths := writeStripLeakedPathsFixture(t, tempRoot)
	protectedBefore := readTextFile(t, paths.protectedConfig)

	output := runStripLeakedPathsScript(t, repoRoot, tempRoot)
	if output != "Leaked path strip completed.\n" {
		t.Fatalf("unexpected script output: %q", output)
	}

	assertFileContent(t, paths.goFile, strings.Join([]string{
		"// Package api Stub summary for internal/api/leaky.go.",
		"package api",
		"",
		"const retainedBodyPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/body.go\"",
		"",
	}, "\n"))
	assertFileContent(t, paths.tsFile, strings.Join([]string{
		"// Module summary for ui/src/header.ts.",
		"const retainedBodyPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/body.ts\"",
		"",
	}, "\n"))
	assertFileContent(t, paths.dirmapFile, strings.Join([]string{
		"<!-- [scrai:start] -->",
		"| File | Summary |",
		"| --- | --- |",
		"| leaky.go | Package api Stub summary for internal/api/leaky.go. |",
		"<!-- [scrai:end] -->",
		"",
	}, "\n"))
	assertFileContent(t, paths.protectedConfig, protectedBefore)
}

func TestStripLeakedPathsScriptIsIdempotent(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	tempRoot := t.TempDir()
	paths := writeStripLeakedPathsFixture(t, tempRoot)

	firstOutput := runStripLeakedPathsScript(t, repoRoot, tempRoot)
	firstSnapshot := snapshotFiles(t, paths.allFiles())
	secondOutput := runStripLeakedPathsScript(t, repoRoot, tempRoot)
	secondSnapshot := snapshotFiles(t, paths.allFiles())

	if firstOutput != "Leaked path strip completed.\n" || secondOutput != firstOutput {
		t.Fatalf("script output drift: first=%q second=%q", firstOutput, secondOutput)
	}
	for path, firstContent := range firstSnapshot {
		if secondSnapshot[path] != firstContent {
			t.Fatalf("file changed on second run: %s", path)
		}
	}
}

type stripFixturePaths struct {
	goFile          string
	tsFile          string
	dirmapFile      string
	protectedConfig string
}

func (paths stripFixturePaths) allFiles() []string {
	return []string{paths.goFile, paths.tsFile, paths.dirmapFile, paths.protectedConfig}
}

func writeStripLeakedPathsFixture(t *testing.T, root string) stripFixturePaths {
	t.Helper()
	writePathLeakFixtureDebbie(t, root)

	paths := stripFixturePaths{
		goFile:          filepath.Join(root, "internal", "api", "leaky.go"),
		tsFile:          filepath.Join(root, "ui", "src", "header.ts"),
		dirmapFile:      filepath.Join(root, "internal", "api", "DIRMAP.md"),
		protectedConfig: filepath.Join(root, ".debbie.toml"),
	}
	writeTextFile(t, paths.goFile, strings.Join([]string{
		"// Package api Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go.",
		"package api",
		"",
		"const retainedBodyPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/body.go\"",
		"",
	}, "\n"))
	writeTextFile(t, paths.tsFile, strings.Join([]string{
		"// Module summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/header.ts.",
		"const retainedBodyPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/body.ts\"",
		"",
	}, "\n"))
	writeTextFile(t, paths.dirmapFile, strings.Join([]string{
		"<!-- [scrai:start] -->",
		"| File | Summary |",
		"| --- | --- |",
		"| leaky.go | Package api Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go. |",
		"<!-- [scrai:end] -->",
		"",
	}, "\n"))
	return paths
}

func runStripLeakedPathsScript(t *testing.T, repoRoot, scanRoot string) string {
	t.Helper()
	cmd := exec.Command("bash", stripLeakedPathsScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "STRIP_LEAKED_PATHS_ROOT="+scanRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("strip leaked paths script failed: %v output=%s", err, output)
	}
	return string(output)
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(content)
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	if actual := readTextFile(t, path); actual != expected {
		t.Fatalf("unexpected content for %s:\nwant:\n%s\ngot:\n%s", path, expected, actual)
	}
}

func snapshotFiles(t *testing.T, paths []string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string, len(paths))
	for _, path := range paths {
		snapshot[path] = readTextFile(t, path)
	}
	return snapshot
}
