package codehealth

import (
	"regexp"
	"strings"
	"testing"
)

func playwrightConfigDisablesWebServerForExternalServer(config, envName string) bool {
	config = stripTypeScriptComments(config)
	defineConfigIndex := strings.Index(config, "export default defineConfig(")
	if defineConfigIndex < 0 {
		return false
	}
	webServerOffset := strings.Index(config[defineConfigIndex:], "webServer:")
	if webServerOffset < 0 {
		return false
	}
	webServerIndex := defineConfigIndex + webServerOffset
	expression := config[webServerIndex+len("webServer:"):]
	condition, branches, found := strings.Cut(expression, "?")
	if !found {
		return false
	}
	truthyBranch, standaloneBranch, found := strings.Cut(branches, ":")
	if !found || strings.TrimSpace(truthyBranch) != "undefined" {
		return false
	}
	standaloneBranch = strings.TrimSpace(standaloneBranch)
	if !strings.HasPrefix(standaloneBranch, "{") || !strings.Contains(standaloneBranch, "command:") {
		return false
	}
	return typeScriptConditionUsesPositiveEnv(
		config[:webServerIndex],
		strings.TrimSpace(condition),
		"process.env."+envName,
	)
}

func typeScriptConditionUsesPositiveEnv(declarations, condition, envRef string) bool {
	if typeScriptExpressionIsPositiveEnv(condition, envRef) {
		return true
	}
	if !regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`).MatchString(condition) {
		return false
	}
	declaration := regexp.MustCompile(`(?m)\bconst[ \t]+` + regexp.QuoteMeta(condition) + `[ \t]*=[ \t]*([^;\r\n]+)`).FindStringSubmatch(declarations)
	return len(declaration) == 2 && typeScriptExpressionIsPositiveEnv(declaration[1], envRef)
}

func typeScriptExpressionIsPositiveEnv(expression, envRef string) bool {
	compact := strings.Join(strings.Fields(expression), "")
	switch compact {
	case envRef,
		envRef + `==="1"`,
		envRef + `==='1'`,
		envRef + `=="1"`,
		envRef + `=='1'`:
		return true
	default:
		return false
	}
}

// shellLiteralWordValue performs shell quote removal for a literal assignment
// value. Dynamic expansions are rejected because their runtime value cannot be
// proven by this static wiring contract.
func shellLiteralWordValue(word string) (string, bool) {
	var value strings.Builder
	var quote byte
	for index := 0; index < len(word); index++ {
		char := word[index]
		switch quote {
		case '\'':
			if char == '\'' {
				quote = 0
			} else {
				value.WriteByte(char)
			}
		case '"':
			switch char {
			case '"':
				quote = 0
			case '$', '`':
				return "", false
			case '\\':
				if index+1 >= len(word) {
					return "", false
				}
				next := word[index+1]
				if strings.ContainsRune("$`\"\\", rune(next)) {
					index++
					value.WriteByte(next)
				} else {
					value.WriteByte(char)
				}
			default:
				value.WriteByte(char)
			}
		default:
			switch char {
			case '\'', '"':
				quote = char
			case '\\':
				if index+1 >= len(word) {
					return "", false
				}
				index++
				value.WriteByte(word[index])
			case '$', '`', '~', '{', '}':
				return "", false
			default:
				value.WriteByte(char)
			}
		}
	}
	return value.String(), quote == 0
}

func stripTypeScriptComments(content string) string {
	var stripped strings.Builder
	var quote byte
	lineComment := false
	blockComment := false
	escaped := false
	for index := 0; index < len(content); index++ {
		char := content[index]
		next := byte(0)
		if index+1 < len(content) {
			next = content[index+1]
		}
		if lineComment {
			if char == '\n' {
				lineComment = false
				stripped.WriteByte(char)
			}
			continue
		}
		if blockComment {
			if char == '*' && next == '/' {
				blockComment = false
				index++
			} else if char == '\n' {
				stripped.WriteByte(char)
			}
			continue
		}
		if quote != 0 {
			stripped.WriteByte(char)
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		switch {
		case char == '/' && next == '/':
			lineComment = true
			index++
		case char == '/' && next == '*':
			blockComment = true
			index++
		case char == '\'', char == '"', char == '`':
			quote = char
			stripped.WriteByte(char)
		default:
			stripped.WriteByte(char)
		}
	}
	return stripped.String()
}

func TestPlaywrightConfigOwnershipGuardRejectsFalsePositiveFixtures(t *testing.T) {
	t.Parallel()

	t.Run("standalone mode has no server owner", func(t *testing.T) {
		config := `export default defineConfig({
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : undefined,
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("the external-server guard must retain a Playwright webServer in standalone mode")
		}
	})

	t.Run("block comment is not config", func(t *testing.T) {
		config := `/*
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : {
    command: "npm run decoy",
  },
*/
export default defineConfig({
  webServer: { command: "npm run dev", reuseExistingServer: false },
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("block-commented external-server guard must not satisfy webServer ownership")
		}
	})

	t.Run("dead string is not config", func(t *testing.T) {
		config := `const decoy = "webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : { command: 'npm run decoy' }";
export default defineConfig({
  webServer: { command: "npm run dev", reuseExistingServer: false },
});`
		if playwrightConfigDisablesWebServerForExternalServer(config, demoExternalServerEnv) {
			t.Fatal("an unused string before defineConfig must not satisfy webServer ownership")
		}
	})
}

func TestShellBlockRunsCommandWithEnvEvaluatesAssignmentValue(t *testing.T) {
	t.Parallel()

	args := []string{"exec", "$AYB_BIN", "demo", "$name"}
	for _, fixture := range []struct {
		name  string
		block string
	}{
		{"quoted literal suffix", `AYB_AUTH_MAGIC_LINK_ENABLED=true'"' exec "$AYB_BIN" demo "$name"`},
		{"quoted literal wrapping", `AYB_AUTH_MAGIC_LINK_ENABLED="'true'" exec "$AYB_BIN" demo "$name"`},
		{"unbalanced single quote", `AYB_AUTH_MAGIC_LINK_ENABLED='true exec "$AYB_BIN" demo "$name"`},
		{"unbalanced double quote", `AYB_AUTH_MAGIC_LINK_ENABLED="true exec "$AYB_BIN" demo "$name"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if shellBlockRunsCommandWithEnv(fixture.block, args, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
				t.Fatalf("assignment value in %q does not shell-evaluate to exact true and must be rejected", fixture.block)
			}
		})
	}

	for _, block := range []string{
		`AYB_AUTH_MAGIC_LINK_ENABLED="true" exec "$AYB_BIN" demo "$name"`,
		`AYB_AUTH_MAGIC_LINK_ENABLED='tr'"ue" exec "$AYB_BIN" demo "$name"`,
	} {
		if !shellBlockRunsCommandWithEnv(block, args, "AYB_AUTH_MAGIC_LINK_ENABLED", "true") {
			t.Errorf("assignment value in %q shell-evaluates to exact true and must be accepted", block)
		}
	}
}
