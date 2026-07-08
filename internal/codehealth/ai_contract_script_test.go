package codehealth

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const checkAIContractGotestsumScript = "scripts/check-aicontract-gotestsum.sh"

func TestCheckAIContractGotestsumScriptPassesWhenRequiredTestsPass(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"run","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealAnthropicBYOK"}
`))
	if err != nil {
		t.Fatalf("expected script success, got error: %v output=%s", err, output)
	}
	if !strings.Contains(output, "required contract tests passed: 4") {
		t.Fatalf("expected pass count output, got: %s", output)
	}
	for _, testName := range []string{
		"TestOllamaContractGenerateText",
		"TestOllamaContractGenerateEmbedding",
		"TestAnthropicContractGenerateText",
		"TestMoviesChatContractStreamWithRealAnthropicBYOK",
	} {
		if !strings.Contains(output, "AI contract required test passed: "+testName) {
			t.Fatalf("expected passing-test proof for %s, got: %s", testName, output)
		}
	}
}

func TestCheckAIContractGotestsumScriptFailsWhenRequiredTestIsMissing(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "missing TestMoviesChatContractStreamWithRealAnthropicBYOK") {
		t.Fatalf("expected missing-test failure, got: %s", output)
	}
}

func TestCheckAIContractGotestsumScriptFailsWhenRequiredTestSkips(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"skip","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealAnthropicBYOK"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "skipped TestAnthropicContractGenerateText") {
		t.Fatalf("expected skipped-test failure, got: %s", output)
	}
}

func TestCheckAIContractGotestsumScriptFailsWhenFinalStatusIsNotPass(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"fail","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealAnthropicBYOK"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "final status fail for TestMoviesChatContractStreamWithRealAnthropicBYOK") {
		t.Fatalf("expected final-status failure, got: %s", output)
	}
}

func runAIContractGotestsumGuard(t *testing.T, report string) (string, error) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	reportPath := filepath.Join(t.TempDir(), "aicontract.json")
	writeTextFile(t, reportPath, report)

	cmd := exec.Command("bash", checkAIContractGotestsumScript, reportPath)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func gotestsumReportJSONLines(content string) string {
	return strings.TrimSpace(content) + "\n"
}
