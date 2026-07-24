package codehealth

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowTagPushFailuresAreFatal(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "release.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs["release"]
	if !ok {
		t.Fatal("release workflow is missing jobs.release")
	}

	step, ok := releaseWorkflowNamedStep(job, "Create and push tag (workflow_dispatch only)")
	if !ok {
		t.Fatal("release workflow is missing the tag creation step")
	}

	run := step.Run
	requireContainsAll(t, run, []string{
		"set -euo pipefail",
		`git tag "$VERSION"`,
		`git push origin "$VERSION"`,
	})
	requireDoesNotContainAny(t, run, []string{
		`git tag "$VERSION" || true`,
		`git push origin "$VERSION" || true`,
		"continue-on-error",
	})
}

func releaseWorkflowNamedStep(job githubActionsJob, name string) (githubActionsStep, bool) {
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Name) == name {
			return step, true
		}
	}
	return githubActionsStep{}, false
}
