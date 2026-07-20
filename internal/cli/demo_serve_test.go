package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/allyourbase/ayb/examples"
)

func testDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<html><body>SPA</body></html>")},
		"assets/index-abc123.js":  {Data: []byte("console.log('app')")},
		"assets/index-abc123.css": {Data: []byte("body{margin:0}")},
		"assets/logo.svg":         {Data: []byte("<svg></svg>")},
		"favicon.ico":             {Data: []byte("icon")},
	}
}

func TestDemoFileHandler_RootServesIndexHTML(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA") {
		t.Error("root path did not serve index.html content")
	}
}

func TestDemoFileHandler_ExactFileServed(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "console.log") {
		t.Error("JS file content not served")
	}
}

func TestDemoFileHandler_SPAFallbackForUnknownPath(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/polls/123", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA") {
		t.Error("SPA fallback did not serve index.html for unknown path")
	}
}

func TestDemoFileHandler_CSSServedWithCorrectType(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.css", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected Content-Type to contain text/css, got %q", ct)
	}
}

func TestDemoFileHandler_JSServedWithCorrectType(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header for .js file, got empty")
	}
}

func TestDemoFileHandler_StaticAssetsCached(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "max-age=") {
		t.Errorf("expected Cache-Control with max-age for static asset, got %q", cc)
	}
}

func TestDemoFileHandler_IndexHTMLNotCached(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "" {
		t.Errorf("index.html should not have Cache-Control, got %q", cc)
	}
}

func TestServeDemoFile_ReturnsFalseForMissingFile(t *testing.T) {
	w := httptest.NewRecorder()
	ok := serveDemoFile(w, testDistFS(), "nonexistent.txt")
	if ok {
		t.Error("expected false for missing file")
	}
}

func TestServeDemoFile_ReturnsFalseForDirectory(t *testing.T) {
	w := httptest.NewRecorder()
	ok := serveDemoFile(w, testDistFS(), "assets")
	if ok {
		t.Error("expected false for directory path")
	}
}

func TestDemoFileHandler_FaviconServed(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for favicon, got %d", w.Code)
	}
	if w.Body.String() != "icon" {
		t.Error("favicon content not served correctly")
	}
}

func TestDemoFileHandler_WithRealDemoDist(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls", "movies"} {
		t.Run(name, func(t *testing.T) {
			distFS, err := examples.DemoDist(name)
			if err != nil {
				t.Fatalf("DemoDist(%q): %v", name, err)
			}
			handler := demoFileHandler(distFS)

			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			handler(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("root: expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), "<html") && !strings.Contains(w.Body.String(), "<!doctype") && !strings.Contains(w.Body.String(), "<!DOCTYPE") {
				t.Errorf("root: response doesn't look like HTML: %s", w.Body.String()[:min(100, w.Body.Len())])
			}

			req = httptest.NewRequest("GET", "/some/deep/route", nil)
			w = httptest.NewRecorder()
			handler(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("SPA fallback: expected 200, got %d", w.Code)
			}
		})
	}
}

func TestDemoAdminAuthProxyInjection(t *testing.T) {
	var gotAdminPath, gotNonAdminPath string
	var gotAdminAuth, gotNonAdminAuth string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/") {
			gotAdminPath = r.URL.Path
			gotAdminAuth = r.Header.Get("Authorization")
		} else {
			gotNonAdminPath = r.URL.Path
			gotNonAdminAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	mux := buildDemoMux(testDistFS(), backend.URL, "test-admin-token-123")

	req := httptest.NewRequest("GET", "/api/admin/movies/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if gotAdminPath != "/api/admin/movies/search" {
		t.Errorf("admin path not proxied, got %q", gotAdminPath)
	}
	if gotAdminAuth != "Bearer test-admin-token-123" {
		t.Errorf("admin auth not injected, got %q", gotAdminAuth)
	}

	req = httptest.NewRequest("GET", "/api/auth/me", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if gotNonAdminPath != "/api/auth/me" {
		t.Errorf("non-admin path not proxied, got %q", gotNonAdminPath)
	}
	if gotNonAdminAuth != "" {
		t.Errorf("non-admin path should not have auth injected, got %q", gotNonAdminAuth)
	}
}

func TestDemoAdminAuthProxyNoTokenNoInjection(t *testing.T) {
	var gotAuth string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	mux := buildDemoMux(testDistFS(), backend.URL, "")

	req := httptest.NewRequest("GET", "/api/admin/movies/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if gotAuth != "" {
		t.Errorf("empty admin token should not inject auth, got %q", gotAuth)
	}
}

func TestDemoProxyPreservesInboundHost(t *testing.T) {
	var backendHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	mux := buildDemoMux(testDistFS(), backend.URL, "")
	req := httptest.NewRequest("GET", "http://demo.example.test:5173/api/realtime/ws", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if backendHost != req.Host {
		t.Fatalf("backend Host = %q, want inbound Host %q", backendHost, req.Host)
	}
}
