package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const checkFileSizesScript = "scripts/check-file-sizes.sh"

func TestCheckFileSizesScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		files             map[string]int
		goAllowlist       string
		tsAllowlist       string
		wantExitCode      int
		wantExactOutput   string
		wantOutput        []string
		wantExcludedPaths []string
	}{
		{
			name: "go oversized missing allowlist fails",
			files: map[string]int{
				"internal/oversized.go": 501,
			},
			wantExitCode: 1,
			wantExactOutput: strings.Join([]string{
				"Go source-size guardrail failed (limit 500 lines).",
				"Oversized files not covered by allowlist or with stale counts:",
				"internal/oversized.go:501 (missing from allowlist)",
				"",
			}, "\n"),
		},
		{
			name: "go allowlist equal passes",
			files: map[string]int{
				"internal/oversized.go": 501,
			},
			goAllowlist:     "internal/oversized.go:501\n",
			wantExactOutput: cleanScriptOutput(),
		},
		{
			name: "go allowlist shrink passes",
			files: map[string]int{
				"internal/oversized.go": 501,
			},
			goAllowlist:     "internal/oversized.go:520\n",
			wantExactOutput: cleanScriptOutput(),
		},
		{
			name: "go allowlist growth fails",
			files: map[string]int{
				"internal/oversized.go": 521,
			},
			goAllowlist:  "internal/oversized.go:520\n",
			wantExitCode: 1,
			wantExactOutput: strings.Join([]string{
				"Go source-size guardrail failed (limit 500 lines).",
				"Oversized files not covered by allowlist or with stale counts:",
				"internal/oversized.go:521 (allowlist has 520)",
				"",
			}, "\n"),
		},
		{
			name:            "go malformed allowlist fails",
			goAllowlist:     "internal/oversized.go\n",
			wantExitCode:    1,
			wantExactOutput: "Invalid allowlist entry in ${GO_ALLOWLIST}: internal/oversized.go\n",
		},
		{
			name: "typescript below and at limits pass",
			files: map[string]int{
				"sdk/src/below.ts":            799,
				"sdk/src/at.ts":               800,
				"ui/src/components/below.tsx": 599,
				"ui/src/components/at.tsx":    600,
			},
			wantExactOutput: cleanScriptOutput(),
		},
		{
			name: "typescript oversized fails",
			files: map[string]int{
				"sdk/src/oversized.ts":            801,
				"ui/src/components/oversized.tsx": 601,
			},
			wantExitCode: 1,
			wantExactOutput: strings.Join([]string{
				"Go source-size guardrail passed.",
				"TypeScript source-size guardrail failed (limits: .ts 800 lines, .tsx 600 lines).",
				"Oversized TypeScript files not covered by allowlist or with stale counts:",
				"sdk/src/oversized.ts:801 (missing from allowlist)",
				"ui/src/components/oversized.tsx:601 (missing from allowlist)",
				"",
			}, "\n"),
		},
		{
			name: "typescript allowlist equal and shrink pass",
			files: map[string]int{
				"sdk/src/equal.ts":             801,
				"ui/src/components/shrink.tsx": 601,
			},
			tsAllowlist: strings.Join([]string{
				"sdk/src/equal.ts:801",
				"ui/src/components/shrink.tsx:620",
				"",
			}, "\n"),
			wantExactOutput: cleanScriptOutput(),
		},
		{
			name: "typescript allowlist growth fails",
			files: map[string]int{
				"sdk/src/growth.ts": 822,
			},
			tsAllowlist:  "sdk/src/growth.ts:821\n",
			wantExitCode: 1,
			wantExactOutput: strings.Join([]string{
				"Go source-size guardrail passed.",
				"TypeScript source-size guardrail failed (limits: .ts 800 lines, .tsx 600 lines).",
				"Oversized TypeScript files not covered by allowlist or with stale counts:",
				"sdk/src/growth.ts:822 (allowlist has 821)",
				"",
			}, "\n"),
		},
		{
			name:            "typescript malformed allowlist fails",
			tsAllowlist:     "./sdk/src/oversized.ts:801\n",
			wantExitCode:    1,
			wantExactOutput: "Invalid allowlist path (must not start with ./): ./sdk/src/oversized.ts:801\n",
		},
		{
			name: "typescript stale and missing entries fail configuration",
			files: map[string]int{
				"sdk/src/present.ts": 100,
			},
			tsAllowlist: strings.Join([]string{
				"sdk/src/present.ts:801",
				"sdk/src/missing.ts:801",
				"",
			}, "\n"),
			wantExitCode: 1,
			wantOutput: []string{
				"Stale allowlist entry in ${TS_ALLOWLIST}: sdk/src/present.ts:801 (file has 100 lines)",
				"Stale allowlist entry in ${TS_ALLOWLIST}: sdk/src/missing.ts:801 (file missing)",
			},
		},
		{
			name: "typescript scan prunes and excludes non production files",
			files: map[string]int{
				"node_modules/pkg/oversized.ts":  801,
				"dist/oversized.ts":              801,
				"ui/src/__tests__/oversized.tsx": 601,
				"sdk/src/types.d.ts":             801,
				"sdk/src/oversized.test.ts":      801,
				"ui/src/oversized.test.tsx":      601,
				"sdk/src/oversized.spec.ts":      801,
				"ui/src/oversized.spec.tsx":      601,
				"internal/oversized_test.go":     501,
				"internal/not_a_go_test.test.go": 501,
			},
			wantExitCode: 1,
			wantExactOutput: strings.Join([]string{
				"Go source-size guardrail failed (limit 500 lines).",
				"Oversized files not covered by allowlist or with stale counts:",
				"internal/not_a_go_test.test.go:501 (missing from allowlist)",
				"",
			}, "\n"),
			wantExcludedPaths: []string{
				"node_modules/pkg/oversized.ts",
				"dist/oversized.ts",
				"ui/src/__tests__/oversized.tsx",
				"sdk/src/types.d.ts",
				"sdk/src/oversized.test.ts",
				"ui/src/oversized.test.tsx",
				"sdk/src/oversized.spec.ts",
				"ui/src/oversized.spec.tsx",
				"internal/oversized_test.go",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := runCheckFileSizesFixture(t, tt.files, tt.goAllowlist, tt.tsAllowlist)
			if result.exitCode != tt.wantExitCode {
				t.Fatalf("exit code = %d, want %d, output:\n%s", result.exitCode, tt.wantExitCode, result.output)
			}

			wantExact := expandAllowlistPaths(tt.wantExactOutput, result.goAllowlistPath, result.tsAllowlistPath)
			if wantExact != "" && result.output != wantExact {
				t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", wantExact, result.output)
			}
			for _, want := range tt.wantOutput {
				want = expandAllowlistPaths(want, result.goAllowlistPath, result.tsAllowlistPath)
				if !strings.Contains(result.output, want) {
					t.Fatalf("expected output to contain %q, got:\n%s", want, result.output)
				}
			}
			for _, excluded := range tt.wantExcludedPaths {
				if strings.Contains(result.output, excluded) {
					t.Fatalf("expected %s to be excluded, got:\n%s", excluded, result.output)
				}
			}
		})
	}
}

