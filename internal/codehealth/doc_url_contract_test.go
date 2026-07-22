package codehealth

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const allyourbaseDocURLBase = "https://allyourbase.io"

var allowedDocURLLiteralFiles = map[string]struct{}{
	"internal/httputil/response.go": {},
	// Sibling-owned start*.go cleanup closes separately; reviewed 2026-07-20.
	"internal/cli/start_banner.go": {},
	// Prose/template markdown links, not response doc_url owners; reviewed 2026-07-20 at merge.
	"internal/cli/demo.go":                           {},
	"internal/scaffold/scaffold_templates_common.go": {},
}

type docURLLiteralFinding struct {
	Path string
	Line int
}

func TestDocURLLiteralHasSingleOwner(t *testing.T) {
	t.Parallel()

	findings, err := scanDocURLLiterals(findRepoRoot(t), allowedDocURLLiteralFiles)
	if err != nil {
		t.Fatalf("scan documentation URL literals: %v", err)
	}
	if len(findings) > 0 {
		t.Fatalf("documentation URL literals found outside the canonical owner:\n%s", formatDocURLLiteralFindings(findings))
	}
}

func TestDocURLLiteralScannerKnownAnswer(t *testing.T) {
	t.Parallel()

	fixtureRoot := t.TempDir()
	writeTextFile(t, filepath.Join(fixtureRoot, "internal", "httputil", "response.go"), `package httputil

const docs = "`+allyourbaseDocURLBase+`/guide/configuration"
`)
	writeTextFile(t, filepath.Join(fixtureRoot, "internal", "cli", "start_banner.go"), `package cli

const docs = "`+allyourbaseDocURLBase+`/guide/configuration"
`)
	writeTextFile(t, filepath.Join(fixtureRoot, "internal", "auth", "handler_test.go"), `package auth

const docs = "`+allyourbaseDocURLBase+`/guide/authentication"
`)
	writeTextFile(t, filepath.Join(fixtureRoot, "internal", "auth", "handler.go"), `package auth

const docs = "`+allyourbaseDocURLBase+`/guide/authentication"
const warning = "See `+allyourbaseDocURLBase+`/guide/authentication for details"
`)

	findings, err := scanDocURLLiterals(fixtureRoot, allowedDocURLLiteralFiles)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	want := []docURLLiteralFinding{
		{Path: "internal/auth/handler.go", Line: 3},
		{Path: "internal/auth/handler.go", Line: 4},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Fatalf("fixture findings mismatch\nwant: %#v\n got: %#v", want, findings)
	}

	missingRoot := filepath.Join(fixtureRoot, "missing")
	if _, err := scanDocURLLiterals(missingRoot, allowedDocURLLiteralFiles); err == nil {
		t.Fatal("scan missing fixture root succeeded, want error")
	}
}

func scanDocURLLiterals(repoRoot string, allowedFiles map[string]struct{}) ([]docURLLiteralFinding, error) {
	internalRoot := filepath.Join(repoRoot, "internal")
	findings := make([]docURLLiteralFinding, 0)
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if _, allowed := allowedFiles[relativePath]; allowed {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, content, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(value, allyourbaseDocURLBase) {
				return true
			}
			position := fileSet.Position(lit.Pos())
			findings = append(findings, docURLLiteralFinding{Path: relativePath, Line: position.Line})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

func formatDocURLLiteralFindings(findings []docURLLiteralFinding) string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, fmt.Sprintf("%s:%d", finding.Path, finding.Line))
	}
	return strings.Join(lines, "\n")
}
