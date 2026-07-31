package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const closeoutRepairAncestryScript = "scripts/check-closeout-repair-ancestry.sh"
const foundingCloseoutSpecimen = "Ship disposition: not-shipping — jul29_12am final revalidation rendered GO for evidence tree `fb6f291e7bade309afc90d98beed5490180cd814`; this maintenance lane cuts no release, and release prep remains a separate human-triggered act — unshipped inventory owner: CHANGELOG.md [Unreleased] (promoted at next release-prep).\nUnreleased inventory owner: CHANGELOG.md [Unreleased] (promoted at next release-prep)\n\n# jul28_1pm Launch Gate Closeout\n\n## Verdict\n\nGO on pinned evidence tree `fb6f291e7bade309afc90d98beed5490180cd814`.\n\nThe accepted Stage 1 receipt is the scored union evidence owner. On a clean worktree pinned to that SHA,\n`make test-everything` exited `0`, and all nine owners in its final `TEST SUMMARY` were green:\n\n- `Go unit tests`\n- `Integration tests`\n- `SDK tests`\n- `All SDK tests`\n- `UI component tests`\n- `Playwright e2e`\n- `Demo app E2E`\n- `Cross-demo smoke (Playwright)`\n- `API smoke tests`\n\nThe complete union output and exit status are preserved at\n`logs/stage_01_make_test_everything_fb6f291e7bade309afc90d98beed5490180cd814.log` and\n`logs/stage_01_make_test_everything_fb6f291e7bade309afc90d98beed5490180cd814.status`.\n\n## Lane Outcomes\n\n- L1 SDK-python env isolation: merged; `All SDK tests` is green in the accepted union receipt.\n- L2 API-smoke base URL resolution: merged; `API smoke tests` is green in the accepted union receipt.\n- L3 dashboard Playwright contract fixes: merged; `Playwright e2e` is green in the accepted union receipt.\n- L4 movies cross-demo contract fix: merged; `Cross-demo smoke (Playwright)` is green in the accepted union receipt.\n- L5 Supabase-S3 storage pull migration: merged as additive adoption depth. The evidence tree contains\n  `storage-s3-endpoint` in `internal/cli/migrate_supabase.go` and `TestE2E_StorageMigrationFromS3` in\n  `internal/sbmigrate/storage_s3_integration_test.go`. Live confirmation against\n  `https://<project>.storage.supabase.co/storage/v1/s3` remains credential-gated and is not claimed here.\n- L6 go/no-go dossier and closeout: recorded an honest NO-GO for its earlier pinned specimen and cut no release.\n- L7 final revalidation: established the fresh green union and companion-gate receipts for\n  `fb6f291e7bade309afc90d98beed5490180cd814`, reconciled this closeout, and cuts no release.\n\n## Rubric Evidence\n\n- Union result: PASS. The Makefile-owned verdict target `make test-everything` exited `0`; all nine named owners\n  were green on the pinned evidence tree.\n- Formerly red owners: `Go unit tests`, `Integration tests`, and `Demo app E2E` are green in the accepted union\n  receipt.\n- Previously green owners: `SDK tests`, `All SDK tests`, `UI component tests`, `Playwright e2e`,\n  `Cross-demo smoke (Playwright)`, and `API smoke tests` remained green.\n- L5 residual: credential-free source and integration-test evidence are present; credential-gated live\n  Supabase-S3 confirmation remains unclaimed.\n\n## Companion Gates\n\nThe accepted Stage 2 receipts for the same pinned evidence tree record exit `0` for:\n\n- `make test-sdk-integration`\n- `go test ./internal/docs -count=1`\n- `go test ./internal/codehealth -count=1`\n- `gofmt -l .` with empty output\n- `bash scripts/check-file-sizes.sh`\n- `go mod tidy`\n- `git diff --exit-code -- go.mod go.sum`\n\nTheir complete outputs and exit statuses are preserved under\n`logs/stage_02_*_fb6f291e7bade309afc90d98beed5490180cd814.{log,status}`.\n\n## Residuals\n\n- Live Supabase-S3 confirmation remains credential-gated and was not part of the accepted receipts.\n- The GO verdict authorizes no release, deploy, tag, SAML/CSP promotion, or credential proof. Preparing a future\n  release from `CHANGELOG.md` `[Unreleased]` remains a separate human-triggered act.\n\n## Validation References\n\n- Stage 1 union evidence:\n  `logs/stage_01_make_test_everything_fb6f291e7bade309afc90d98beed5490180cd814.{log,status}`.\n- Stage 2 companion evidence:\n  `logs/stage_02_*_fb6f291e7bade309afc90d98beed5490180cd814.{log,status}`.\n"

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

