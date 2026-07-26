package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAdminAnalyticsRequestsNilPoolReturns503 verifies 503 when no DB is configured.
func TestAdminAnalyticsRequestsNilPoolReturns503(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestAdminAnalyticsRequestsRequiresAdminAuth verifies unauthenticated requests are rejected.
func TestAdminAnalyticsRequestsRequiresAdminAuth(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAnalyticsRequestStreamRequiresAdminAuth(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests/stream", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAnalyticsRequestStreamNilPoolReturns503(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAdminAnalyticsRequestStreamBadQueryParams(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithPassword(t, "testpass")
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests/stream?status_class=1xx", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid status_class")
}

func TestAdminAnalyticsRequestStreamRequiresFlusher(t *testing.T) {
	t.Parallel()

	srv, cleanup := newRequestLogsServerWithUnreachablePool(t)
	defer cleanup()
	token := adminLogin(t, srv)

	w := &nonFlushingResponseWriter{header: http.Header{}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.status)
	testutil.Contains(t, w.body.String(), "streaming is not supported")
}

func TestAdminAnalyticsRequestStreamQueryFailureEmitsTerminalError(t *testing.T) {
	t.Parallel()

	srv, cleanup := newRequestLogsServerWithUnreachablePool(t)
	defer cleanup()
	token := adminLogin(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/admin/analytics/requests/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	testutil.Contains(t, w.Body.String(), "event: error")
	testutil.Contains(t, w.Body.String(), "failed to query request logs")
}

func TestAdminAnalyticsRequestStreamCanceledContextReturnsWithoutErrorEvent(t *testing.T) {
	t.Parallel()

	srv, cleanup := newRequestLogsServerWithUnreachablePool(t)
	defer cleanup()
	token := adminLogin(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/admin/analytics/requests/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	if strings.Contains(w.Body.String(), "event: error") {
		t.Fatalf("canceled request should not emit an error event: %s", w.Body.String())
	}
}

// TestAdminAnalyticsRequestsBadQueryParams verifies invalid query params return 400.
func TestAdminAnalyticsRequestsBadQueryParams(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)
	token := adminLogin(t, srv)

	cases := []struct {
		name   string
		query  string
		status int
	}{
		{name: "invalid status", query: "status=abc", status: http.StatusBadRequest},
		{name: "invalid limit", query: "limit=bad", status: http.StatusBadRequest},
		{name: "invalid offset", query: "offset=bad", status: http.StatusBadRequest},
		{name: "invalid from", query: "from=notadate", status: http.StatusBadRequest},
		{name: "invalid to", query: "to=notadate", status: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?"+tc.query, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			srv.Router().ServeHTTP(w, req)
			testutil.Equal(t, tc.status, w.Code)
		})
	}
}

type nonFlushingResponseWriter struct {
	header http.Header
	body   strings.Builder
	status int
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	return w.header
}

func (w *nonFlushingResponseWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *nonFlushingResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func newRequestLogsServerWithUnreachablePool(t *testing.T) (*server.Server, func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://127.0.0.1:1/ayb?connect_timeout=1")
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, nil, nil)
	return srv, pool.Close
}

func TestAdminAnalyticsRequestsToBeforeFromReturns400(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)
	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?from=2026-03-02&to=2026-03-01", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}
