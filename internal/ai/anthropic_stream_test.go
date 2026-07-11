package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicGenerateTextStreamRequestAndDeltas(t *testing.T) {
	requestSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSeen = true
		assertAnthropicStreamHeaders(t, r)

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		assertAnthropicStreamRequest(t, got, 1024, nil)

		fmt.Fprintln(w, `event: message_start`)
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_1"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hel"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_stop`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(0, nil))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	got := readStreamWithSmallBuffer(t, stream)
	if !requestSeen {
		t.Fatal("server did not receive request")
	}
	if got != "hello" {
		t.Fatalf("stream text = %q; want hello", got)
	}
	for _, forbidden := range []string{"event:", "data:", `"delta"`, "\n"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stream text %q contains raw framing %q", got, forbidden)
		}
	}
}

func TestAnthropicGenerateTextStreamSplitsDeltasFromSingleResponseBlob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Join([]string{
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"first"}}`,
			"",
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"second"}}`,
			"",
			`data: {"type":"message_stop"}`,
			"",
		}, "\n"))
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(0, nil))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	buf := make([]byte, 64)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("first Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "first" {
		t.Fatalf("first Read = %q; want first", buf[:n])
	}

	n, err = stream.Read(buf)
	if err != nil {
		t.Fatalf("second Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "second" {
		t.Fatalf("second Read = %q; want second", buf[:n])
	}

	n, err = stream.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("third Read = %d, %v; want 0, EOF", n, err)
	}
}

func TestAnthropicGenerateTextStreamCallerOptions(t *testing.T) {
	temperature := 0.25
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAnthropicStreamHeaders(t, r)

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		assertAnthropicStreamRequest(t, got, 7, &temperature)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(7, &temperature))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("stream text = %q; want empty", got)
	}
}

func TestAnthropicGenerateTextStreamIgnoresNonTextEventsAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `event: ping`)
		fmt.Fprintln(w, `data: {"type":"ping"}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{}"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"kept"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"leaked"}}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(0, nil))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "kept" {
		t.Fatalf("stream text = %q; want kept", got)
	}
}

func TestAnthropicGenerateTextStreamMalformedSSEDataReadErrorAfterPendingBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(0, nil))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	buf := make([]byte, 2)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("first Read error = %v", err)
	}
	if string(buf[:n]) != "ok" {
		t.Fatalf("first Read = %q; want ok", string(buf[:n]))
	}
	n, err = stream.Read(buf)
	if err == nil {
		t.Fatalf("second Read returned n=%d, nil error; want parser error", n)
	}
	if !strings.Contains(err.Error(), "anthropic: unmarshal stream data") {
		t.Fatalf("second Read error = %v; want unmarshal stream data error", err)
	}
}

func TestAnthropicGenerateTextStreamHTTPError(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		retryable bool
	}{
		{name: "bad request", status: http.StatusBadRequest, body: "bad request body", retryable: false},
		{name: "rate limit", status: http.StatusTooManyRequests, body: "rate limited", retryable: true},
		{name: "server error", status: http.StatusInternalServerError, body: strings.Repeat("x", 250), retryable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			p := NewAnthropicProvider("test-key", srv.URL)
			stream, err := p.GenerateTextStream(context.Background(), anthropicStreamTestRequest(0, nil))
			if stream != nil {
				stream.Close()
				t.Fatal("stream returned for non-200 response")
			}

			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %T %v; want *ProviderError", err, err)
			}
			if providerErr.Provider != "anthropic" {
				t.Errorf("Provider = %q; want anthropic", providerErr.Provider)
			}
			if providerErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d; want %d", providerErr.StatusCode, tt.status)
			}
			if providerErr.IsRetryable() != tt.retryable {
				t.Errorf("IsRetryable = %v; want %v", providerErr.IsRetryable(), tt.retryable)
			}
			wantMessage := fmt.Sprintf("HTTP %d: %s", tt.status, truncate(tt.body, 200))
			if providerErr.Message != wantMessage {
				t.Errorf("Message = %q; want %q", providerErr.Message, wantMessage)
			}
		})
	}
}