func TestCheckTargetRunsCloseoutRepairAncestryGuard(t *testing.T) {
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("make", "-n", "check")
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run make check: %v output=%s", err, output)
	}

	const enforcingCommand = "bash scripts/check-closeout-repair-ancestry.sh --require-corpus"
	if count := strings.Count(string(output), enforcingCommand); count != 1 {
		t.Fatalf("enforcing closeout ancestry command count = %d, want 1, output:\n%s", count, output)
	}
}

func TestCloseoutRepairAncestryFailsOnFoundingSpecimen(t *testing.T) {
	gitFixture := newCloseoutRepairGitFixture(t)
	closeoutDir := t.TempDir()
	artifactPath := filepath.Join(closeoutDir, "jul28_1pm_launch_gate_green_closeout.md")
	writeTextFile(t, artifactPath, foundingCloseoutSpecimen)

	result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
	if result.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=1 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
	)
	assertCloseoutRepairOutput(t, result.output,
		"UNCREDITED: certifying artifact",
		"jul28_1pm_launch_gate_green_closeout.md has no qualifying ## Repairs declaration",
	)
	if strings.Contains(result.output, "not a unique resolvable commit") {
		t.Fatalf("declaration failure resolved the certified SHA before rejecting it:\n%s", result.output)
	}

	repoRoot := findRepoRoot(t)
	if err := exec.Command("git", "-C", repoRoot, "cat-file", "-e", "22b9bc90e^{commit}").Run(); err != nil {
		if _, absent := err.(*exec.ExitError); absent {
			return
		}
		t.Fatalf("probe founding commit: %v", err)
	}
	source := runCloseoutRepairFixtureGit(t, repoRoot, "", "show", "22b9bc90e:chats/icg/jul28_1pm_launch_gate_green_closeout.md")
	if source+"\n" != foundingCloseoutSpecimen {
		t.Fatal("embedded founding specimen differs from 22b9bc90e")
	}
}

func TestCloseoutRepairAncestryRequiresRepairDeclaration(t *testing.T) {
	gitFixture := newCloseoutRepairGitFixture(t)
	testCases := []struct {
		name, repairs, wantSummary string
		wantExit                   int
	}{
		{"absent section", "", "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=1 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", 1},
		{"empty section", "## Repairs\n", "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=1 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", 1},
		{"ancestor credit", "## Repairs\n- Guard repaired at " + gitFixture.ancestorSHA + "\n", "SUMMARY result=PASS artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=1 violations=0 indeterminate=0 stale_carveouts=0", 0},
		{"explicit zero", "## Repairs\n* Zero repairs: this certifying closeout credits no repair commits.\n", "SUMMARY result=PASS artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			closeoutDir := t.TempDir()
			artifact := "## Verdict\nGO on pinned evidence tree " + gitFixture.certifiedSHA + "\n" + testCase.repairs
			writeTextFile(t, filepath.Join(closeoutDir, "declaration.md"), artifact)
			result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
			if result.exitCode != testCase.wantExit {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, testCase.wantExit, result.output)
			}
			assertCloseoutRepairSummary(t, result.output, testCase.wantSummary)
		})
	}
}

