package server_test

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var adminScriptSrcPattern = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)

func mustScriptAssetPath(t *testing.T, html string) string {
	t.Helper()
	match := adminScriptSrcPattern.FindStringSubmatch(html)
	if len(match) != 2 {
		t.Fatalf("expected index html to include a script src, got: %q", html)
	}
	return match[1]
}

// --- CORS tests ---

func TestCORSHeaders(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com", "http://other.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PUT")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "DELETE")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSMultiOriginSecondMatch(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com", "http://other.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://other.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "http://other.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

func TestCORSNonMatchingOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSNoOriginHeader(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()
	cfg := config.Default() // defaults to ["*"]
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/schema", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORSPreflightSpecificOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://example.com"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/schema", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

func TestCORSWildcard(t *testing.T) {
	t.Parallel()
	cfg := config.Default() // defaults to ["*"]
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Equal(t, "", w.Header().Get("Vary"))
}

// --- CORS preflight on OAuth provider endpoints ---

func newServerWithAuth(t *testing.T) *server.Server {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-for-cors-tests"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	return server.New(cfg, logger, ch, nil, authSvc, nil)
}

func TestCORSPreflightOAuthTokenEndpoint(t *testing.T) {
	t.Parallel()
	srv := newServerWithAuth(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/token", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	testutil.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORSPreflightOAuthRevokeEndpoint(t *testing.T) {
	t.Parallel()
	srv := newServerWithAuth(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/revoke", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNoContent, w.Code)
	testutil.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	testutil.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORSPostOAuthTokenEndpointWithOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.CORSAllowedOrigins = []string{"http://spa-client.example.com"}
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-for-cors"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", nil)
	req.Header.Set("Origin", "http://spa-client.example.com")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// The actual response may be an error (missing params), but the CORS headers
	// must be present so the browser allows the SPA to read the response.
	testutil.Equal(t, "http://spa-client.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	testutil.Contains(t, w.Header().Get("Vary"), "Origin")
}

// TestRequestIDHeader removed — never tested request IDs (no X-Request-Id middleware
// exists). Was just a duplicate of TestHealthEndpoint in server_test.go.

// --- Admin SPA ---

func TestAdminPathServesHTML(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/admin"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
	scriptPath := mustScriptAssetPath(t, w.Body.String())
	testutil.True(t, strings.HasPrefix(scriptPath, "/admin/assets/"))
	testutil.True(t, strings.HasSuffix(scriptPath, ".js"))
}

func TestAdminSPAFallback(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/some/deep/route", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "text/html")
	testutil.Contains(t, w.Body.String(), "<!DOCTYPE html>")
}

func TestAdminStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, "", w.Header().Get("Cache-Control"))
}

func TestAdminPlaceholderAssetServedUnderDefaultPath(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	htmlReq := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	htmlW := httptest.NewRecorder()
	srv.Router().ServeHTTP(htmlW, htmlReq)

	testutil.Equal(t, http.StatusOK, htmlW.Code)
	assertAdminBaseMeta(t, htmlW.Body.String(), "/admin/")
	scriptPath := mustScriptAssetPath(t, htmlW.Body.String())
	testutil.True(t, strings.HasPrefix(scriptPath, "/admin/assets/"))

	req := httptest.NewRequest(http.MethodGet, scriptPath, nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "javascript")
	testutil.Equal(t, "public, max-age=1209600", w.Header().Get("Cache-Control"))
	testutil.True(t, w.Body.Len() > 0)
}

func TestAdminPlaceholderAssetServedUnderCustomPath(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/console"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	htmlReq := httptest.NewRequest(http.MethodGet, "/console/", nil)
	htmlW := httptest.NewRecorder()
	srv.Router().ServeHTTP(htmlW, htmlReq)

	testutil.Equal(t, http.StatusOK, htmlW.Code)
	assertAdminBaseMeta(t, htmlW.Body.String(), "/console/")
	htmlScriptPath := mustScriptAssetPath(t, htmlW.Body.String())
	testutil.True(t, strings.HasPrefix(htmlScriptPath, "/console/assets/"))

	indexReq := httptest.NewRequest(http.MethodGet, "/console/index.html", nil)
	indexW := httptest.NewRecorder()
	srv.Router().ServeHTTP(indexW, indexReq)

	testutil.Equal(t, http.StatusOK, indexW.Code)
	testutil.Contains(t, indexW.Header().Get("Content-Type"), "text/html")
	indexScriptPath := mustScriptAssetPath(t, indexW.Body.String())
	testutil.True(t, strings.HasPrefix(indexScriptPath, "/console/assets/"))

	req := httptest.NewRequest(http.MethodGet, htmlScriptPath, nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Header().Get("Content-Type"), "javascript")
	testutil.Equal(t, "public, max-age=1209600", w.Header().Get("Cache-Control"))
	testutil.True(t, w.Body.Len() > 0)
}

func TestAdminPlaceholderAssetServedUnderRootPath(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	assertAdminBaseMeta(t, w.Body.String(), "/")
	scriptPath := mustScriptAssetPath(t, w.Body.String())
	testutil.True(t, strings.HasPrefix(scriptPath, "/assets/"))
}

func assertAdminBaseMeta(t *testing.T, html, adminBase string) {
	t.Helper()
	meta := `<meta name="ayb-admin-base" content="` + adminBase + `">`
	testutil.Equal(t, 1, strings.Count(html, meta))
	testutil.Equal(t, 1, strings.Count(html, `name="ayb-admin-base"`))
}

func TestAdminDisabled(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Admin.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

// --- StartWithReady ---

func TestStartWithReadySignalsReady(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 19876
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.StartWithReady(ready)
	}()

	select {
	case <-ready:
		// Verify the server is actually serving HTTP after the ready signal.
		resp, err := http.Get("http://127.0.0.1:19876/health")
		testutil.NoError(t, err)
		resp.Body.Close()
		testutil.Equal(t, http.StatusOK, resp.StatusCode)

		err = srv.Shutdown(context.Background())
		testutil.NoError(t, err)
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	srv := server.New(cfg, logger, ch, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	testutil.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	testutil.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	testutil.Equal(t, "camera=(), microphone=(), geolocation=()", w.Header().Get("Permissions-Policy"))
	testutil.Equal(t, "none", w.Header().Get("X-Permitted-Cross-Domain-Policies"))
	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatalf("expected Content-Security-Policy header to be set")
	}
	testutil.Equal(t, "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'", csp)
	testutil.Equal(t, "", w.Header().Get("Content-Security-Policy-Report-Only"))
	testutil.Contains(t, csp, "default-src 'self'")
	testutil.Contains(t, csp, "object-src 'none'")
	testutil.Contains(t, csp, "frame-ancestors 'none'")
	testutil.Contains(t, csp, "base-uri 'self'")

	scriptSrc := ""
	for _, segment := range strings.Split(csp, ";") {
		directive := strings.TrimSpace(segment)
		if directive == "script-src" || strings.HasPrefix(directive, "script-src ") {
			scriptSrc = directive
			break
		}
	}
	if scriptSrc == "" {
		t.Fatalf("expected Content-Security-Policy to include script-src directive: %q", csp)
	}
	testutil.False(t, strings.Contains(scriptSrc, "'unsafe-inline'"))
	testutil.False(t, strings.Contains(scriptSrc, "'unsafe-eval'"))

	// HSTS should NOT be set for plain HTTP requests.
	testutil.Equal(t, "", w.Header().Get("Strict-Transport-Security"))

	// HSTS should NOT trust X-Forwarded-Proto from a direct public client.
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req2.Header.Set("X-Forwarded-Proto", "https")
	req2.RemoteAddr = "198.51.100.20:1234"
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	testutil.Equal(t, "", w2.Header().Get("Strict-Transport-Security"))

	// HSTS should be set when a trusted private/loopback reverse proxy reports HTTPS.
	reqProxy := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqProxy.Header.Set("X-Forwarded-Proto", "https")
	reqProxy.RemoteAddr = "127.0.0.1:1234"
	wProxy := httptest.NewRecorder()
	srv.Router().ServeHTTP(wProxy, reqProxy)
	testutil.Equal(t, "max-age=63072000; includeSubDomains", wProxy.Header().Get("Strict-Transport-Security"))

	// HSTS should also be set when a trusted private-network reverse proxy reports HTTPS.
	reqPrivateProxy := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqPrivateProxy.Header.Set("X-Forwarded-Proto", "https")
	reqPrivateProxy.RemoteAddr = "10.0.0.8:1234"
	wPrivateProxy := httptest.NewRecorder()
	srv.Router().ServeHTTP(wPrivateProxy, reqPrivateProxy)
	testutil.Equal(t, "max-age=63072000; includeSubDomains", wPrivateProxy.Header().Get("Strict-Transport-Security"))

	// HSTS should also be set for requests that terminate TLS directly.
	req3 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req3.TLS = &tls.ConnectionState{}
	w3 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w3, req3)
	testutil.Equal(t, "max-age=63072000; includeSubDomains", w3.Header().Get("Strict-Transport-Security"))
}
