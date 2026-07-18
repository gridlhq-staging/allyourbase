package codehealth

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// releaseSalvageScript is the release-lane salvage ancestry guard that Stage 2
// implements. Until then this contract test is RED because the file is absent.
const releaseSalvageScript = "scripts/check-release-lane-salvage.sh"

// salvageUnresolvableSHA is a syntactically valid 40-hex object id that is not a
// commit in the fixture repository, so ancestry resolution must fail for it.
// salvageNonHexArg is a 40-character command-line value that is not hex and must
// therefore be rejected as a usage error rather than treated as a commit id.
var (
	salvageUnresolvableSHA = strings.Repeat("a", 40)
	salvageNonHexArg       = strings.Repeat("z", 40)
)

// salvageFixture is a throwaway git repository with an ancestor commit (shaA on
// main) and an unmerged side-branch commit (shaB) used to exercise the guard.
type salvageFixture struct {
	repoDir string
	shaA    string
	shaB    string
}

// salvageResult captures the guard script's combined output and exit code.
type salvageResult struct {
	output   string
	exitCode int
}

// salvageContractCase is one table-driven expectation. The func fields defer to
// runtime because the argument list and expected substrings depend on the
// fixture's freshly-generated commit SHAs.
type salvageContractCase struct {
	name         string
	buildArgs    func(fixture salvageFixture, followupsPath string) []string
	followups    func(fixture salvageFixture) string
	wantExit     int
	wantContains func(fixture salvageFixture) []string
	wantAbsent   func(fixture salvageFixture) []string
	wantUsage    bool
}

func TestCheckReleaseLaneSalvageScript(t *testing.T) {
	t.Parallel()

	for _, tc := range salvageContractCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := buildSalvageFixture(t)
			followupsPath := ""
			if tc.followups != nil {
				followupsPath = filepath.Join(t.TempDir(), "followups.txt")
				writeTextFile(t, followupsPath, tc.followups(fixture))
			}

			result := runReleaseSalvageScript(t, fixture.repoDir, tc.buildArgs(fixture, followupsPath))
			if result.exitCode != tc.wantExit {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, tc.wantExit, result.output)
			}
			assertSalvageOutput(t, tc, fixture, result)
		})
	}
}

func assertSalvageOutput(t *testing.T, tc salvageContractCase, fixture salvageFixture, result salvageResult) {
	t.Helper()

	if tc.wantContains != nil {
		for _, want := range tc.wantContains(fixture) {
			if !strings.Contains(result.output, want) {
				t.Fatalf("expected output to name %q, got:\n%s", want, result.output)
			}
		}
	}
	if tc.wantAbsent != nil {
		for _, absent := range tc.wantAbsent(fixture) {
			if strings.Contains(result.output, absent) {
				t.Fatalf("expected output to omit %q, got:\n%s", absent, result.output)
			}
		}
	}
	if tc.wantUsage && !strings.Contains(strings.ToLower(result.output), "usage") {
		t.Fatalf("expected usage text, got:\n%s", result.output)
	}
}

func salvageContractCases() []salvageContractCase {
	cases := salvageAncestryCases()
	cases = append(cases, salvageFollowupCases()...)
	cases = append(cases, salvageUsageCases()...)
	return cases
}

// salvageAncestryCases cover resolution against the base ref with no follow-up file.
func salvageAncestryCases() []salvageContractCase {
	orphanNamesB := func(fixture salvageFixture) []string { return []string{fixture.shaB} }
	return []salvageContractCase{
		{
			name:         "reject orphaned side branch commit",
			buildArgs:    func(f salvageFixture, _ string) []string { return salvageArgs("", f.shaB) },
			wantExit:     1,
			wantContains: orphanNamesB,
		},
		{
			name:      "accept ancestor commit on base",
			buildArgs: func(f salvageFixture, _ string) []string { return salvageArgs("", f.shaA) },
			wantExit:  0,
		},
		{
			name:         "reject mixed commits reports only orphan",
			buildArgs:    func(f salvageFixture, _ string) []string { return salvageArgs("", f.shaA, f.shaB) },
			wantExit:     1,
			wantContains: orphanNamesB,
			wantAbsent:   func(f salvageFixture) []string { return []string{f.shaA} },
		},
		{
			name:         "reject unresolvable sha without followup",
			buildArgs:    func(f salvageFixture, _ string) []string { return salvageArgs("", salvageUnresolvableSHA) },
			wantExit:     1,
			wantContains: func(salvageFixture) []string { return []string{salvageUnresolvableSHA} },
		},
	}
}

