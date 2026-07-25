package codehealth

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var demoCIWiringDemoDirs = []string{"instantsearch_demo", "kanban", "live-polls", "movies"}

type demoPackageManifest struct {
	Scripts map[string]string `json:"scripts"`
}

func TestNoDemoHasTwoLockfiles(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	lockfileNames := []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock"}
	for _, demo := range demoCIWiringDemoDirs {
		demo := demo
		t.Run(demo, func(t *testing.T) {
			var found []string
			for _, name := range lockfileNames {
				path := filepath.Join(repoRoot, "examples", demo, name)
				if _, err := os.Stat(path); err == nil {
					readRepoText(t, path)
					found = append(found, name)
				} else if !os.IsNotExist(err) {
					t.Fatalf("inspect examples/%s/%s failed: %v", demo, name, err)
				}
			}
			if len(found) > 1 {
				t.Fatalf("examples/%s has conflicting lockfiles: %s", demo, strings.Join(found, ", "))
			}
		})
	}
}

func TestDemoPackageManagerOwnership(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	npmOwnedDemos := []string{"kanban", "live-polls", "movies", "instantsearch_demo"}
	for _, demo := range npmOwnedDemos {
		demo := demo
		t.Run(demo, func(t *testing.T) {
			for _, name := range []string{"pnpm-lock.yaml", "pnpm-workspace.yaml", "yarn.lock"} {
				path := filepath.Join(repoRoot, "examples", demo, name)
				if _, err := os.Stat(path); err == nil {
					t.Fatalf("examples/%s is npm-owned and must not carry %s", demo, name)
				} else if !os.IsNotExist(err) {
					t.Fatalf("inspect examples/%s/%s failed: %v", demo, name, err)
				}
			}
		})
	}
}

func TestDemoPlaywrightReportsStayUntrackedRuntimeArtifacts(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	gitignore := readRepoText(t, filepath.Join(repoRoot, ".gitignore"))
	requireContainsAll(t, gitignore, []string{"examples/*/playwright-report/"})

	reportPaths := make([]string, 0, len(demoCIWiringDemoDirs))
	for _, demo := range demoCIWiringDemoDirs {
		reportPaths = append(reportPaths, filepath.Join("examples", demo, "playwright-report", "results.json"))
	}
	args := append([]string{"ls-files", "--"}, reportPaths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	if tracked := strings.TrimSpace(string(output)); tracked != "" {
		t.Fatalf("demo Playwright JSON reports are runtime artifacts and must not be tracked:\n%s", tracked)
	}
}

func TestDemoBuildArtifactsAreGuardedByLaunchJob(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}
	job, ok := workflow.Jobs["demo-launch"]
	if !ok {
		t.Fatal("workflow is missing jobs.demo-launch")
	}
	for index, step := range job.Steps {
		if step.Name != "Build demo apps" {
			continue
		}
		if index+1 >= len(job.Steps) {
			t.Fatal("demo-launch Build demo apps must be followed by the demo dist guard")
		}
		job.Steps = []githubActionsStep{job.Steps[index+1]}
		workflow.Jobs["demo-launch"] = job
		if !workflowJobHasRunStep(workflow, "demo-launch", "git diff --exit-code -- examples/kanban/dist examples/live-polls/dist examples/movies/dist examples/instantsearch_demo/dist") {
			t.Fatal("demo-launch Build demo apps must be immediately followed by the blocking demo dist guard")
		}
		return
	}
	t.Fatal("demo-launch is missing Build demo apps")
}

