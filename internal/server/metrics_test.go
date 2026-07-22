package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

const metricsAuthTokenWarningMessage = "metrics.auth_token is empty; tokenless metrics access is loopback-only. Configure metrics.auth_token for remote scrapers or reverse proxies. See https://allyourbase.io/guide/configuration"

func newTestServerWithConfig(t *testing.T, cfg *config.Config) *server.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	return server.New(cfg, logger, ch, nil, nil, nil)
}

func newTestServerWithLogger(t *testing.T, cfg *config.Config, logger *slog.Logger) *server.Server {
	t.Helper()
	ch := schema.NewCacheHolder(nil, logger)
	return server.New(cfg, logger, ch, nil, nil, nil)
}

func newMetricsRequest(remoteAddr, auth string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = remoteAddr
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return req
}

func scrapeMetrics(t *testing.T, srv *server.Server, remoteAddr, auth string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, newMetricsRequest(remoteAddr, auth))
	return w
}

func generateInstrumentedRequest(t *testing.T, srv *server.Server) {
	t.Helper()
	wHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Router().ServeHTTP(wHealth, reqHealth)
	testutil.Equal(t, http.StatusOK, wHealth.Code)
}

func assertMetricsSuccess(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	testutil.Contains(t, w.Body.String(), "ayb_http_requests_total")
}

func assertMetricsForbidden(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	testutil.Equal(t, http.StatusForbidden, w.Code)
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		DocURL  string `json:"doc_url"`
	}
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	testutil.Equal(t, http.StatusForbidden, body.Code)
	testutil.Equal(t, "metrics endpoint access from non-loopback addresses requires metrics.auth_token", body.Message)
	testutil.Equal(t, "https://allyourbase.io/guide/configuration", body.DocURL)
}

func TestMetricsEndpointEnabledExposesPrometheusMetrics(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	// Generate at least one instrumented request before scraping.
	generateInstrumentedRequest(t, srv)

	w := scrapeMetrics(t, srv, "127.0.0.1:1234", "")

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	body := w.Body.String()
	testutil.Contains(t, body, "ayb_http_requests_total")
	testutil.Contains(t, body, "ayb_http_request_duration_seconds_bucket")
	testutil.Contains(t, body, `route="/health"`)
	testutil.Contains(t, body, `le="+Inf"`)
}

func TestMetricsEndpointTokenlessScrapeRequiresLoopbackRemoteAddr(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)
	generateInstrumentedRequest(t, srv)

	for _, tc := range []struct {
		name       string
		remoteAddr string
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:1234"},
		{name: "ipv6 loopback", remoteAddr: "[::1]:1234"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertMetricsSuccess(t, scrapeMetrics(t, srv, tc.remoteAddr, ""))
		})
	}
}

func TestMetricsEndpointTokenlessScrapeRejectsNonLoopbackAndUntrustedSources(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	for _, tc := range []struct {
		name       string
		remoteAddr string
		headers    map[string]string
	}{
		{name: "non loopback", remoteAddr: "10.0.0.5:1234"},
		{
			name:       "forwarded for loopback ignored",
			remoteAddr: "10.0.0.5:1234",
			headers:    map[string]string{"X-Forwarded-For": "127.0.0.1"},
		},
		{
			name:       "real ip loopback ignored",
			remoteAddr: "10.0.0.5:1234",
			headers:    map[string]string{"X-Real-IP": "127.0.0.1"},
		},
		{name: "empty", remoteAddr: ""},
		{name: "malformed", remoteAddr: "not-a-socket-address"},
		{name: "missing port", remoteAddr: "127.0.0.1"},
		{name: "hostname", remoteAddr: "localhost:1234"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newMetricsRequest(tc.remoteAddr, "")
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}
			srv.Router().ServeHTTP(w, req)
			assertMetricsForbidden(t, w)
		})
	}
}

func TestMetricsEndpointAuthTokenEnforced(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.AuthToken = "metrics-secret"
	srv := newTestServerWithConfig(t, cfg)

	for _, tc := range []struct {
		name   string
		auth   string
		status int
	}{
		{name: "missing bearer token", auth: "", status: http.StatusUnauthorized},
		{name: "wrong bearer token", auth: "Bearer nope", status: http.StatusUnauthorized},
		{name: "lowercase bearer scheme", auth: "bearer metrics-secret", status: http.StatusOK},
		{name: "correct bearer token", auth: "Bearer metrics-secret", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := scrapeMetrics(t, srv, "10.0.0.5:1234", tc.auth)
			testutil.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusUnauthorized {
				testutil.Equal(t, `Bearer realm="metrics"`, w.Header().Get("WWW-Authenticate"))
				testutil.Equal(t, "{\"code\":401,\"message\":\"metrics endpoint unauthorized\"}\n", w.Body.String())
			}
		})
	}

	t.Run("correct token with malformed source address", func(t *testing.T) {
		w := scrapeMetrics(t, srv, "not-a-socket-address", "Bearer metrics-secret")
		testutil.Equal(t, http.StatusOK, w.Code)
	})

	for _, tc := range []struct {
		name       string
		remoteAddr string
		auth       string
	}{
		{name: "missing token with malformed source address", remoteAddr: "not-a-socket-address", auth: ""},
		{name: "wrong token with malformed source address", remoteAddr: "not-a-socket-address", auth: "Bearer nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := scrapeMetrics(t, srv, tc.remoteAddr, tc.auth)
			testutil.Equal(t, http.StatusUnauthorized, w.Code)
			testutil.Equal(t, `Bearer realm="metrics"`, w.Header().Get("WWW-Authenticate"))
			testutil.Equal(t, "{\"code\":401,\"message\":\"metrics endpoint unauthorized\"}\n", w.Body.String())
		})
	}
}

