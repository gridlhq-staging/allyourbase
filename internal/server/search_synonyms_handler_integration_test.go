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

const searchSynonymsAdminPassword = "search-synonyms-admin-pass"
const searchSynonymsJWTSecret = "search-synonyms-secret-that-is-at-least-32-chars"

type synonymGroupsResponse struct {
	Groups []synonymGroupResponse `json:"groups"`
}

type synonymGroupResponse struct {
	Terms []string `json:"terms"`
}

func resetSearchSynonymsSchema(t *testing.T, ctx context.Context) {
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
		INSERT INTO authors (name) VALUES ('Alice'), ('Bob');
		INSERT INTO posts (title, body, author_id, status) VALUES
			('Cyberpunk', 'a classic scifi tale', 1, 'published'),
			('Foundation', 'a classic science fiction novel', 1, 'published'),
			('Romance Read', 'a cozy romance novel', 2, 'draft');
	`)
	testutil.NoError(t, err)
}

func setupSearchSynonymsServer(t *testing.T, ctx context.Context, authEnabled bool) (*server.Server, *auth.Service) {
	t.Helper()
	resetSearchSynonymsSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = searchSynonymsAdminPassword
	var authSvc *auth.Service
	if authEnabled {
		cfg.Auth.Enabled = true
		cfg.Auth.JWTSecret = searchSynonymsJWTSecret
		authSvc = auth.NewService(sharedPG.Pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	}

	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil), authSvc
}

func loginSearchSynonymsAdmin(t *testing.T, srv *server.Server) string {
	t.Helper()

	w := doSearchSynonymsJSON(t, srv, http.MethodPost, "/api/admin/auth", map[string]string{
		"password": searchSynonymsAdminPassword,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.True(t, resp["token"] != "", "expected admin token")
	return resp["token"]
}

func doSearchSynonymsJSON(t *testing.T, srv *server.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
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

func doSearchSynonymsRaw(t *testing.T, srv *server.Server, method, path, body, token string) *httptest.ResponseRecorder {
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

func decodeSynonymError(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func decodeSynonymGroups(t *testing.T, w *httptest.ResponseRecorder) synonymGroupsResponse {
	t.Helper()

	var resp synonymGroupsResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func putSynonymGroups(t *testing.T, srv *server.Server, token string, groups [][]string) *httptest.ResponseRecorder {
	t.Helper()

	payload := map[string]any{"groups": make([]map[string][]string, len(groups))}
	for i, terms := range groups {
		payload["groups"].([]map[string][]string)[i] = map[string][]string{"terms": terms}
	}
	return doSearchSynonymsJSON(t, srv, http.MethodPut, "/api/collections/posts/synonyms", payload, token)
}

func assertAdminSynonymRejection(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	resp := decodeSynonymError(t, w)
	testutil.Equal(t, http.StatusUnauthorized, resp.Code)
	testutil.Equal(t, "admin authentication required", resp.Message)
}

func assertSynonymGroups(t *testing.T, want, got synonymGroupsResponse) {
	t.Helper()

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("synonym groups mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}

func synonymRowCount(t *testing.T, ctx context.Context, schemaName, tableName string) int {
	t.Helper()

	var count int
	err := sharedPG.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
	`, schemaName, tableName).Scan(&count)
	testutil.NoError(t, err)
	return count
}

func TestSearchSynonymsAdminGate(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSynonymsServer(t, ctx, false)

	assertAdminSynonymRejection(t, doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/synonyms", nil, ""))
	assertAdminSynonymRejection(t, doSearchSynonymsJSON(t, srv, http.MethodPut, "/api/collections/posts/synonyms", map[string]any{
		"groups": []map[string][]string{{"terms": []string{"scifi", "science fiction"}}},
	}, ""))
}

func TestSearchSynonymsPutReplacesGroupsAndGetReturnsNormalizedGroups(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSynonymsServer(t, ctx, false)
	adminToken := loginSearchSynonymsAdmin(t, srv)

	w := putSynonymGroups(t, srv, adminToken, [][]string{
		{" SciFi ", "Science Fiction"},
		{"AI", "Artificial Intelligence", "Machine Learning"},
	})
	testutil.StatusCode(t, http.StatusOK, w.Code)
	assertSynonymGroups(t, synonymGroupsResponse{Groups: []synonymGroupResponse{
		{Terms: []string{"ai", "artificial intelligence", "machine learning"}},
		{Terms: []string{"science fiction", "scifi"}},
	}}, decodeSynonymGroups(t, w))
	testutil.Equal(t, 5, synonymRowCount(t, ctx, "public", "posts"))

	w = doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/synonyms", nil, adminToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	assertSynonymGroups(t, synonymGroupsResponse{Groups: []synonymGroupResponse{
		{Terms: []string{"ai", "artificial intelligence", "machine learning"}},
		{Terms: []string{"science fiction", "scifi"}},
	}}, decodeSynonymGroups(t, w))

	w = putSynonymGroups(t, srv, adminToken, [][]string{{"SciFi", "Science Fiction"}})
	testutil.StatusCode(t, http.StatusOK, w.Code)
	assertSynonymGroups(t, synonymGroupsResponse{Groups: []synonymGroupResponse{
		{Terms: []string{"science fiction", "scifi"}},
	}}, decodeSynonymGroups(t, w))
	testutil.Equal(t, 2, synonymRowCount(t, ctx, "public", "posts"))
}

