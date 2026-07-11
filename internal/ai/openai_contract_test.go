//go:build aicontract

package ai

import (
	"testing"

	"github.com/allyourbase/ayb/internal/config"
)

func TestOpenAIContractGenerateText(t *testing.T) {
	apiKey := requireContractEnv(t, "OPENAI_API_KEY")
	model := requireContractEnv(t, "AYB_AICONTRACT_OPENAI_MODEL")
	const sentinel = "AYB_OPENAI_CONTRACT_OK"

	cfg := config.AIConfig{
		DefaultProvider: "openai",
		DefaultModel:    model,
		Providers: map[string]config.ProviderConfig{
			"openai": {
				APIKey:       apiKey,
				DefaultModel: model,
			},
		},
	}

	provider, resolvedModel := resolveContractProvider(t, "openai", cfg)
	temperature := 0.0
	resp, err := provider.GenerateText(contractContext(t), GenerateTextRequest{
		Model: resolvedModel,
		Messages: []Message{
			TextMessage("user", "Reply with exactly "+sentinel+" and no other text."),
		},
		MaxTokens:   32,
		Temperature: &temperature,
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}

	assertExactContractText(t, resp.Text, sentinel)
	if resp.Model == "" {
		t.Fatal("Model is empty")
	}
	if resp.FinishReason == "" {
		t.Fatal("FinishReason is empty")
	}
	assertTextUsage(t, resp.Usage)
}
