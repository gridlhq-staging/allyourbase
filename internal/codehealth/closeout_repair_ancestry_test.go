package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const closeoutRepairAncestryScript = "scripts/check-closeout-repair-ancestry.sh"

type closeoutRepairAncestryResult struct {
	output   string
	exitCode int
}

type closeoutRepairGitFixture struct {
	repoRoot     string
	ancestorSHA  string
	certifiedSHA string
	repairSHAs   []string
}

func TestCheckCloseoutRepairAncestryRejectsNonAncestorRepairs(t *testing.T) {
	t.Parallel()

	gitFixture := newCloseoutRepairGitFixture(t)
	closeoutDir := t.TempDir()
	writeTextFile(t, filepath.Join(closeoutDir, "historical-incident.md"), strings.Join([]string{
		"# Historical Incident",
		"",
		"## Verdict",
		"GO on pinned evidence tree " + gitFixture.certifiedSHA,
		"",
		"## Repairs",
		"- Shell detector repaired at " + gitFixture.repairSHAs[0],
		"- Fixture parser repaired at " + gitFixture.repairSHAs[1],
		"- Ancestry guard repaired at " + gitFixture.repairSHAs[2],
		"",
	}, "\n"))

	result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
	if result.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 repair_shas_checked=3 violations=3 indeterminate=0",
	)
	assertCloseoutRepairOutput(t, result.output,
		gitFixture.repairSHAs[0],
		gitFixture.repairSHAs[1],
		gitFixture.repairSHAs[2],
	)
}

func TestCheckCloseoutRepairAncestryAcceptsAncestorRepair(t *testing.T) {
	t.Parallel()

	gitFixture := newCloseoutRepairGitFixture(t)
	closeoutDir := t.TempDir()
	writeTextFile(t, filepath.Join(closeoutDir, "ancestor-repair.md"), strings.Join([]string{
		"# Ancestor Repair",
		"",
		"## Verdict",
		"GO on pinned evidence tree " + gitFixture.certifiedSHA,
		"",
		"## Repairs",
		"- Detector contract repaired at " + gitFixture.ancestorSHA,
		"",
	}, "\n"))

	result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=PASS artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 repair_shas_checked=1 violations=0 indeterminate=0",
	)
	assertCloseoutRepairOutput(t, result.output,
		gitFixture.ancestorSHA,
	)
}

func TestCheckCloseoutRepairAncestryReportsVacuousSkippedCorpus(t *testing.T) {
	t.Parallel()

	closeoutDir := t.TempDir()
	writeTextFile(t, filepath.Join(closeoutDir, "withdrawn-closeout.md"), strings.Join([]string{
		"# Withdrawn Closeout",
		"",
		"## Verdict",
		"GO WITHDRAWN",
		"",
		"Earlier historical notes mentioned GO on pinned evidence tree fb6f291e7bade309afc90d98beed5490180cd814.",
		"",
		"## Repairs",
		"- Historical note repaired at 0cfda34eb",
		"",
	}, "\n"))

	result := runCloseoutRepairAncestryScript(t, findRepoRoot(t), closeoutDir)
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=1 repair_shas_checked=0 violations=0 indeterminate=0",
	)
}

func TestCheckCloseoutRepairAncestryDefaultsToChatsICG(t *testing.T) {
	t.Parallel()

	fixtureRoot := t.TempDir()
	writeTextFile(t, filepath.Join(fixtureRoot, "chats", "icg", "withdrawn-closeout.md"), strings.Join([]string{
		"# Withdrawn Closeout",
		"",
		"## Verdict",
		"GO WITHDRAWN",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(fixtureRoot, "outside-default-scan.md"), "# Not a closeout\n")

	result := runCloseoutRepairAncestryScript(t, fixtureRoot)
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=1 repair_shas_checked=0 violations=0 indeterminate=0",
	)
}

func TestCheckCloseoutRepairAncestryFailsClosedOnInvalidReferences(t *testing.T) {
	t.Parallel()

	gitFixture := newCloseoutRepairGitFixture(t)
	certifiedSHA := gitFixture.certifiedSHA
	repairSHA := gitFixture.ancestorSHA
	testCases := []struct {
		name        string
		verdict     string
		repair      string
		wantSummary string
		wantError   string
	}{
		{
			name:        "unmatched verdict emphasis",
			verdict:     "**GO on pinned evidence tree " + certifiedSHA,
			repair:      repairSHA,
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=1",
			wantError:   "unmatched Markdown emphasis wrapper",
		},
		{
			name:        "unresolvable certified SHA",
			verdict:     "GO on pinned evidence tree 1111111",
			repair:      repairSHA,
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=1",
			wantError:   "certified SHA 1111111",
		},
		{
			name:        "repair reference continues after SHA",
			verdict:     "GO on pinned evidence tree " + certifiedSHA,
			repair:      repairSHA + " and later prose",
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=1",
			wantError:   "continues after credited SHA",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			closeoutDir := t.TempDir()
			writeTextFile(t, filepath.Join(closeoutDir, "invalid-reference.md"), strings.Join([]string{
				"## Verdict",
				testCase.verdict,
				"## Repairs",
				"- Detector contract repaired at " + testCase.repair,
			}, "\n"))

			result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
			if result.exitCode != 1 {
				t.Fatalf("exit code = %d, want 1, output:\n%s", result.exitCode, result.output)
			}
			assertCloseoutRepairSummary(t, result.output, testCase.wantSummary)
			assertCloseoutRepairOutput(t, result.output, testCase.wantError)
		})
	}
}

