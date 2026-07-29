package allyourbase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRPCRequestContract(t *testing.T) {
	requestFixture := mustLoadContractFixture(t, "rpc_request.json")
	var args map[string]int
	if err := json.Unmarshal(requestFixture, &args); err != nil {
		t.Fatalf("decode rpc request fixture: %v", err)
	}

	const functionName = "schema/add?mode#v1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		expectedPath := "/api/rpc/" + url.PathEscape(functionName)
		if r.URL.EscapedPath() != expectedPath {
			t.Errorf("escaped path = %q, want %q", r.URL.EscapedPath(), expectedPath)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("raw query = %q, want empty", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer rpc-token" {
			t.Errorf("authorization = %q, want %q", got, "Bearer rpc-token")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q, want %q", got, "application/json")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		} else if !bytes.Equal(body, requestFixture) {
			t.Errorf("request body = %q, want fixture %q", body, requestFixture)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, WithAPIKey("rpc-token"))
	if _, err := client.RPC(context.Background(), functionName, args); err != nil {
		t.Fatalf("RPC: %v", err)
	}
}

func TestRPCResponseContract(t *testing.T) {
	responseFixture := mustLoadContractFixture(t, "rpc_response.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(responseFixture)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	got, err := client.RPC(context.Background(), "sdk_contract_add", map[string]int{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("RPC: %v", err)
	}
	if !bytes.Equal(got, responseFixture) {
		t.Fatalf("response = %q, want fixture %q", got, responseFixture)
	}
}

func TestRPCEmptyResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "no content", status: http.StatusNoContent},
		{name: "empty success", status: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			got, err := NewClient(server.URL).RPC(context.Background(), "empty", nil)
			if err != nil {
				t.Fatalf("RPC: %v", err)
			}
			if got != nil {
				t.Fatalf("response = %q, want nil", got)
			}
		})
	}
}

func TestRPCNormalizesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"invalid rpc arguments","code":"rpc/invalid-arguments"}`))
	}))
	defer server.Close()

	got, err := NewClient(server.URL).RPC(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != nil {
		t.Fatalf("response = %q, want nil", got)
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", apiErr.Status, http.StatusUnprocessableEntity)
	}
	if apiErr.Message != "invalid rpc arguments" {
		t.Errorf("message = %q, want %q", apiErr.Message, "invalid rpc arguments")
	}
	if apiErr.Code != "rpc/invalid-arguments" {
		t.Errorf("code = %q, want %q", apiErr.Code, "rpc/invalid-arguments")
	}
}
