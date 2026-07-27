package codehealth

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	pushContractJob      = "push-contract"
	pushContractWorkflow = ".github/workflows/push_contract.yml"
	pushContractCommand  = "go test -tags=pushcontract -run Contract -count=1 ./internal/push/..."
	pushContractRun      = "go test -tags=pushcontract -run 'Contract' -count=1 ./internal/push/..."
)

func TestPushContractWorkflowIsBlocking(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, pushContractWorkflow)
	workflowText := readRepoText(t, workflowPath)
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(workflowText), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs[pushContractJob]
	if !ok {
		t.Fatalf("workflow is missing jobs.%s", pushContractJob)
	}
	if strings.TrimSpace(job.If) != "" {
		t.Errorf("%s must not declare a job-level if: %q", pushContractJob, job.If)
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		t.Errorf("%s must not set continue-on-error", pushContractJob)
	}
	for _, step := range job.Steps {
		if strings.TrimSpace(step.If) != "" {
			t.Errorf("%s step %q must not declare an if: %q", pushContractJob, step.Name, step.If)
		}
		if githubActionsContinueOnErrorEnabled(step.ContinueOnError) {
			t.Errorf("%s step %q must not set continue-on-error", pushContractJob, step.Name)
		}
	}
	if !workflowJobHasRunStep(workflow, pushContractJob, pushContractCommand) {
		t.Fatalf("%s must run the push contract suite unconditionally from the repository root", pushContractJob)
	}
	if !strings.Contains(workflowText, pushContractRun) {
		t.Fatalf("%s must use the exact run command %q", pushContractJob, pushContractRun)
	}
	if !strings.Contains(workflowText, "go-version-file: go.mod") || strings.Contains(workflowText, "go-version:") {
		t.Fatal("push contract workflow must source its Go version from go.mod")
	}
	requireJobUsesAction(t, job, githubActionsCheckoutAction)
	requireJobUsesAction(t, job, githubActionsSetupGoAction)
}

func TestPushContractWorkflowRequiresFCMCredentials(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, pushContractWorkflow)
	workflowText := readRepoText(t, workflowPath)
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(workflowText), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	job, ok := workflow.Jobs[pushContractJob]
	if !ok {
		t.Fatalf("workflow is missing jobs.%s", pushContractJob)
	}

	const (
		secretName      = "AYB_PUSH_FCM_SERVICE_ACCOUNT_JSON"
		credentialsEnv  = "AYB_PUSH_CONTRACT_FCM_CREDENTIALS"
		credentialsPath = "$RUNNER_TEMP/fcm-contract-creds.json"
	)
	var materializesCredentials bool
	for _, step := range job.Steps {
		if step.Env[secretName] != "${{ secrets."+secretName+" }}" {
			continue
		}
		materializesCredentials =
			strings.Contains(step.Run, `if [ -z "${AYB_PUSH_FCM_SERVICE_ACCOUNT_JSON:-}" ]`) &&
				strings.Contains(step.Run, secretName+" secret is required") &&
				strings.Contains(step.Run, `printf '%s' "$AYB_PUSH_FCM_SERVICE_ACCOUNT_JSON" > "`+credentialsPath+`"`) &&
				strings.Contains(step.Run, `printf 'AYB_PUSH_CONTRACT_FCM_CREDENTIALS=%s\n' "`+credentialsPath+`" >> "$GITHUB_ENV"`)
	}
	if !materializesCredentials {
		t.Fatalf("%s must fail when %s is empty, materialize it under RUNNER_TEMP, and export %s",
			pushContractJob, secretName, credentialsEnv)
	}
	if strings.Contains(workflowText, "AYB_PUSH_CONTRACT_APNS_") {
		t.Fatal("APNS contract env must remain unset until a complete, separate APNS credential set is provisioned")
	}
}
