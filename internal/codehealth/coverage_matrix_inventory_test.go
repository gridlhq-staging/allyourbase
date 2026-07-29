package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Column offsets into an "## Admin degraded-state inventory" row, after the
// leading and trailing table pipes have been dropped.
const (
	inventoryColumnScreen = iota
	inventoryColumnComponent
	inventoryColumnRequires
	inventoryColumnLoading
	inventoryColumnEmpty
	inventoryColumnError
	inventoryColumnRetry
	inventoryColumnEvidence
	inventoryColumnScreenSpec
	inventoryColumnUnmockedProof
	inventoryColumnCount
)

const degradedStateHeading = "## Admin degraded-state inventory"

// Canonical totals derived by hand from the audited rows. The matrix owns the
// rows, check-coverage-matrix.sh derives the totals from them, and this test
// owns the expected values — so a silently flipped status cell fails here.
var canonicalDegradedStateTotals = []string{
	"DEGRADED_STATE_LOADING:present=47 missing=0 not-applicable=3",
	"DEGRADED_STATE_EMPTY:present=43 missing=0 not-applicable=7",
	"DEGRADED_STATE_ERROR:present=47 missing=0 not-applicable=3",
	"DEGRADED_STATE_RETRY:present=47 missing=0 not-applicable=3",
	"DEGRADED_STATE_SCREEN_SPEC:present=49 missing=1",
	"DEGRADED_STATE_UNMOCKED_PROOF:proofs=47 screens=27",
}

func TestCheckCoverageMatrixScriptReportsDegradedStateInventoryTotals(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	output, err := runCoverageMatrixScript(t, repoRoot, "")
	if err != nil {
		t.Fatalf("expected canonical coverage matrix success, got error: %v output=%s", err, output)
	}

	screenCount := adminViewCount(t, repoRoot)
	wantInventory := "DEGRADED_STATE_INVENTORY:" + intToString(screenCount) + "/" + intToString(screenCount)
	if !strings.Contains(output, wantInventory) {
		t.Fatalf("expected %q in output, got: %s", wantInventory, output)
	}
	for _, want := range canonicalDegradedStateTotals {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output, got: %s", want, output)
		}
	}
}

func TestCheckCoverageMatrixScriptFailsForDegradedStateInventoryDefects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mutate      func(t *testing.T, section string) string
		wantMessage string
	}{
		{
			name: "registry id absent from inventory",
			mutate: func(t *testing.T, section string) string {
				return removeInventoryRow(t, section, "webhooks")
			},
			wantMessage: "missing degraded-state rows for registry screens: webhooks",
		},
		{
			name: "registry id listed twice",
			mutate: func(t *testing.T, section string) string {
				return duplicateInventoryRow(t, section, "webhooks")
			},
			wantMessage: "duplicate degraded-state rows: webhooks",
		},
		{
			name: "row for a screen that is not in the registry",
			mutate: func(t *testing.T, section string) string {
				return renameInventoryRow(t, section, "webhooks", "not-a-registry-screen")
			},
			wantMessage: "degraded-state rows for unknown screens: not-a-registry-screen",
		},
		{
			name: "blank status field",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnEmpty, "")
			},
			wantMessage: "webhooks empty status is \"\"",
		},
		{
			name: "status outside the controlled vocabulary",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnLoading, "partial")
			},
			wantMessage: "webhooks loading status is \"partial\"",
		},
		{
			name: "evidence entry set disagrees with the present statuses",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnRetry, "missing")
			},
			wantMessage: "webhooks has evidence for retry but its status is \"missing\"",
		},
		{
			name: "component file does not exist",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnComponent, "`Nonexistent.tsx`")
			},
			wantMessage: "webhooks component not found: ui/src/components/Nonexistent.tsx",
		},
		{
			name: "evidence file does not exist",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnEvidence,
					"loading=Nonexistent.tsx:L1; empty=Webhooks.tsx:L154; error=Webhooks.tsx:L121; retry=Webhooks.tsx:L129")
			},
			wantMessage: "webhooks loading evidence not found: ui/src/components/Nonexistent.tsx",
		},
		{
			name: "evidence line is past the end of the evidence file",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnEvidence,
					"loading=Webhooks.tsx:L99999; empty=Webhooks.tsx:L154; error=Webhooks.tsx:L121; retry=Webhooks.tsx:L129")
			},
			wantMessage: "webhooks loading evidence line 99999 is past the end of ui/src/components/Webhooks.tsx",
		},
		{
			name: "screen spec file does not exist",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnScreenSpec, "`nonexistent_spec.md`")
			},
			wantMessage: "webhooks screen spec not found: docs/reference/screen_specs/nonexistent_spec.md",
		},
		{
			name: "unmocked proof file does not exist",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnUnmockedProof,
					"empty=smoke/nonexistent.spec.ts")
			},
			wantMessage: "webhooks empty unmocked proof not found: ui/browser-tests-unmocked/smoke/nonexistent.spec.ts",
		},
		{
			name: "unmocked proof outside the smoke and full suites",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnUnmockedProof,
					"empty=fixtures/admin.ts")
			},
			wantMessage: "webhooks empty unmocked proof must be a smoke/ or full/ spec",
		},
		{
			name: "capability requirement disagrees with the registry",
			mutate: func(t *testing.T, section string) string {
				return mutateInventoryCell(t, section, "incidents", inventoryColumnRequires, "none")
			},
			wantMessage: "incidents requires is \"none\" but the registry declares \"status\"",
		},
		{
			name: "error not-applicable while retry is still claimed",
			mutate: func(t *testing.T, section string) string {
				section = mutateInventoryCell(t, section, "webhooks", inventoryColumnError, "not-applicable")
				return mutateInventoryCell(t, section, "webhooks", inventoryColumnEvidence,
					"loading=Webhooks.tsx:L107; empty=Webhooks.tsx:L154; retry=Webhooks.tsx:L129")
			},
			wantMessage: "webhooks retry status must be \"not-applicable\" when error is \"not-applicable\"",
		},
		{
			name: "inventory section absent",
			mutate: func(t *testing.T, _ string) string {
				return ""
			},
			wantMessage: "missing the \"" + degradedStateHeading + "\" section",
		},
	}

	repoRoot := findRepoRoot(t)
	canonical := canonicalDegradedStateSection(t, repoRoot)

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			matrixPath := filepath.Join(t.TempDir(), "COVERAGE_MATRIX.md")
			gapSummary := coverageMatrixGapSummaryMarkdown(t, repoRoot, 0, 0, 0, 25)
			writeTextFile(t, matrixPath, gapSummary+testCase.mutate(t, canonical))

			output, err := runCoverageMatrixScript(t, repoRoot, matrixPath)
			if err == nil {
				t.Fatalf("expected script failure, got success: %s", output)
			}
			if !strings.Contains(output, testCase.wantMessage) {
				t.Fatalf("expected failure output to contain %q, got: %s", testCase.wantMessage, output)
			}
		})
	}
}