func TestCheckCloseoutRepairAncestryReportsMissingDirectoryAsVacuous(t *testing.T) {
	t.Parallel()

	closeoutDir := filepath.Join(t.TempDir(), "missing")
	result := runCloseoutRepairAncestryScript(t, findRepoRoot(t), closeoutDir)
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=VACUOUS artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=0",
	)
	assertCloseoutRepairOutput(t, result.output, "does not exist")
}

func TestCheckCloseoutRepairAncestryRejectsNonDirectoryInput(t *testing.T) {
	t.Parallel()

	closeoutPath := filepath.Join(t.TempDir(), "closeout.md")
	writeTextFile(t, closeoutPath, "# Not a directory\n")
	result := runCloseoutRepairAncestryScript(t, findRepoRoot(t), closeoutPath)
	if result.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=FAIL artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=1",
	)
	assertCloseoutRepairOutput(t, result.output, "exists but is not a directory")
}

func TestCheckCloseoutRepairAncestryRejectsSymlinkedInputs(t *testing.T) {
	t.Parallel()

	gitFixture := newCloseoutRepairGitFixture(t)
	fixtureRoot := t.TempDir()
	targetDir := filepath.Join(fixtureRoot, "target")
	targetArtifact := filepath.Join(targetDir, "violation.md")
	writeTextFile(t, targetArtifact, strings.Join([]string{
		"## Verdict",
		"GO on pinned evidence tree " + gitFixture.certifiedSHA,
		"## Repairs",
		"- Detector repaired at " + gitFixture.repairSHAs[0],
	}, "\n"))

	testCases := []struct {
		name       string
		prepareDir func(*testing.T) string
	}{
		{
			name: "directory symlink",
			prepareDir: func(t *testing.T) string {
				closeoutDir := filepath.Join(fixtureRoot, "directory-link")
				if err := os.Symlink(targetDir, closeoutDir); err != nil {
					t.Fatalf("create directory symlink: %v", err)
				}
				return closeoutDir
			},
		},
		{
			name: "artifact symlink",
			prepareDir: func(t *testing.T) string {
				closeoutDir := filepath.Join(fixtureRoot, "artifact-link-corpus")
				if err := os.Mkdir(closeoutDir, 0o755); err != nil {
					t.Fatalf("create closeout directory: %v", err)
				}
				if err := os.Symlink(targetArtifact, filepath.Join(closeoutDir, "violation.md")); err != nil {
					t.Fatalf("create artifact symlink: %v", err)
				}
				return closeoutDir
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			closeoutDir := testCase.prepareDir(t)
			result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
			if result.exitCode != 1 {
				t.Fatalf("exit code = %d, want 1, output:\n%s", result.exitCode, result.output)
			}
			assertCloseoutRepairSummary(t, result.output,
				"SUMMARY result=FAIL artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 repair_shas_checked=0 violations=0 indeterminate=1",
			)
			assertCloseoutRepairOutput(t, result.output, "symbolic link")
		})
	}
}

func runCloseoutRepairAncestryScript(t *testing.T, workingDir string, scriptArgs ...string) closeoutRepairAncestryResult {
	t.Helper()

	repoRoot := findRepoRoot(t)
	scriptPath := filepath.Join(repoRoot, filepath.FromSlash(closeoutRepairAncestryScript))
	args := append([]string{scriptPath}, scriptArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = workingDir

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("script execution failed: %v output=%s", err, output)
		}
		exitCode = exitError.ExitCode()
	}
	return closeoutRepairAncestryResult{output: string(output), exitCode: exitCode}
}

func newCloseoutRepairGitFixture(t *testing.T) closeoutRepairGitFixture {
	t.Helper()

	repoRoot := t.TempDir()
	runCloseoutRepairFixtureGit(t, repoRoot, "", "init", "--quiet")
	emptyTree := runCloseoutRepairFixtureGit(t, repoRoot, "", "mktree")
	commit := func(label, parent string) string {
		t.Helper()
		args := []string{
			"-c", "user.name=Closeout Fixture",
			"-c", "user.email=closeout-fixture@example.invalid",
			"commit-tree", emptyTree, "-m", label,
		}
		if parent != "" {
			args = append(args, "-p", parent)
		}
		return runCloseoutRepairFixtureGit(t, repoRoot, "", args...)
	}

	ancestorSHA := commit("ancestor fixture", "")
	certifiedSHA := commit("certified fixture", ancestorSHA)
	repairSHAs := []string{commit("repair fixture one", ""), "", ""}
	repairSHAs[1] = commit("repair fixture two", repairSHAs[0])
	repairSHAs[2] = commit("repair fixture three", repairSHAs[1])

	return closeoutRepairGitFixture{
		repoRoot:     repoRoot,
		ancestorSHA:  ancestorSHA,
		certifiedSHA: certifiedSHA,
		repairSHAs:   repairSHAs,
	}
}

func runCloseoutRepairFixtureGit(t *testing.T, workingDir, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = workingDir
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v output=%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func assertCloseoutRepairOutput(t *testing.T, output string, wantSubstrings ...string) {
	t.Helper()

	for _, want := range wantSubstrings {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func assertCloseoutRepairSummary(t *testing.T, output, want string) {
	t.Helper()

	var summaries []string
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "SUMMARY ") {
			summaries = append(summaries, line)
		}
	}
	if len(summaries) != 1 {
		t.Fatalf("summary line count = %d, want 1, output:\n%s", len(summaries), output)
	}
	if summaries[0] != want {
		t.Fatalf("summary mismatch\nwant: %s\ngot:  %s", want, summaries[0])
	}
}
