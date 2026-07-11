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

func TestOllamaGenerateTextStreamRequestAndDeltas(t *testing.T) {
	requestSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSeen = true
		if r.Method != "POST" || r.URL.Path != "/api/chat" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q; want application/json", got)
		}

		var got ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		assertOllamaStreamRequest(t, got)

		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"hel"},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"lo"},"done":true}`)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), ollamaStreamTestRequest())
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
	for _, forbidden := range []string{`"message"`, `"role"`, `"done"`, "\n"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stream text %q contains raw framing %q", got, forbidden)
		}
	}
}

func TestOllamaGenerateTextStreamHTTPError(t *testing.T) {
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

			p := NewOllamaProvider(srv.URL)
			stream, err := p.GenerateTextStream(context.Background(), ollamaStreamTestRequest())
			if stream != nil {
				stream.Close()
				t.Fatal("stream returned for non-200 response")
			}

			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %T %v; want *ProviderError", err, err)
			}
			if providerErr.Provider != "ollama" {
				t.Errorf("Provider = %q; want ollama", providerErr.Provider)
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

func TestOllamaGenerateTextStreamMalformedNDJSONReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ok"},"done":false}`)
		fmt.Fprintln(w, `{"message":`)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	stream, err := p.GenerateTextStream(context.Background(), ollamaStreamTestRequest())
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
	if !strings.Contains(err.Error(), "ollama: unmarshal stream line") {
		t.Fatalf("second Read error = %v; want unmarshal stream line error", err)
	}
}

func TestOllamaGenerateTextStreamIgnoresProviderClientTimeoutWhileReading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "response writer does not support flush", http.StatusInternalServerError)
			return
		}

		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"slow "},"done":false}`)
		flusher.Flush()

		time.Sleep(60 * time.Millisecond)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"stream"},"done":true}`)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL)
	p.client.Timeout = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, err := p.GenerateTextStream(ctx, ollamaStreamTestRequest())
	if err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}
	defer stream.Close()

	got := readStreamWithSmallBuffer(t, stream)
	if got != "slow stream" {
		t.Fatalf("stream text = %q; want slow stream", got)
	}
}

func assertOllamaStreamRequest(t *testing.T, got ollamaRequest) {
	t.Helper()

	if got.Model != "llama3" {
		t.Errorf("model = %q; want llama3", got.Model)
	}
	if !got.Stream {
		t.Error("stream = false; want true")
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages count = %d; want 2", len(got.Messages))
	}
	wantMessages := []ollamaMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "Hello from text blocks"},
	}
	for i, want := range wantMessages {
		if got.Messages[i] != want {
			t.Errorf("messages[%d] = %+v; want %+v", i, got.Messages[i], want)
		}
	}
	if got.Options == nil {
		t.Fatal("options is nil")
	}
	if got.Options.NumPredict == nil || *got.Options.NumPredict != 7 {
		t.Fatalf("options.num_predict = %v; want 7", got.Options.NumPredict)
	}
	if got.Options.Temperature == nil || *got.Options.Temperature != 0 {
		t.Fatalf("options.temperature = %v; want 0", got.Options.Temperature)
	}
}

func ollamaStreamTestRequest() GenerateTextRequest {
	temperature := 0.0
	return GenerateTextRequest{
		Model:        "llama3",
		SystemPrompt: "system prompt",
		Messages: []Message{{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: "Hello "},
				{Type: "image_url", ImageURL: "https://example.test/image.png"},
				{Type: "text", Text: "from text blocks"},
			},
		}},
		MaxTokens:   7,
		Temperature: &temperature,
	}
}

func readStreamWithSmallBuffer(t *testing.T, stream io.Reader) string {
	t.Helper()

	var out strings.Builder
	buf := make([]byte, 2)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			return out.String()
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
}
