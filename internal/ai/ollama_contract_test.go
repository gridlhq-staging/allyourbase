//go:build aicontract

package ai

import (
	"testing"

	"github.com/allyourbase/ayb/internal/config"
)

func TestOllamaContractGenerateText(t *testing.T) {
	cfg := ollamaContractConfig()
	provider, model := resolveContractProvider(t, "ollama", cfg)
	const sentinel = "AYB_OLLAMA_CONTRACT_OK"

	temperature := 0.0
	resp, err := provider.GenerateText(contractContext(t), GenerateTextRequest{
		Model: model,
		Messages: []Message{
			TextMessage("user", "Reply with exactly this uppercase token and no other text: "+sentinel),
		},
		MaxTokens:   16,
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

func TestOllamaContractGenerateEmbedding(t *testing.T) {
	cfg := ollamaContractConfig()
	provider, _ := resolveContractProvider(t, "ollama", cfg)
	embeddingProvider, ok := provider.(EmbeddingProvider)
	if !ok {
		t.Fatalf("resolved provider %T does not implement EmbeddingProvider", provider)
	}

	model := cfg.EmbeddingModel
	wantDimension, ok := cfg.EmbeddingDimension("ollama", model)
	if !ok {
		t.Fatalf("missing configured embedding dimension for ollama:%s", model)
	}

	resp, err := embeddingProvider.GenerateEmbedding(contractContext(t), EmbeddingRequest{
		Model: model,
		Input: []string{"allyourbase contract embedding"},
	})
	if err != nil {
		t.Fatalf("GenerateEmbedding: %v", err)
	}

	if len(resp.Embeddings) != 1 {
		t.Fatalf("embeddings count = %d; want 1", len(resp.Embeddings))
	}
	if len(resp.Embeddings[0]) != wantDimension {
		t.Fatalf("embedding dimension = %d; want %d", len(resp.Embeddings[0]), wantDimension)
	}
	if resp.Model == "" {
		t.Fatal("Model is empty")
	}
}

func ollamaContractConfig() config.AIConfig {
	return config.AIConfig{
		DefaultProvider:   "ollama",
		DefaultModel:      "llama3.2:1b",
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "nomic-embed-text",
		EmbeddingDimensions: map[string]int{
			"ollama:nomic-embed-text": 768,
		},
		Providers: map[string]config.ProviderConfig{
			"ollama": {
				DefaultModel: "llama3.2:1b",
			},
		},
	}
}
