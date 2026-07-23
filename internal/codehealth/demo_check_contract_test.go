package codehealth

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDemoCheckTargetComposition(t *testing.T) {
	repoRoot := findRepoRoot(t)
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))

	if !makefileDeclaresPhony(makefile, "check-demo-specs") {
		t.Fatal("Makefile .PHONY must include check-demo-specs")
	}
	if !makefileDeclaresPhony(makefile, "demo-check") {
		t.Fatal("Makefile .PHONY must include demo-check")
	}

	checkDemoSpecsRecipe := makeTargetRecipe(t, repoRoot, "check-demo-specs")
	requireRecipeCommand(t, checkDemoSpecsRecipe, "check-demo-specs", []string{
		"bash",
		"scripts/check_screen_spec.sh",
		"docs/reference/demo_specs",
	})
	if !containsStringField(makeTargetPrerequisites(makefile, "check"), "check-demo-specs") {
		t.Fatal("check target must include check-demo-specs as a prerequisite")
	}

	demoCheckRecipe := makeTargetRecipe(t, repoRoot, "demo-check")
	requireContainsAll(t, demoCheckRecipe, []string{"set -euo pipefail", "DEMO_CHECK_SKIP_LIVE"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"$(MAKE)", "check-demo-specs"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"$(MAKE)", "test-demos-unit"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"$(MAKE)", "test-demo-instantsearch"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"$(MAKE)", "test-push-smoke"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"bash", "scripts/demo_freshness_check.sh"})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{
		"gh", "workflow", "run", "cross_demo_live.yml", "-R", "AllyourbaseHQ/allyourbase",
	})
	requireRecipeCommand(t, demoCheckRecipe, "demo-check", []string{"gh", "run", "watch"})
	requireContainsAll(t, demoCheckRecipe, []string{
		`gh run watch "$$run_id" -R AllyourbaseHQ/allyourbase --exit-status`,
		"grep -Eq '^[0-9]+$$'",
		"DEMO-CHECK ARM live-workflow: SKIP",
		"DEMO-CHECK ARM live-workflow: PASS",
	})

	for _, arm := range demoCheckArms() {
		arm := arm
		t.Run(arm.name, func(t *testing.T) {
			requireContainsAll(t, demoCheckRecipe, []string{arm.envAssignment})
			requireRecipeCommand(t, demoCheckRecipe, arm.name, arm.command)
			requireRecipeCommand(t, demoCheckRecipe, arm.name+" guard", []string{
				"scripts/check-playwright-executed.sh",
				arm.reportPath,
				arm.project,
			})
			requireArmPassAfterGuard(t, demoCheckRecipe, arm.name, arm.reportPath, arm.project)
		})
	}

	t.Run("matchers reject non executed text", func(t *testing.T) {
		falseChecks := []struct {
			name    string
			recipe  string
			command []string
		}{
			{"echoed make arm", "echo $(MAKE) test-demos-unit", []string{"$(MAKE)", "test-demos-unit"}},
			{"commented make arm", "# $(MAKE) test-demos-unit", []string{"$(MAKE)", "test-demos-unit"}},
			{"error ignored make arm", "-$(MAKE) test-demos-unit", []string{"$(MAKE)", "test-demos-unit"}},
			{"short circuited make arm", "echo ok || $(MAKE) test-demos-unit", []string{"$(MAKE)", "test-demos-unit"}},
			{"neutralized guard", "scripts/check-playwright-executed.sh examples/kanban/playwright-report/results.json kanban || true", []string{"scripts/check-playwright-executed.sh", "examples/kanban/playwright-report/results.json", "kanban"}},
		}
		for _, check := range falseChecks {
			if recipeContainsCommandWithArgs(check.recipe, check.command) {
				t.Fatalf("%s must not satisfy executable command matching", check.name)
			}
		}
	})
}

func TestDemoFreshnessCheckScript(t *testing.T) {
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("bash", "scripts/demo_freshness_check_test.sh")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("demo freshness known-answer test failed: %v\n%s", err, output)
	}
}

type demoCheckArm struct {
	name          string
	envAssignment string
	command       []string
	reportPath    string
	project       string
}

