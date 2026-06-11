//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

const searchSettingsAdminPassword = "search-settings-admin-pass"
const searchSettingsJWTSecret = "search-settings-secret-that-is-at-least-32-chars"

type searchSettingsResponse struct {
	Attributes []searchSettingsAttribute `json:"attributes"`
}

type searchSettingsAttribute struct {
	Column string `json:"column"`
	Weight string `json:"weight"`
}

func TestSearchSettingsPutAndGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSettingsServer(t, ctx, false)
	adminToken := loginSearchSettingsAdmin(t, srv)

	want := searchSettingsResponse{Attributes: []searchSettingsAttribute{
		{Column: "title", Weight: "high"},
		{Column: "body", Weight: "low"},
	}}
	w := doSearchSettingsJSON(t, srv, http.MethodPut, "/api/collections/posts/search-settings", want, adminToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	assertSearchSettings(t, want, decodeSearchSettings(t, w))

	w = doSearchSettingsJSON(t, srv, http.MethodGet, "/api/collections/posts/search-settings", nil, adminToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	assertSearchSettings(t, want, decodeSearchSettings(t, w))
}

func TestSearchSettingsAdminGate(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSettingsServer(t, ctx, false)

	assertAdminSearchSettingsRejection(t, doSearchSettingsJSON(t, srv, http.MethodGet, "/api/collections/posts/search-settings", nil, ""))
	assertAdminSearchSettingsRejection(t, doSearchSettingsJSON(t, srv, http.MethodPut, "/api/collections/posts/search-settings", searchSettingsResponse{
		Attributes: []searchSettingsAttribute{{Column: "title", Weight: "high"}},
	}, ""))
}

func TestSearchSettingsValidationAndCollectionErrors(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSettingsServer(t, ctx, false)
	adminToken := loginSearchSettingsAdmin(t, srv)

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		rawBody string
		status  int
		want    string
	}{
		{name: "malformed_json", method: http.MethodPut, path: "/api/collections/posts/search-settings", rawBody: `{"attributes":`, status: http.StatusBadRequest, want: "invalid JSON body"},
		{name: "unknown_weight", method: http.MethodPut, path: "/api/collections/posts/search-settings", body: searchSettingsResponse{Attributes: []searchSettingsAttribute{{Column: "title", Weight: "heavy"}}}, status: http.StatusBadRequest, want: "unknown search setting attribute weight: heavy"},
		{name: "unknown_collection", method: http.MethodPut, path: "/api/collections/missing/search-settings", body: searchSettingsResponse{Attributes: []searchSettingsAttribute{{Column: "title", Weight: "high"}}}, status: http.StatusNotFound, want: "collection not found"},
		{name: "sql_injection_table_name", method: http.MethodGet, path: "/api/collections/posts%3Bdrop%20table%20posts/search-settings", status: http.StatusNotFound, want: "collection not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tt.rawBody != "" {
				w = doSearchSettingsRaw(t, srv, tt.method, tt.path, tt.rawBody, adminToken)
			} else {
				w = doSearchSettingsJSON(t, srv, tt.method, tt.path, tt.body, adminToken)
			}
			testutil.StatusCode(t, tt.status, w.Code)
			resp := decodeSearchSettingsError(t, w)
			testutil.Equal(t, w.Code, resp.Code)
			testutil.Equal(t, tt.want, resp.Message)
		})
	}
}

func TestSearchSettingsSchemaCacheNotReady(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	cfg := config.Default()
	cfg.Admin.Password = searchSettingsAdminPassword
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	adminToken := loginSearchSettingsAdmin(t, srv)

	w := doSearchSettingsJSON(t, srv, http.MethodGet, "/api/collections/posts/search-settings", nil, adminToken)
	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeSearchSettingsError(t, w)
	testutil.Equal(t, http.StatusServiceUnavailable, resp.Code)
	testutil.Equal(t, "schema cache not ready", resp.Message)
}

func TestSearchSettingsAuthBoundaryRejectsUserTokens(t *testing.T) {
	ctx := context.Background()
	srv, authSvc := setupSearchSettingsServer(t, ctx, true)

	user, err := auth.CreateUser(ctx, sharedPG.Pool, "settings-user@example.com", "password123", 8)
	testutil.NoError(t, err)
	userJWT, err := authSvc.IssueTestToken(user.ID, user.Email)
	testutil.NoError(t, err)
	apiKey, _, err := authSvc.CreateAPIKey(ctx, user.ID, "settings-boundary", auth.CreateAPIKeyOptions{Scope: auth.ScopeReadOnly})
	testutil.NoError(t, err)

	for _, token := range []string{userJWT, apiKey} {
		w := doSearchSettingsJSON(t, srv, http.MethodGet, "/api/collections/posts/", nil, token)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		assertAdminSearchSettingsRejection(t, doSearchSettingsJSON(t, srv, http.MethodGet, "/api/collections/posts/search-settings", nil, token))
	}
}

func resetSearchSettingsSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			author_id INTEGER REFERENCES authors(id),
			status TEXT DEFAULT 'draft'
		);
	`)
	testutil.NoError(t, err)
}

func setupSearchSettingsServer(t *testing.T, ctx context.Context, authEnabled bool) (*server.Server, *auth.Service) {
	t.Helper()
	resetSearchSettingsSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = searchSettingsAdminPassword
	var authSvc *auth.Service
	if authEnabled {
		cfg.Auth.Enabled = true
		cfg.Auth.JWTSecret = searchSettingsJWTSecret
		authSvc = auth.NewService(sharedPG.Pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	}

	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil), authSvc
}

func loginSearchSettingsAdmin(t *testing.T, srv *server.Server) string {
	t.Helper()

	w := doSearchSettingsJSON(t, srv, http.MethodPost, "/api/admin/auth", map[string]string{
		"password": searchSettingsAdminPassword,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp["token"] != "", "expected admin token")
	return resp["token"]
}

func doSearchSettingsJSON(t *testing.T, srv *server.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		testutil.NoError(t, err)
		reqBody = bytes.NewReader(data)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func doSearchSettingsRaw(t *testing.T, srv *server.Server, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func decodeSearchSettings(t *testing.T, w *httptest.ResponseRecorder) searchSettingsResponse {
	t.Helper()

	var resp searchSettingsResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func decodeSearchSettingsError(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func assertAdminSearchSettingsRejection(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	resp := decodeSearchSettingsError(t, w)
	testutil.Equal(t, http.StatusUnauthorized, resp.Code)
	testutil.Equal(t, "admin authentication required", resp.Message)
}

func assertSearchSettings(t *testing.T, want, got searchSettingsResponse) {
	t.Helper()

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("search settings mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}
