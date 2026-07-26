//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newRequestLoggerTestDB(t *testing.T) *testutil.PGContainer {
	t.Helper()
	ctx := context.Background()
	db, cleanup := testutil.StartPostgresForTestMain(ctx)
	t.Cleanup(cleanup)
	return db
}

func ensureIntegrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	runner := migrations.NewRunner(pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func newRequestLoggerServerForIntegration(
	t *testing.T,
	pool *pgxpool.Pool,
	batchSize,
	flushIntervalSecs int,
) *Server {
	t.Helper()
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx, pool)

	_, err := pool.Exec(ctx, "TRUNCATE TABLE _ayb_request_logs")
	testutil.NoError(t, err)

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Logging.RequestLogBatchSize = batchSize
	cfg.Logging.RequestLogFlushIntervalSecs = flushIntervalSecs
	cfg.Logging.RequestLogQueueSize = 32

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := schema.NewCacheHolder(pool, logger)
	testutil.NoError(t, cache.Load(ctx))

	srv := newServer(cfg, logger, cache, pool, nil, nil, nil)
	srv.startRequestLogger(context.Background())
	t.Cleanup(func() {
		_ = srv.requestLogger.Shutdown(context.Background())
	})

	return srv
}

func requestAdminToken(t *testing.T, srv *Server) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth", strings.NewReader(`{"password":"testpass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	token := body["token"]
	testutil.True(t, token != "", "expected admin token")
	return token
}

func waitForRequestLogCount(t *testing.T, pool *pgxpool.Pool, requestIDs []string, expected int) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	var count int

	query := "SELECT COUNT(*) FROM _ayb_request_logs"
	var args []any
	if len(requestIDs) > 0 {
		query = "SELECT COUNT(*) FROM _ayb_request_logs WHERE request_id = ANY($1::text[])"
		args = []any{requestIDs}
	}
	for {
		err := pool.QueryRow(ctx, query, args...).Scan(&count)
		if err != nil {
			t.Fatalf("query request logs count: %v", err)
		}
		if count >= expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request logs count, got %d want %d", count, expected)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForRequestLogByRequestID(
	t *testing.T,
	pool *pgxpool.Pool,
	requestID string,
) (method, path string, status int, requestSize, responseSize int64, remoteIP string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := pool.QueryRow(ctx,
			`SELECT method, path, status_code, request_size, response_size, COALESCE(host(ip_address), '')
			 FROM _ayb_request_logs WHERE request_id = $1`,
			requestID,
		).Scan(&method, &path, &status, &requestSize, &responseSize, &remoteIP)
		if err == nil {
			return
		}
		if err != pgx.ErrNoRows {
			t.Fatalf("query request log by request_id: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request log row %q", requestID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type seededRequestLog struct {
	method     string
	path       string
	status     int
	durationMS int64
	timestamp  time.Time
	requestID  string
}

func seedRequestLogs(t *testing.T, pool *pgxpool.Pool, rows []seededRequestLog) {
	t.Helper()
	ctx := context.Background()
	for _, row := range rows {
		_, err := pool.Exec(ctx,
			`INSERT INTO _ayb_request_logs (method, path, status_code, duration_ms, request_size, response_size, timestamp, request_id)
			 VALUES ($1, $2, $3, $4, 0, 0, $5, $6)`,
			row.method, row.path, row.status, row.durationMS, row.timestamp, row.requestID,
		)
		testutil.NoError(t, err)
	}
}

func requestAdminRequestLogs(
	t *testing.T,
	srv *Server,
	token string,
	query url.Values,
) adminRequestLogListResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?"+query.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var response adminRequestLogListResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestRequestLoggerIntegrationWritesRequestLogRowAfterHTTP(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 1)

	reqID := "integration-req-fields"
	req := httptest.NewRequest(http.MethodGet, "/health", strings.NewReader(`{"ping":"pong"}`))
	req.RemoteAddr = "198.51.100.1:1234"
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
	method, path, status, requestSize, responseSize, remoteIP := waitForRequestLogByRequestID(t, db.Pool, reqID)
	testutil.Equal(t, http.MethodGet, method)
	testutil.Equal(t, "/health", path)
	testutil.Equal(t, http.StatusOK, status)
	testutil.Equal(t, int64(len(`{"ping":"pong"}`)), requestSize)
	testutil.True(t, responseSize > 0, "response size should be tracked")
	testutil.Equal(t, "198.51.100.1", remoteIP)
}

func TestRequestLoggerIntegrationFlushesAtBatchSize(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 2, 60)

	reqIDs := []string{"batch-1", "batch-2"}
	for _, reqID := range reqIDs {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Request-Id", reqID)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.StatusCode(t, http.StatusOK, w.Code)
	}
	waitForRequestLogCount(t, db.Pool, reqIDs, 2)
}

func TestRequestLoggerIntegrationFlushesAtInterval(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 1)
	reqID := "interval-flush"

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
}

