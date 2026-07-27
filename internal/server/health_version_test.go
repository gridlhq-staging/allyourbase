package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/version"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthEndpointIncludesVersionWithoutDatabase(t *testing.T) {
	prev := version.Get()
	version.Set("9.9.9-test")
	t.Cleanup(func() { version.Set(prev) })

	srv := newTestServer(t, newCacheHolderWithSchema(nil))
	w := requestHealth(t, srv)

	testutil.Equal(t, http.StatusOK, w.Code)
	assertHealthField(t, w, "status", "ok")
	assertHealthField(t, w, "database", "not configured")
	assertHealthField(t, w, "version", "9.9.9-test")
}

func TestHealthEndpointIncludesVersionWhenDatabaseUnreachable(t *testing.T) {
	prev := version.Get()
	version.Set("9.9.9-test")
	t.Cleanup(func() { version.Set(prev) })

	srv, cleanup := newHealthVersionServerWithUnreachablePool(t)
	t.Cleanup(cleanup)
	w := requestHealth(t, srv)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	assertHealthField(t, w, "status", "degraded")
	assertHealthField(t, w, "database", "unreachable")
	assertHealthField(t, w, "version", "9.9.9-test")
}

func requestHealth(t *testing.T, srv *server.Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func assertHealthField(t *testing.T, w *httptest.ResponseRecorder, field string, expected string) {
	t.Helper()
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, expected, body[field])
}

func newHealthVersionServerWithUnreachablePool(t *testing.T) (*server.Server, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://127.0.0.1:1/ayb?connect_timeout=1")
	testutil.NoError(t, err)

	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(pool, logger)
	return server.New(cfg, logger, ch, pool, nil, nil), pool.Close
}