func TestCloseoutRepairAncestryClassifiesFirstVerdictContent(t *testing.T) {
	gitFixture := newCloseoutRepairGitFixture(t)
	testCases := []struct {
		name, heading, verdict, repairs, wantSummary, wantDiagnostic string
		wantExit                                                     int
	}{
		{"certifying", "## Verdict", "GO on pinned evidence tree " + gitFixture.certifiedSHA, "## Repairs\n* Zero repairs: this certifying closeout credits no repair commits.\n", "SUMMARY result=PASS artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
		{"no go", "## Verdict", "NO-GO on the all-green automation launch gate.", "", "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
		{"withdrawn", "## Verdict", "**GO WITHDRAWN. No verdict currently stands. Re-certification is pending.**", "", "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
		{"demo unbound", "## Verdict", "The batch closes. The falsifiable claim is now: every public demo passes the default aggregate `make demo-check`, covering screen specs, demo unit suites, desktop and mobile Chromium flows, critical/serious accessibility checks, InstantSearch, push smoke, freshness, and the live production workflow arm.", "", "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=1 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
		{"ready unbound", "## Verdict", "**READY** to cut `v0.0.9-beta`.", "", "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=1 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
		{"unknown exact verdict", "## Verdict", "MAYBE pending further evidence.", "", "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0", "UNKNOWN_VERDICT", 1},
		{"plural heading remains outside grammar", "## Verdicts", "**READY** to cut `v0.0.9-beta`.", "", "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=1 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0", "", 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			closeoutDir := t.TempDir()
			writeTextFile(t, filepath.Join(closeoutDir, "verdict.md"), testCase.heading+"\n\n"+testCase.verdict+"\n"+testCase.repairs)
			result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, closeoutDir)
			if result.exitCode != testCase.wantExit {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, testCase.wantExit, result.output)
			}
			assertCloseoutRepairSummary(t, result.output, testCase.wantSummary)
			if testCase.wantDiagnostic != "" {
				assertCloseoutRepairOutput(t, result.output, testCase.wantDiagnostic)
			}
		})
	}
}

func TestCloseoutRepairAncestryRequireCorpus(t *testing.T) {
	gitFixture := newCloseoutRepairGitFixture(t)
	missingDir := filepath.Join(t.TempDir(), "missing")
	emptyCorpus := t.TempDir()
	writeTextFile(t, filepath.Join(emptyCorpus, "no-go.md"), "## Verdict\nNO-GO on measured evidence.\n")
	testCases := []struct {
		name string
		args []string
		exit int
		want string
	}{
		{"missing tolerant", []string{missingDir}, 0, "SUMMARY result=VACUOUS artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0"},
		{"missing enforcing", []string{"--require-corpus", missingDir}, 1, "SUMMARY result=FAIL artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0"},
		{"zero certifying tolerant", []string{emptyCorpus}, 0, "SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0"},
		{"zero certifying enforcing", []string{"--require-corpus", emptyCorpus}, 1, "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, testCase.args...)
			if result.exitCode != testCase.exit {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, testCase.exit, result.output)
			}
			assertCloseoutRepairSummary(t, result.output, testCase.want)
		})
	}
}

