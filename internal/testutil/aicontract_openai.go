//go:build aicontract

package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const openAIContractModelsURL = "https://api.openai.com/v1/models"

var openAIContractPreferredModelPrefixes = []string{
	"gpt-4o-mini",
	"gpt-4.1-mini",
	"gpt-4o",
	"gpt-4.1",
	"gpt-3.5-turbo",
}

// ResolveOpenAIContractModel returns the explicit override model when set, or
// probes the live OpenAI models API and selects a currently available chat
// model suitable for the contract lane.
func ResolveOpenAIContractModel(t testing.TB, apiKey string) string {
	t.Helper()

	if override := strings.TrimSpace(os.Getenv("AYB_AICONTRACT_OPENAI_MODEL")); override != "" {
		return override
	}
	if strings.TrimSpace(apiKey) == "" {
		t.Fatal("apiKey must be non-empty when resolving the OpenAI contract model")
	}

	models := fetchOpenAIContractModels(t, apiKey)
	model, ok := pickOpenAIContractModel(models)
	if !ok {
		t.Fatalf("no chat-capable OpenAI contract model found in live models response: %v", models)
	}
	return model
}

func fetchOpenAIContractModels(t testing.TB, apiKey string) []string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, openAIContractModelsURL, nil)
	if err != nil {
		t.Fatalf("create OpenAI models request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request OpenAI models: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("read OpenAI models response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("OpenAI models probe returned HTTP %d: %s", resp.StatusCode, truncateForContract(body, 400))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode OpenAI models response: %v", err)
	}

	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" {
			models = append(models, id)
		}
	}
	return models
}

func pickOpenAIContractModel(models []string) (string, bool) {
	for _, prefix := range openAIContractPreferredModelPrefixes {
		for _, model := range models {
			if (model == prefix || strings.HasPrefix(model, prefix+"-")) && isOpenAIContractChatModel(model) {
				return model, true
			}
		}
	}

	for _, model := range models {
		if isOpenAIContractChatModel(model) {
			return model, true
		}
	}
	return "", false
}

func isOpenAIContractChatModel(model string) bool {
	if !hasOpenAIContractChatPrefix(model) {
		return false
	}
	for _, forbidden := range []string{
		"audio",
		"embedding",
		"image",
		"instruct",
		"moderation",
		"realtime",
		"search",
		"transcribe",
		"tts",
		"whisper",
	} {
		if strings.Contains(model, forbidden) {
			return false
		}
	}
	return true
}

func hasOpenAIContractChatPrefix(model string) bool {
	return strings.HasPrefix(model, "gpt") ||
		strings.HasPrefix(model, "chatgpt") ||
		hasOpenAIReasoningModelPrefix(model)
}

func hasOpenAIReasoningModelPrefix(model string) bool {
	if len(model) < 2 || model[0] != 'o' {
		return false
	}
	return model[1] >= '0' && model[1] <= '9'
}

func truncateForContract(body []byte, max int) string {
	text := strings.TrimSpace(string(body))
	if len(text) <= max {
		return text
	}
	return fmt.Sprintf("%s...", text[:max])
}
