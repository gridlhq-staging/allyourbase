package codehealth

import (
	"os"
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
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOpenAIContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealBYOKProviders"}
`))
	if err != nil {
		t.Fatalf("expected script success, got error: %v output=%s", err, output)
	}
	if !strings.Contains(output, "required contract tests passed: 7") {
		t.Fatalf("expected pass count output, got: %s", output)
	}
	for _, testName := range []string{
		"TestOllamaContractGenerateText",
		"TestOllamaContractGenerateTextStream",
		"TestOllamaContractGenerateEmbedding",
		"TestAnthropicContractGenerateText",
		"TestAnthropicContractGenerateTextStream",
		"TestOpenAIContractGenerateText",
		"TestMoviesChatContractStreamWithRealBYOKProviders",
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
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOpenAIContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealBYOKProviders"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "missing TestAnthropicContractGenerateTextStream") {
		t.Fatalf("expected missing-test failure, got: %s", output)
	}
}

func TestCheckAIContractGotestsumScriptFailsWhenRequiredTestSkips(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"skip","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOpenAIContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealBYOKProviders"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "skipped TestOllamaContractGenerateTextStream") {
		t.Fatalf("expected skipped-test failure, got: %s", output)
	}
}

func TestCheckAIContractGotestsumScriptFailsWhenFinalStatusIsNotPass(t *testing.T) {
	t.Parallel()

	output, err := runAIContractGotestsumGuard(t, gotestsumReportJSONLines(`
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOllamaContractGenerateEmbedding"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateText"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestAnthropicContractGenerateTextStream"}
{"Action":"pass","Package":"github.com/allyourbase/ayb/internal/ai","Test":"TestOpenAIContractGenerateText"}
{"Action":"fail","Package":"github.com/allyourbase/ayb/internal/server","Test":"TestMoviesChatContractStreamWithRealBYOKProviders"}
`))
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(output, "final status fail for TestMoviesChatContractStreamWithRealBYOKProviders") {
		t.Fatalf("expected final-status failure, got: %s", output)
	}
}

func TestOpenAIContractRequiresExplicitModelEnv(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	sourcePath := filepath.Join(repoRoot, "internal", "ai", "openai_contract_test.go")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read OpenAI contract test: %v", err)
	}
	source := string(sourceBytes)
	if !strings.Contains(source, `requireContractEnv(t, "AYB_AICONTRACT_OPENAI_MODEL")`) {
		t.Fatal("OpenAI contract test must require AYB_AICONTRACT_OPENAI_MODEL explicitly")
	}
	if strings.Contains(source, "ResolveOpenAIContractModel") {
		t.Fatal("OpenAI provider contract test must not bypass the explicit model env gate")
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