// salvageFollowupCases cover follow-up ownership matching in the --followups file.
func salvageFollowupCases() []salvageContractCase {
	checkB := func(f salvageFixture, p string) []string { return salvageArgs(p, f.shaB) }
	orphanNamesB := func(fixture salvageFixture) []string { return []string{fixture.shaB} }
	return []salvageContractCase{
		{
			name:      "accept orphan named by full sha followup",
			followups: func(f salvageFixture) string { return f.shaB + "\n" },
			buildArgs: checkB,
			wantExit:  0,
		},
		{
			name:      "accept orphan named by twelve hex prefix",
			followups: func(f salvageFixture) string { return f.shaB[:12] + "\n" },
			buildArgs: checkB,
			wantExit:  0,
		},
		{
			name:         "reject orphan named by eight hex prefix",
			followups:    func(f salvageFixture) string { return f.shaB[:8] + "\n" },
			buildArgs:    checkB,
			wantExit:     1,
			wantContains: orphanNamesB,
		},
		{
			name:         "reject followup file omitting the sha",
			followups:    func(salvageFixture) string { return salvageUnresolvableSHA + "\n" },
			buildArgs:    checkB,
			wantExit:     1,
			wantContains: orphanNamesB,
		},
		{
			name:         "reject token embedding prefix off the start",
			followups:    func(f salvageFixture) string { return divergentHexDigit(f.shaB[0]) + f.shaB[:12] + "\n" },
			buildArgs:    checkB,
			wantExit:     1,
			wantContains: orphanNamesB,
		},
		{
			name:         "reject token whose thirteenth char diverges",
			followups:    func(f salvageFixture) string { return f.shaB[:12] + divergentHexDigit(f.shaB[12]) + "\n" },
			buildArgs:    checkB,
			wantExit:     1,
			wantContains: orphanNamesB,
		},
		{
			name:      "accept unresolvable sha named by followup",
			followups: func(salvageFixture) string { return salvageUnresolvableSHA + "\n" },
			buildArgs: func(f salvageFixture, p string) []string { return salvageArgs(p, salvageUnresolvableSHA) },
			wantExit:  0,
		},
	}
}

// salvageUsageCases cover argument-shape rejections that must exit 2 with usage text.
func salvageUsageCases() []salvageContractCase {
	return []salvageContractCase{
		{
			name:      "usage error for zero shas",
			buildArgs: func(salvageFixture, string) []string { return salvageArgs("") },
			wantExit:  2,
			wantUsage: true,
		},
		{
			name:      "usage error for short command line sha",
			buildArgs: func(f salvageFixture, _ string) []string { return salvageArgs("", f.shaB[:12]) },
			wantExit:  2,
			wantUsage: true,
		},
		{
			name:      "usage error for non hex command line value",
			buildArgs: func(salvageFixture, string) []string { return salvageArgs("", salvageNonHexArg) },
			wantExit:  2,
			wantUsage: true,
		},
	}
}

// salvageArgs builds the guard's command line. --base main lets the fixture skip
// an origin/main remote; --followups is included only when a file path is given.
func salvageArgs(followupsPath string, shas ...string) []string {
	args := []string{"--base", "main"}
	if followupsPath != "" {
		args = append(args, "--followups", followupsPath)
	}
	return append(args, shas...)
}

func buildSalvageFixture(t *testing.T) salvageFixture {
	t.Helper()

	repoDir := t.TempDir()
	runGitCommand(t, repoDir, "init", "-b", "main")
	commitSalvageChange(t, repoDir, "commit A on main")
	shaA := runGitCommand(t, repoDir, "rev-parse", "HEAD")

	runGitCommand(t, repoDir, "checkout", "-b", "salvage-side")
	commitSalvageChange(t, repoDir, "commit B on side branch")
	shaB := runGitCommand(t, repoDir, "rev-parse", "HEAD")

	runGitCommand(t, repoDir, "checkout", "main")
	return salvageFixture{repoDir: repoDir, shaA: shaA, shaB: shaB}
}

// commitSalvageChange records an empty commit with an explicit identity because
// .debbie.toml syncs internal/ to public CI runners without a configured git user.
func commitSalvageChange(t *testing.T, repoDir, message string) {
	t.Helper()
	runGitCommand(t, repoDir,
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"commit", "--allow-empty", "-m", message)
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func runReleaseSalvageScript(t *testing.T, repoDir string, args []string) salvageResult {
	t.Helper()

	scriptPath := filepath.Join(findRepoRoot(t), filepath.FromSlash(releaseSalvageScript))
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = repoDir

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("script execution failed: %v output=%s", err, output)
		}
		exitCode = exitError.ExitCode()
	}
	return salvageResult{output: string(output), exitCode: exitCode}
}

// divergentHexDigit returns a hex digit guaranteed to differ from c so callers
// can build tokens that share a prefix with a SHA but are not prefixes of it.
func divergentHexDigit(c byte) string {
	if c == '0' {
		return "1"
	}
	return "0"
}
