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

	provider, resolvedModel := resolveContractProvider(t, "anthropic", anthropicContractConfig(apiKey, model))
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

func TestAnthropicContractGenerateTextStream(t *testing.T) {
	apiKey := requireContractEnv(t, "ANTHROPIC_API_KEY")
	model := requireContractEnv(t, "AYB_AICONTRACT_ANTHROPIC_MODEL")
	provider, resolvedModel := resolveContractProvider(t, "anthropic", anthropicContractConfig(apiKey, model))
	streamingProvider, ok := provider.(StreamingProvider)
	if !ok {
		t.Fatalf("resolved provider %T does not implement StreamingProvider", provider)
	}

	const sentinel = "AYB_ANTHROPIC_STREAM_OK"
	temperature := 0.0
	stream, err := streamingProvider.GenerateTextStream(contractContext(t), GenerateTextRequest{
		Model: resolvedModel,
		Messages: []Message{
			TextMessage("user", "Reply with exactly "+sentinel+" and no other text."),
		},
		MaxTokens:   32,
		Temperature: &temperature,
	})
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	result := readContractStream(t, stream)
	assertExactContractText(t, result.Text, sentinel)
}

func anthropicContractConfig(apiKey, model string) config.AIConfig {
	return config.AIConfig{
		DefaultProvider: "anthropic",
		DefaultModel:    model,
		Providers: map[string]config.ProviderConfig{
			"anthropic": {
				APIKey:       apiKey,
				DefaultModel: model,
			},
		},
	}
}
