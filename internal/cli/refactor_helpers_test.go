package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildQueryRequestBuildsURLAndAuthHeader(t *testing.T) {
	req, err := buildQueryRequest(queryRequestConfig{
		table:   "posts",
		token:   "tok",
		baseURL: "http://127.0.0.1:8090",
		filter:  "status='active'",
		sort:    "-created_at",
		fields:  "id,title",
		expand:  "author",
		page:    2,
		limit:   50,
	})
	if err != nil {
		t.Fatalf("buildQueryRequest returned error: %v", err)
	}
	if req.Method != "GET" {
		t.Fatalf("expected GET method, got %q", req.Method)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("expected bearer auth header, got %q", got)
	}

	values := req.URL.Query()
	expect := map[string]string{
		"filter":  "status='active'",
		"sort":    "-created_at",
		"fields":  "id,title",
		"expand":  "author",
		"page":    "2",
		"perPage": "50",
	}
	for key, want := range expect {
		if got := values.Get(key); got != want {
			t.Fatalf("expected %s=%q, got %q", key, want, got)
		}
	}
}

func TestBuildQueryRequestEscapesTablePathSegment(t *testing.T) {
	req, err := buildQueryRequest(queryRequestConfig{
		table:   "posts/../../admin?x=1",
		baseURL: "http://127.0.0.1:8090",
		page:    1,
		limit:   20,
	})
	if err != nil {
		t.Fatalf("buildQueryRequest returned error: %v", err)
	}
	if got, want := req.URL.EscapedPath(), "/api/collections/posts%2F..%2F..%2Fadmin%3Fx=1"; got != want {
		t.Fatalf("expected escaped collection path %q, got %q", want, got)
	}
	if req.URL.Query().Get("x") != "" {
		t.Fatalf("table name injected query parameter into %q", req.URL.String())
	}
}

func TestStorageCommandsEscapePathSegments(t *testing.T) {
	bucket := "bucket/../../admin?x=1"
	name := "object/../../secrets?y=1"
	expectedRequests := []string{
		"GET /api/storage/bucket%2F..%2F..%2Fadmin%3Fx=1",
		"DELETE /api/storage/bucket%2F..%2F..%2Fadmin%3Fx=1/object%2F..%2F..%2Fsecrets%3Fy=1",
	}
	var gotRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequests = append(gotRequests, r.Method+" "+r.RequestURI)
		if r.URL.Query().Get("x") != "" || r.URL.Query().Get("y") != "" {
			t.Fatalf("storage names injected query parameters into %q", r.RequestURI)
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"items":[],"totalItems":0}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.Flags().String("admin-token", "tok", "")
	cmd.Flags().String("url", server.URL, "")

	captureStdout(t, func() {
		if err := runStorageLs(cmd, []string{bucket}); err != nil {
			t.Fatalf("runStorageLs returned error: %v", err)
		}
		if err := runStorageDelete(cmd, []string{bucket, name}); err != nil {
			t.Fatalf("runStorageDelete returned error: %v", err)
		}
	})
	if len(gotRequests) != len(expectedRequests) {
		t.Fatalf("expected %d requests, got %d: %v", len(expectedRequests), len(gotRequests), gotRequests)
	}
	for i, want := range expectedRequests {
		if gotRequests[i] != want {
			t.Fatalf("request %d: expected %q, got %q", i, want, gotRequests[i])
		}
	}
}

func TestRenderQueryResultsCSV(t *testing.T) {
	payload := []byte(`{"items":[{"id":1,"title":"one","note":null}],"page":1,"perPage":20,"totalItems":1,"totalPages":1}`)

	output := captureStdout(t, func() {
		if err := renderQueryResults(payload, "csv", "id,title,note"); err != nil {
			t.Fatalf("renderQueryResults returned error: %v", err)
		}
	})
	if !strings.Contains(output, "id,title,note") {
		t.Fatalf("expected CSV header, got %q", output)
	}
	if !strings.Contains(output, "1,one,") {
		t.Fatalf("expected CSV row, got %q", output)
	}
}

func TestParseInvokeHeadersValidation(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("header", nil, "")

	headers, err := parseInvokeHeaders(cmd, []string{"bad"})
	if err != nil {
		t.Fatalf("expected implicit invalid headers to be ignored, got error: %v", err)
	}
	if len(headers) != 0 {
		t.Fatalf("expected no headers for implicit invalid header, got %v", headers)
	}

	if err := cmd.Flags().Set("header", "bad"); err != nil {
		t.Fatalf("set header flag: %v", err)
	}
	if _, err := parseInvokeHeaders(cmd, []string{"bad"}); err == nil {
		t.Fatal("expected explicit invalid header to return an error")
	}
}

