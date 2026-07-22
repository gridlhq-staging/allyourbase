package codehealth

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const demoExternalServerEnv = "AYB_DEMO_EXTERNAL_SERVER"

func TestLiveDemoRunnerHasSingleAppServerOwnerInCI(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	runDemoE2E := shellFunctionBody(t,
		readRepoText(t, filepath.Join(repoRoot, "_dev", "manual_smoke_tests", "18_demo_e2e.test.sh")),
		"run_demo_e2e",
	)
	commands := executableShellCommandBlocks(runDemoE2E)
	demoLaunchIndex := shellCommandBlockIndexContaining(commands, `"$AYB_BIN" demo "$name"`)
	if demoLaunchIndex < 0 {
		t.Fatal("run_demo_e2e must start the live demo server with $AYB_BIN demo $name before invoking Playwright")
	}
	playwrightIndex := shellCommandBlockIndex(commands, []string{"npx", "playwright", "test"})
	if playwrightIndex < 0 {
		t.Fatal("run_demo_e2e must invoke Playwright after starting the live demo server")
	}
	if demoLaunchIndex > playwrightIndex {
		t.Fatal("run_demo_e2e must start $AYB_BIN demo $name before invoking Playwright")
	}
	playwrightCommand := commands[playwrightIndex]
	if !shellBlockRunsCommandWithEnv(playwrightCommand, []string{"npx", "playwright", "test"}, demoExternalServerEnv, "1") {
		t.Fatalf("run_demo_e2e Playwright invocation must pass %s=1 so configs disable their own webServer; got %q",
			demoExternalServerEnv, playwrightCommand)
	}

	for _, demo := range []string{"kanban", "live-polls", "movies"} {
		demo := demo
		t.Run(demo, func(t *testing.T) {
			config := readRepoText(t, filepath.Join(repoRoot, "examples", demo, "playwright.config.ts"))
			if !playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
				t.Fatalf("examples/%s/playwright.config.ts must set webServer to undefined when %s is present, instead of only reading the env var while Playwright still starts another server",
					demo, demoExternalServerEnv)
			}
			requireDoesNotContainAny(t, config, []string{
				"reuseExistingServer: true,",
				"reuseExistingServer: true\n",
			})
			if strings.Contains(config, "port: Number(process.env.AYB_DEMO_APP_PORT)") {
				t.Fatalf("examples/%s/playwright.config.ts must not infer external server ownership from AYB_DEMO_APP_PORT", demo)
			}
		})
	}
}

func TestDemoRunnerInvocationHasSingleAppServerOwner(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	runDemoE2E := shellFunctionBody(t,
		readRepoText(t, filepath.Join(repoRoot, "_dev", "manual_smoke_tests", "18_demo_e2e.test.sh")),
		"run_demo_e2e",
	)

	t.Run("live runner owns the selected app port", func(t *testing.T) {
		observations := runDemoRunnerHarness(t, repoRoot, runDemoE2E, true)
		requireSingleDemoAppServerOwner(t, observations)

		selectedPort := extractOutputValue(t, observations, "picked|")
		if selectedPort == "5173" || selectedPort == "5175" || selectedPort == "5177" {
			t.Fatalf("isolated picker selected universal demo port %s", selectedPort)
		}
		requireOutputContains(t, observations, "ayb|"+selectedPort+"|unset")
		requireOutputContains(t, observations, "npm|ci|unset")
		requireOutputContains(t, observations, "npx|install|unset")
		requireOutputContains(t, observations, "npx|test|"+selectedPort+"|1")
		if starts := strings.Count(observations, "owner|ayb\n"); starts != 1 {
			t.Fatalf("AYB runner must start exactly once, got %d starts in:\n%s", starts, observations)
		}
	})

	t.Run("both runner and Playwright ownership is rejected", func(t *testing.T) {
		bothOwnerBody := strings.ReplaceAll(runDemoE2E, demoExternalServerEnv+"=1 ", "")
		requireDemoAppServerOwnerCountRejected(t, runDemoRunnerHarness(t, repoRoot, bothOwnerBody, true), 2)
	})

	t.Run("no app server ownership is rejected", func(t *testing.T) {
		noOwnerBody := strings.ReplaceAll(runDemoE2E, demoExternalServerEnv+"=1 ", "")
		noOwnerBody = strings.Replace(noOwnerBody, "npx playwright test", demoExternalServerEnv+"=1 npx playwright test", 1)
		noOwnerBody = strings.ReplaceAll(noOwnerBody, `exec "$AYB_BIN" demo "$name"`, "exec true")
		requireDemoAppServerOwnerCountRejected(t, runDemoRunnerHarness(t, repoRoot, noOwnerBody, false), 0)
	})
}