func TestDemoUnitSuitesAreInvokedByAMakeTarget(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	manifests := readDemoPackageManifests(t, repoRoot)
	requireDiscoveredDemoDirs(t, manifests)
	recipe := makeTargetRecipe(t, repoRoot, "test-demos-unit")
	for _, demo := range demoCIWiringDemoDirs {
		demo := demo
		manifest := manifests[demo]
		t.Run(demo, func(t *testing.T) {
			if strings.TrimSpace(manifest.Scripts["test"]) == "" {
				t.Skipf("%s package.json does not declare scripts.test", demo)
			}
			if !recipeContainsScopedCommand(recipe, demo, "npm test") {
				t.Fatalf("expected test-demos-unit to run %q from examples/%s", "npm test", demo)
			}
		})
	}
}
func TestDemoBrowserTestLintIsInvokedByTheLintGate(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	manifests := readDemoPackageManifests(t, repoRoot)
	requireDiscoveredDemoDirs(t, manifests)
	recipe := makeTargetRecipe(t, repoRoot, "check-browser-tests-lint")
	for _, demo := range demoCIWiringDemoDirs {
		demo := demo
		manifest := manifests[demo]
		t.Run(demo, func(t *testing.T) {
			if strings.TrimSpace(manifest.Scripts["lint:browser-tests"]) == "" {
				t.Fatalf("examples/%s/package.json must declare a non-empty scripts[lint:browser-tests]", demo)
			}
			if !recipeContainsScopedCommand(recipe, demo, "npm run lint:browser-tests") {
				t.Fatalf("expected check-browser-tests-lint to run %q from examples/%s", "npm run lint:browser-tests", demo)
			}
		})
	}
}
func TestDemoPlaywrightConfigsAreExecutionGuarded(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	for _, demo := range demoCIWiringDemoDirs {
		demo := demo
		t.Run(demo, func(t *testing.T) {
			configPath := filepath.Join(repoRoot, "examples", demo, "playwright.config.ts")
			config := readRepoText(t, configPath)
			requireContainsAll(t, config, []string{
				"projects:",
				fmt.Sprintf(`name: "%s"`, demo),
				`["json", { outputFile: "playwright-report/results.json" }]`,
			})
		})
	}
	t.Run("kanban_fails_closed_on_prebound_demo_port", func(t *testing.T) {
		configPath := filepath.Join(repoRoot, "examples", "kanban", "playwright.config.ts")
		config := readRepoText(t, configPath)
		requireContainsAll(t, config, []string{
			"process.env.AYB_DEMO_APP_PORT",
			"port: testPort",
			"reuseExistingServer: false",
		})
	})
	t.Run("kanban_focused_config_fails_closed_and_starts_demo_runtime", func(t *testing.T) {
		config := readRepoText(t, filepath.Join(repoRoot, "examples", "kanban", "playwright.config.ts"))
		requireContainsAll(t, config, []string{
			"webServer:", "../../ayb", "demo kanban", "AYB_SERVER_PORT", "AYB_DATABASE_EMBEDDED_PORT",
			"export AYB_SERVER_PORT", "export AYB_DATABASE_EMBEDDED_PORT", "export AYB_DATABASE_EMBEDDED_DATA_DIR",
			"export HOME", "AYB_AUTH_RATE_LIMIT=10000", "AYB_RATE_LIMIT_API=10000/min", "demo_pid", "trap cleanup EXIT INT TERM",
			`AYB_DATABASE_EMBEDDED_DATA_DIR="$(mktemp`, `gracefulShutdown: { signal: "SIGINT", timeout: 10000 }`,
		})
		requireDoesNotContainAny(t, config, []string{"npm run dev", "vite", `export AYB_SERVER_PORT="$(pick_free_port`,
			`export AYB_DATABASE_EMBEDDED_PORT="$(pick_free_port`, `export AYB_DATABASE_EMBEDDED_DATA_DIR="$(mktemp`})
		if strings.Index(config, "trap cleanup EXIT INT TERM") > strings.Index(config, `AYB_DATABASE_EMBEDDED_DATA_DIR="$(mktemp`) {
			t.Fatal("Kanban runtime cleanup trap must be installed before temporary runtime state is allocated")
		}
	})
	t.Run("manual_smoke_harness", func(t *testing.T) {
		scriptPath := filepath.Join(repoRoot, "_dev", "manual_smoke_tests", "18_demo_e2e.test.sh")
		runDemoE2E := shellFunctionBody(t, readRepoText(t, scriptPath), "run_demo_e2e")
		commands := executableShellCommandBlocks(runDemoE2E)
		playwrightCommandIndex := shellCommandBlockIndex(commands, []string{"npx", "playwright", "test"})
		if playwrightCommandIndex < 0 {
			t.Fatal("run_demo_e2e must invoke Playwright")
		}
		playwrightCommand := commands[playwrightCommandIndex]
		if strings.HasPrefix(strings.TrimSpace(playwrightCommand), "if ") {
			t.Errorf("run_demo_e2e must not make the execution guard conditional on Playwright success: %s", playwrightCommand)
		}
		if !strings.Contains(runDemoE2E, `playwright_status=${PIPESTATUS[0]}`) {
			t.Error("run_demo_e2e must capture Playwright pipeline status before running the execution guard")
		}
		if strings.Contains(playwrightCommand, "--reporter") {
			t.Errorf("run_demo_e2e Playwright command must not pass a CLI --reporter override: %s", playwrightCommand)
		}
		guardCommand := ""
		for _, command := range commands[playwrightCommandIndex+1:] {
			if shellBlockRunsCommandWithArgs(command, []string{
				"scripts/check-playwright-executed.sh",
				"$example_dir/playwright-report/results.json",
				"$name",
			}) {
				guardCommand = command
				break
			}
		}
		if guardCommand == "" {
			t.Error("run_demo_e2e must call scripts/check-playwright-executed.sh after Playwright")
			return
		}
	})
	t.Run("instantsearch_make_guard", func(t *testing.T) {
		recipe := makeTargetRecipe(t, repoRoot, "test-demo-instantsearch")
		if !recipeContainsCommandWithArgs(recipe, []string{
			"scripts/check-playwright-executed.sh",
			"examples/instantsearch_demo/playwright-report/results.json",
			"instantsearch_demo",
		}) {
			t.Fatal("expected test-demo-instantsearch to run bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo")
		}
		requireContainsAll(t, recipe, []string{
			"browser_status=0",
			"npm run test:browser-tests || browser_status=$$?",
			"guard_status=0",
			"scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo || guard_status=$$?",
			`test "$$browser_status" -eq 0 && test "$$guard_status" -eq 0`,
		})
	})
	t.Run("instantsearch_browser_script_reporter", func(t *testing.T) {
		manifest := readDemoPackageManifest(t, filepath.Join(repoRoot, "examples", "instantsearch_demo", "package.json"))
		scriptName := "test:browser-tests"
		scriptValue := manifest.Scripts[scriptName]
		if strings.TrimSpace(scriptValue) == "" {
			t.Fatalf("examples/instantsearch_demo/package.json must declare scripts[%s]", scriptName)
		}
		t.Logf("examples/instantsearch_demo scripts[%s] = %q", scriptName, scriptValue)
		requireDoesNotContainAny(t, scriptValue, []string{"--reporter"})
	})
	t.Run("movies_browser_script_reporter", func(t *testing.T) {
		manifest := readDemoPackageManifest(t, filepath.Join(repoRoot, "examples", "movies", "package.json"))
		scriptName := "test:e2e"
		scriptValue := manifest.Scripts[scriptName]
		if strings.TrimSpace(scriptValue) == "" {
			t.Fatalf("examples/movies/package.json must declare scripts[%s]", scriptName)
		}
		t.Logf("examples/movies scripts[%s] = %q", scriptName, scriptValue)
		requireDoesNotContainAny(t, scriptValue, []string{"--reporter"})
	})
}
func TestInstantSearchDeployRunsItsTests(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "deploy_live_instantsearch.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}
	deployJob, ok := workflow.Jobs["deploy-live-instantsearch"]
	if !ok {
		t.Fatal("workflow is missing jobs.deploy-live-instantsearch")
	}
	if _, ok := workflow.Jobs["test-instantsearch"]; !ok {
		t.Fatal("workflow is missing jobs.test-instantsearch")
	}
	if !jobNeeds(deployJob, "test-instantsearch") {
		t.Fatal("deploy-live-instantsearch must depend on test-instantsearch")
	}
	if !workflowJobHasRunStep(workflow, "test-instantsearch", "make test-demo-instantsearch") {
		t.Fatal("test-instantsearch must run make test-demo-instantsearch")
	}
	testJob := workflow.Jobs["test-instantsearch"]
	if githubActionsPermissionGrantsWrite(workflow.Permissions, "deployments") {
		t.Fatal("deploy_live_instantsearch.yml must not grant deployments: write at workflow scope")
	}
	if githubActionsPermissionGrantsWrite(testJob.Permissions, "deployments") {
		t.Fatal("test-instantsearch must not have deployments: write")
	}
	if !githubActionsPermissionGrantsWrite(deployJob.Permissions, "deployments") {
		t.Fatal("deploy-live-instantsearch must declare deployments: write at job scope")
	}
}
func TestDemoCIJobsPinNewGitHubActions(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}
	t.Run("demo-instantsearch", func(t *testing.T) {
		job, ok := workflow.Jobs["demo-instantsearch"]
		if !ok {
			t.Fatal("workflow is missing jobs.demo-instantsearch")
		}
		requireJobUsesAction(t, job, "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd")
		requireJobUsesAction(t, job, githubActionsSetupNode22Action)
		requireJobUsesAction(t, job, "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16")
	})
	t.Run("demo-unit", func(t *testing.T) {
		job, ok := workflow.Jobs["demo-unit"]
		if !ok {
			t.Fatal("workflow is missing jobs.demo-unit")
		}
		requireJobUsesAction(t, job, "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd")
		requireJobUsesAction(t, job, githubActionsSetupNode22Action)
	})
}

