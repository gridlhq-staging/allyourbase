package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestServer(t *testing.T, schemaCache *schema.CacheHolder) *server.Server {
	t.Helper()
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, logger, schemaCache, nil, nil, nil)
}

// newCacheHolderWithSchema creates a CacheHolder with an optional pre-loaded schema for tests.
func newCacheHolderWithSchema(sc *schema.SchemaCache) *schema.CacheHolder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	if sc != nil {
		ch.SetForTesting(sc)
	}
	return ch
}

func TestHealthEndpoint(t *testing.T) {
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", body["status"])
}

func TestSchemaEndpointNotReady(t *testing.T) {
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	testutil.Equal(t, 503, body.Code)
	testutil.Contains(t, body.Message, "schema cache not ready")
}

func TestSchemaEndpointReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	ch.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {Schema: "public", Name: "users", Kind: "table"},
		},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})

	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	testutil.NoError(t, err)
	tables, ok := body["tables"].(map[string]any)
	testutil.True(t, ok, "tables should be a map")
	testutil.Equal(t, 1, len(tables))
	usersRaw, ok := tables["public.users"].(map[string]any)
	testutil.True(t, ok, "public.users should be a map")
	testutil.Equal(t, "users", usersRaw["name"])
	testutil.Equal(t, "public", usersRaw["schema"])
	testutil.Equal(t, "table", usersRaw["kind"])
}

func TestRouterSetup(t *testing.T) {
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	// Health endpoint exists.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	// Schema endpoint exists (returns 503 since cache not loaded).
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Unknown API route returns 404.
	req = httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}


// TestCacheHolderGetBeforeLoad verifies that Get() returns nil before Load().
func TestCacheHolderGetBeforeLoad(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	got := ch.Get()
	testutil.True(t, got == nil, "expected nil before Load()")
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	ch := newCacheHolderWithSchema(nil)
	srv := newTestServer(t, ch)

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/yaml", w.Header().Get("Content-Type"))
	testutil.Contains(t, w.Body.String(), "openapi: 3.0.3")
	testutil.Contains(t, w.Body.String(), "AllYourBase API")
}

// TestCacheHolderReadyChannel verifies the ready channel is open before Load().
func TestCacheHolderReadyChannel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	// Ready channel should not be closed yet.
	select {
	case <-ch.Ready():
		t.Fatal("ready channel should not be closed before Load()")
	default:
		// Expected.
	}
}

// --- Security wiring tests ---

// TestSchemaEndpointRequiresAuthWhenConfigured verifies that /api/schema returns
// 401 when authSvc is configured and no bearer token is provided.
func TestSchemaEndpointRequiresAuthWhenConfigured(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	ch.SetForTesting(&schema.SchemaCache{
		Tables:  map[string]*schema.Table{"public.t": {Schema: "public", Name: "t", Kind: "table"}},
		Schemas: []string{"public"},
		BuiltAt: time.Now(),
	})
	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// Without auth header → 401.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// With valid JWT → 200.
	jwt, err := authSvc.IssueTestToken("user-1", "test@example.com")
	testutil.NoError(t, err)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
}

// TestAdminAuthRateLimited verifies that the rate limiter middleware is wired
// to the /api/admin/auth endpoint by exhausting the limit and getting 429.
func TestAdminAuthRateLimited(t *testing.T) {
	srv := newTestServerWithPassword(t, "testpass")

	// Admin rate limiter is set to 5 attempts/min per IP.
	// Send 5 requests to exhaust the limit.
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		srv.Router().ServeHTTP(w, req)
		// These should be 401 (wrong password), not 429 yet.
		testutil.Equal(t, http.StatusUnauthorized, w.Code)
	}

	// 6th request should be rate limited.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many requests")
	testutil.True(t, w.Header().Get("Retry-After") != "", "should have Retry-After header")
}

// TestStorageWriteRoutesRequireAuth verifies that storage upload and delete
// routes return 401 when authSvc is configured but no token is provided.
func TestStorageWriteRoutesRequireAuth(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)

	authSvc := auth.NewService(nil, "test-secret-that-is-at-least-32-chars!!", time.Hour, 7*24*time.Hour, 8, logger)
	localBackend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	storageSvc := storage.NewService(nil, localBackend, "sign-key-for-test", logger)

	srv := server.New(cfg, logger, ch, nil, authSvc, storageSvc)

	// POST (upload) without auth → 401.
	var body strings.Builder
	mpw := multipart.NewWriter(&body)
	fw, err := mpw.CreateFormFile("file", "test.txt")
	testutil.NoError(t, err)
	fw.Write([]byte("hello"))
	mpw.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/storage/mybucket", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// DELETE without auth → 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/storage/mybucket/test.txt", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	// Sign endpoint without auth → 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/storage/mybucket/test.txt/sign", nil)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