func TestMoviesMagicLinkUsesCanonicalAuthContract(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	moviesConfig := readRepoText(t, filepath.Join(repoRoot, "examples", "movies", "playwright.config.ts"))
	requireContainsAll(t, moviesConfig, []string{
		"webServer:",
		"AYB_AUTH_MAGIC_LINK_ENABLED=true",
		"AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true",
	})

	helper := readRepoText(t, filepath.Join(repoRoot, "examples", "movies", "e2e", "helpers.ts"))
	magicLinkCapture := requireDelimitedText(t, helper,
		"export async function recordNextMagicLinkRequest",
		"export async function failNextLogout",
	)
	requireContainsAll(t, magicLinkCapture, []string{
		"const response = await route.fetch();",
		"await route.fulfill({ response });",
		"responseStatus: response.status()",
	})
	requireDoesNotContainAny(t, magicLinkCapture, []string{
		"status: 200",
		"body: JSON.stringify",
	})

	spec := readRepoText(t, filepath.Join(repoRoot, "examples", "movies", "e2e", "movies_states.spec.ts"))
	requireContainsAll(t, spec, []string{
		`await page.getByLabel("Email").fill("magic-state@example.test");`,
		`await page.getByRole("button", { name: "Email me a magic link" }).click();`,
		`method: "POST",`,
		`path: "/api/auth/magic-link",`,
		`jsonBody: { email: "magic-state@example.test" },`,
		`responseStatus: 200,`,
		`"We sent a magic link to magic-state@example.test. Check your inbox.",`,
	})

	runDemoE2E := shellFunctionBody(t,
		readRepoText(t, filepath.Join(repoRoot, "_dev", "manual_smoke_tests", "18_demo_e2e.test.sh")),
		"run_demo_e2e",
	)
	commands := executableShellCommandBlocks(runDemoE2E)
	launchIndex := shellCommandBlockIndexContaining(commands, `"$AYB_BIN" demo "$name"`)
	if launchIndex < 0 {
		t.Fatal("run_demo_e2e must launch the Movies server with $AYB_BIN demo $name")
	}
	if !runDemoE2EScopesMagicLinkToMoviesLaunch(commands) {
		t.Fatalf("run_demo_e2e Movies server launch must set AYB_AUTH_MAGIC_LINK_ENABLED=true only for the movies branch when the signed-out Movies Playwright state expects POST /api/auth/magic-link to return 200; launch command got %q",
			commands[launchIndex])
	}
}

func TestDemoRuntimeCIWiringRejectsFalsePositiveFixtures(t *testing.T) {
	t.Parallel()

	t.Run("external server comment does not disable webServer", func(t *testing.T) {
		config := `// process.env.AYB_DEMO_EXTERNAL_SERVER
export default defineConfig({
  webServer: { command: "npm run dev", reuseExistingServer: false },
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("commented external-server env reference must not satisfy webServer ownership")
		}
	})

	t.Run("external server dead read does not disable webServer", func(t *testing.T) {
		config := `const external = process.env.AYB_DEMO_EXTERNAL_SERVER;
export default defineConfig({
  webServer: { command: "npm run dev", reuseExistingServer: false },
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("dead external-server env read must not satisfy webServer ownership")
		}
	})

	t.Run("conditional undefined disables Playwright webServer", func(t *testing.T) {
		config := `export default defineConfig({
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : {
    command: "npm run dev",
    reuseExistingServer: false,
  },
});`
		if !playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("external-server env must be accepted when it structurally selects undefined webServer")
		}
	})

	t.Run("reversed external server ternary is rejected", func(t *testing.T) {
		config := `export default defineConfig({
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? makeWebServer() : undefined,
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("external-server env must select undefined in the truthy branch")
		}
	})

	t.Run("unscoped magic link launch is rejected", func(t *testing.T) {
		commands := []string{`AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`}
		if runDemoE2EScopesMagicLinkToMoviesLaunch(commands) {
			t.Fatal("common demo launch must not satisfy Movies-only magic-link ownership")
		}
	})

	t.Run("detached movies condition does not scope magic link launch", func(t *testing.T) {
		commands := []string{`if [ "$name" = "movies" ]; then echo "movies"; fi
AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`}
		if runDemoE2EScopesMagicLinkToMoviesLaunch(commands) {
			t.Fatal("Movies comparison in the same command block must not satisfy an unconditional magic-link launch")
		}
	})

	t.Run("movies scoped magic link launch is accepted", func(t *testing.T) {
		commands := []string{`if [ "$name" = "movies" ]; then AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"; fi`}
		if !runDemoE2EScopesMagicLinkToMoviesLaunch(commands) {
			t.Fatal("Movies branch launch should satisfy Movies-only magic-link ownership")
		}
	})
}

