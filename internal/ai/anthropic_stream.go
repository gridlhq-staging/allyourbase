package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GenerateTextStream starts an Anthropic Messages request and returns decoded text deltas.
func (p *AnthropicProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	payload, err := json.Marshal(buildAnthropicRequest(req, true))
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal stream request: %w", err)
	}

	httpReq, err := p.newMessagesHTTPRequest(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("anthropic: create stream request: %w", err)
	}

	resp, err := streamHTTPClientWithoutTimeout(p.client).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if readErr != nil {
			return nil, fmt.Errorf("anthropic: read stream response: %w", readErr)
		}
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)),
			Provider:   "anthropic",
		}
	}

	return newLineStreamReader(resp.Body, parseAnthropicStreamLine), nil
}

// parseAnthropicStreamLine decodes text deltas, completion, and errors from an Anthropic SSE line.
func parseAnthropicStreamLine(line []byte) (streamLineResult, error) {
	text := strings.TrimSpace(string(line))
	if text == "" || strings.HasPrefix(text, ":") || strings.HasPrefix(text, "event:") {
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

	var data anthropicStreamData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return streamLineResult{}, fmt.Errorf("anthropic: unmarshal stream data: %w", err)
	}

	switch data.Type {
	case "content_block_delta":
		if data.Delta.Type == "text_delta" && data.Delta.Text != "" {
			return streamLineResult{Delta: data.Delta.Text, Emit: true}, nil
		}
	case "message_stop":
		return streamLineResult{Done: true}, nil
	case "error":
		if data.Error != nil {
			return streamLineResult{}, fmt.Errorf("anthropic: stream error: %s: %s", data.Error.Type, data.Error.Message)
		}
		return streamLineResult{}, fmt.Errorf("anthropic: stream error")
	}
	return streamLineResult{}, nil
}

type anthropicStreamData struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