func TestMetricsEndpointDisabledReturnsNotFound(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Enabled = false
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestMetricsEndpointRespectsCustomPath(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Metrics.Path = "/internal-metrics"
	srv := newTestServerWithConfig(t, cfg)

	wDefault := httptest.NewRecorder()
	reqDefault := newMetricsRequest("127.0.0.1:1234", "")
	srv.Router().ServeHTTP(wDefault, reqDefault)
	testutil.Equal(t, http.StatusNotFound, wDefault.Code)

	wCustom := httptest.NewRecorder()
	reqCustom := httptest.NewRequest(http.MethodGet, "/internal-metrics", nil)
	reqCustom.RemoteAddr = "127.0.0.1:1234"
	srv.Router().ServeHTTP(wCustom, reqCustom)
	testutil.Equal(t, http.StatusOK, wCustom.Code)
	testutil.Contains(t, wCustom.Body.String(), "ayb_http_requests_total")
}

func TestMetricsInfraGaugeNamesPresent(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	w := scrapeMetrics(t, srv, "127.0.0.1:1234", "")

	testutil.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	testutil.Contains(t, body, "ayb_db_pool_total")
	testutil.Contains(t, body, "ayb_db_pool_idle")
	testutil.Contains(t, body, "ayb_db_pool_in_use")
	testutil.Contains(t, body, "ayb_db_pool_max")
	testutil.Contains(t, body, "ayb_storage_bytes_total")
}

func TestMetricsEndpointIncludesRealtimeMetrics(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	w := scrapeMetrics(t, srv, "127.0.0.1:1234", "")

	testutil.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	for _, want := range []string{
		`ayb_realtime_connections_active{transport="sse"}`,
		`ayb_realtime_connections_active{transport="ws"}`,
		`ayb_realtime_channels_active{type="broadcast"}`,
		`ayb_realtime_channels_active{type="presence"}`,
		"ayb_realtime_broadcast_messages_total",
		"ayb_realtime_presence_syncs_total",
	} {
		testutil.Contains(t, body, want)
	}
}

func TestMetricsMiddlewareUsesRoutePatternsNotRawPaths(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	// Hit two concrete paths that should map to the same route pattern label.
	for _, p := range []string{"/functions/v1/foo", "/functions/v1/bar"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	}

	w := scrapeMetrics(t, srv, "127.0.0.1:1234", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	testutil.Contains(t, body, `route="/functions/v1/{name}"`)
	testutil.True(t, !strings.Contains(body, `route="/functions/v1/foo"`), "raw path label leaked into metrics")
	testutil.True(t, !strings.Contains(body, `route="/functions/v1/bar"`), "raw path label leaked into metrics")
}

func TestMetricsEndpointExposesConfiguredHistogramBucketBoundaries(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	srv := newTestServerWithConfig(t, cfg)

	generateInstrumentedRequest(t, srv)

	w := scrapeMetrics(t, srv, "127.0.0.1:1234", "")
	testutil.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	for _, bound := range []string{
		"0.005",
		"0.01",
		"0.025",
		"0.05",
		"0.1",
		"0.25",
		"0.5",
		"1",
		"2.5",
		"5",
		"10",
		"+Inf",
	} {
		pat := `(?m)^ayb_http_request_duration_seconds_bucket\{[^}]*le="` + regexp.QuoteMeta(bound) + `"[^}]*\}\s+\d+(\.\d+)?$`
		testutil.True(t, regexp.MustCompile(pat).MatchString(body), "missing histogram bucket boundary %q in metrics output", bound)
	}
}

func TestMetricsAuthTokenWarning(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(*config.Config)
		want   int
	}{
		{
			name: "enabled with empty token warns once",
			want: 1,
		},
		{
			name: "disabled metrics does not warn",
			mutate: func(cfg *config.Config) {
				cfg.Metrics.Enabled = false
			},
			want: 0,
		},
		{
			name: "configured token does not warn",
			mutate: func(cfg *config.Config) {
				cfg.Metrics.AuthToken = "metrics-secret"
			},
			want: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := config.Default()
			cfg.Admin.Password = "test-admin-password"
			if tc.mutate != nil {
				tc.mutate(cfg)
			}

			_ = newTestServerWithLogger(t, cfg, logger)

			logs := logBuf.String()
			testutil.Equal(t, tc.want, countWarnMessage(logs, metricsAuthTokenWarningMessage))
			if tc.want > 0 {
				testutil.Contains(t, logs, "metrics.auth_token")
				testutil.Contains(t, logs, "tokenless metrics access is loopback-only")
				testutil.Contains(t, logs, "https://allyourbase.io/guide/configuration")
			}
		})
	}
}

func countWarnMessage(logs, message string) int {
	count := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, message) {
			count++
		}
	}
	return count
}
