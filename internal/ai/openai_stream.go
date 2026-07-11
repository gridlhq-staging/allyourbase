package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GenerateTextStream starts an OpenAI chat completions request and returns decoded text deltas.
func (p *OpenAIProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	body := buildOpenAIChatRequest(req, true)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := streamHTTPClientWithoutTimeout(p.client).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if readErr != nil {
			return nil, fmt.Errorf("openai: read stream response: %w", readErr)
		}
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "openai",
		}
	}

	return newLineStreamReader(resp.Body, parseOpenAIStreamLine), nil
}

// parseOpenAIStreamLine extracts text deltas from OpenAI chat completion SSE data lines.
func parseOpenAIStreamLine(line []byte) (streamLineResult, error) {
	text := strings.TrimSpace(string(line))
	if text == "" || strings.HasPrefix(text, ":") {
		return streamLineResult{}, nil
	}

	payload, ok := strings.CutPrefix(text, "data:")
	if !ok {
		return streamLineResult{}, nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "[DONE]" {
		return streamLineResult{Done: true}, nil
	}

	var data openAIStreamData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return streamLineResult{}, fmt.Errorf("openai: unmarshal stream data: %w", err)
	}
	if data.Error != nil {
		return streamLineResult{}, fmt.Errorf("openai: stream error: %s: %s", data.Error.Type, data.Error.Message)
	}
	if len(data.Choices) == 0 || data.Choices[0].Delta.Content == "" {
		return streamLineResult{}, nil
	}
	return streamLineResult{Delta: data.Choices[0].Delta.Content, Emit: true}, nil
}

type openAIStreamData struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