const (
	moviesRealProviderScript    = "_dev/manual_smoke_tests/19_movies_real_provider.test.sh"
	moviesRealProviderTarget    = "test-demo-movies-real-provider"
	moviesRealProviderJob       = "demo-movies-real-provider"
	moviesRealProviderPrintFlag = "--print-ollama-install-version"
)

// TestMoviesRealProviderSmokeIsWiredAndUnskippable pins the movies real-provider
// smoke as a recurring, fail-closed CI input reachable through one Make target,
// with the smoke script as the only owner of the pinned Ollama version.
func TestMoviesRealProviderSmokeIsWiredAndUnskippable(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))
	script := readRepoText(t, filepath.Join(repoRoot, moviesRealProviderScript))
	workflowText := readRepoText(t, filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(workflowText), &workflow); err != nil {
		t.Fatalf("decode .github/workflows/ci.yml failed: %v", err)
	}

	t.Run("make_target_runs_the_smoke_and_stays_out_of_test_all", func(t *testing.T) {
		if !makefileDeclaresPhony(makefile, moviesRealProviderTarget) {
			t.Errorf(".PHONY must declare %s", moviesRealProviderTarget)
		}
		prerequisites := makeTargetPrerequisites(makefile, moviesRealProviderTarget)
		if !containsStringField(prerequisites, "build") {
			t.Errorf("%s must depend on build; got prerequisites %v", moviesRealProviderTarget, prerequisites)
		}
		recipe := extractMakeTargetRecipe(makefile, moviesRealProviderTarget)
		if !recipeContainsCommandWithArgs(recipe, []string{"AYB_BIN=$(CURDIR)/ayb", "bash", moviesRealProviderScript}) {
			t.Errorf("%s must run AYB_BIN=$(CURDIR)/ayb bash %s from the repository root; got recipe:\n%s",
				moviesRealProviderTarget, moviesRealProviderScript, recipe)
		}
		if containsStringField(makeTargetPrerequisites(makefile, "test-all"), moviesRealProviderTarget) {
			t.Errorf("test-all must not depend on %s; its model download is too heavy for the fast lane", moviesRealProviderTarget)
		}
		if strings.Contains(extractMakeTargetRecipe(makefile, "test-all"), moviesRealProviderTarget) {
			t.Errorf("test-all must not invoke %s", moviesRealProviderTarget)
		}
	})

	t.Run("ci_job_runs_the_make_target_unconditionally", func(t *testing.T) {
		job, ok := workflow.Jobs[moviesRealProviderJob]
		if !ok {
			t.Fatalf("ci.yml is missing jobs.%s", moviesRealProviderJob)
		}
		if strings.TrimSpace(job.If) != "" {
			t.Errorf("%s must not declare a job-level if: %q", moviesRealProviderJob, job.If)
		}
		if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
			t.Errorf("%s must not set continue-on-error", moviesRealProviderJob)
		}
		for _, step := range job.Steps {
			if strings.TrimSpace(step.If) != "" {
				t.Errorf("%s step %q must not declare an if: %q", moviesRealProviderJob, step.Name, step.If)
			}
		}
		if !workflowJobHasRunStep(workflow, moviesRealProviderJob, "make "+moviesRealProviderTarget) {
			t.Errorf("%s must run make %s from the repository root with its failure gating the job",
				moviesRealProviderJob, moviesRealProviderTarget)
		}
		requireWorkflowJobPreparesJavaScriptPrerequisites(t, workflow, makefile, moviesRealProviderJob, moviesRealProviderTarget)
		requireJobUsesAction(t, job, "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd")
		requireJobUsesAction(t, job, "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16")
		if job.TimeoutMinutes <= 0 {
			t.Errorf("%s must declare an explicit timeout-minutes sized from the measured pull/smoke runtime", moviesRealProviderJob)
		}
		if !workflowJobInstallsVerifiedOllamaArchiveBeforeSmoke(job) {
			t.Errorf("%s must install the pinned Ollama release archive with checksum verification before make %s", moviesRealProviderJob, moviesRealProviderTarget)
		}
	})

	t.Run("script_owns_the_pinned_ollama_version", func(t *testing.T) {
		if !strings.Contains(script, moviesRealProviderPrintFlag) {
			t.Fatalf("%s must expose the machine-readable %s mode", moviesRealProviderScript, moviesRealProviderPrintFlag)
		}
		if !workflowJobRunsScriptWithFlag(workflow, moviesRealProviderJob, moviesRealProviderScript, moviesRealProviderPrintFlag) {
			t.Errorf("%s must obtain the Ollama version by running %s %s", moviesRealProviderJob, moviesRealProviderScript, moviesRealProviderPrintFlag)
		}
		pinned := pinnedOllamaInstallVersion(t, script)
		if strings.Contains(workflowText, pinned) {
			t.Errorf("ci.yml must not repeat the pinned Ollama version %q; %s owns it", pinned, moviesRealProviderScript)
		}
		if regexp.MustCompile(`OLLAMA_VERSION=[0-9]`).MatchString(workflowText) {
			t.Errorf("ci.yml must not hardcode a second literal Ollama version")
		}
	})

	t.Run("script_provider_path_has_no_skip_gate", func(t *testing.T) {
		mainBody := shellFunctionBody(t, script, "main")
		mainCommands := executableShellCommandBlocks(mainBody)
		ollamaCommands := executableShellCommandBlocks(shellFunctionBody(t, script, "start_real_ollama"))
		requireShellCommandsRun(t, "main", mainCommands, [][]string{
			{"start_real_ollama"},
			{"assert_direct_ollama_embeddings"},
			{"start_movies_demo"},
			{"assert_movies_search"},
			{"assert_movies_note_embeddings_are_input_sensitive"},
			{"assert_movies_note_embedding_uses_real_provider"},
		})
		requireShellCommandsRun(t, "start_real_ollama", ollamaCommands, [][]string{
			{"ollama", "serve"},
			{"ollama", "pull"},
		})
		if regexp.MustCompile(`(?i)skip`).MatchString(script) {
			t.Errorf("%s must not offer a skip gate for the real-provider path", moviesRealProviderScript)
		}
		requireMoviesRealProviderDefaultPath(t, mainBody)
		for _, fixture := range []struct {
			name string
			body string
		}{
			{"opt-in guard", `if [ "${RUN_REAL_PROVIDER:-}" != "1" ]; then exit 0; fi
start_real_ollama
assert_direct_ollama_embeddings`},
			{"exit zero guard", `exit 0
start_real_ollama
assert_direct_ollama_embeddings`},
			{"return zero guard", `return 0
start_real_ollama
assert_direct_ollama_embeddings`},
		} {
			t.Run("rejects_"+strings.ReplaceAll(fixture.name, " ", "_"), func(t *testing.T) {
				if moviesRealProviderDefaultPathIsUnskippable(fixture.body) {
					t.Fatalf("default real-provider path accepted %s before start_real_ollama", fixture.name)
				}
			})
		}
		for _, fixture := range []struct {
			name string
			body string
		}{
			{"print version branch", `case "$MODE" in
    --print-ollama-install-version)
        printf '%s\n' "$OLLAMA_INSTALL_VERSION"
        exit 0
        ;;
esac
start_real_ollama`},
			{"fake provider branch", `if [ "$MODE" = "--fake-provider-red" ]; then
    run_fake_provider_red
    exit 1
fi
start_real_ollama`},
		} {
			t.Run("accepts_"+strings.ReplaceAll(fixture.name, " ", "_"), func(t *testing.T) {
				requireMoviesRealProviderDefaultPath(t, fixture.body)
			})
		}
	})

	t.Run("debbie_sync_covers_the_script", func(t *testing.T) {
		debbiePath := filepath.Join(repoRoot, ".debbie.toml")
		if _, err := os.Stat(debbiePath); os.IsNotExist(err) {
			t.Skip(".debbie.toml is intentionally absent from public mirrors")
		}
		debbie := readRepoText(t, debbiePath)
		if !strings.Contains(debbie, `"`+moviesRealProviderScript+`"`) {
			t.Errorf(".debbie.toml [sync].files must whitelist %s", moviesRealProviderScript)
		}
		if !strings.Contains(debbie, `path = "tests/"`) {
			t.Errorf(`.debbie.toml must keep syncing the tests/ directory that owns tests/port_helpers.sh`)
		}
	})
}