func demoCheckArms() []demoCheckArm {
	return []demoCheckArm{
		{
			name:          "kanban-desktop",
			envAssignment: "AYB_DEMO_APP_PORT=45173",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=kanban", "--retries=0"},
			reportPath:    "examples/kanban/playwright-report/results.json",
			project:       "kanban",
		},
		{
			name:          "live-polls-desktop",
			envAssignment: "AYB_DEMO_APP_PORT=45175",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=live-polls", "--retries=0"},
			reportPath:    "examples/live-polls/playwright-report/results.json",
			project:       "live-polls",
		},
		{
			name:          "movies-desktop",
			envAssignment: "AYB_DEMO_APP_PORT=45177",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=movies", "--retries=0"},
			reportPath:    "examples/movies/playwright-report/results.json",
			project:       "movies",
		},
		{
			name:          "kanban-a11y",
			envAssignment: "AYB_DEMO_APP_PORT=45183",
			command:       []string{"npm", "run", "test:e2e", "--", "tests/a11y.spec.ts", "--project=kanban", "--retries=0"},
			reportPath:    "examples/kanban/playwright-report/results.json",
			project:       "kanban",
		},
		{
			name:          "live-polls-a11y",
			envAssignment: "AYB_DEMO_APP_PORT=45185",
			command:       []string{"npm", "run", "test:e2e", "--", "e2e/a11y.spec.ts", "--project=live-polls", "--retries=0"},
			reportPath:    "examples/live-polls/playwright-report/results.json",
			project:       "live-polls",
		},
		{
			name:          "movies-a11y",
			envAssignment: "AYB_DEMO_APP_PORT=45187",
			command:       []string{"npm", "run", "test:e2e", "--", "e2e/a11y.spec.ts", "--project=movies", "--retries=0"},
			reportPath:    "examples/movies/playwright-report/results.json",
			project:       "movies",
		},
		{
			name:          "instantsearch-a11y",
			envAssignment: "AYB_APP_PORT=45196",
			command:       []string{"npm", "run", "test:browser-tests", "--", "browser-tests-unmocked/smoke/a11y.spec.ts", "--project=instantsearch_demo", "--retries=0"},
			reportPath:    "examples/instantsearch_demo/playwright-report/results.json",
			project:       "instantsearch_demo",
		},
		{
			name:          "kanban-mobile",
			envAssignment: "AYB_DEMO_APP_PORT=45193",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=kanban-mobile", "--retries=0"},
			reportPath:    "examples/kanban/playwright-report/results.json",
			project:       "kanban-mobile",
		},
		{
			name:          "live-polls-mobile",
			envAssignment: "AYB_DEMO_APP_PORT=45195",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=live-polls-mobile", "--retries=0"},
			reportPath:    "examples/live-polls/playwright-report/results.json",
			project:       "live-polls-mobile",
		},
		{
			name:          "movies-mobile",
			envAssignment: "AYB_DEMO_APP_PORT=45197",
			command:       []string{"npm", "run", "test:e2e", "--", "--project=movies-mobile", "--retries=0"},
			reportPath:    "examples/movies/playwright-report/results.json",
			project:       "movies-mobile",
		},
		{
			name:          "instantsearch-mobile",
			envAssignment: "AYB_APP_PORT=45198",
			command:       []string{"npm", "run", "test:browser-tests", "--", "--project=instantsearch_demo-mobile", "--retries=0"},
			reportPath:    "examples/instantsearch_demo/playwright-report/results.json",
			project:       "instantsearch_demo-mobile",
		},
	}
}

func requireRecipeCommand(t *testing.T, recipe, label string, command []string) {
	t.Helper()
	if recipeContainsCommandWithArgs(recipe, command) {
		return
	}
	t.Fatalf("%s must execute %q\nrecipe:\n%s", label, strings.Join(command, " "), recipe)
}

func requireArmPassAfterGuard(t *testing.T, recipe, arm, reportPath, project string) {
	t.Helper()
	guardPattern := regexp.MustCompile(regexp.QuoteMeta("scripts/check-playwright-executed.sh") + `\s+` + regexp.QuoteMeta(reportPath) + `\s+` + regexp.QuoteMeta(project))
	guard := guardPattern.FindStringIndex(recipe)
	if guard == nil {
		t.Fatalf("%s must run Playwright execution guard for %s", arm, project)
	}
	passIndex := strings.Index(recipe[guard[1]:], "DEMO-CHECK ARM "+arm+": PASS")
	if passIndex < 0 {
		t.Fatalf("%s must print PASS only after its Playwright execution guard", arm)
	}
}