func TestRequestLoggerIntegrationHighConcurrencyDoesNotBlockWithFullQueue(t *testing.T) {
	flush := make(chan struct{})
	cfg := RequestLogConfig{
		Enabled:           true,
		BatchSize:         1,
		FlushIntervalSecs: 60,
		QueueSize:         2,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rl := newRequestLoggerWithFlush(cfg, logger, func(ctx context.Context, entries []RequestLogEntry) error {
		<-flush
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	rl.Start(ctx)
	t.Cleanup(func() {
		cancel()
		close(flush)
		_ = rl.Shutdown(context.Background())
	})

	handler := requestLogMiddleware(rl, func() *logging.DrainManager { return nil })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	start := time.Now()
	for range 50 {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
	duration := time.Since(start)
	testutil.True(t, duration < 500*time.Millisecond, "request logging should not block the request path when queue is full")
	testutil.True(t, rl.DropCount() > 0, "expected at least one dropped request log entry when queue is full")
}

func TestRequestLoggerIntegrationShutdownFlushesPending(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 1000, 60)
	reqID := "shutdown-flush"

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", reqID)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	testutil.NoError(t, srv.requestLogger.Shutdown(context.Background()))
	waitForRequestLogCount(t, db.Pool, []string{reqID}, 1)
}

func TestAdminRequestLogsEndpointFiltersByMethodPathStatusAndTime(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/api/collections/orders", status: 200, timestamp: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC), requestID: "seed-a"},
		{method: http.MethodPost, path: "/api/collections/orders", status: 500, timestamp: time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC), requestID: "seed-b"},
		{method: http.MethodGet, path: "/api/collections/users", status: 500, timestamp: time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC), requestID: "seed-c"},
		{method: http.MethodGet, path: "/api/health", status: 500, timestamp: time.Date(2026, 2, 3, 9, 30, 0, 0, time.UTC), requestID: "seed-d"},
	})

	token := requestAdminToken(t, srv)

	q := url.Values{}
	q.Set("method", "GET")
	q.Set("path", "/api/collections/users")
	q.Set("status", "500")
	q.Set("from", "2026-02-02T00:00:00Z")
	q.Set("to", "2026-02-02T23:59:59Z")
	q.Set("limit", "20")
	q.Set("offset", "0")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp struct {
		Items []struct {
			ID         string `json:"id"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			StatusCode int    `json:"status_code"`
		} `json:"items"`
		Count  int `json:"count"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 1, resp.Count)
	testutil.Equal(t, 1, len(resp.Items))
	testutil.Equal(t, "GET", resp.Items[0].Method)
	testutil.Equal(t, "/api/collections/users", resp.Items[0].Path)
	testutil.Equal(t, 500, resp.Items[0].StatusCode)
}