func TestAnthropicGenerateTextStreamIgnoresProviderClientTimeoutWhileReading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "response writer does not support flush", http.StatusInternalServerError)
			return
		}

		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"slow "}}`)
		fmt.Fprintln(w)
		flusher.Flush()

		time.Sleep(60 * time.Millisecond)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"stream"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", srv.URL)
	p.client.Timeout = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, err := p.GenerateTextStream(ctx, anthropicStreamTestRequest(0, nil))
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	got := readStreamWithSmallBuffer(t, stream)
	if got != "slow stream" {
		t.Fatalf("stream text = %q; want slow stream", got)
	}
}

func assertAnthropicStreamHeaders(t *testing.T, r *http.Request) {
	t.Helper()

	if r.Method != "POST" || r.URL.Path != "/messages" {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}
	if got := r.Header.Get("x-api-key"); got != "test-key" {
		t.Errorf("x-api-key = %q; want test-key", got)
	}
	if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
		t.Errorf("anthropic-version = %q; want %q", got, anthropicVersion)
	}
}

func assertAnthropicStreamRequest(t *testing.T, got map[string]any, maxTokens int, temperature *float64) {
	t.Helper()

	if got["model"] != "claude-test" {
		t.Errorf("model = %q; want claude-test", got["model"])
	}
	if got["system"] != "system prompt" {
		t.Errorf("system = %q; want system prompt", got["system"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v; want true", got["stream"])
	}
	if got["max_tokens"] != float64(maxTokens) {
		t.Errorf("max_tokens = %v; want %d", got["max_tokens"], maxTokens)
	}
	if temperature == nil {
		if _, ok := got["temperature"]; ok {
			t.Errorf("temperature = %v; want omitted", got["temperature"])
		}
	} else if got["temperature"] != *temperature {
		t.Errorf("temperature = %v; want %v", got["temperature"], *temperature)
	}

	messages, ok := got["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v; want one user message", got["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("messages[0] = %#v; want object", messages[0])
	}
	if message["role"] != "user" {
		t.Errorf("messages[0].role = %q; want user", message["role"])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("messages[0].content = %#v; want three blocks", message["content"])
	}
	assertAnthropicContentBlock(t, content[0], "text", "Hello ")
	assertAnthropicImageBlock(t, content[1], "https://example.test/image.png")
	assertAnthropicContentBlock(t, content[2], "text", "from Anthropic")
}

func assertAnthropicContentBlock(t *testing.T, got any, contentType, text string) {
	t.Helper()

	block, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("content block = %#v; want object", got)
	}
	if block["type"] != contentType {
		t.Errorf("content block type = %q; want %q", block["type"], contentType)
	}
	if block["text"] != text {
		t.Errorf("content block text = %q; want %q", block["text"], text)
	}
}

func assertAnthropicImageBlock(t *testing.T, got any, url string) {
	t.Helper()

	block, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("content block = %#v; want object", got)
	}
	if block["type"] != "image" {
		t.Errorf("content block type = %q; want image", block["type"])
	}
	source, ok := block["source"].(map[string]any)
	if !ok {
		t.Fatalf("content block source = %#v; want object", block["source"])
	}
	if source["type"] != "url" {
		t.Errorf("content block source type = %q; want url", source["type"])
	}
	if source["url"] != url {
		t.Errorf("content block source url = %q; want %q", source["url"], url)
	}
}

func anthropicStreamTestRequest(maxTokens int, temperature *float64) GenerateTextRequest {
	return GenerateTextRequest{
		Model:        "claude-test",
		SystemPrompt: "system prompt",
		Messages: []Message{{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: "Hello "},
				{Type: "image_url", ImageURL: "https://example.test/image.png"},
				{Type: "text", Text: "from Anthropic"},
			},
		}},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
}