func runCoverageMatrixScript(t *testing.T, repoRoot, matrixPath string) (string, error) {
	t.Helper()

	cmd := exec.Command("bash", checkCoverageMatrixScript)
	cmd.Dir = repoRoot
	if matrixPath != "" {
		cmd.Env = append(os.Environ(), "COVERAGE_MATRIX_PATH="+matrixPath)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func adminViewCount(t *testing.T, repoRoot string) int {
	t.Helper()

	source, err := os.ReadFile(filepath.Join(repoRoot, coverageMatrixAdminViewsPath))
	if err != nil {
		t.Fatalf("read admin views registry: %v", err)
	}
	return len(parseStringLiteralArray(t, string(source), "ADMIN_VIEWS", "admin views registry"))
}

// canonicalDegradedStateSection returns the shipped inventory section verbatim
// so red fixtures mutate real content rather than a hand-written stand-in that
// could drift away from what the gate actually parses.
func canonicalDegradedStateSection(t *testing.T, repoRoot string) string {
	t.Helper()

	source, err := os.ReadFile(filepath.Join(repoRoot, "scripts/COVERAGE_MATRIX.md"))
	if err != nil {
		t.Fatalf("read coverage matrix: %v", err)
	}
	start := strings.Index(string(source), degradedStateHeading)
	if start < 0 {
		t.Fatalf("coverage matrix is missing the %q section", degradedStateHeading)
	}

	section := string(source)[start:]
	if next := strings.Index(section[len(degradedStateHeading):], "\n## "); next >= 0 {
		section = section[:len(degradedStateHeading)+next+1]
	}
	return section
}

func inventoryRowLine(t *testing.T, section, screenID string) string {
	t.Helper()

	prefix := "| `" + screenID + "` |"
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("degraded-state inventory has no row for %q", screenID)
	return ""
}

func removeInventoryRow(t *testing.T, section, screenID string) string {
	t.Helper()

	return strings.Replace(section, inventoryRowLine(t, section, screenID)+"\n", "", 1)
}

func duplicateInventoryRow(t *testing.T, section, screenID string) string {
	t.Helper()

	row := inventoryRowLine(t, section, screenID)
	return strings.Replace(section, row+"\n", row+"\n"+row+"\n", 1)
}

func renameInventoryRow(t *testing.T, section, screenID, replacement string) string {
	t.Helper()

	return mutateInventoryCell(t, section, screenID, inventoryColumnScreen, "`"+replacement+"`")
}

func mutateInventoryCell(t *testing.T, section, screenID string, column int, value string) string {
	t.Helper()

	row := inventoryRowLine(t, section, screenID)
	cells := strings.Split(strings.Trim(row, "|"), "|")
	if len(cells) != inventoryColumnCount {
		t.Fatalf("row for %q has %d cells, want %d: %s", screenID, len(cells), inventoryColumnCount, row)
	}

	cells[column] = " " + value + " "
	return strings.Replace(section, row, "|"+strings.Join(cells, "|")+"|", 1)
}
