package codehealth

import (
	"fmt"
	"go/version"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

const (
	govulncheckTarget  = "govulncheck"
	govulncheckVersion = "v1.6.0"
	goToolchainFloor   = "go1.26.5"
)

func TestMakefileGovulncheck(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))

	if !makefileDeclaresPhony(makefile, govulncheckTarget) {
		t.Fatalf(".PHONY must declare %s", govulncheckTarget)
	}
	if !strings.Contains(makefile, "\nGOVULNCHECK_VERSION ?= "+govulncheckVersion+"\n") {
		t.Fatalf("Makefile must pin GOVULNCHECK_VERSION to %s", govulncheckVersion)
	}
	if !containsStringField(makeTargetPrerequisites(makefile, "check"), govulncheckTarget) {
		t.Fatalf("check must depend on %s; got %v", govulncheckTarget, makeTargetPrerequisites(makefile, "check"))
	}

	recipe := strings.TrimSpace(extractMakeTargetRecipe(makefile, govulncheckTarget))
	wantRecipe := "GOTOOLCHAIN=auto go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./..."
	if recipe != wantRecipe {
		t.Fatalf("%s recipe = %q, want %q", govulncheckTarget, recipe, wantRecipe)
	}
}

func TestGoModToolchainFloor(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	goMod := readRepoText(t, filepath.Join(repoRoot, "go.mod"))

	if problem := goModToolchainFloorProblem([]byte(goMod), goToolchainFloor); problem != "" {
		t.Fatal(problem)
	}
}

func TestGoModToolchainFloorCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		toolchain   string
		wantProblem string
	}{
		{name: "exact floor", toolchain: goToolchainFloor},
		{name: "newer patch", toolchain: "go1.26.6"},
		{name: "newer minor", toolchain: "go1.27.0"},
		{name: "below patch", toolchain: "go1.26.4", wantProblem: "below required floor"},
		{name: "below minor", toolchain: "go1.25.99", wantProblem: "below required floor"},
		{name: "missing toolchain", wantProblem: "missing toolchain"},
		{name: "malformed toolchain", toolchain: "go1.26.x", wantProblem: "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problem := goModToolchainFloorProblem(testGoModWithToolchain(tc.toolchain), goToolchainFloor)
			if tc.wantProblem == "" && problem != "" {
				t.Fatalf("problem = %q, want none", problem)
			}
			if tc.wantProblem != "" && !strings.Contains(problem, tc.wantProblem) {
				t.Fatalf("problem = %q, want substring %q", problem, tc.wantProblem)
			}
		})
	}
}

func goModToolchainFloorProblem(data []byte, floor string) string {
	moduleFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Sprintf("parse go.mod failed: %v", err)
	}
	if moduleFile.Toolchain == nil || strings.TrimSpace(moduleFile.Toolchain.Name) == "" {
		return "go.mod is missing toolchain"
	}

	toolchain := strings.TrimSpace(moduleFile.Toolchain.Name)
	if !version.IsValid(toolchain) {
		return fmt.Sprintf("go.mod toolchain %q is invalid", toolchain)
	}
	if version.Compare(toolchain, floor) < 0 {
		return fmt.Sprintf("go.mod toolchain %s is below required floor %s", toolchain, floor)
	}
	return ""
}

func testGoModWithToolchain(toolchain string) []byte {
	lines := []string{
		"module example.com/test",
		"",
		"go 1.25.0",
	}
	if toolchain != "" {
		lines = append(lines, "", "toolchain "+toolchain)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