func workflowJobInstallsVerifiedOllamaArchiveBeforeSmoke(job githubActionsJob) bool {
	installIndex := -1
	smokeIndex := -1
	commandIndex := 0
	for _, step := range job.Steps {
		if installIndex < 0 &&
			strings.Contains(step.Run, "curl --proto '=https' --tlsv1.2 -fsSLo") &&
			strings.Contains(step.Run, "ollama-linux-amd64.tar.zst") &&
			strings.Contains(step.Run, "github.com/ollama/ollama/releases/download/v${version}/") &&
			strings.Contains(step.Run, "sha256sum.txt") &&
			strings.Contains(step.Run, "sha256sum -c -") &&
			strings.Contains(step.Run, "sudo tar --zstd -xf") &&
			!strings.Contains(step.Run, "install.sh") {
			installIndex = commandIndex
		}
		for _, block := range executableShellCommandBlocks(step.Run) {
			if shellBlockRunsCommandWithArgs(block, []string{"make", moviesRealProviderTarget}) {
				smokeIndex = commandIndex
			}
			commandIndex++
		}
	}
	return installIndex >= 0 && smokeIndex > installIndex
}

func requireMoviesRealProviderDefaultPath(t *testing.T, mainBody string) {
	t.Helper()
	if !moviesRealProviderDefaultPathIsUnskippable(mainBody) {
		t.Fatal("default real-provider path must not contain an opt-in or early-success guard before start_real_ollama")
	}
}