func TestCloseoutRepairAncestryRequiresLiveHistoricalCarveouts(t *testing.T) {
	gitFixture := newCloseoutRepairGitFixture(t)
	defaultCorpus := filepath.Join(gitFixture.repoRoot, "chats", "icg")
	writeTextFile(t, filepath.Join(defaultCorpus, "current_closeout.md"), strings.Join([]string{
		"## Verdict",
		"GO on pinned evidence tree " + gitFixture.certifiedSHA,
		"## Repairs",
		"* Zero repairs: this certifying closeout credits no repair commits.",
	}, "\n"))
	writeTextFile(t, filepath.Join(defaultCorpus, "renamed_demo_closeout.md"), strings.Join([]string{
		"## Verdict",
		"The batch closes. The falsifiable claim is now: every public demo passes the default aggregate `make demo-check`, covering screen specs, demo unit suites, desktop and mobile Chromium flows, critical/serious accessibility checks, InstantSearch, push smoke, freshness, and the live production workflow arm.",
	}, "\n"))
	writeTextFile(t, filepath.Join(defaultCorpus, "renamed_ready_closeout.md"), strings.Join([]string{
		"## Verdict",
		"**READY** to cut `v0.0.9-beta`.",
	}, "\n"))

	defaultResult := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, "--require-corpus", defaultCorpus)
	if defaultResult.exitCode != 0 {
		t.Fatalf("explicit non-default corpus exit code = %d, want 0, output:\n%s", defaultResult.exitCode, defaultResult.output)
	}
	assertCloseoutRepairSummary(t, defaultResult.output,
		"SUMMARY result=PASS artifacts_scanned=3 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=2 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
	)

	requiredResult := runCloseoutRepairAncestryScript(t, gitFixture.repoRoot, "--require-corpus")
	if requiredResult.exitCode != 1 {
		t.Fatalf("default corpus exit code = %d, want 1, output:\n%s", requiredResult.exitCode, requiredResult.output)
	}
	assertCloseoutRepairSummary(t, requiredResult.output,
		"SUMMARY result=FAIL artifacts_scanned=3 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=2 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=2",
	)
	assertCloseoutRepairOutput(t, requiredResult.output,
		"STALE_CARVEOUT:",
		"jul21_6pm_demo_perfect_closeout.md",
		"jun01_pm_0_closeout.md",
	)
	if strings.Contains(requiredResult.output, "REQUIRED_CORPUS:") {
		t.Fatalf("stale carveout fixture tripped an unrelated corpus requirement:\n%s", requiredResult.output)
	}
}

func TestCloseoutRepairAncestrySurfacesCloseoutsWithoutVerdictSection(t *testing.T) {
	closeoutDir := t.TempDir()
	writeTextFile(t, filepath.Join(closeoutDir, "missing_verdict_closeout.md"), strings.Join([]string{
		"# Missing Verdict Closeout",
		"",
		"## Repairs",
		"* Zero repairs: this certifying closeout credits no repair commits.",
	}, "\n"))
	writeTextFile(t, filepath.Join(closeoutDir, "plural_verdicts_closeout.md"), strings.Join([]string{
		"# Plural Verdict Closeout",
		"",
		"## Verdicts",
		"**READY** to cut `v0.0.9-beta`.",
	}, "\n"))
	writeTextFile(t, filepath.Join(closeoutDir, "ordinary_note.md"), "# Ordinary note\n")

	result := runCloseoutRepairAncestryScript(t, findRepoRoot(t), closeoutDir)
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0, output:\n%s", result.exitCode, result.output)
	}
	assertCloseoutRepairSummary(t, result.output,
		"SUMMARY result=VACUOUS artifacts_scanned=3 artifacts_with_certified_sha=0 skipped_artifacts=1 no_verdict_section=2 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
	)
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
		"SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=3 violations=3 indeterminate=0 stale_carveouts=0",
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
		"SUMMARY result=PASS artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=1 violations=0 indeterminate=0 stale_carveouts=0",
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
		"SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
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
		"SUMMARY result=VACUOUS artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=1 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
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
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0",
			wantError:   "unmatched Markdown emphasis wrapper",
		},
		{
			name:        "unresolvable certified SHA",
			verdict:     "GO on pinned evidence tree 1111111",
			repair:      repairSHA,
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0",
			wantError:   "certified SHA 1111111",
		},
		{
			name:        "repair reference continues after SHA",
			verdict:     "GO on pinned evidence tree " + certifiedSHA,
			repair:      repairSHA + " and later prose",
			wantSummary: "SUMMARY result=FAIL artifacts_scanned=1 artifacts_with_certified_sha=1 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=1 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0",
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
		"SUMMARY result=VACUOUS artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=0 stale_carveouts=0",
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
		"SUMMARY result=FAIL artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0",
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
				"SUMMARY result=FAIL artifacts_scanned=0 artifacts_with_certified_sha=0 skipped_artifacts=0 no_verdict_section=0 non_certifying=0 unbound_verdict=0 uncredited=0 repair_shas_checked=0 violations=0 indeterminate=1 stale_carveouts=0",
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
