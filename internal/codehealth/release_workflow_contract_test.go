package codehealth

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflowPinsEveryActionToACommitSHA(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "release.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	for jobName, job := range workflow.Jobs {
		for index, step := range job.Steps {
			if strings.TrimSpace(step.Uses) == "" {
				continue
			}
			if !githubActionsRefIsCommitPinned(step.Uses) {
				t.Fatalf("jobs.%s.steps[%d].uses = %q, want immutable owner/name@40-hex commit SHA", jobName, index, step.Uses)
			}
		}
	}
}

func TestReleaseWorkflowSanitizesWorkflowDispatchVersionInput(t *testing.T) {
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

	step, ok := releaseWorkflowNamedStep(job, "Validate workflow_dispatch version")
	if !ok {
		t.Fatal("release workflow is missing the version validation step")
	}
	if step.Env["INPUT_VERSION"] != "${{ github.event.inputs.version }}" {
		t.Fatalf("validation INPUT_VERSION = %q, want workflow_dispatch version input", step.Env["INPUT_VERSION"])
	}
	requireContainsAll(t, step.Run, []string{
		"set -euo pipefail",
		`''|*[!0-9A-Za-z._-]*)`,
		`echo "RELEASE_VERSION=v$INPUT_VERSION" >> "$GITHUB_ENV"`,
	})

	for _, downstreamStepName := range []string{
		"Create and push tag (workflow_dispatch only)",
		"Set GORELEASER_CURRENT_TAG",
		"Dispatch Docker workflow at release tag",
	} {
		downstreamStep, ok := releaseWorkflowNamedStep(job, downstreamStepName)
		if !ok {
			t.Fatalf("release workflow is missing step %q", downstreamStepName)
		}
		if strings.Contains(downstreamStep.Run, "${{ github.event.inputs.version }}") {
			t.Fatalf("%s run script interpolates github.event.inputs.version directly", downstreamStepName)
		}
	}
}

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
		`git tag "$RELEASE_VERSION"`,
		`git push origin "$RELEASE_VERSION"`,
	})
	requireDoesNotContainAny(t, run, []string{
		`git tag "$RELEASE_VERSION" || true`,
		`git push origin "$RELEASE_VERSION" || true`,
		"${{ github.event.inputs.version }}",
		"continue-on-error",
	})
}

func TestReleaseWorkflowDispatchesDockerAtTag(t *testing.T) {
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
	if !githubActionsPermissionGrantsWrite(job.Permissions, "actions") {
		t.Fatal("release job needs actions: write to dispatch docker.yml")
	}

	step, ok := releaseWorkflowNamedStep(job, "Dispatch Docker workflow at release tag")
	if !ok {
		t.Fatal("release workflow is missing the Docker dispatch step")
	}
	requireContainsAll(t, step.Run, []string{
		"set -euo pipefail",
		`gh workflow run docker.yml --repo "${GITHUB_REPOSITORY}" --ref "$RELEASE_VERSION"`,
	})
	if strings.Contains(step.Run, "${{ github.event.inputs.version }}") {
		t.Fatal("Docker dispatch step interpolates github.event.inputs.version directly")
	}
	if step.Env["GH_TOKEN"] != "${{ secrets.GITHUB_TOKEN }}" {
		t.Fatalf("Docker dispatch GH_TOKEN = %q, want the job-scoped GITHUB_TOKEN", step.Env["GH_TOKEN"])
	}
}

func releaseWorkflowNamedStep(job githubActionsJob, name string) (githubActionsStep, bool) {
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Name) == name {
			return step, true
		}
	}
	return githubActionsStep{}, false
}