func moviesRealProviderDefaultPathIsUnskippable(mainBody string) bool {
	defaultBody := moviesRealProviderDefaultPathBody(mainBody)
	for _, segment := range shellCommandSegments(defaultBody) {
		if shellSegmentStartsWithFields(segment, []string{"start_real_ollama"}) {
			return true
		}
		if shellSegmentIsEarlySuccess(segment) || shellSegmentLooksLikeOptInGuard(segment) {
			return false
		}
	}
	return false
}

func moviesRealProviderDefaultPathBody(mainBody string) string {
	var lines []string
	skipModeCase := false
	skipFakeBranch := false
	for _, line := range strings.Split(mainBody, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, `case "$MODE" in`):
			skipModeCase = true
			continue
		case skipModeCase:
			if trimmed == "esac" {
				skipModeCase = false
			}
			continue
		case strings.Contains(trimmed, `"$MODE" = "--fake-provider-red"`):
			skipFakeBranch = true
			continue
		case skipFakeBranch:
			if trimmed == "fi" {
				skipFakeBranch = false
			}
			continue
		default:
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func shellSegmentIsEarlySuccess(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "exit", "return":
		if len(fields) < 2 {
			return true
		}
		return strings.Trim(fields[1], `"'`) == "0"
	default:
		return false
	}
}

