package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const checkUIBundleSizeScript = "scripts/check_ui_bundle_size.sh"

func TestCheckUIBundleSizeScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		indexHTML       string
		assets          map[string]string
		budget          string
		wantExitCode    int
		wantExactOutput string
	}{
		{
			name:      "entry below budget passes",
			indexHTML: moduleScriptHTML("/admin/assets/entry.js"),
			assets: map[string]string{
				"assets/entry.js": "123456789",
			},
			budget:          "10",
			wantExactOutput: passBundleOutput("${DIST}/assets/entry.js", 9, 10),
		},
		{
			name:      "entry at budget passes",
			indexHTML: moduleScriptHTML("/admin/assets/entry.js"),
			assets: map[string]string{
				"assets/entry.js": "1234567890",
			},
			budget:          "10",
			wantExactOutput: passBundleOutput("${DIST}/assets/entry.js", 10, 10),
		},
		{
			name:            "no module entry fails",
			indexHTML:       `<!doctype html><script src="/admin/assets/entry.js"></script>`,
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: "Expected exactly one module script in ${DIST}/index.html, found 0.\n",
		},
		{
			name: "multiple module entries fail",
			indexHTML: strings.Join([]string{
				moduleScriptHTML("/admin/assets/entry.js"),
				moduleScriptHTML("/admin/assets/other.js"),
			}, "\n"),
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: "Expected exactly one module script in ${DIST}/index.html, found 2.\n",
		},
		{
			name: "inline module alongside entry fails",
			indexHTML: strings.Join([]string{
				moduleScriptHTML("/admin/assets/entry.js"),
				inlineModuleScriptHTML(`console.log("inline")`),
			}, "\n"),
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: "Expected exactly one module script in ${DIST}/index.html, found 2.\n",
		},
		{
			name:            "single inline module without src fails",
			indexHTML:       inlineModuleScriptHTML(`console.log("inline")`),
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: "Expected the sole module script in ${DIST}/index.html to have a src attribute.\n",
		},
		{
			name:            "missing entry asset fails",
			indexHTML:       moduleScriptHTML("/admin/assets/missing.js"),
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: "Module entry /admin/assets/missing.js did not resolve to a regular file under ${DIST}.\n",
		},
		{
			name:            "non numeric budget fails",
			indexHTML:       moduleScriptHTML("/admin/assets/entry.js"),
			budget:          "10kb",
			wantExitCode:    1,
			wantExactOutput: "UI bundle size budget must be digits only, got 10kb.\n",
		},
		{
			name:      "entry over budget fails",
			indexHTML: moduleScriptHTML("/admin/assets/entry.js"),
			assets: map[string]string{
				"assets/entry.js": "12345678901",
			},
			budget:          "10",
			wantExitCode:    1,
			wantExactOutput: failBundleOutput("${DIST}/assets/entry.js", 11, 10),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := runCheckUIBundleSizeFixture(t, tt.indexHTML, tt.assets, tt.budget)
			if result.exitCode != tt.wantExitCode {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, tt.wantExitCode, result.output)
			}

			want := strings.ReplaceAll(tt.wantExactOutput, "${DIST}", result.distPath)
			if result.output != want {
				t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", want, result.output)
			}
		})
	}
}

type checkUIBundleSizeResult struct {
	output   string
	exitCode int
	distPath string
}

func runCheckUIBundleSizeFixture(t *testing.T, indexHTML string, assets map[string]string, budget string) checkUIBundleSizeResult {
	t.Helper()

	repoRoot := findRepoRoot(t)
	distPath := filepath.Join(t.TempDir(), "dist")
	writeTextFile(t, filepath.Join(distPath, "index.html"), indexHTML)
	for path, content := range assets {
		writeTextFile(t, filepath.Join(distPath, filepath.FromSlash(path)), content)
	}
	resolvedDistPath, err := filepath.EvalSymlinks(distPath)
	if err != nil {
		t.Fatalf("resolve dist path failed: %v", err)
	}

	cmd := exec.Command("bash", checkUIBundleSizeScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CHECK_UI_BUNDLE_DIST="+resolvedDistPath,
		"CHECK_UI_BUNDLE_BUDGET="+budget,
	)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("script execution failed: %v output=%s", err, output)
		}
		exitCode = exitError.ExitCode()
	}

	return checkUIBundleSizeResult{
		output:   string(output),
		exitCode: exitCode,
		distPath: resolvedDistPath,
	}
}

func moduleScriptHTML(src string) string {
	return `<!doctype html><script type="module" crossorigin src="` + src + `"></script>`
}

func inlineModuleScriptHTML(body string) string {
	return `<!doctype html><script type="module">` + body + `</script>`
}

func passBundleOutput(entry string, actual, budget int) string {
	return strings.Join([]string{
		"UI bundle size guardrail passed.",
		"Entry: " + entry,
		"Actual bytes: " + intString(actual),
		"Budget bytes: " + intString(budget),
		"",
	}, "\n")
}

func failBundleOutput(entry string, actual, budget int) string {
	return strings.Join([]string{
		"UI bundle size guardrail failed.",
		"Entry: " + entry,
		"Actual bytes: " + intString(actual),
		"Budget bytes: " + intString(budget),
		"",
	}, "\n")
}

func intString(value int) string {
	return strconv.Itoa(value)
}