type checkFileSizesResult struct {
	output          string
	exitCode        int
	goAllowlistPath string
	tsAllowlistPath string
}

func runCheckFileSizesFixture(t *testing.T, files map[string]int, goAllowlist, tsAllowlist string) checkFileSizesResult {
	t.Helper()

	repoRoot := findRepoRoot(t)
	tempRoot := t.TempDir()
	goAllowlistPath := filepath.Join(tempRoot, "allowlist-oversized.txt")
	tsAllowlistPath := filepath.Join(tempRoot, "allowlist-oversized-typescript.txt")

	for path, lineCount := range files {
		writeLinesFile(t, filepath.Join(tempRoot, filepath.FromSlash(path)), lineCount)
	}
	writeTextFile(t, goAllowlistPath, goAllowlist)
	writeTextFile(t, tsAllowlistPath, tsAllowlist)

	cmd := exec.Command("bash", checkFileSizesScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CHECK_FILE_SIZES_ROOT="+tempRoot,
		"CHECK_FILE_SIZES_ALLOWLIST="+goAllowlistPath,
		"CHECK_FILE_SIZES_TS_ALLOWLIST="+tsAllowlistPath,
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

	return checkFileSizesResult{
		output:          string(output),
		exitCode:        exitCode,
		goAllowlistPath: goAllowlistPath,
		tsAllowlistPath: tsAllowlistPath,
	}
}

func cleanScriptOutput() string {
	return "Go source-size guardrail passed.\nTypeScript source-size guardrail passed.\n"
}

func expandAllowlistPaths(text, goAllowlistPath, tsAllowlistPath string) string {
	text = strings.ReplaceAll(text, "${GO_ALLOWLIST}", goAllowlistPath)
	text = strings.ReplaceAll(text, "${TS_ALLOWLIST}", tsAllowlistPath)
	return text
}

func writeLinesFile(t *testing.T, path string, lineCount int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	lines := make([]string, lineCount)
	for i := range lines {
		lines[i] = "x"
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		t.Fatalf("repo root not found from %s: %v", workingDirectory, err)
	}
	return repoRoot
}