func shellSegmentLooksLikeOptInGuard(segment string) bool {
	upper := strings.ToUpper(segment)
	return strings.HasPrefix(strings.TrimSpace(segment), "if ") &&
		(strings.Contains(upper, "RUN_REAL") || strings.Contains(upper, "ENABLE") || strings.Contains(upper, "OPT_IN"))
}

func TestDemoCIWiringShellAssertionsRejectNonExecutedText(t *testing.T) {
	t.Parallel()
	const demo = "live-polls"
	guardArgs := []string{
		"scripts/check-playwright-executed.sh",
		"$example_dir/playwright-report/results.json",
		"$name",
	}
	falseChecks := []struct {
		name    string
		matched bool
	}{
		{"comments and echoed scoped npm test", recipeContainsScopedCommand("# cd examples/live-polls && npm test\n@echo cd examples/live-polls && npm test", demo, "npm test")},
		{"echoed npm test after scoped cd", recipeContainsScopedCommand("cd examples/live-polls && echo npm test", demo, "npm test")},
		{"npm test after changing away from demo", recipeContainsScopedCommand("cd examples/live-polls && cd ../movies && npm test", demo, "npm test")},
		{"short-circuited fallback scoped npm test", recipeContainsScopedCommand("echo ok || cd examples/live-polls && npm test", demo, "npm test")},
		{"Make error-ignored scoped npm test", recipeContainsScopedCommand("-cd examples/live-polls && npm test", demo, "npm test")},
		{"Make error-ignored Playwright guard", recipeContainsCommandWithArgs("-bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo", []string{
			"scripts/check-playwright-executed.sh",
			"examples/instantsearch_demo/playwright-report/results.json",
			"instantsearch_demo",
		})},
		{"echoed workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "echo make test-demo-instantsearch"}}}, "make test-demo-instantsearch")},
		{"known-dead && workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "false && make test-demo-instantsearch"}}}, "make test-demo-instantsearch")},
		{"short-circuited fallback workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "echo ok || make test-demo-instantsearch"}}}, "make test-demo-instantsearch")},
		{"continue-on-error workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch", ContinueOnError: yaml.Node{Kind: yaml.ScalarNode, Value: "true"}}}}, "make test-demo-instantsearch")},
		{"demo working-directory workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch", WorkingDirectory: "examples/instantsearch_demo"}}}, "make test-demo-instantsearch")},
		{"workflow defaults working-directory workflow make command", workflowJobHasRunStep(githubActionsWorkflow{
			Defaults: githubActionsDefaults{Run: githubActionsRunDefaults{WorkingDirectory: "examples/instantsearch_demo"}},
			Jobs:     map[string]githubActionsJob{"test-instantsearch": {Steps: []githubActionsStep{{Run: "make test-demo-instantsearch"}}}},
		}, "test-instantsearch", "make test-demo-instantsearch")},
		{"job defaults working-directory workflow make command", workflowJobHasRunStep(githubActionsWorkflow{
			Jobs: map[string]githubActionsJob{"test-instantsearch": {
				Defaults: githubActionsDefaults{Run: githubActionsRunDefaults{WorkingDirectory: "examples/instantsearch_demo"}},
				Steps:    []githubActionsStep{{Run: "make test-demo-instantsearch"}},
			}},
		}, "test-instantsearch", "make test-demo-instantsearch")},
		{"job-level continue-on-error workflow make command", workflowJobHasRunStep(githubActionsWorkflow{
			Jobs: map[string]githubActionsJob{"test-instantsearch": {
				ContinueOnError: yaml.Node{Kind: yaml.ScalarNode, Value: "true"},
				Steps:           []githubActionsStep{{Run: "make test-demo-instantsearch"}},
			}},
		}, "test-instantsearch", "make test-demo-instantsearch")},
		{"error-neutralized workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch || true"}}}, "make test-demo-instantsearch")},
		{"echo-neutralized workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch || echo skipped"}}}, "make test-demo-instantsearch")},
		{"exit-zero-neutralized workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch || exit 0"}}}, "make test-demo-instantsearch")},
		{"date-fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || date"}}}, "make test-demo-movies-real-provider")},
		{"arbitrary-success-fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || pwd"}}}, "make test-demo-movies-real-provider")},
		{"brace-group success fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || { date; }"}}}, "make test-demo-movies-real-provider")},
		{"multiline brace-group success fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || {\n  date\n}"}}}, "make test-demo-movies-real-provider")},
		{"echoed die brace-group fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || { echo die; }"}}}, "make test-demo-movies-real-provider")},
		{"echoed exit brace-group fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || { echo exit 1; }"}}}, "make test-demo-movies-real-provider")},
		{"overridden false brace-group fallback-neutralized movies smoke", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-movies-real-provider || { false; date; }"}}}, "make test-demo-movies-real-provider")},
		{"error-neutralized Make recipe guard", recipeContainsCommandWithArgs("@bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo || true", []string{
			"scripts/check-playwright-executed.sh",
			"examples/instantsearch_demo/playwright-report/results.json",
			"instantsearch_demo",
		})},
		{"step-level if workflow make command", jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch", If: "success()"}}}, "make test-demo-instantsearch")},
		{"job-level if workflow make command", workflowJobHasRunStep(githubActionsWorkflow{
			Jobs: map[string]githubActionsJob{"test-instantsearch": {
				If:    "github.event_name == 'push'",
				Steps: []githubActionsStep{{Run: "make test-demo-instantsearch"}},
			}},
		}, "test-instantsearch", "make test-demo-instantsearch")},
		{"echoed Playwright guard", shellBlockRunsCommandWithArgs(`echo scripts/check-playwright-executed.sh $example_dir/playwright-report/results.json $name`, guardArgs)},
		{"known-dead && Playwright guard", shellBlockRunsCommandWithArgs(`false && bash scripts/check-playwright-executed.sh $example_dir/playwright-report/results.json $name`, guardArgs)},
		{"short-circuited fallback Playwright guard", shellBlockRunsCommandWithArgs(`echo ok || bash scripts/check-playwright-executed.sh $example_dir/playwright-report/results.json $name`, guardArgs)},
	}
	for _, check := range falseChecks {
		if check.matched {
			t.Fatalf("%s must not satisfy executable command matching", check.name)
		}
	}
	realJob := githubActionsJob{Steps: []githubActionsStep{{Run: "echo starting; make test-demo-instantsearch"}}}
	if !recipeContainsScopedCommand("@echo preparing; cd examples/live-polls && npm test", demo, "npm test") {
		t.Fatal("a real scoped npm test command after an echo should satisfy scoped execution")
	}
	if !recipeContainsScopedCommand("@cd examples/live-polls && npm test", demo, "npm test") {
		t.Fatal("a real scoped npm test command with a silent Make prefix should satisfy scoped execution")
	}
	if recipeContainsScopedCommand("false && cd examples/live-polls && npm test", demo, "npm test") {
		t.Fatal("known-dead && branches must not satisfy scoped execution")
	}
	if !jobHasRunStep(realJob, "make test-demo-instantsearch") {
		t.Fatal("real workflow make command after an echo should satisfy execution")
	}
	if !shellBlockRunsCommandWithArgs(`bash scripts/check-playwright-executed.sh $example_dir/playwright-report/results.json $name`, guardArgs) {
		t.Fatal("guard execution must bind the report path and project name to the same command")
	}
	if !jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch || make_status=$?"}}}, "make test-demo-instantsearch") {
		t.Fatal("capturing a command's exit status into a variable must still satisfy execution")
	}
	if !jobHasRunStep(githubActionsJob{Steps: []githubActionsStep{{Run: "make test-demo-instantsearch || { echo failed; exit 1; }"}}}, "make test-demo-instantsearch") {
		t.Fatal("a brace-group fallback that explicitly fails must still satisfy execution")
	}
	if !shellBlockRunsCommandWithArgs("OLLAMA_HOST=127.0.0.1:11434 ollama serve", []string{"ollama", "serve"}) {
		t.Fatal("a leading environment assignment must not hide the executed command")
	}
	commands := executableShellCommandBlocks("echo npx playwright test\nnpx playwright test")
	if got := shellCommandBlockIndex(commands, []string{"npx", "playwright", "test"}); got != 1 {
		t.Fatalf("Playwright command lookup must ignore echoed text; got block index %d", got)
	}
}