func TestPlaywrightConfigExternalServerGuardFixtures(t *testing.T) {
	t.Parallel()

	t.Run("local external server boolean", func(t *testing.T) {
		config := `const externalServer = process.env.AYB_DEMO_EXTERNAL_SERVER === "1";
export default defineConfig({
  webServer: externalServer ? undefined : {
    command: "npm run dev",
    reuseExistingServer: false,
  },
});`
		if !playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("a local boolean derived from the external-server env must be accepted when it selects undefined webServer")
		}
	})

	t.Run("multiline external server ternary", func(t *testing.T) {
		config := `export default defineConfig({
  webServer:
    process.env.AYB_DEMO_EXTERNAL_SERVER === "1"
      ? undefined
      : {
          command: "npm run dev",
          reuseExistingServer: false,
        },
});`
		if !playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("a multiline external-server ternary must be accepted when its truthy branch selects undefined webServer")
		}
	})
}

func TestMoviesMagicLinkScopeRejectsAlternateArms(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name, command string
	}{
		{"else", `if [ "$name" = "movies" ]; then :; else AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"; fi`},
		{"elif", `if [ "$name" = "movies" ]; then :; elif [ "$name" = "kanban" ]; then AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"; fi`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if runDemoE2EScopesMagicLinkToMoviesLaunch([]string{fixture.command}) {
				t.Fatalf("a magic-link launch in the Movies condition's %s arm must not satisfy Movies-only ownership", fixture.name)
			}
		})
	}
}

func TestShellBlockRunsCommandWithEnvRequiresLeadingAssignment(t *testing.T) {
	t.Parallel()

	t.Run("leading assignment prefix is accepted", func(t *testing.T) {
		block := `AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`
		if !shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("a leading env assignment before the matched command must be accepted")
		}
	})

	t.Run("stacked leading assignments are accepted", func(t *testing.T) {
		block := `HOME="$runtime_home" AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`
		if !shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("an env assignment preceded only by other assignments must be accepted")
		}
	})

	t.Run("magic-link assignment in argument position is rejected", func(t *testing.T) {
		block := `run_launcher AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`
		if shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("an env assignment after a bare command word is an argument, not an env prefix, and must be rejected")
		}
	})

	t.Run("external-server assignment in argument position is rejected", func(t *testing.T) {
		block := `some_wrapper AYB_DEMO_EXTERNAL_SERVER=1 npx playwright test`
		if shellBlockRunsCommandWithEnv(block, []string{"npx", "playwright", "test"}, "AYB_DEMO_EXTERNAL_SERVER", "1") {
			t.Fatal("an argument-position external-server assignment must not satisfy the runtime contract")
		}
	})

	t.Run("quoted magic-link assignment word is rejected", func(t *testing.T) {
		block := `"AYB_AUTH_MAGIC_LINK_ENABLED=true" exec "$AYB_BIN" demo "$name"`
		if shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("a fully quoted assignment word is a command name in Bash, not an env prefix, and must be rejected")
		}
	})

	t.Run("quoted external-server assignment word is rejected", func(t *testing.T) {
		block := `"AYB_DEMO_EXTERNAL_SERVER=1" npx playwright test`
		if shellBlockRunsCommandWithEnv(block, []string{"npx", "playwright", "test"}, "AYB_DEMO_EXTERNAL_SERVER", "1") {
			t.Fatal("a fully quoted external-server assignment word must not satisfy the runtime contract")
		}
	})

	t.Run("quoted preceding pseudo-assignment demotes the real assignment", func(t *testing.T) {
		block := `"HOME=/tmp" AYB_AUTH_MAGIC_LINK_ENABLED=true exec "$AYB_BIN" demo "$name"`
		if shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("a quoted word before the assignment is a command name, so the real assignment is argument-position and must be rejected")
		}
	})

	t.Run("quoted value on a leading assignment is still accepted", func(t *testing.T) {
		block := `HOME="$runtime_home" AYB_AUTH_MAGIC_LINK_ENABLED="true" exec "$AYB_BIN" demo "$name"`
		if !shellBlockRunsCommandWithEnv(block, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Fatal("an unquoted NAME= with a quoted value is a valid shell assignment and must be accepted")
		}
	})
}