func TestSearchSynonymsValidation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSynonymsServer(t, ctx, false)
	adminToken := loginSearchSynonymsAdmin(t, srv)

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		rawBody string
		status  int
		want    string
	}{
		{name: "malformed_json", method: http.MethodPut, path: "/api/collections/posts/synonyms", rawBody: `{"groups":`, status: http.StatusBadRequest, want: "invalid JSON body"},
		{name: "missing_groups", method: http.MethodPut, path: "/api/collections/posts/synonyms", body: map[string]any{}, status: http.StatusBadRequest, want: "groups is required"},
		{name: "unknown_collection", method: http.MethodPut, path: "/api/collections/missing/synonyms", body: map[string]any{"groups": []map[string][]string{{"terms": []string{"a", "b"}}}}, status: http.StatusNotFound, want: "collection not found"},
		{name: "sql_injection_table_name", method: http.MethodGet, path: "/api/collections/posts%3Bdrop%20table%20posts/synonyms", status: http.StatusNotFound, want: "collection not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tt.rawBody != "" {
				w = doSearchSynonymsRaw(t, srv, tt.method, tt.path, tt.rawBody, adminToken)
			} else {
				w = doSearchSynonymsJSON(t, srv, tt.method, tt.path, tt.body, adminToken)
			}
			testutil.StatusCode(t, tt.status, w.Code)
			resp := decodeSynonymError(t, w)
			testutil.Equal(t, w.Code, resp.Code)
			testutil.Equal(t, tt.want, resp.Message)
		})
	}
}

func TestSearchSynonymsSchemaCacheNotReady(t *testing.T) {
	ctx := context.Background()
	resetSearchSynonymsSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	cfg := config.Default()
	cfg.Admin.Password = searchSynonymsAdminPassword
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	adminToken := loginSearchSynonymsAdmin(t, srv)

	w := doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/synonyms", nil, adminToken)
	testutil.StatusCode(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeSynonymError(t, w)
	testutil.Equal(t, http.StatusServiceUnavailable, resp.Code)
	testutil.Equal(t, "schema cache not ready", resp.Message)
}

func TestSearchSynonymsAuthBoundaryRejectsUserTokens(t *testing.T) {
	ctx := context.Background()
	srv, authSvc := setupSearchSynonymsServer(t, ctx, true)

	user, err := auth.CreateUser(ctx, sharedPG.Pool, "synonyms-user@example.com", "password123", 8)
	testutil.NoError(t, err)
	userJWT, err := authSvc.IssueTestToken(user.ID, user.Email)
	testutil.NoError(t, err)
	apiKey, _, err := authSvc.CreateAPIKey(ctx, user.ID, "synonyms-boundary", auth.CreateAPIKeyOptions{Scope: auth.ScopeReadOnly})
	testutil.NoError(t, err)

	for _, token := range []string{userJWT, apiKey} {
		w := doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/", nil, token)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		assertAdminSynonymRejection(t, doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/synonyms", nil, token))
	}
}

func TestSearchSynonymsPutFeedsSearchExpansion(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupSearchSynonymsServer(t, ctx, false)
	adminToken := loginSearchSynonymsAdmin(t, srv)

	w := putSynonymGroups(t, srv, adminToken, [][]string{{"scifi", "science fiction"}})
	testutil.StatusCode(t, http.StatusOK, w.Code)

	titles := searchSynonymTitles(t, srv, "search=scifi")
	testutil.True(t, containsSearchSynonymTitle(titles, "Foundation"), "expected synonym-only Foundation row after PUT, got %v", titles)

	w = putSynonymGroups(t, srv, adminToken, [][]string{{"ai", "artificial intelligence"}})
	testutil.StatusCode(t, http.StatusOK, w.Code)

	titles = searchSynonymTitles(t, srv, "search=scifi")
	testutil.False(t, containsSearchSynonymTitle(titles, "Foundation"), "expected Foundation row to disappear after replacement PUT, got %v", titles)
}

func searchSynonymTitles(t *testing.T, srv *server.Server, query string) []string {
	t.Helper()

	w := doSearchSynonymsJSON(t, srv, http.MethodGet, "/api/collections/posts/?"+query, nil, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	titles := make([]string, len(body.Items))
	for i, item := range body.Items {
		title, ok := item["title"].(string)
		testutil.True(t, ok, "expected title string")
		titles[i] = title
	}
	return titles
}

func containsSearchSynonymTitle(titles []string, want string) bool {
	for _, title := range titles {
		if title == want {
			return true
		}
	}
	return false
}