func workflowJobRunsScriptWithFlag(workflow githubActionsWorkflow, jobName, script, flag string) bool {
	job, ok := workflow.Jobs[jobName]
	if !ok {
		return false
	}
	for _, step := range job.Steps {
		if !githubActionsStepIsRootGating(workflow.Defaults, job, step) {
			continue
		}
		for _, block := range executableShellCommandBlocks(step.Run) {
			if shellBlockRunsCommandWithArgs(block, []string{script, flag}) {
				return true
			}
		}
	}
	return false
}

func requireShellCommandsRun(t *testing.T, name string, commands []string, wanted [][]string) {
	t.Helper()
	for _, want := range wanted {
		if shellCommandBlockIndex(commands, want) < 0 {
			t.Errorf("%s must execute %q", name, strings.Join(want, " "))
		}
	}
}

func recipeContainsScopedCommand(recipe, demo, command string) bool {
	for _, block := range executableShellCommandBlocks(recipe) {
		if shellBlockRunsScopedCommand(block, demo, strings.Fields(command)) {
			return true
		}
	}
	return false
}

func recipeContainsCommandWithArgs(recipe string, args []string) bool {
	for _, block := range executableShellCommandBlocks(recipe) {
		if shellBlockRunsCommandWithArgs(block, args) {
			return true
		}
	}
	return false
}

