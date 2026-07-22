package codehealth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	pushSmokeScript = "_dev/manual_smoke_tests/20_push_smoke.test.sh"
	pushSmokeTarget = "test-push-smoke"
	pushSmokeJob    = "push-smoke"
)

func TestPushSmokeMakeTargetIsDedicated(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))

	if !makefileDeclaresPhony(makefile, pushSmokeTarget) {
		t.Fatalf(".PHONY must declare %s", pushSmokeTarget)
	}
	prerequisites := makeTargetPrerequisites(makefile, pushSmokeTarget)
	if len(prerequisites) != 1 || prerequisites[0] != "build" {
		t.Fatalf("%s must depend only on build; got %v", pushSmokeTarget, prerequisites)
	}
	recipe := strings.TrimSpace(extractMakeTargetRecipe(makefile, pushSmokeTarget))
	wantRecipe := "AYB_BIN=$(CURDIR)/ayb bash " + pushSmokeScript
	if recipe != wantRecipe {
		t.Fatalf("%s recipe = %q, want %q", pushSmokeTarget, recipe, wantRecipe)
	}

	for _, aggregate := range []string{
		"test-all",
		"test-full",
		"test-everything",
		"test-demo-e2e",
		"test-demo-launch",
		"test-demo-cross-smoke",
	} {
		if containsStringField(makeTargetPrerequisites(makefile, aggregate), pushSmokeTarget) {
			t.Errorf("%s must not depend on %s", aggregate, pushSmokeTarget)
		}
		if strings.Contains(extractMakeTargetRecipe(makefile, aggregate), pushSmokeTarget) {
			t.Errorf("%s must not invoke %s", aggregate, pushSmokeTarget)
		}
	}
}

func TestPushSmokeWorkflowJobIsBlockingAndMinimal(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs[pushSmokeJob]
	if !ok {
		t.Fatalf("workflow is missing jobs.%s", pushSmokeJob)
	}
	if strings.TrimSpace(job.If) != "" {
		t.Errorf("%s must not declare a job-level if: %q", pushSmokeJob, job.If)
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		t.Errorf("%s must not set continue-on-error", pushSmokeJob)
	}
	for _, step := range job.Steps {
		if strings.TrimSpace(step.If) != "" {
			t.Errorf("%s step %q must not declare an if: %q", pushSmokeJob, step.Name, step.If)
		}
	}
	if !workflowJobHasRunStep(workflow, pushSmokeJob, "make "+pushSmokeTarget) {
		t.Fatalf("%s must run make %s unconditionally from the repository root", pushSmokeJob, pushSmokeTarget)
	}
	requireWorkflowJobPreparesJavaScriptPrerequisites(t, workflow, makefile, pushSmokeJob, pushSmokeTarget)
	requireJobUsesAction(t, job, "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd")
	requireJobUsesAction(t, job, "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16")
	if !pushSmokeJobInstallsPrerequisites(job) {
		t.Fatalf("%s must install jq and lsof explicitly before running %s", pushSmokeJob, pushSmokeTarget)
	}
}

func TestPushSmokeScriptIsProjectedWhenCIInvokesIt(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	if !workflowJobHasRunStep(workflow, pushSmokeJob, "make "+pushSmokeTarget) {
		t.Fatalf("%s must run make %s unconditionally from the repository root", pushSmokeJob, pushSmokeTarget)
	}
	if !recipeContainsCommandWithArgs(extractMakeTargetRecipe(makefile, pushSmokeTarget), []string{pushSmokeScript}) {
		t.Fatalf("%s must execute exact script %s", pushSmokeTarget, pushSmokeScript)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, pushSmokeScript)); err != nil {
		t.Fatalf("source script %s must exist: %v", pushSmokeScript, err)
	}

	syncScope, err := loadDebbieSyncScope(repoRoot)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip(".debbie.toml absent; public mirror topology may omit dev-source projection checks")
	}
	if err != nil {
		t.Fatalf("load .debbie.toml sync scope failed: %v", err)
	}
	if _, ok := syncScope.files[pushSmokeScript]; !ok {
		t.Fatalf(".debbie.toml [sync].files must contain exact %q because CI %s runs make %s and that target executes it",
			pushSmokeScript, pushSmokeJob, pushSmokeTarget)
	}
}

func TestPushSmokeScriptProjectionRejectsCommentedDebbieEntry(t *testing.T) {
	t.Parallel()

	scope := parseDebbieSyncScope(strings.Join([]string{
		"[sync]",
		"files = [",
		`  "README.md",`,
		`  # "_dev/manual_smoke_tests/20_push_smoke.test.sh",`,
		"]",
		"",
	}, "\n"))
	if _, ok := scope.files[pushSmokeScript]; ok {
		t.Fatalf("commented .debbie.toml [sync].files entry %q must not count as public projection", pushSmokeScript)
	}
	if _, ok := scope.files["README.md"]; !ok {
		t.Fatal("active .debbie.toml [sync].files entries must still be parsed")
	}
}

func pushSmokeJobInstallsPrerequisites(job githubActionsJob) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "apt-get install") &&
			strings.Contains(step.Run, "jq") &&
			strings.Contains(step.Run, "lsof") {
			return true
		}
	}
	return false
}