func TestRenderInvokeResultTextOutput(t *testing.T) {
	body := []byte(`{"statusCode":201,"headers":{"X-B":["2"],"X-A":["1"]},"body":"created"}`)

	output := captureStdout(t, func() {
		if err := renderInvokeResult(body, "text"); err != nil {
			t.Fatalf("renderInvokeResult returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Status: 201") {
		t.Fatalf("expected status line, got %q", output)
	}
	if !strings.Contains(output, "X-A: 1") || !strings.Contains(output, "X-B: 2") {
		t.Fatalf("expected headers, got %q", output)
	}
	if !strings.Contains(output, "Body:\ncreated") {
		t.Fatalf("expected response body, got %q", output)
	}
}

func TestUninstallPreflightNotInstalledJSON(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	aybDir := filepath.Join(homeDir, ".ayb")

	var proceed bool
	output := captureStdout(t, func() {
		var err error
		proceed, err = uninstallPreflight(&cobra.Command{}, aybDir, true, false, false)
		if err != nil {
			t.Fatalf("uninstallPreflight returned error: %v", err)
		}
	})
	if proceed {
		t.Fatal("expected preflight to stop when ~/.ayb is missing")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q: %v", output, err)
	}
	if parsed["status"] != "not_installed" {
		t.Fatalf("expected status not_installed, got %v", parsed["status"])
	}
}

func TestStreamRoutingUninstallDeclineUsesStderr(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	resetJSONFlag()
	aybDir := filepath.Join(homeDir, ".ayb")
	if err := os.MkdirAll(aybDir, 0o755); err != nil {
		t.Fatalf("create AYB directory: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	uninstallCmd.SetIn(strings.NewReader("n\n"))
	uninstallCmd.SetOut(&stdout)
	uninstallCmd.SetErr(&stderr)
	t.Cleanup(func() {
		uninstallCmd.SetIn(nil)
		uninstallCmd.SetOut(nil)
		uninstallCmd.SetErr(nil)
		if err := uninstallCmd.Flags().Set("purge", "false"); err != nil {
			t.Errorf("reset purge flag: %v", err)
		}
		if err := uninstallCmd.Flags().Set("yes", "false"); err != nil {
			t.Errorf("reset yes flag: %v", err)
		}
	})
	if err := uninstallCmd.Flags().Set("purge", "true"); err != nil {
		t.Fatalf("set purge flag: %v", err)
	}
	if err := uninstallCmd.Flags().Set("yes", "false"); err != nil {
		t.Fatalf("clear yes flag: %v", err)
	}

	if err := runUninstall(uninstallCmd, nil); err != nil {
		t.Fatalf("declining uninstall returned an error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout when declining purge, got %q", stdout.String())
	}
	for _, expected := range []string{"delete your embedded database", "Continue? [y/N]", "Aborted."} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("expected stderr to contain %q, got %q", expected, stderr.String())
		}
	}
	if _, err := os.Stat(aybDir); err != nil {
		t.Fatalf("declined purge should preserve %s: %v", aybDir, err)
	}
}

func TestExecuteRemovalsAndRenderUninstallResult(t *testing.T) {
	homeDir := t.TempDir()
	aybDir := filepath.Join(homeDir, ".ayb")
	binPath := filepath.Join(aybDir, "bin", "ayb")
	if err := os.MkdirAll(filepath.Join(aybDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(aybDir, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	removed, dataPreserved := executeRemovals(aybDir, binPath, false)
	if !dataPreserved {
		t.Fatal("expected data to be preserved for non-purge uninstall")
	}
	if len(removed) == 0 {
		t.Fatal("expected at least one removed path")
	}

	output := captureStdout(t, func() {
		if err := renderUninstallResult(false, removed, nil, dataPreserved, aybDir); err != nil {
			t.Fatalf("renderUninstallResult returned error: %v", err)
		}
	})
	if !strings.Contains(output, "AYB uninstalled.") {
		t.Fatalf("expected uninstall status output, got %q", output)
	}
	if !strings.Contains(output, filepath.Join(aybDir, "data")) {
		t.Fatalf("expected preserved data notice, got %q", output)
	}
}

func TestBuildQueryRequestWithEmptyOptionalValues(t *testing.T) {
	req, err := buildQueryRequest(queryRequestConfig{
		table:   "things",
		baseURL: "http://127.0.0.1:8090",
		page:    1,
		limit:   20,
	})
	if err != nil {
		t.Fatalf("buildQueryRequest returned error: %v", err)
	}
	_, hasAuth := req.Header["Authorization"]
	if hasAuth {
		t.Fatalf("did not expect authorization header when token is empty: %v", req.Header)
	}
	query := req.URL.RawQuery
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if values.Get("page") != "1" || values.Get("perPage") != "20" {
		t.Fatalf("expected pagination query values, got %q", query)
	}
}