func shellBlockRunsScopedCommand(block, demo string, commandFields []string) bool {
	seenScopedCD := false
	for _, segment := range shellCommandSegments(block) {
		if shellSegmentChangesToDemo(segment, demo) {
			seenScopedCD = true
			continue
		}
		if shellSegmentStartsWithCommand(segment, "cd") {
			seenScopedCD = false
			continue
		}
		if seenScopedCD && shellSegmentStartsWithFields(segment, commandFields) {
			return true
		}
	}
	return false
}

func shellSegmentChangesToDemo(segment, demo string) bool {
	fields := strings.Fields(segment)
	if len(fields) < 2 || fields[0] != "cd" {
		return false
	}
	return strings.Trim(fields[1], `"'`) == "examples/"+demo
}

func missingStrings(want, got []string) []string {
	gotSet := make(map[string]struct{}, len(got))
	for _, value := range got {
		gotSet[value] = struct{}{}
	}
	var missing []string
	for _, value := range want {
		if _, ok := gotSet[value]; !ok {
			missing = append(missing, value)
		}
	}
	return missing
}

func readDemoPackageManifests(t *testing.T, repoRoot string) map[string]demoPackageManifest {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot, "examples", "*", "package.json"))
	if err != nil {
		t.Fatalf("glob demo package manifests failed: %v", err)
	}
	manifests := make(map[string]demoPackageManifest, len(paths))
	for _, path := range paths {
		demo := filepath.Base(filepath.Dir(path))
		manifests[demo] = readDemoPackageManifest(t, path)
	}
	return manifests
}
func readDemoPackageManifest(t *testing.T, path string) demoPackageManifest {
	t.Helper()
	var manifest demoPackageManifest
	if err := json.Unmarshal([]byte(readRepoText(t, path)), &manifest); err != nil {
		t.Fatalf("decode %s failed: %v", path, err)
	}
	if manifest.Scripts == nil {
		manifest.Scripts = map[string]string{}
	}
	return manifest
}
func requireDiscoveredDemoDirs(t *testing.T, manifests map[string]demoPackageManifest) {
	t.Helper()
	got := make([]string, 0, len(manifests))
	for demo := range manifests {
		got = append(got, demo)
	}
	sort.Strings(got)
	want := append([]string(nil), demoCIWiringDemoDirs...)
	sort.Strings(want)
	if strings.Join(got, "\n") == strings.Join(want, "\n") {
		return
	}
	t.Fatalf("discovered demo package directories mismatch: missing=%v unexpected=%v got=%v want=%v", missingStrings(want, got), missingStrings(got, want), got, want)
}
func pinnedOllamaInstallVersion(t *testing.T, script string) string {
	t.Helper()
	pattern := regexp.MustCompile(`OLLAMA_INSTALL_VERSION="\$\{OLLAMA_INSTALL_VERSION:-([^}"]+)\}"`)
	match := pattern.FindStringSubmatch(script)
	if match == nil {
		t.Fatalf("%s must pin OLLAMA_INSTALL_VERSION with an overridable default", moviesRealProviderScript)
	}
	return match[1]
}