func TestAdminRequestLogsEndpointFiltersStatusDurationAndReturnsTotalCount(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	baseTime := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/stage3/a", status: 200, durationMS: 0, timestamp: baseTime, requestID: "stage3-a"},
		{method: http.MethodGet, path: "/stage3/b", status: 204, durationMS: 50, timestamp: baseTime.Add(time.Hour), requestID: "stage3-b"},
		{method: http.MethodGet, path: "/stage3/c", status: 299, durationMS: 100, timestamp: baseTime.Add(2 * time.Hour), requestID: "stage3-c"},
		{method: http.MethodGet, path: "/stage3/d", status: 301, durationMS: 150, timestamp: baseTime.Add(3 * time.Hour), requestID: "stage3-d"},
		{method: http.MethodGet, path: "/stage3/e", status: 404, durationMS: 200, timestamp: baseTime.Add(4 * time.Hour), requestID: "stage3-e"},
		{method: http.MethodGet, path: "/stage3/f", status: 500, durationMS: 250, timestamp: baseTime.Add(5 * time.Hour), requestID: "stage3-f"},
		{method: http.MethodGet, path: "/stage3/g", status: 503, durationMS: 300, timestamp: baseTime.Add(6 * time.Hour), requestID: "stage3-g"},
	})
	token := requestAdminToken(t, srv)

	tests := []struct {
		name           string
		query          url.Values
		wantRequestIDs []string
		wantCount      int
		wantLimit      int
		wantOffset     int
	}{
		{name: "2xx", query: url.Values{"status_class": {"2xx"}}, wantRequestIDs: []string{"stage3-c", "stage3-b", "stage3-a"}, wantCount: 3, wantLimit: 100},
		{name: "3xx", query: url.Values{"status_class": {"3xx"}}, wantRequestIDs: []string{"stage3-d"}, wantCount: 1, wantLimit: 100},
		{name: "4xx", query: url.Values{"status_class": {"4xx"}}, wantRequestIDs: []string{"stage3-e"}, wantCount: 1, wantLimit: 100},
		{name: "5xx", query: url.Values{"status_class": {"5xx"}}, wantRequestIDs: []string{"stage3-g", "stage3-f"}, wantCount: 2, wantLimit: 100},
		{name: "minimum inclusive", query: url.Values{"min_duration_ms": {"200"}}, wantRequestIDs: []string{"stage3-g", "stage3-f", "stage3-e"}, wantCount: 3, wantLimit: 100},
		{name: "maximum inclusive", query: url.Values{"max_duration_ms": {"100"}}, wantRequestIDs: []string{"stage3-c", "stage3-b", "stage3-a"}, wantCount: 3, wantLimit: 100},
		{name: "combined inclusive", query: url.Values{"min_duration_ms": {"100"}, "max_duration_ms": {"250"}}, wantRequestIDs: []string{"stage3-f", "stage3-e", "stage3-d", "stage3-c"}, wantCount: 4, wantLimit: 100},
		{name: "first page", query: url.Values{"status_class": {"2xx"}, "limit": {"2"}}, wantRequestIDs: []string{"stage3-c", "stage3-b"}, wantCount: 3, wantLimit: 2},
		{name: "offset beyond matches", query: url.Values{"status_class": {"2xx"}, "limit": {"2"}, "offset": {"5"}}, wantRequestIDs: []string{}, wantCount: 3, wantLimit: 2, wantOffset: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.query.Set("path", "/stage3/*")
			response := requestAdminRequestLogs(t, srv, token, tt.query)
			requestIDs := make([]string, 0, len(response.Items))
			for _, item := range response.Items {
				if item.RequestID == nil {
					t.Fatalf("request log item %q has no request ID", item.ID)
				}
				requestIDs = append(requestIDs, *item.RequestID)
			}
			if !slices.Equal(tt.wantRequestIDs, requestIDs) {
				t.Fatalf("unexpected ordered request IDs: got %v want %v", requestIDs, tt.wantRequestIDs)
			}
			testutil.Equal(t, tt.wantCount, response.Count)
			testutil.Equal(t, tt.wantLimit, response.Limit)
			testutil.Equal(t, tt.wantOffset, response.Offset)
		})
	}
}

func TestAdminRequestLogsEndpointPagesPastSharedTimestampWithStableCursor(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	sharedTimestamp := time.Date(2026, 7, 26, 11, 0, 0, 123000000, time.UTC)
	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/cursor-tie", status: 200, timestamp: sharedTimestamp, requestID: "tie-a"},
		{method: http.MethodGet, path: "/cursor-tie", status: 200, timestamp: sharedTimestamp, requestID: "tie-b"},
		{method: http.MethodGet, path: "/cursor-tie", status: 200, timestamp: sharedTimestamp, requestID: "tie-c"},
	})
	token := requestAdminToken(t, srv)

	firstPage := requestAdminRequestLogs(t, srv, token, url.Values{
		"path":  {"/cursor-tie"},
		"limit": {"2"},
	})
	testutil.Equal(t, 3, firstPage.Count)
	testutil.Equal(t, 2, len(firstPage.Items))

	lastItem := firstPage.Items[len(firstPage.Items)-1]
	secondPage := requestAdminRequestLogs(t, srv, token, url.Values{
		"path":             {"/cursor-tie"},
		"limit":            {"2"},
		"cursor_timestamp": {lastItem.Timestamp.Format(time.RFC3339Nano)},
		"cursor_id":        {lastItem.ID},
	})

	testutil.Equal(t, 3, secondPage.Count)
	testutil.Equal(t, 1, len(secondPage.Items))
	firstPageIDs := map[string]bool{
		firstPage.Items[0].ID: true,
		firstPage.Items[1].ID: true,
	}
	testutil.True(
		t,
		!firstPageIDs[secondPage.Items[0].ID],
		"cursor page should contain the remaining tied-timestamp row",
	)
}