func runDemoE2EScopesMagicLinkToMoviesLaunch(commands []string) bool {
	for _, command := range commands {
		if !strings.Contains(command, `"$AYB_BIN" demo "$name"`) ||
			!strings.Contains(command, "AYB_AUTH_MAGIC_LINK_ENABLED=true") {
			continue
		}
		if shellIfMoviesBranchRunsMagicLinkLaunch(command) {
			return true
		}
	}
	return false
}

func shellIfMoviesBranchRunsMagicLinkLaunch(command string) bool {
	for _, condition := range []string{
		`if [ "$name" = "movies" ]; then`,
		`if [ "$name" == "movies" ]; then`,
		`if [[ "$name" = "movies" ]]; then`,
		`if [[ "$name" == "movies" ]]; then`,
	} {
		conditionIndex := strings.Index(command, condition)
		if conditionIndex < 0 {
			continue
		}
		branchStart := conditionIndex + len(condition)
		branch, found := shellTopLevelThenArm(command[branchStart:])
		if !found {
			continue
		}
		if shellBlockRunsCommandWithEnv(branch, []string{"exec", "$AYB_BIN", "demo", "$name"}, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			return true
		}
	}
	return false
}

func shellTopLevelThenArm(content string) (string, bool) {
	segmentStart := 0
	nestedIfDepth := 0
	for index := 0; index <= len(content); index++ {
		if index < len(content) && !isShellCommandListDelimiter(rune(content[index])) {
			continue
		}
		segment := strings.TrimSpace(content[segmentStart:index])
		fields := strings.Fields(segment)
		if len(fields) > 0 {
			switch fields[0] {
			case "if":
				nestedIfDepth++
			case "else", "elif":
				if nestedIfDepth == 0 {
					return content[:segmentStart], true
				}
			case "fi":
				if nestedIfDepth == 0 {
					return content[:segmentStart], true
				}
				nestedIfDepth--
			}
		}
		segmentStart = index + 1
	}
	return "", false
}

func shellBlockRunsCommandWithEnv(block string, args []string, name, value string) bool {
	for _, segment := range shellCommandSegments(block) {
		fields := strings.Fields(segment)
		for index, field := range fields {
			if !shellFieldAssignsEnv(field, name, value) {
				continue
			}
			// A NAME=value token only exports into the following command's
			// environment when it is a *leading* assignment: every field before it
			// must also be a shell assignment. Once a bare command word appears,
			// any later NAME=value token is a plain argument, not an env prefix.
			if !allShellLeadingAssignments(fields[:index]) {
				continue
			}
			if shellSegmentStartsWithFields(strings.Join(fields[index+1:], " "), args) {
				return true
			}
		}
	}
	return false
}

// shellEnvAssignmentPattern matches a raw shell word whose NAME and `=` are
// unquoted. It is deliberately applied to the un-trimmed field: an identifier
// character can never be a quote, so a word that begins with `"` or `'`
// (e.g. `"NAME=value"`) fails the pattern. That mirrors Bash, which only
// recognizes a word as an environment assignment when the NAME= prefix is
// literal — a fully quoted word is parsed as a command name instead.
var shellEnvAssignmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// shellFieldIsLeadingAssignment is the canonical classifier for whether a raw
// shell word is recognized as an environment assignment prefix. Both the
// matched assignment and every field preceding it are validated through here so
// quoting semantics are enforced in exactly one place.
func shellFieldIsLeadingAssignment(field string) bool {
	return shellEnvAssignmentPattern.MatchString(field)
}

// shellFieldAssignsEnv reports whether a raw shell word is a leading assignment
// whose literal, quote-removed value exactly matches value.
func shellFieldAssignsEnv(field, name, value string) bool {
	if !shellFieldIsLeadingAssignment(field) {
		return false
	}
	fieldName, fieldValue, found := strings.Cut(field, "=")
	if !found || fieldName != name {
		return false
	}
	evaluatedValue, valid := shellLiteralWordValue(fieldValue)
	return valid && evaluatedValue == value
}

func allShellLeadingAssignments(fields []string) bool {
	for _, field := range fields {
		if !shellFieldIsLeadingAssignment(field) {
			return false
		}
	}
	return true
}

func shellCommandBlockIndexContaining(commands []string, substring string) int {
	for index, command := range commands {
		if strings.Contains(command, substring) {
			return index
		}
	}
	return -1
}

func requireDelimitedText(t *testing.T, content, start, end string) string {
	t.Helper()
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		t.Fatalf("expected content to include %q", start)
	}
	body := content[startIndex:]
	endIndex := strings.Index(body, end)
	if endIndex < 0 {
		t.Fatalf("expected content after %q to include %q", start, end)
	}
	return body[:endIndex]
}
