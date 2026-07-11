package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GenerateTextStream starts an Ollama chat request and returns decoded text deltas.
func (p *OllamaProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	body := buildOllamaChatRequest(req, true)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := streamHTTPClientWithoutTimeout(p.client).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if readErr != nil {
			return nil, fmt.Errorf("ollama: read stream response: %w", readErr)
		}
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "ollama",
		}
	}

	return newLineStreamReader(resp.Body, parseOllamaStreamLine), nil
}

func parseOllamaStreamLine(line []byte) (streamLineResult, error) {
	var resp ollamaResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return streamLineResult{}, fmt.Errorf("ollama: unmarshal stream line: %w", err)
	}
	return streamLineResult{Delta: resp.Message.Content, Emit: resp.Message.Content != ""}, nil
}
