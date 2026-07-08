//go:build aicontract

package ai

import (
	"testing"

	"github.com/allyourbase/ayb/internal/config"
)

func TestAnthropicContractGenerateText(t *testing.T) {
	apiKey := requireContractEnv(t, "ANTHROPIC_API_KEY")
	model := requireContractEnv(t, "AYB_AICONTRACT_ANTHROPIC_MODEL")
	const sentinel = "AYB_ANTHROPIC_CONTRACT_OK"

	cfg := config.AIConfig{
		DefaultProvider: "anthropic",
		DefaultModel:    model,
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				APIKey:       apiKey,
				DefaultModel: model,
			},
		},
	}

	provider, resolvedModel := resolveContractProvider(t, "anthropic", cfg)
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
