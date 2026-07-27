package codehealth

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSDKSwiftWorkflowRunsExecutableTestRunner(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs["sdk-swift"]
	if !ok {
		t.Fatal("workflow is missing jobs.sdk-swift")
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		t.Error("sdk-swift must not set continue-on-error")
	}
	if strings.TrimSpace(job.If) != "" {
		t.Errorf("sdk-swift must not declare a job-level if: %q", job.If)
	}
	if job.Defaults.Run.WorkingDirectory != "sdk_swift" {
		t.Errorf("sdk-swift working-directory = %q, want sdk_swift", job.Defaults.Run.WorkingDirectory)
	}

	var runsBuild, runsTestRunner, runsSwiftTest bool
	for _, step := range job.Steps {
		if githubActionsContinueOnErrorEnabled(step.ContinueOnError) {
			t.Errorf("sdk-swift step %q must not set continue-on-error", step.Name)
		}
		if strings.TrimSpace(step.If) != "" {
			t.Errorf("sdk-swift step %q must not declare an if: %q", step.Name, step.If)
		}
		if step.WorkingDirectory != "" && step.WorkingDirectory != "sdk_swift" {
			t.Errorf("sdk-swift step %q working-directory = %q, want sdk_swift", step.Name, step.WorkingDirectory)
		}
		for _, command := range executableShellCommandBlocks(step.Run) {
			runsBuild = runsBuild || shellBlockRunsCommandWithArgs(command, []string{"swift", "build"})
			runsTestRunner = runsTestRunner ||
				shellBlockRunsCommandWithArgs(command, []string{"swift", "run", "AllyourbaseTestRunner"})
			runsSwiftTest = runsSwiftTest || shellBlockRunsCommandWithArgs(command, []string{"swift", "test"})
		}
	}

	if !runsBuild {
		t.Error("sdk-swift must run swift build")
	}
	if !runsTestRunner {
		t.Error("sdk-swift must run swift run AllyourbaseTestRunner")
	}
	if runsSwiftTest {
		t.Error("sdk-swift must not run swift test")
	}
}
