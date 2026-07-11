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
)

func TestOpenAIGenerateTextStreamRequestAndDeltas(t *testing.T) {
	requestSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSeen = true
		assertOpenAIStreamHeaders(t, r)

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		assertOpenAIStreamRequest(t, got)

		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hel"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"lo"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
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
	for _, forbidden := range []string{"data:", `"delta"`, `"role"`, `"finish_reason"`, "\n"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stream text %q contains raw framing %q", got, forbidden)
		}
	}
}

func TestOpenAIGenerateTextStreamFramingAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, `: keepalive`)
		fmt.Fprintln(w, `event: completion.chunk`)
		fmt.Fprintln(w, `retry: 1000`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"role":"assistant"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"kept"},"finish_reason":null}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: [DONE]`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"leaked"}}]}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
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

func TestOpenAIGenerateTextStreamSplitsDeltasFromSingleResponseBlob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"first"}}]}`,
			"",
			`data: {"choices":[{"delta":{"content":"second"}}]}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n"))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
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

func TestOpenAIGenerateTextStreamMalformedSSEDataReadErrorAfterPendingBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"ok"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"choices":`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
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
	if !strings.Contains(err.Error(), "openai: unmarshal stream data") {
		t.Fatalf("second Read error = %v; want unmarshal stream data error", err)
	}
}

func TestOpenAIGenerateTextStreamInBandErrorReadErrorAfterPendingBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"ok"}}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"error":{"type":"rate_limit_error","message":"too many requests"}}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
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
		t.Fatalf("second Read returned n=%d, nil error; want stream error", n)
	}
	if !strings.Contains(err.Error(), "openai: stream error: rate_limit_error: too many requests") {
		t.Fatalf("second Read error = %v; want in-band stream error", err)
	}
}

func TestOpenAIGenerateTextStreamHTTPError(t *testing.T) {
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

			p := NewOpenAIProvider("test-key", srv.URL)
			stream, err := p.GenerateTextStream(context.Background(), openAIStreamTestRequest())
			if stream != nil {
				stream.Close()
				t.Fatal("stream returned for non-200 response")
			}

			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %T %v; want *ProviderError", err, err)
			}
			if providerErr.Provider != "openai" {
				t.Errorf("Provider = %q; want openai", providerErr.Provider)
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

func assertOpenAIStreamHeaders(t *testing.T, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q; want Bearer test-key", got)
	}
}

func assertOpenAIStreamRequest(t *testing.T, got map[string]any) {
	t.Helper()

	if got["model"] != "gpt-test" {
		t.Errorf("model = %q; want gpt-test", got["model"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v; want true", got["stream"])
	}
	if got["max_tokens"] != float64(7) {
		t.Fatalf("max_tokens = %v; want 7", got["max_tokens"])
	}
	if got["temperature"] != float64(0) {
		t.Fatalf("temperature = %v; want 0", got["temperature"])
	}

	messages, ok := got["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v; want two messages", got["messages"])
	}
	assertOpenAIMessageObject(t, messages[0], "system", "system prompt")
	assertOpenAIMultimodalUserMessage(t, messages[1])
}

func assertOpenAIMessageObject(t *testing.T, got any, role, content string) {
	t.Helper()

	message, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("message = %#v; want object", got)
	}
	if message["role"] != role {
		t.Errorf("message.role = %q; want %q", message["role"], role)
	}
	if message["content"] != content {
		t.Errorf("message.content = %q; want %q", message["content"], content)
	}
}

func assertOpenAIMultimodalUserMessage(t *testing.T, got any) {
	t.Helper()

	message, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("message = %#v; want object", got)
	}
	if message["role"] != "user" {
		t.Errorf("message.role = %q; want user", message["role"])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("message.content = %#v; want three parts", message["content"])
	}
	assertOpenAITextPart(t, content[0], "Hello ")
	assertOpenAIImagePart(t, content[1], "https://example.test/image.png")
	assertOpenAITextPart(t, content[2], "from OpenAI")
}

func assertOpenAITextPart(t *testing.T, got any, text string) {
	t.Helper()

	part, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("content part = %#v; want object", got)
	}
	if part["type"] != "text" {
		t.Errorf("part.type = %q; want text", part["type"])
	}
	if part["text"] != text {
		t.Errorf("part.text = %q; want %q", part["text"], text)
	}
}

func assertOpenAIImagePart(t *testing.T, got any, url string) {
	t.Helper()

	part, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("content part = %#v; want object", got)
	}
	if part["type"] != "image_url" {
		t.Errorf("part.type = %q; want image_url", part["type"])
	}
	imageURL, ok := part["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("part.image_url = %#v; want object", part["image_url"])
	}
	if imageURL["url"] != url {
		t.Errorf("part.image_url.url = %q; want %q", imageURL["url"], url)
	}
}

func openAIStreamTestRequest() GenerateTextRequest {
	temperature := 0.0
	return GenerateTextRequest{
		Model:        "gpt-test",
		SystemPrompt: "system prompt",
		Messages: []Message{{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: "Hello "},
				{Type: "image_url", ImageURL: "https://example.test/image.png"},
				{Type: "text", Text: "from OpenAI"},
			},
		}},
		MaxTokens:   7,
		Temperature: &temperature,
	}
}
