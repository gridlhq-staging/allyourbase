package codehealth

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type launchProbeScheduleTrigger struct {
	Cron string `yaml:"cron"`
}

type launchProbeWorkflowRunTrigger struct {
	Workflows []string `yaml:"workflows"`
	Types     []string `yaml:"types"`
}

type launchProbeWorkflowMetadata struct {
	Name string `yaml:"name"`
	Jobs map[string]struct {
		RunsOn string `yaml:"runs-on"`
	} `yaml:"jobs"`
}

func TestLaunchProbeWorkflow(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "launch_probe.yml")
	workflowText := readRepoText(t, workflowPath)

	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(workflowText), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}
	var metadata launchProbeWorkflowMetadata
	if err := yaml.Unmarshal([]byte(workflowText), &metadata); err != nil {
		t.Fatalf("decode %s metadata failed: %v", workflowPath, err)
	}

	if metadata.Name != "Launch Probe" {
		t.Fatalf("workflow name = %q, want Launch Probe", metadata.Name)
	}
	requireLaunchProbeTriggers(t, workflow)
	requireLaunchProbePermissions(t, workflow.Permissions)
	requireLaunchProbeJob(t, workflow, metadata)
}

func requireLaunchProbeTriggers(t *testing.T, workflow githubActionsWorkflow) {
	t.Helper()

	if len(workflow.On) != 3 {
		t.Fatalf("launch probe trigger count = %d, want exactly workflow_dispatch, schedule, and workflow_run", len(workflow.On))
	}
	dispatch, ok := workflow.On["workflow_dispatch"]
	if !ok || dispatch.Kind != yaml.MappingNode || len(dispatch.Content) != 0 {
		t.Fatal("launch probe must declare workflow_dispatch: {}")
	}

	var schedules []launchProbeScheduleTrigger
	if node, ok := workflow.On["schedule"]; !ok {
		t.Fatal("launch probe must declare a schedule trigger")
	} else if err := node.Decode(&schedules); err != nil {
		t.Fatalf("decode launch probe schedule: %v", err)
	}
	if len(schedules) != 1 || schedules[0].Cron != "17 13 * * *" {
		t.Fatalf("launch probe schedules = %+v, want one daily 17 13 * * * cron distinct from cross-demo-live", schedules)
	}

	var workflowRun launchProbeWorkflowRunTrigger
	if node, ok := workflow.On["workflow_run"]; !ok {
		t.Fatal("launch probe must declare a workflow_run trigger")
	} else if err := node.Decode(&workflowRun); err != nil {
		t.Fatalf("decode launch probe workflow_run: %v", err)
	}
	if !slices.Equal(workflowRun.Workflows, []string{"Release"}) ||
		!slices.Equal(workflowRun.Types, []string{"completed"}) {
		t.Fatalf("launch probe workflow_run = %+v, want Release completed", workflowRun)
	}
}

func requireLaunchProbePermissions(t *testing.T, permissions map[string]string) {
	t.Helper()

	want := map[string]string{"contents": "read", "actions": "read"}
	if len(permissions) != len(want) {
		t.Fatalf("launch probe permissions = %v, want exactly %v", permissions, want)
	}
	for name, level := range want {
		if permissions[name] != level {
			t.Fatalf("launch probe permission %s = %q, want %q", name, permissions[name], level)
		}
	}
}

func requireLaunchProbeJob(t *testing.T, workflow githubActionsWorkflow, metadata launchProbeWorkflowMetadata) {
	t.Helper()

	if len(workflow.Jobs) != 1 {
		t.Fatalf("launch probe jobs = %d, want exactly one", len(workflow.Jobs))
	}
	job, ok := workflow.Jobs["launch-probe"]
	if !ok {
		t.Fatal("launch probe workflow is missing jobs.launch-probe")
	}
	if metadata.Jobs["launch-probe"].RunsOn != "ubuntu-latest" {
		t.Fatalf("launch probe runner = %q, want ubuntu-latest", metadata.Jobs["launch-probe"].RunsOn)
	}
	if got, want := strings.TrimSpace(job.If), "github.event_name != 'workflow_run' || github.event.workflow_run.conclusion == 'success'"; got != want {
		t.Fatalf("launch probe job if = %q, want %q", got, want)
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		t.Fatal("launch probe job must not ignore failures")
	}
	requireLaunchProbeCheckout(t, job)
	requireLaunchProbeCommand(t, workflow.Defaults, job)
}

func requireLaunchProbeCheckout(t *testing.T, job githubActionsJob) {
	t.Helper()

	if !slices.Equal(githubActionsJobActionRefs(job), []string{githubActionsCheckoutAction}) {
		t.Fatalf("launch probe action refs = %v, want only %s", githubActionsJobActionRefs(job), githubActionsCheckoutAction)
	}
	checkout, ok := findJobActionStep(job, githubActionsCheckoutAction)
	if !ok {
		t.Fatalf("launch probe job must use %s", githubActionsCheckoutAction)
	}
	if checkout.With["persist-credentials"] != "false" {
		t.Fatalf("checkout persist-credentials = %q, want false", checkout.With["persist-credentials"])
	}
	if strings.TrimSpace(checkout.If) != "" || githubActionsContinueOnErrorEnabled(checkout.ContinueOnError) {
		t.Fatal("launch probe checkout must not be skippable or ignore failures")
	}
}

func requireLaunchProbeCommand(t *testing.T, defaults githubActionsDefaults, job githubActionsJob) {
	t.Helper()

	for _, step := range job.Steps {
		if strings.TrimSpace(step.Run) != "make launch-check" {
			continue
		}
		if strings.TrimSpace(githubActionsEffectiveWorkingDirectory(defaults, job.Defaults, step.WorkingDirectory)) != "" {
			t.Fatal("launch-check must run from the repository root")
		}
		if strings.TrimSpace(step.If) != "" || githubActionsContinueOnErrorEnabled(step.ContinueOnError) {
			t.Fatal("launch-check step must not be skippable or ignore failures")
		}
		if step.Env["GH_TOKEN"] != "${{ github.token }}" {
			t.Fatalf("launch-check GH_TOKEN = %q, want workflow-scoped github.token", step.Env["GH_TOKEN"])
		}
		return
	}
	t.Fatal("launch probe job must run make launch-check")
}
