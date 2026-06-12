package codehealth

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var leakedWorktreePathPattern = regexp.MustCompile(`/Users/[^/\s|]+/parallel_development/.*/allyourbase_dev/`)

type leakedPathFinding struct {
	Path    string
	Line    int
	Snippet string
}

func TestLeakedPathsGuardCatchesPlantedLeak(t *testing.T) {
	t.Parallel()

	tempRoot := t.TempDir()
	writePathLeakFixtureDebbie(t, tempRoot)
	writeTextFile(t, filepath.Join(tempRoot, "internal", "api", "leaky.go"), strings.Join([]string{
		"// Package api Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go.",
		"package api",
		"",
		"const bodyPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/body.go\"",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(tempRoot, "internal", "api", "DIRMAP.md"), strings.Join([]string{
		"<!-- [scrai:start] -->",
		"| File | Summary |",
		"| --- | --- |",
		"| leaky.go | Package api Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go. |",
		"<!-- [scrai:end] -->",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(tempRoot, "ui", "src", "header.ts"), strings.Join([]string{
		"// Module summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/header.ts.",
		"import { value } from './value'",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(tempRoot, "ui", "src", "body_only.ts"), strings.Join([]string{
		"export const browserPathPattern = /\\/Users/i",
		"export const localPath = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/body_only.ts\"",
		"",
	}, "\n"))

	findings, err := scanLeakedPaths(tempRoot)
	if err != nil {
		t.Fatalf("scan leaked paths failed: %v", err)
	}

	assertFindingPresent(t, findings, "internal/api/leaky.go", "Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go.")
	assertFindingPresent(t, findings, "internal/api/DIRMAP.md", "| leaky.go | Package api Stub summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/internal/api/leaky.go. |")
	assertFindingPresent(t, findings, "ui/src/header.ts", "Module summary for /Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev/ui/src/header.ts.")
	assertFindingAbsent(t, findings, "ui/src/body_only.ts")
}

func TestNoLeakedWorktreePaths(t *testing.T) {
	t.Parallel()

	root := findRepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, ".debbie.toml")); errors.Is(err, fs.ErrNotExist) {
		t.Skip(".debbie.toml absent (public mirror layout); leak guard runs on the dev source")
	}
	findings, err := scanLeakedPaths(root)
	if err != nil {
		t.Fatalf("scan leaked paths failed: %v", err)
	}
	if len(findings) > 0 {
		t.Fatalf("leaked worktree paths found:\n%s", formatLeakedPathFindings(findings))
	}
}

func scanLeakedPaths(root string) ([]leakedPathFinding, error) {
	syncScope, err := loadDebbieSyncScope(root)
	if err != nil {
		return nil, err
	}

	findings := make([]leakedPathFinding, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)

		if entry.IsDir() {
			if shouldSkipLeakedPathDir(relativePath) {
				return filepath.SkipDir
			}
			return nil
		}
		if !syncScope.includes(relativePath) || !supportsLeakedPathSurface(relativePath) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range leakedPathSurfaceLines(relativePath, string(content)) {
			if !containsLeakedWorktreePath(line.Text) {
				continue
			}
			findings = append(findings, leakedPathFinding{
				Path:    relativePath,
				Line:    line.Number,
				Snippet: strings.TrimSpace(line.Text),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Snippet < findings[j].Snippet
	})
	return findings, nil
}

func formatLeakedPathFindings(findings []leakedPathFinding) string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, fmt.Sprintf("%s:%d: %s", finding.Path, finding.Line, strings.TrimSpace(finding.Snippet)))
	}
	return strings.Join(lines, "\n")
}

type debbieSyncScope struct {
	dirs  []debbieSyncDir
	files map[string]struct{}
}

type debbieSyncDir struct {
	path     string
	excludes []string
}

type surfaceLine struct {
	Number int
	Text   string
}

func loadDebbieSyncScope(root string) (debbieSyncScope, error) {
	content, err := os.ReadFile(filepath.Join(root, ".debbie.toml"))
	if err != nil {
		return debbieSyncScope{}, fmt.Errorf("read .debbie.toml: %w", err)
	}
	return parseDebbieSyncScope(string(content)), nil
}

func parseDebbieSyncScope(content string) debbieSyncScope {
	scope := debbieSyncScope{files: make(map[string]struct{})}
	section := ""
	currentDir := -1
	arrayTarget := ""

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if arrayTarget != "" {
			applyDebbieArrayValues(&scope, currentDir, arrayTarget, trimmed)
			if strings.Contains(trimmed, "]") {
				arrayTarget = ""
			}
			continue
		}
		if updateDebbieSection(trimmed, &section, &currentDir, &scope) {
			continue
		}
		if section == "sync" && strings.HasPrefix(trimmed, "files") {
			arrayTarget = parseDebbieArrayLine(&scope, currentDir, "files", trimmed)
		}
		if section == "sync.dirs" && strings.HasPrefix(trimmed, "path") {
			scope.dirs[currentDir].path = normalizeDebbiePath(firstQuotedValue(trimmed))
		}
		if section == "sync.dirs" && strings.HasPrefix(trimmed, "exclude") {
			arrayTarget = parseDebbieArrayLine(&scope, currentDir, "exclude", trimmed)
		}
	}
	return scope
}

func updateDebbieSection(trimmed string, section *string, currentDir *int, scope *debbieSyncScope) bool {
	switch {
	case trimmed == "[sync]":
		*section = "sync"
		*currentDir = -1
		return true
	case trimmed == "[[sync.dirs]]":
		*section = "sync.dirs"
		scope.dirs = append(scope.dirs, debbieSyncDir{})
		*currentDir = len(scope.dirs) - 1
		return true
	case strings.HasPrefix(trimmed, "["):
		*section = ""
		*currentDir = -1
		return true
	default:
		return false
	}
}

func parseDebbieArrayLine(scope *debbieSyncScope, currentDir int, target, line string) string {
	applyDebbieArrayValues(scope, currentDir, target, line)
	if strings.Contains(line, "[") && !strings.Contains(line, "]") {
		return target
	}
	return ""
}

func applyDebbieArrayValues(scope *debbieSyncScope, currentDir int, target, line string) {
	for _, value := range quotedValues(line) {
		normalized := normalizeDebbiePath(value)
		if target == "files" {
			scope.files[normalized] = struct{}{}
		}
		if target == "exclude" && currentDir >= 0 {
			scope.dirs[currentDir].excludes = append(scope.dirs[currentDir].excludes, normalized)
		}
	}
}

func quotedValues(line string) []string {
	matches := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(line, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

func firstQuotedValue(line string) string {
	values := quotedValues(line)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func normalizeDebbiePath(path string) string {
	normalized := filepath.ToSlash(strings.TrimPrefix(path, "./"))
	return strings.TrimSuffix(normalized, "/")
}

func (scope debbieSyncScope) includes(relativePath string) bool {
	if _, ok := scope.files[relativePath]; ok {
		return true
	}
	for _, dir := range scope.dirs {
		if !pathIsWithinDir(relativePath, dir.path) {
			continue
		}
		if matchesAnyDebbieExclude(strings.TrimPrefix(relativePath, dir.path+"/"), dir.excludes) {
			return false
		}
		return true
	}
	return false
}

func pathIsWithinDir(relativePath, dir string) bool {
	return relativePath == dir || strings.HasPrefix(relativePath, dir+"/")
}

func matchesAnyDebbieExclude(relativePath string, excludes []string) bool {
	for _, exclude := range excludes {
		if relativePath == exclude || strings.HasPrefix(relativePath, exclude+"/") {
			return true
		}
		if matched, _ := filepath.Match(exclude, filepath.Base(relativePath)); matched {
			return true
		}
	}
	return false
}

func shouldSkipLeakedPathDir(relativePath string) bool {
	base := filepath.Base(relativePath)
	switch base {
	case ".git", ".dart_tool", ".gradle", ".vite", "__pycache__", "build", "coverage", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func supportsLeakedPathSurface(relativePath string) bool {
	if filepath.Base(relativePath) == "DIRMAP.md" {
		return true
	}
	return strings.HasSuffix(relativePath, ".go") || strings.HasSuffix(relativePath, ".ts") || strings.HasSuffix(relativePath, ".tsx")
}

func leakedPathSurfaceLines(relativePath, content string) []surfaceLine {
	switch {
	case filepath.Base(relativePath) == "DIRMAP.md":
		return dirmapRows(content)
	case strings.HasSuffix(relativePath, ".go"):
		return leadingGoPackageCommentLines(content)
	case strings.HasSuffix(relativePath, ".ts") || strings.HasSuffix(relativePath, ".tsx"):
		return leadingTypeScriptCommentLines(content)
	default:
		return nil
	}
}

func dirmapRows(content string) []surfaceLine {
	lines := strings.Split(content, "\n")
	rows := make([]surfaceLine, 0)
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			rows = append(rows, surfaceLine{Number: index + 1, Text: line})
		}
	}
	return rows
}

func leadingGoPackageCommentLines(content string) []surfaceLine {
	lines := strings.Split(content, "\n")
	leadingLines := make([]surfaceLine, 0)
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			break
		}
		leadingLines = append(leadingLines, surfaceLine{Number: index + 1, Text: line})
	}
	return leadingLines
}

func leadingTypeScriptCommentLines(content string) []surfaceLine {
	lines := strings.Split(content, "\n")
	leadingLines := make([]surfaceLine, 0)
	inBlockComment := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isLeadingTypeScriptCommentLine(trimmed, &inBlockComment) {
			leadingLines = append(leadingLines, surfaceLine{Number: index + 1, Text: line})
			continue
		}
		if trimmed == "" {
			continue
		}
		break
	}
	return leadingLines
}

func isLeadingTypeScriptCommentLine(trimmed string, inBlockComment *bool) bool {
	if *inBlockComment {
		if strings.Contains(trimmed, "*/") {
			*inBlockComment = false
		}
		return true
	}
	if strings.HasPrefix(trimmed, "//") {
		return true
	}
	if strings.HasPrefix(trimmed, "/*") {
		*inBlockComment = !strings.Contains(trimmed, "*/")
		return true
	}
	return false
}

func containsLeakedWorktreePath(line string) bool {
	return leakedWorktreePathPattern.MatchString(line) || strings.Contains(line, "parallel_development")
}

func assertFindingPresent(t *testing.T, findings []leakedPathFinding, path, snippet string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Path == path && strings.Contains(finding.Snippet, snippet) {
			return
		}
	}
	t.Fatalf("expected finding for %s containing %q, got:\n%s", path, snippet, formatLeakedPathFindings(findings))
}

func assertFindingAbsent(t *testing.T, findings []leakedPathFinding, path string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Path == path {
			t.Fatalf("expected no finding for %s, got: %q", path, finding.Snippet)
		}
	}
}

func writePathLeakFixtureDebbie(t *testing.T, root string) {
	t.Helper()
	writeTextFile(t, filepath.Join(root, ".debbie.toml"), strings.Join([]string{
		"[repos.dev]",
		"path = \"/Users/alice/parallel_development/allyourbase_dev/jun10/allyourbase_dev\"",
		"",
		"[sync]",
		"files = [\"README.md\"]",
		"",
		"[[sync.dirs]]",
		"path = \"internal/\"",
		"",
		"[[sync.dirs]]",
		"path = \"ui/\"",
		"",
	}, "\n"))
}
