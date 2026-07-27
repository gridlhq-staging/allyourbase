package codehealth

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	sdkGoE2EJob    = "test-sdk-integration"
	sdkGoE2ETarget = "test-sdk-integration"
)

func TestSDKGoE2EWorkflowRunsCanonicalMakeTarget(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs[sdkGoE2EJob]
	if !ok {
		t.Fatalf("workflow is missing jobs.%s", sdkGoE2EJob)
	}
	if strings.TrimSpace(job.If) != "" {
		t.Errorf("%s must not declare a job-level if: %q", sdkGoE2EJob, job.If)
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		t.Errorf("%s must not set continue-on-error", sdkGoE2EJob)
	}
	if !workflowJobHasRunStep(workflow, sdkGoE2EJob, "make "+sdkGoE2ETarget) {
		t.Fatalf("%s must run make %s unconditionally from the repository root; current SDK integration run step is %q",
			sdkGoE2EJob, sdkGoE2ETarget, sdkGoE2ERunStepCommand(job))
	}
}

func TestSDKGoE2EMakeTargetRunsLiveGoSuite(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))
	prerequisites := makeTargetPrerequisites(makefile, sdkGoE2ETarget)
	if len(prerequisites) != 1 || prerequisites[0] != "build" {
		t.Fatalf("%s must depend only on build; got %v", sdkGoE2ETarget, prerequisites)
	}

	recipe := extractMakeTargetRecipe(makefile, sdkGoE2ETarget)
	for _, required := range []string{
		"bash scripts/sdk_live_proof_seed.sh",
		"cd ../sdk_go",
		"go test -count=1 -run TestE2E ./... -v",
		"AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true",
		`AYB_TEST_URL="$${AYB_BASE_URL}"`,
		`AYB_TEST_COLLECTION="$${AYB_SDK_LIVE_PROOF_COLLECTION:-sdk_kotlin_search_posts}"`,
		`AYB_TEST_ADMIN_TOKEN="$${AYB_ADMIN_TOKEN}"`,
	} {
		if !strings.Contains(recipe, required) {
			t.Fatalf("%s recipe must contain %q", sdkGoE2ETarget, required)
		}
	}
}

func sdkGoE2ERunStepCommand(job githubActionsJob) string {
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Name) == "Run SDK integration tests" {
			return strings.TrimSpace(step.Run)
		}
	}
	return ""
}
