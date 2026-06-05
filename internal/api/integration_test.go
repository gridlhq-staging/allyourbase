//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		cleanup()
		panic(fmt.Sprintf("bootstrap migrations: %v", err))
	}
	if _, err := runner.Run(ctx); err != nil {
		cleanup()
		panic(fmt.Sprintf("run migrations: %v", err))
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

// resetAndSeedDB drops the public schema and recreates the test tables with seed data.
func resetAndSeedDB(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	// Re-apply shared migrations after schema reset so integration tests validate
	// runtime capabilities (including pg_trgm) via the canonical setup path.
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations after schema reset: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations after schema reset: %v", err)
	}

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
			status TEXT DEFAULT 'draft',
			created_at TIMESTAMPTZ DEFAULT now()
		);
		CREATE TABLE tags (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);

		INSERT INTO authors (name) VALUES ('Alice'), ('Bob');
		INSERT INTO posts (title, body, author_id, status) VALUES
			('First Post', 'Hello world', 1, 'published'),
			('Second Post', 'Another post', 1, 'draft'),
			('Bob Post', 'By Bob', 2, 'published');
		INSERT INTO tags (name) VALUES ('go'), ('api'), ('test');
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupTestServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()

	logger := testutil.DiscardLogger()
	return setupTestServerWithLogger(t, ctx, logger)
}

func setupTestServerWithLogger(t *testing.T, ctx context.Context, logger *slog.Logger) (*server.Server, *testutil.PGContainer) {
	t.Helper()

	resetAndSeedDB(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	return srv, sharedPG
}

func doRequest(t *testing.T, srv *server.Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequestWithClaims(t, srv, method, path, body, nil)
}

func pgTrgmInstalled(t *testing.T, ctx context.Context, pg *testutil.PGContainer) bool {
	t.Helper()
	var installed bool
	err := pg.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')`).Scan(&installed)
	testutil.NoError(t, err)
	return installed
}

// doRequestWithClaims performs an HTTP request with JWT claims injected into the
// request context. Used by RLS tests to simulate authenticated users.
func doRequestWithClaims(t *testing.T, srv *server.Server, method, path string, body any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if claims != nil {
		ctx := auth.ContextWithClaims(req.Context(), claims)
		req = req.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("parsing JSON response: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

// jsonNum extracts a float64 from a JSON-decoded map value.
func jsonNum(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T: %v", v, v)
	}
	return f
}

func jsonStr(t *testing.T, v any) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", v, v)
	}
	return s
}

func jsonItems(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", body["items"])
	}
	items := make([]map[string]any, len(raw))
	for i, v := range raw {
		items[i] = v.(map[string]any)
	}
	return items
}

// --- List tests ---

func TestListRecords(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 20.0, jsonNum(t, body["perPage"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
}

func TestListPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=1&perPage=2", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["perPage"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalPages"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
}

func TestListPaginationPage2(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?page=2&perPage=2&sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 2.0, jsonNum(t, body["page"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	// Page 2 with perPage=2 sorted by id should return the 3rd post (Bob Post).
	testutil.Equal(t, "Bob Post", jsonStr(t, items[0]["title"]))
}

func TestListSkipTotal(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?skipTotal=true", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, -1.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, -1.0, jsonNum(t, body["totalPages"]))
	// Verify items are still returned even when totals are skipped.
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
}

func TestListWithSort(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?sort=-id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, items[0]["id"])) // highest ID first
	testutil.Equal(t, 2.0, jsonNum(t, items[1]["id"]))
	testutil.Equal(t, 1.0, jsonNum(t, items[2]["id"])) // lowest ID last
}

func TestListWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?fields=id,title&sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.True(t, len(items) > 0, "expected items")
	first := items[0]
	testutil.Equal(t, 1.0, jsonNum(t, first["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, first["title"]))
	_, hasBody := first["body"]
	testutil.False(t, hasBody, "body field should not be present")
	_, hasStatus := first["status"]
	testutil.False(t, hasStatus, "status field should not be present")
}

func TestListWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
	// Verify every returned item actually has status=published.
	for _, item := range items {
		testutil.Equal(t, "published", jsonStr(t, item["status"]))
	}
}

func TestListWithFilterAnd(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'+AND+author_id%3D1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, "published", jsonStr(t, items[0]["status"]))
	testutil.Equal(t, 1.0, jsonNum(t, items[0]["author_id"]))
}

func TestListInvalidFilterIntegration(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=nonexistent%3D'x'", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestListCollectionNotFoundIntegration(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Read single record tests ---

func TestReadRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
}

func TestReadRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/999", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestReadRecordWithFields(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/1?fields=id,title", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
	_, hasBody := body["body"]
	testutil.False(t, hasBody, "body should not be present")
	_, hasStatus := body["status"]
	testutil.False(t, hasStatus, "status should not be present")
}

// --- Create tests ---

func TestCreateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"name": "Charlie"}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Charlie", jsonStr(t, body["name"]))
	testutil.NotNil(t, body["id"])
}

func TestCreateRecordInvalidJSON(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	req := httptest.NewRequest("POST", "/api/collections/authors/", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "POST", "/api/collections/authors/", map[string]any{})
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCreateRecordNotNullViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// authors.name is NOT NULL.
	data := map[string]any{"id": 100}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "missing required")
}

func TestCreateRecordUniqueViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// tags.name has UNIQUE constraint.
	data := map[string]any{"name": "go"} // already exists
	w := doRequest(t, srv, "POST", "/api/collections/tags/", data)
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, resp["message"]), "unique constraint violation")
}

// --- Update tests ---

func TestUpdateRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "Updated Title"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Updated Title", jsonStr(t, body["title"]))
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
}

func TestUpdateRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"title": "nope"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/999", data)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestUpdateRecordEmptyBody(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", map[string]any{})
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

// --- Delete tests ---

func TestDeleteRecord(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/3", nil)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Verify it's gone.
	w = doRequest(t, srv, "GET", "/api/collections/tags/3", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestDeleteRecordNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "DELETE", "/api/collections/tags/999", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Expand tests ---

func TestReadWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by FK column name (author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author_id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, "Alice", jsonStr(t, author["name"]))
}

func TestReadWithExpandFriendlyName(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Test expand by friendly name (author, derived from author_id).
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=author", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present in response")
	}

	expandMap := expandData.(map[string]any)
	author, ok := expandMap["author"].(map[string]any)
	if !ok {
		t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
	}
	testutil.Equal(t, "Alice", jsonStr(t, author["name"]))
}

func TestListWithExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?expand=author", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items))

	// Build expected author_id -> name mapping from fixtures.
	wantAuthor := map[float64]string{1: "Alice", 2: "Bob"}

	// Every post with an author_id should have the correct expand.author.
	for _, item := range items {
		if item["author_id"] == nil {
			continue
		}
		authorID := jsonNum(t, item["author_id"])
		expandData, ok := item["expand"]
		if !ok {
			t.Fatalf("expand key not present on post with author_id=%v", authorID)
		}
		expandMap := expandData.(map[string]any)
		author, ok := expandMap["author"].(map[string]any)
		if !ok {
			t.Fatalf("expected expand.author to be a map, got %T", expandMap["author"])
		}
		// Verify the expanded author matches the post's author_id.
		testutil.Equal(t, authorID, jsonNum(t, author["id"]))
		testutil.Equal(t, wantAuthor[authorID], jsonStr(t, author["name"]))
	}
}

// --- One-to-many expand test ---

func TestListWithOneToManyExpand(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Expand posts from an author (one-to-many).
	w := doRequest(t, srv, "GET", "/api/collections/authors/1?expand=posts", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Alice", jsonStr(t, body["name"]))

	expandData, ok := body["expand"]
	if !ok {
		t.Fatal("expand key not present — one-to-many expand failed")
	}
	expandMap := expandData.(map[string]any)
	posts, ok := expandMap["posts"].([]any)
	if !ok {
		t.Fatalf("expected expand.posts to be an array, got %T", expandMap["posts"])
	}
	testutil.Equal(t, 2, len(posts)) // Alice has 2 posts
	// Verify the expanded posts are actually Alice's posts.
	titles := make(map[string]bool)
	for _, p := range posts {
		post := p.(map[string]any)
		titles[post["title"].(string)] = true
	}
	testutil.True(t, titles["First Post"], "expected 'First Post' in Alice's expanded posts")
	testutil.True(t, titles["Second Post"], "expected 'Second Post' in Alice's expanded posts")
}

// --- Validation tests ---

func TestCreateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value", "also_fake": 123}
	w := doRequest(t, srv, "POST", "/api/collections/authors/", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

func TestUpdateAllUnknownColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	data := map[string]any{"nonexistent_col": "value"}
	w := doRequest(t, srv, "PATCH", "/api/collections/posts/1", data)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "no recognized columns")
}

// --- Edge case tests ---

func TestViewReadOnly(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a view.
	_, err := pg.Pool.Exec(ctx, `CREATE VIEW active_posts AS SELECT * FROM posts WHERE status = 'published'`)
	if err != nil {
		t.Fatalf("creating view: %v", err)
	}

	// Reload schema to pick up the view.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("reloading schema: %v", err)
	}
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// GET should work.
	w := doRequest(t, srv, "GET", "/api/collections/active_posts/", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// POST should be rejected.
	data := map[string]any{"title": "test"}
	w = doRequest(t, srv, "POST", "/api/collections/active_posts/", data)
	testutil.StatusCode(t, http.StatusMethodNotAllowed, w.Code)
}

// --- Error format tests ---

func TestErrorResponseFormat(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 404.0, jsonNum(t, body["code"]))
	msg, ok := body["message"].(string)
	testutil.True(t, ok, "expected message to be a string")
	testutil.Contains(t, msg, "not found")
}

// --- Full-text search tests ---

func TestSearchBasic(t *testing.T) {
	ctx := context.Background()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	srv, _ := setupTestServerWithLogger(t, ctx, logger)

	// Search for "Alice" — only appears in authors, not in posts.
	// Search for "Bob" — matches "Bob Post" title and "By Bob" body (1 post).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=Bob", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status: got %d, want %d\nbody: %s\nlogs:\n%s", w.Code, http.StatusOK, w.Body.String(), logBuf.String())
	}

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	// Verify both title and body to ensure search actually worked on content.
	testutil.Equal(t, "Bob Post", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, "By Bob", jsonStr(t, items[0]["body"]))
	testutil.Equal(t, 2.0, jsonNum(t, items[0]["author_id"]))
}

func TestSearchMatchesContent(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// "Hello world" appears only in First Post's body.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=hello+world", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
}

func TestSearchNoResults(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=zzz_nonexistent_xyz", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 0, len(items))
	testutil.Equal(t, 0.0, jsonNum(t, body["totalItems"]))
}

func TestSearchWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Search for "post" (all 3 match) but filter to only published (2 match).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post&filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalItems"]))

	// Verify all returned items are published.
	for _, item := range items {
		testutil.Equal(t, "published", jsonStr(t, item["status"]))
	}
}

func TestSearchWithFilterFacetsMatchesReturnedScope(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post&filter=status%3D'published'&facets=status,author_id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 2, len(items))

	facetsRaw, ok := body["facets"].(map[string]any)
	testutil.True(t, ok, "expected facets object")
	statusRaw, ok := facetsRaw["status"].([]any)
	testutil.True(t, ok, "expected status facet bucket")
	testutil.Equal(t, 1, len(statusRaw))
	statusBucket := statusRaw[0].(map[string]any)
	testutil.Equal(t, "published", jsonStr(t, statusBucket["value"]))
	testutil.Equal(t, 2.0, jsonNum(t, statusBucket["count"]))

	authorRaw, ok := facetsRaw["author_id"].([]any)
	testutil.True(t, ok, "expected author_id facet bucket")
	testutil.Equal(t, 2, len(authorRaw))
}

func TestListResponseOmitsFacetsWhenNotRequested(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	if _, ok := body["facets"]; ok {
		t.Fatal("expected facets to be omitted when not requested")
	}
}

func TestSearchWithPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Search for "post" (3 results), paginate to 1 per page.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=post&perPage=1&page=1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 3.0, jsonNum(t, body["totalPages"]))
}

func TestSearchNoTextColumnsTable(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with no text columns.
	_, err := pg.Pool.Exec(ctx, `CREATE TABLE counters (id SERIAL PRIMARY KEY, count INTEGER)`)
	if err != nil {
		t.Fatalf("creating counters table: %v", err)
	}

	// Reload schema to pick up the new table.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("reloading schema: %v", err)
	}
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "GET", "/api/collections/counters/?search=test", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "no text columns")
}

func TestSearchEmptyString(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Empty search param should be ignored (return all records).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items)) // all posts returned
}

func TestSearchWhitespaceOnly(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Whitespace-only search should be treated as empty (trimmed by handler).
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=+++", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 3, len(items)) // all posts returned
}

func setupSearchHighlightCollisionServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		ALTER TABLE posts
			ADD COLUMN "__search_highlight" TEXT DEFAULT 'stored old highlight column',
			ADD COLUMN "__ayb_search_highlight" TEXT DEFAULT 'stored ayb highlight column'
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil), sharedPG
}

func setupSearchHighlightResponseCollisionServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		ALTER TABLE posts ADD COLUMN "_highlight" TEXT DEFAULT 'stored user highlight column'
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
}

func TestSearchHighlight(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchHighlightCollisionServer(t, ctx)
	if !pgTrgmInstalled(t, ctx, pg) {
		t.Fatal("shared migration/setup path must install pg_trgm for fuzzy highlight coverage")
	}

	t.Run("highlight_true_returns_marked_excerpt", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/posts/?search=hello&highlight=true", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)
		testutil.Equal(t, 1, len(items))
		testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
		testutil.Contains(t, jsonStr(t, items[0]["_highlight"]), "<b>Hello</b> world")
		testutil.Equal(t, "stored old highlight column", jsonStr(t, items[0]["__search_highlight"]))
		testutil.Equal(t, "stored ayb highlight column", jsonStr(t, items[0]["__ayb_search_highlight"]))
	})

	t.Run("highlight_escapes_stored_html", func(t *testing.T) {
		_, err := pg.Pool.Exec(ctx, `
			INSERT INTO posts (title, body, author_id, status)
			VALUES ('Markup Post', 'Needle <script>alert(1)</script> & <img src=x>', 1, 'published')
		`)
		testutil.NoError(t, err)

		w := doRequest(t, srv, "GET", "/api/collections/posts/?search=needle&highlight=true", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)
		testutil.Equal(t, 1, len(items))
		highlight := jsonStr(t, items[0]["_highlight"])
		testutil.Contains(t, highlight, "<b>Needle</b>")
		testutil.Contains(t, highlight, "&lt;script&gt;alert(1)&lt;/script&gt;")
		testutil.Contains(t, highlight, "&amp;")
		if strings.Contains(highlight, "<script>") || strings.Contains(highlight, "<img") {
			t.Fatalf("expected stored markup to be escaped in highlight, got %q", highlight)
		}
	})

	t.Run("highlight_absent_omits_field", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/posts/?search=hello", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)
		testutil.Equal(t, 1, len(items))
		if _, ok := items[0]["_highlight"]; ok {
			t.Fatal("expected _highlight to be omitted when highlight is absent")
		}
		testutil.Equal(t, "stored old highlight column", jsonStr(t, items[0]["__search_highlight"]))
		testutil.Equal(t, "stored ayb highlight column", jsonStr(t, items[0]["__ayb_search_highlight"]))
	})

	t.Run("fuzzy_only_highlight_is_present_without_markup", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/posts/?search=Frist+Post&fuzzy=true&highlight=true", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)
		testutil.Equal(t, 1, len(items))
		highlight := jsonStr(t, items[0]["_highlight"])
		testutil.Contains(t, highlight, "First Post")
		if strings.Contains(highlight, "<b>") || strings.Contains(highlight, "</b>") {
			t.Fatalf("expected fuzzy-only highlight without exact-search markup, got %q", highlight)
		}
	})
}

func TestSearchHighlightRejectsResponseFieldCollision(t *testing.T) {
	ctx := context.Background()
	srv := setupSearchHighlightResponseCollisionServer(t, ctx)

	t.Run("offset", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/posts/?search=hello&highlight=true", nil)
		testutil.StatusCode(t, http.StatusBadRequest, w.Code)

		body := parseJSON(t, w)
		testutil.Contains(t, jsonStr(t, body["message"]), `"_highlight" column`)
	})

	t.Run("cursor", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/posts/?cursor=&perPage=1&sort=id&search=hello&highlight=true", nil)
		testutil.StatusCode(t, http.StatusBadRequest, w.Code)

		body := parseJSON(t, w)
		testutil.Contains(t, jsonStr(t, body["message"]), `"_highlight" column`)
	})
}

func TestSearchTypoThreshold(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)
	if !pgTrgmInstalled(t, ctx, pg) {
		t.Fatal("shared migration/setup path must install pg_trgm for fuzzy integration coverage")
	}

	tests := []struct {
		name          string
		url           string
		wantCount     int
		wantTitle     string
		wantExactRows bool
	}{
		{
			name:          "default_threshold_matches_typo",
			url:           "/api/collections/posts/?search=Frist+Post&fuzzy=true",
			wantCount:     1,
			wantTitle:     "First Post",
			wantExactRows: true,
		},
		{
			name:          "strict_threshold_misses_typo",
			url:           "/api/collections/posts/?search=Frist+Post&fuzzy=true&typo_threshold=0.5",
			wantCount:     0,
			wantExactRows: true,
		},
		{
			name:      "loose_threshold_matches_typo",
			url:       "/api/collections/posts/?search=Frist+Post&fuzzy=true&typo_threshold=0.1",
			wantTitle: "First Post",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, srv, "GET", tc.url, nil)
			testutil.StatusCode(t, http.StatusOK, w.Code)

			body := parseJSON(t, w)
			items := jsonItems(t, body)
			if tc.wantExactRows {
				testutil.Equal(t, tc.wantCount, len(items))
			}
			if tc.wantExactRows && tc.wantTitle != "" {
				testutil.Equal(t, tc.wantTitle, jsonStr(t, items[0]["title"]))
				return
			}
			if tc.wantTitle != "" {
				for _, item := range items {
					if jsonStr(t, item["title"]) == tc.wantTitle {
						return
					}
				}
				t.Fatalf("expected search results to include title %q, got %v", tc.wantTitle, items)
			}
		})
	}
}

func TestSearchFuzzyFalseUsesExactSearch(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=Frist+Post&fuzzy=false", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 0, len(items))
}

func TestSearchRejectsInvalidFuzzyParams(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	tests := []struct {
		name     string
		url      string
		contains string
	}{
		{
			name:     "fuzzy_without_search",
			url:      "/api/collections/posts/?fuzzy=true",
			contains: "fuzzy",
		},
		{
			name:     "fuzzy_invalid_boolean",
			url:      "/api/collections/posts/?search=post&fuzzy=notabool",
			contains: "boolean",
		},
		{
			name:     "typo_threshold_without_fuzzy",
			url:      "/api/collections/posts/?search=post&typo_threshold=0.5",
			contains: "fuzzy",
		},
		{
			name:     "typo_threshold_not_number",
			url:      "/api/collections/posts/?search=post&fuzzy=true&typo_threshold=not-a-number",
			contains: "typo_threshold",
		},
		{
			name:     "typo_threshold_below_zero",
			url:      "/api/collections/posts/?search=post&fuzzy=true&typo_threshold=-0.01",
			contains: "typo_threshold",
		},
		{
			name:     "typo_threshold_above_one",
			url:      "/api/collections/posts/?search=post&fuzzy=true&typo_threshold=1.01",
			contains: "typo_threshold",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, srv, "GET", tc.url, nil)
			testutil.StatusCode(t, http.StatusBadRequest, w.Code)
			body := parseJSON(t, w)
			testutil.Contains(t, strings.ToLower(jsonStr(t, body["message"])), strings.ToLower(tc.contains))
		})
	}
}

func TestPGTrgmReadiness(t *testing.T) {
	ctx := context.Background()
	_, pg := setupTestServer(t, ctx)

	var availableVersion string
	err := pg.Pool.QueryRow(ctx, `SELECT default_version FROM pg_available_extensions WHERE name = 'pg_trgm'`).Scan(&availableVersion)
	if err != nil {
		t.Fatalf("pg_trgm unavailable from pg_available_extensions (binary/config mismatch): %v", err)
	}
	if strings.TrimSpace(availableVersion) == "" {
		t.Fatal("pg_trgm available entry has empty default version (binary/config mismatch)")
	}

	if !pgTrgmInstalled(t, ctx, pg) {
		t.Fatal("shared migration/setup path must install pg_trgm when extension is available")
	}

	var installedVersion string
	err = pg.Pool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = 'pg_trgm'`).Scan(&installedVersion)
	testutil.NoError(t, err)
	if strings.TrimSpace(installedVersion) == "" {
		t.Fatal("pg_trgm installed with empty extversion")
	}
}

// --- Combined sort + filter + pagination ---

func TestCombinedFilterSortPagination(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Filter published, sort by id desc, page 1 perPage 1.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?filter=status%3D'published'&sort=-id&page=1&perPage=1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, 2.0, jsonNum(t, body["totalItems"]))
	testutil.Equal(t, 2.0, jsonNum(t, body["totalPages"]))

	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, 3.0, jsonNum(t, items[0]["id"])) // Bob Post, highest published ID
}

// --- API hardening: FK expand edge cases ---

func TestExpandCircularReferenceSelfReferential(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with self-referential FK (e.g., users.manager_id -> users.id).
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			manager_id INTEGER REFERENCES users(id)
		);
		INSERT INTO users (name, manager_id) VALUES
			('Alice', NULL),
			('Bob', 1),
			('Charlie', 2);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Expand manager.manager (two levels deep).
	w := doRequest(t, srv, "GET", "/api/collections/users/3?expand=manager.manager", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Charlie", jsonStr(t, body["name"]))

	// Verify expand.manager exists.
	expand := body["expand"].(map[string]any)
	manager := expand["manager"].(map[string]any)
	testutil.Equal(t, "Bob", jsonStr(t, manager["name"]))

	// Verify expand.manager.expand.manager exists (Alice, two levels deep).
	managerExpand := manager["expand"].(map[string]any)
	grandManager := managerExpand["manager"].(map[string]any)
	testutil.Equal(t, "Alice", jsonStr(t, grandManager["name"]))
}

func TestExpandMaxDepthEnforced(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create self-referential table.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE categories (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES categories(id)
		);
		INSERT INTO categories (name, parent_id) VALUES
			('Root', NULL),
			('Level1', 1),
			('Level2', 2),
			('Level3', 3);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Try to expand 3 levels (parent.parent.parent), but maxExpandDepth is 2.
	w := doRequest(t, srv, "GET", "/api/collections/categories/4?expand=parent.parent.parent", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	testutil.Equal(t, "Level3", jsonStr(t, body["name"]))

	// Should have expand.parent (Level2).
	expand := body["expand"].(map[string]any)
	parent := expand["parent"].(map[string]any)
	testutil.Equal(t, "Level2", jsonStr(t, parent["name"]))

	// Should have expand.parent.expand.parent (Level1).
	parentExpand := parent["expand"].(map[string]any)
	grandParent := parentExpand["parent"].(map[string]any)
	testutil.Equal(t, "Level1", jsonStr(t, grandParent["name"]))

	// Should NOT have a third level (depth limit enforced).
	_, hasThirdLevel := grandParent["expand"]
	testutil.False(t, hasThirdLevel, "max depth should prevent third level expand")
}

func TestExpandMissingRelation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Try to expand a nonexistent relation.
	w := doRequest(t, srv, "GET", "/api/collections/posts/1?expand=nonexistent", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	// Verify the post was returned correctly with all expected fields.
	testutil.Equal(t, 1.0, jsonNum(t, body["id"]))
	testutil.Equal(t, "First Post", jsonStr(t, body["title"]))
	testutil.Equal(t, "Hello world", jsonStr(t, body["body"]))
	testutil.Equal(t, 1.0, jsonNum(t, body["author_id"]))
	testutil.Equal(t, "published", jsonStr(t, body["status"]))
	// The expand key should either be absent or not contain the nonexistent relation.
	expand, hasExpand := body["expand"]
	if !hasExpand {
		// expand key absent is valid — nonexistent relation correctly ignored.
		return
	}
	expandMap := expand.(map[string]any)
	_, hasNonexistent := expandMap["nonexistent"]
	testutil.False(t, hasNonexistent, "nonexistent relation should not be in expand")
}

// --- API hardening: Batch operation rollback ---

func TestBatchCreatePartialFailureRollback(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with a unique constraint.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE emails (
			id SERIAL PRIMARY KEY,
			address TEXT NOT NULL UNIQUE
		);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Batch insert: third record duplicates first, triggering unique constraint violation.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "create", "body": map[string]any{"address": "alice@example.com"}},
			{"method": "create", "body": map[string]any{"address": "bob@example.com"}},
			{"method": "create", "body": map[string]any{"address": "alice@example.com"}}, // duplicate
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/emails/batch", batch)

	// Batch should fail with conflict (duplicate key violation).
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	// Verify NO records were inserted (full rollback).
	var count int
	err = pg.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM emails").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestBatchUpdatePartialFailureRollback(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Seed data: tags table already exists from resetAndSeedDB with unique constraint on name.
	// Insert two additional tags with known IDs for this test.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO tags (id, name) VALUES (100, 'original1'), (101, 'original2')
		ON CONFLICT (name) DO NOTHING;
	`)
	testutil.NoError(t, err)

	// Batch update via POST (batch endpoint is POST only) with BatchRequest format.
	// Try to set both to same name (violates unique constraint on name).
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "update", "id": "100", "body": map[string]any{"name": "updated"}},
			{"method": "update", "id": "101", "body": map[string]any{"name": "updated"}}, // duplicate
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/tags/batch", batch)

	// Should fail with conflict (unique constraint violation).
	testutil.StatusCode(t, http.StatusConflict, w.Code)

	// Verify BOTH records remain unchanged (full rollback).
	var name1, name2 string
	err = pg.Pool.QueryRow(ctx, "SELECT name FROM tags WHERE id = 100").Scan(&name1)
	testutil.NoError(t, err)
	testutil.Equal(t, "original1", name1)

	err = pg.Pool.QueryRow(ctx, "SELECT name FROM tags WHERE id = 101").Scan(&name2)
	testutil.NoError(t, err)
	testutil.Equal(t, "original2", name2)
}

// --- API hardening: RPC edge cases ---

func TestRPCFunctionWithVARIADICArgs(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function with VARIADIC args.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION sum_all(VARIADIC vals INTEGER[]) RETURNS INTEGER AS $$
			SELECT SUM(v) FROM UNNEST(vals) AS v;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call with array of values.
	body := map[string]any{
		"vals": []int{1, 2, 3, 4, 5},
	}
	w := doRequest(t, srv, "POST", "/api/rpc/sum_all", body)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Single-column result is unwrapped to a scalar by the RPC handler.
	var result float64
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, 15.0, result)
}

func TestRPCFunctionWithOUTParameters(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function with OUT parameters.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION get_stats(OUT total INTEGER, OUT avg_val NUMERIC) AS $$
		BEGIN
			SELECT COUNT(*), AVG(id) INTO total, avg_val FROM posts;
		END;
		$$ LANGUAGE plpgsql;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call the function.
	w := doRequest(t, srv, "POST", "/api/rpc/get_stats", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	// OUT parameters return a record with named fields.
	testutil.Equal(t, 3.0, jsonNum(t, result["total"]))
	testutil.Equal(t, 2.0, jsonNum(t, result["avg_val"])) // AVG(1,2,3) = 2
}

func TestRPCFunctionReturningSetOf(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function returning SETOF.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION get_all_author_names() RETURNS SETOF TEXT AS $$
			SELECT name FROM authors ORDER BY id;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/get_all_author_names", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// SETOF returns an array of records (each record is a map with column name as key).
	// For SETOF TEXT, the column is named after the function.
	var result []map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &result)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(result))
	// Extract the single column value from each record.
	names := make([]string, len(result))
	for i, row := range result {
		for _, v := range row {
			names[i] = v.(string)
		}
	}
	testutil.Equal(t, "Alice", names[0])
	testutil.Equal(t, "Bob", names[1])
}

func TestRPCFunctionThatRaisesException(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that raises an exception.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION raise_error() RETURNS VOID AS $$
		BEGIN
			RAISE EXCEPTION 'intentional error';
		END;
		$$ LANGUAGE plpgsql;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/raise_error", nil)
	// P0001 (RAISE EXCEPTION) is mapped to 400 Bad Request by mapPGError.
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "intentional error")
}

func TestRPCFunctionWithNULLHandling(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that handles NULL.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION coalesce_text(val TEXT, fallback TEXT) RETURNS TEXT AS $$
			SELECT COALESCE(val, fallback);
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema so the new function is discoverable.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Call with NULL value.
	body := map[string]any{
		"val":      nil,
		"fallback": "default",
	}
	w := doRequest(t, srv, "POST", "/api/rpc/coalesce_text", body)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Single-column result is unwrapped to a scalar by the RPC handler.
	var result string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	testutil.Equal(t, "default", result)
}

// --- Error path coverage: constraint violations, type errors, FK violations ---

func TestCheckConstraintViolation(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a table with a CHECK constraint.
	_, err := pg.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC NOT NULL CHECK (price > 0)
		);
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	// Insert with price = -1 to trigger CHECK violation.
	body := map[string]any{"name": "Widget", "price": -1}
	w := doRequest(t, srv, "POST", "/api/collections/products/", body)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "check constraint violation")
}

func TestInvalidTypeValue(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// posts.author_id is INTEGER. Pass a string that can't be parsed as int.
	body := map[string]any{"title": "Test", "author_id": "not-a-number"}
	w := doRequest(t, srv, "POST", "/api/collections/posts/", body)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "invalid integer value")
}

func TestDeleteWithForeignKeyViolation(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Try to delete author 1 (Alice) — posts reference her via author_id FK.
	w := doRequest(t, srv, "DELETE", "/api/collections/authors/1", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "foreign key violation")
}

func TestBatchUpdateNotFoundReturns404(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Batch update with a non-existent ID.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "update", "id": "99999", "body": map[string]any{"name": "Ghost"}},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "record not found")
}

func TestBatchDeleteNotFoundReturns404(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Batch delete with a non-existent ID.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "delete", "id": "99999"},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	resp := parseJSON(t, w)
	testutil.Contains(t, resp["message"].(string), "record not found")
}

func TestBatchNotFoundRollsBack(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Batch: create a record, then update a non-existent one.
	// The create should be rolled back.
	batch := map[string]any{
		"operations": []map[string]any{
			{"method": "create", "body": map[string]any{"name": "Charlie"}},
			{"method": "update", "id": "99999", "body": map[string]any{"name": "Ghost"}},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/authors/batch", batch)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	// Verify Charlie was NOT created (transaction rolled back).
	var count int
	err := pg.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM authors WHERE name = 'Charlie'").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestRPCFunctionReturningNULL(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupTestServer(t, ctx)

	// Create a function that returns NULL.
	_, err := pg.Pool.Exec(ctx, `
		CREATE FUNCTION always_null() RETURNS TEXT AS $$
			SELECT NULL::TEXT;
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))
	cfg := config.Default()
	srv = server.New(cfg, logger, ch, pg.Pool, nil, nil)

	w := doRequest(t, srv, "POST", "/api/rpc/always_null", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// A function returning NULL should produce a JSON null response.
	testutil.Equal(t, "null\n", w.Body.String())
}

// --- Aggregate query tests ---

func setupAggregateTestServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()

	resetAndSeedDB(t, ctx)

	// Add products table with numeric columns for aggregate testing.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC NOT NULL,
			quantity INTEGER NOT NULL,
			category TEXT NOT NULL,
			active BOOLEAN DEFAULT true
		);
		INSERT INTO products (name, price, quantity, category, active) VALUES
			('Widget A', 10.50, 100, 'electronics', true),
			('Widget B', 25.00, 50,  'electronics', true),
			('Gadget C', 5.75,  200, 'toys',        true),
			('Gadget D', 15.00, 75,  'toys',        false),
			('Doohickey', 100.00, 10, 'electronics', false);
	`)
	if err != nil {
		t.Fatalf("creating products table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	return srv, sharedPG
}

func jsonResults(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["results"].([]any)
	if !ok {
		t.Fatalf("expected results array, got %T", body["results"])
	}
	results := make([]map[string]any, len(raw))
	for i, v := range raw {
		results[i] = v.(map[string]any)
	}
	return results
}

func TestAggregateCount(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 1, len(results))
	testutil.Equal(t, 5.0, jsonNum(t, results[0]["count"]))
}

func TestAggregateSumAvgGroupBy(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=sum(price),avg(price)&group=category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 2, len(results)) // electronics, toys

	// Build a map by category for deterministic checking.
	byCat := make(map[string]map[string]any)
	for _, r := range results {
		cat := jsonStr(t, r["category"])
		byCat[cat] = r
	}

	// electronics: 10.50 + 25.00 + 100.00 = 135.50, avg = 45.1666...
	elec := byCat["electronics"]
	testutil.NotNil(t, elec)
	sumPrice, _ := elec["sum_price"].(float64)
	testutil.True(t, sumPrice > 135.0 && sumPrice < 136.0, "expected sum_price ~135.5")

	// toys: 5.75 + 15.00 = 20.75, avg = 10.375
	toys := byCat["toys"]
	testutil.NotNil(t, toys)
	toysSum, _ := toys["sum_price"].(float64)
	testutil.True(t, toysSum > 20.0 && toysSum < 21.0, "expected toys sum_price ~20.75")
}

func TestAggregateWithFilter(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count&filter=category%3D'electronics'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 1, len(results))
	testutil.Equal(t, 3.0, jsonNum(t, results[0]["count"])) // 3 electronics
}

func TestAggregateSearchFuzzyBehavior(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupAggregateTestServer(t, ctx)
	if !pgTrgmInstalled(t, ctx, pg) {
		t.Fatal("shared migration/setup path must install pg_trgm for fuzzy aggregate coverage")
	}

	t.Run("fuzzy_true_matches_typo", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count&search=Doohike&fuzzy=true", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)
		body := parseJSON(t, w)
		results := jsonResults(t, body)
		testutil.Equal(t, 1, len(results))
		testutil.Equal(t, 1.0, jsonNum(t, results[0]["count"]))
	})

	t.Run("fuzzy_false_does_not_match_typo", func(t *testing.T) {
		w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count&search=Doohike&fuzzy=false", nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)
		body := parseJSON(t, w)
		results := jsonResults(t, body)
		testutil.Equal(t, 1, len(results))
		testutil.Equal(t, 0.0, jsonNum(t, results[0]["count"]))
	})
}

func TestAggregateSearchRejectsInvalidFuzzyParams(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	tests := []struct {
		name     string
		url      string
		contains string
	}{
		{
			name:     "fuzzy_without_search",
			url:      "/api/collections/products/?aggregate=count&fuzzy=true",
			contains: "fuzzy",
		},
		{
			name:     "fuzzy_invalid_boolean",
			url:      "/api/collections/products/?aggregate=count&search=widget&fuzzy=notabool",
			contains: "boolean",
		},
		{
			name:     "typo_threshold_numeric",
			url:      "/api/collections/products/?aggregate=count&search=widget&typo_threshold=0.5",
			contains: "unsupported parameter",
		},
		{
			name:     "typo_threshold_with_fuzzy",
			url:      "/api/collections/products/?aggregate=count&search=widget&fuzzy=true&typo_threshold=0.5",
			contains: "unsupported parameter",
		},
		{
			name:     "highlight",
			url:      "/api/collections/products/?aggregate=count&search=widget&highlight=true",
			contains: "unsupported parameter",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, srv, "GET", tc.url, nil)
			testutil.StatusCode(t, http.StatusBadRequest, w.Code)
			body := parseJSON(t, w)
			testutil.Contains(t, strings.ToLower(jsonStr(t, body["message"])), strings.ToLower(tc.contains))
		})
	}
}

func TestAggregateMinMax(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=min(price),max(price)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 1, len(results))
	minPrice, _ := results[0]["min_price"].(float64)
	maxPrice, _ := results[0]["max_price"].(float64)
	testutil.True(t, minPrice > 5.0 && minPrice < 6.0, "expected min_price ~5.75")
	testutil.True(t, maxPrice > 99.0 && maxPrice < 101.0, "expected max_price ~100.0")
}

func TestAggregateCountDistinct(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count_distinct(category)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 1, len(results))
	testutil.Equal(t, 2.0, jsonNum(t, results[0]["count_distinct_category"])) // electronics, toys
}

func TestAggregateResponseHasQueryDurationHeader(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	header := w.Header().Get("X-Query-Duration-Ms")
	if header == "" {
		t.Fatalf("expected X-Query-Duration-Ms header")
	}

	ms, err := strconv.ParseInt(header, 10, 64)
	testutil.NoError(t, err)
	testutil.True(t, ms >= 0)
}

func TestAggregateInvalidExpressionIntegration(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	// Unknown function.
	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=median(price)", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	body := parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "unknown aggregate function")

	// Invalid column.
	w = doRequest(t, srv, "GET", "/api/collections/products/?aggregate=sum(nonexistent)", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	body = parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "unknown column")

	// Type mismatch: sum on text column.
	w = doRequest(t, srv, "GET", "/api/collections/products/?aggregate=sum(name)", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	body = parseJSON(t, w)
	testutil.Contains(t, jsonStr(t, body["message"]), "numeric")
}

func TestAggregateMultipleGroupColumns(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count&group=category,active", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	// electronics: 2 active + 1 inactive = 2 groups
	// toys: 1 active + 1 inactive = 2 groups
	// Total: 4 groups
	testutil.Equal(t, 4, len(results))
}

func TestAggregateCountWithFilterActiveOnly(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?aggregate=count&filter=active%3Dtrue", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	results := jsonResults(t, body)
	testutil.Equal(t, 1, len(results))
	testutil.Equal(t, 3.0, jsonNum(t, results[0]["count"])) // 3 active products
}

func TestAggregateCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupAggregateTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/?aggregate=count", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Cursor (keyset) pagination tests ---

func setupCursorTestServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()

	resetAndSeedDB(t, ctx)

	// Create a table with 30 rows for cursor pagination testing.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE items (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			category TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("creating items table: %v", err)
	}

	// Insert 30 rows with distinct timestamps.
	for i := 1; i <= 30; i++ {
		_, err := sharedPG.Pool.Exec(ctx,
			`INSERT INTO items (title, category, created_at) VALUES ($1, $2, $3)`,
			fmt.Sprintf("Item %02d", i),
			map[bool]string{true: "A", false: "B"}[i%3 != 0], // 20 A's, 10 B's
			fmt.Sprintf("2024-01-%02dT00:00:00Z", i),
		)
		if err != nil {
			t.Fatalf("inserting item %d: %v", i, err)
		}
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
}

func TestCursorPaginationForward(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	// Collect all items across multiple cursor pages.
	var allIDs []float64
	nextCursor := ""
	pageCount := 0

	for {
		url := "/api/collections/items/?cursor=" + nextCursor + "&perPage=7&sort=-created_at"
		w := doRequest(t, srv, "GET", url, nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)

		if len(items) == 0 && nextCursor == "" {
			t.Fatal("first page returned zero items")
		}

		for _, item := range items {
			allIDs = append(allIDs, jsonNum(t, item["id"]))
		}

		cursor, ok := body["nextCursor"].(string)
		if !ok || cursor == "" {
			break
		}
		nextCursor = cursor
		pageCount++

		if pageCount > 10 {
			t.Fatal("too many pages — possible infinite loop")
		}
	}

	// Should have collected all 30 items.
	if len(allIDs) != 30 {
		t.Fatalf("expected 30 items total, got %d", len(allIDs))
	}

	// No duplicates.
	seen := make(map[float64]bool)
	for _, id := range allIDs {
		if seen[id] {
			t.Fatalf("duplicate ID %.0f", id)
		}
		seen[id] = true
	}

	// Verify descending order (created_at DESC → higher IDs first).
	for i := 1; i < len(allIDs); i++ {
		if allIDs[i] >= allIDs[i-1] {
			t.Fatalf("items not in descending order at index %d: %.0f >= %.0f", i, allIDs[i], allIDs[i-1])
		}
	}
}

func TestCursorPaginationWithFilter(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	// Paginate only category A items.
	var allIDs []float64
	nextCursor := ""

	for {
		url := "/api/collections/items/?cursor=" + nextCursor + "&perPage=5&sort=id&filter=" + "category%3D'A'"
		w := doRequest(t, srv, "GET", url, nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)

		for _, item := range items {
			allIDs = append(allIDs, jsonNum(t, item["id"]))
			cat := jsonStr(t, item["category"])
			if cat != "A" {
				t.Fatalf("expected category A, got %s", cat)
			}
		}

		cursor, ok := body["nextCursor"].(string)
		if !ok || cursor == "" {
			break
		}
		nextCursor = cursor
	}

	// 20 of 30 items are category A.
	if len(allIDs) != 20 {
		t.Fatalf("expected 20 category A items, got %d", len(allIDs))
	}
}

func TestCursorPaginationWithSearch(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	// Search for "Item 0" (matches Item 01..09).
	var allTitles []string
	nextCursor := ""

	for {
		url := "/api/collections/items/?cursor=" + nextCursor + "&perPage=3&sort=id&search=Item"
		w := doRequest(t, srv, "GET", url, nil)
		testutil.StatusCode(t, http.StatusOK, w.Code)

		body := parseJSON(t, w)
		items := jsonItems(t, body)

		for _, item := range items {
			allTitles = append(allTitles, jsonStr(t, item["title"]))
		}

		cursor, ok := body["nextCursor"].(string)
		if !ok || cursor == "" {
			break
		}
		nextCursor = cursor
	}

	// "Item" matches all 30 items.
	if len(allTitles) != 30 {
		t.Fatalf("expected 30 search results, got %d: %v", len(allTitles), allTitles)
	}
}

func TestCursorPaginationWithSearchHighlightOnPosts(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?cursor=&perPage=1&sort=id&search=hello&highlight=true", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "First Post", jsonStr(t, items[0]["title"]))
	testutil.Contains(t, jsonStr(t, items[0]["_highlight"]), "<b>Hello</b> world")
}

func TestCursorPaginationIncludesFacetsWhenRequested(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/items/?cursor=&perPage=5&sort=id&facets=category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	if _, ok := body["facets"]; !ok {
		t.Fatal("expected cursor response to include facets when requested")
	}
}

func TestCursorMutualExclusionIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/items/?cursor=abc&page=2", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestCursorInvalidIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupCursorTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/items/?cursor=not-valid-base64!!!", nil)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	// Should not be a 500.
	body := parseJSON(t, w)
	msg := jsonStr(t, body["message"])
	if msg == "internal error" {
		t.Fatal("expected user-friendly error, got 'internal error'")
	}
}

// --- Export tests ---

// setupExportTestServer seeds a posts table with known data including CSV edge cases.
func setupExportTestServerWithAPILimits(t *testing.T, ctx context.Context, apiCfg *config.APIConfig) *server.Server {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	// Re-apply shared migrations so system tables (e.g. _ayb_search_synonyms)
	// that live in public are present for the duration of the export tests.
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations after schema reset: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations after schema reset: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			status TEXT DEFAULT 'draft'
		);
		INSERT INTO posts (title, body, status) VALUES
			('First Post', 'Hello world', 'published'),
			('Second Post', 'Another post', 'draft'),
			('Third Post', 'By Bob', 'published'),
			('Comma, in title', 'body with "quotes"', 'published'),
			('Newline
in title', 'normal body', 'draft');
	`)
	if err != nil {
		t.Fatalf("creating export test schema: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	if apiCfg != nil {
		cfg.API = *apiCfg
	}
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	return srv
}

func setupExportTestServer(t *testing.T, ctx context.Context) *server.Server {
	return setupExportTestServerWithAPILimits(t, ctx, nil)
}

func TestExportCSVIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.csv", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Verify content type.
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Fatalf("expected text/csv content type, got %s", ct)
	}

	// Verify content disposition.
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "posts.csv") {
		t.Fatalf("expected posts.csv in Content-Disposition, got %s", cd)
	}

	// Parse the CSV and verify.
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}

	// Header + 5 data rows.
	if len(records) != 6 {
		t.Fatalf("expected 6 CSV rows (1 header + 5 data), got %d", len(records))
	}

	// Verify header contains expected column names.
	header := records[0]
	headerStr := strings.Join(header, ",")
	if !strings.Contains(headerStr, "id") || !strings.Contains(headerStr, "title") {
		t.Fatalf("expected id and title in header: %v", header)
	}
}

func TestExportJSONIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.json", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Verify content type.
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json content type, got %s", ct)
	}

	// Verify content disposition.
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "posts.json") {
		t.Fatalf("expected posts.json in Content-Disposition, got %s", cd)
	}

	// Parse the JSON array.
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing JSON array: %v\nbody: %s", err, w.Body.String())
	}

	if len(items) != 5 {
		t.Fatalf("expected 5 JSON items, got %d", len(items))
	}

	// Verify first item has expected fields.
	titles := make(map[string]bool)
	for _, item := range items {
		title, ok := item["title"].(string)
		if !ok {
			t.Fatalf("expected title string, got %T", item["title"])
		}
		titles[title] = true
	}
	if !titles["First Post"] || !titles["Second Post"] || !titles["Third Post"] {
		t.Fatalf("missing expected titles in JSON export: %v", titles)
	}
}

func TestExportCSVFilteredIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.csv?filter=status%3D'published'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing filtered CSV: %v", err)
	}

	// Header + 3 published rows (First Post, Third Post, Comma in title).
	if len(records) != 4 {
		t.Fatalf("expected 4 CSV rows (1 header + 3 published), got %d\nbody:\n%s", len(records), w.Body.String())
	}
}

func TestExportCSVEdgeCasesIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.csv?sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Parse the CSV — encoding/csv handles RFC 4180 quoting automatically.
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV with edge cases: %v", err)
	}

	// Find the title column index.
	header := records[0]
	titleIdx := -1
	for i, col := range header {
		if col == "title" {
			titleIdx = i
			break
		}
	}
	if titleIdx == -1 {
		t.Fatalf("title column not found in header: %v", header)
	}

	// Verify edge case values are correctly parsed.
	foundComma := false
	foundNewline := false
	for _, row := range records[1:] {
		title := row[titleIdx]
		if strings.Contains(title, "Comma, in title") {
			foundComma = true
		}
		if strings.Contains(title, "Newline\nin title") {
			foundNewline = true
		}
	}
	if !foundComma {
		t.Fatal("CSV did not correctly handle embedded comma in title")
	}
	if !foundNewline {
		t.Fatal("CSV did not correctly handle embedded newline in title")
	}
}

func TestExportCSVBodyWithQuotesIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.csv?sort=id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}

	// Find body column index.
	header := records[0]
	bodyIdx := -1
	for i, col := range header {
		if col == "body" {
			bodyIdx = i
			break
		}
	}
	if bodyIdx == -1 {
		t.Fatalf("body column not found in header: %v", header)
	}

	// Row 4 (0-indexed data row 3) has body with double quotes.
	foundQuotes := false
	for _, row := range records[1:] {
		if strings.Contains(row[bodyIdx], `"quotes"`) {
			foundQuotes = true
			break
		}
	}
	if !foundQuotes {
		t.Fatal("CSV did not correctly handle embedded double quotes in body")
	}
}

func TestExportNonexistentTableIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/nonexistent/export.csv", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)

	w = doRequest(t, srv, "GET", "/api/collections/nonexistent/export.json", nil)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

func TestExportJSONEmptyResultIntegration(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServer(t, ctx)

	// Filter that matches no rows.
	w := doRequest(t, srv, "GET", "/api/collections/posts/export.json?filter=status%3D'archived'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Should return a valid empty JSON array.
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("parsing empty JSON array: %v\nbody: %s", err, w.Body.String())
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for non-matching filter, got %d", len(items))
	}
}

func TestExportCSVRespectsExportMaxRowsLimit(t *testing.T) {
	ctx := context.Background()
	srv := setupExportTestServerWithAPILimits(t, ctx, &config.APIConfig{ExportMaxRows: 2})

	w := doRequest(t, srv, "GET", "/api/collections/posts/export.csv", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 CSV rows (1 header + 2 data) with export limit=2, got %d", len(records))
	}
}

// --- Import integration tests ---

func doImportRequest(t *testing.T, srv *server.Server, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func setupImportTestServerWithAPILimits(t *testing.T, ctx context.Context, apiCfg *config.APIConfig) *server.Server {
	t.Helper()
	resetAndSeedDB(t, ctx)

	// Create a dedicated import_items table with PK and unique constraints.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE import_items (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT DEFAULT 'general'
		);
		INSERT INTO import_items (id, name, category) VALUES
			(100, 'Existing A', 'alpha'),
			(200, 'Existing B', 'beta');
	`)
	if err != nil {
		t.Fatalf("creating import_items table: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}
	cfg := config.Default()
	if apiCfg != nil {
		cfg.API = *apiCfg
	}
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
}

func setupImportTestServer(t *testing.T, ctx context.Context) *server.Server {
	return setupImportTestServerWithAPILimits(t, ctx, nil)
}

func TestImportCSVFullModeSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	csvBody := "id,name,category\n1,Item One,cat1\n2,Item Two,cat2\n3,Item Three,cat3\n"
	w := doImportRequest(t, srv, "/api/collections/import_items/import", "text/csv", csvBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(3), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(3), jsonNum(t, result["inserted"]))
	testutil.Equal(t, float64(0), jsonNum(t, result["failed"]))

	// Verify data in DB.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id IN (1,2,3)").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, count)
}

func TestImportJSONFullModeSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	jsonBody := `[{"id":10,"name":"JSON One","category":"j1"},{"id":11,"name":"JSON Two","category":"j2"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import", "application/json", jsonBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(2), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(2), jsonNum(t, result["inserted"]))

	// Verify data.
	var name string
	err := sharedPG.Pool.QueryRow(ctx, "SELECT name FROM import_items WHERE id = 10").Scan(&name)
	testutil.NoError(t, err)
	testutil.Equal(t, "JSON One", name)
}

func TestImportOnConflictSkip(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// id=100 already exists. With skip, it should be skipped.
	jsonBody := `[{"id":100,"name":"Should Skip","category":"new"},{"id":300,"name":"New Item","category":"new"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import?on_conflict=skip", "application/json", jsonBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(2), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(1), jsonNum(t, result["inserted"]))
	testutil.Equal(t, float64(1), jsonNum(t, result["skipped"]))

	// Verify existing row was NOT updated.
	var name string
	err := sharedPG.Pool.QueryRow(ctx, "SELECT name FROM import_items WHERE id = 100").Scan(&name)
	testutil.NoError(t, err)
	testutil.Equal(t, "Existing A", name)

	// Verify new row was inserted.
	err = sharedPG.Pool.QueryRow(ctx, "SELECT name FROM import_items WHERE id = 300").Scan(&name)
	testutil.NoError(t, err)
	testutil.Equal(t, "New Item", name)
}

func TestImportOnConflictUpdate(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// id=100 already exists. With update, it should be updated.
	jsonBody := `[{"id":100,"name":"Updated A","category":"updated"},{"id":400,"name":"Brand New","category":"new"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import?on_conflict=update", "application/json", jsonBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(2), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(2), jsonNum(t, result["inserted"]))

	// Verify existing row WAS updated.
	var name, category string
	err := sharedPG.Pool.QueryRow(ctx, "SELECT name, category FROM import_items WHERE id = 100").Scan(&name, &category)
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated A", name)
	testutil.Equal(t, "updated", category)

	// Verify new row was inserted.
	err = sharedPG.Pool.QueryRow(ctx, "SELECT name FROM import_items WHERE id = 400").Scan(&name)
	testutil.NoError(t, err)
	testutil.Equal(t, "Brand New", name)
}

func TestImportPartialModeWithErrors(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// id=100 already exists (unique violation in full insert mode).
	// In partial mode, the duplicate row should fail, and the rest should succeed.
	jsonBody := `[{"id":500,"name":"Good One","category":"ok"},{"id":100,"name":"Dup","category":"dup"},{"id":501,"name":"Good Two","category":"ok"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import?mode=partial", "application/json", jsonBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	result := parseJSON(t, w)
	testutil.Equal(t, float64(3), jsonNum(t, result["processed"]))
	testutil.Equal(t, float64(2), jsonNum(t, result["inserted"]))
	testutil.Equal(t, float64(1), jsonNum(t, result["failed"]))

	// Verify valid rows were committed.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id IN (500, 501)").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)
}

func TestImportFullModeRollbackOnError(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServer(t, ctx)

	// First row is valid, second row is a duplicate of existing id=100.
	// In full mode, ALL rows should be rolled back.
	jsonBody := `[{"id":600,"name":"Should Rollback","category":"test"},{"id":100,"name":"Dup","category":"dup"}]`
	w := doImportRequest(t, srv, "/api/collections/import_items/import", "application/json", jsonBody)
	// Expect conflict or error status.
	testutil.True(t, w.Code >= 400, "expected error status, got %d", w.Code)

	// Verify the first row was NOT committed (rollback).
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id = 600").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestImportOversizedBody413(t *testing.T) {
	ctx := context.Background()
	srv := setupImportTestServerWithAPILimits(t, ctx, &config.APIConfig{ImportMaxSizeMB: 1})

	// Generate a body larger than the configured import_max_size_mb (1MB).
	// Use a simple repeating pattern.
	bigBody := strings.Repeat(`{"id":1,"name":"x","category":"y"},`, 500000) // ~17MB
	bigBody = "[" + bigBody[:len(bigBody)-1] + "]"

	w := doImportRequest(t, srv, "/api/collections/import_items/import", "application/json", bigBody)
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, w.Code)

	// Verify no rows were written.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM import_items WHERE id = 1").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

// --- Search synonym expansion tests (Stage 2) ---
//
// These tests exercise per-collection synonym expansion against the
// _ayb_search_synonyms table created by migration 174. Memberships are scoped by
// (schema_name, table_name); within a collection, all rows that share a
// group_id are treated as mutual synonyms during FTS expansion.

// setupSearchSynonymServer seeds the standard fixture, appends rows whose
// content uses the term pair scifi / "science fiction", and reloads the schema
// cache so the API can see the new rows.
func setupSearchSynonymServer(t *testing.T, ctx context.Context) (*server.Server, *testutil.PGContainer) {
	t.Helper()
	srv, pg := setupTestServer(t, ctx)

	// Use independent rows so each can only be matched via synonym expansion
	// from the other's literal term.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status) VALUES
			('Cyberpunk',       'a classic scifi tale',          1, 'published'),
			('Foundation',      'a classic science fiction novel', 1, 'published'),
			('Romance Read',    'a cozy romance novel',          2, 'draft')
	`)
	testutil.NoError(t, err)
	return srv, pg
}

// insertSearchSynonymGroup writes a single synonym group into
// _ayb_search_synonyms for the given collection. Terms are lowercased to
// satisfy the chk_ayb_search_synonyms_term_lowercase migration constraint.
func insertSearchSynonymGroup(t *testing.T, ctx context.Context, pg *testutil.PGContainer, schemaName, tableName string, terms ...string) {
	t.Helper()
	var groupID string
	err := pg.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&groupID)
	testutil.NoError(t, err)
	for _, term := range terms {
		_, err := pg.Pool.Exec(ctx, `
			INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
			VALUES ($1, $2, $3, lower($4))
		`, schemaName, tableName, groupID, term)
		testutil.NoError(t, err)
	}
}

func searchTitles(t *testing.T, srv *server.Server, query string) []string {
	t.Helper()
	w := doRequest(t, srv, "GET", "/api/collections/posts/?"+query, nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	items := jsonItems(t, body)
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = jsonStr(t, item["title"])
	}
	return titles
}

// TestSearchSynonymsBidirectionalExpansion proves that searching either
// configured term returns rows whose content matches the other term.
func TestSearchSynonymsBidirectionalExpansion(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	insertSearchSynonymGroup(t, ctx, pg, "public", "posts", "scifi", "science fiction")

	// Searching the single-word term must return the row that only mentions
	// the multi-word term, and vice versa.
	t.Run("single_term_expands_to_phrase", func(t *testing.T) {
		titles := searchTitles(t, srv, "search=scifi")
		// Should contain both Cyberpunk (literal scifi match) and Foundation (synonym).
		want := map[string]bool{"Cyberpunk": true, "Foundation": true}
		got := map[string]bool{}
		for _, title := range titles {
			got[title] = true
		}
		for title := range want {
			if !got[title] {
				t.Fatalf("expected scifi search to include %q (synonym expansion); got %v", title, titles)
			}
		}
	})

	t.Run("phrase_expands_to_single_term", func(t *testing.T) {
		// URL-encoded "science fiction" with quotes preserved would over-narrow
		// to phrase semantics; use the unquoted form here (multi-term AND).
		titles := searchTitles(t, srv, "search=science+fiction")
		got := map[string]bool{}
		for _, title := range titles {
			got[title] = true
		}
		if !got["Foundation"] {
			t.Fatalf("expected baseline match for Foundation; got %v", titles)
		}
		if !got["Cyberpunk"] {
			t.Fatalf("expected synonym expansion to include Cyberpunk; got %v", titles)
		}
	})
}

// TestSearchSynonymsScopedByCollection proves that a synonym configured for a
// different collection does not leak into the queried collection.
func TestSearchSynonymsScopedByCollection(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	// Register an expansion that would, if leaked, match the 'Romance Read'
	// row by expanding 'scifi' to 'romance'. But it's scoped to a different
	// table, so it must not affect /api/collections/posts/.
	insertSearchSynonymGroup(t, ctx, pg, "public", "authors", "scifi", "romance")

	// Register the same leak-prone expansion for a same-named table in another
	// schema. If buildSearchSQL scoped only by table_name, this would also
	// broaden the public.posts request to the Romance Read row.
	insertSearchSynonymGroup(t, ctx, pg, "app", "posts", "scifi", "romance")

	titles := searchTitles(t, srv, "search=scifi")
	for _, title := range titles {
		if title == "Romance Read" {
			t.Fatalf("synonym from another collection leaked into public.posts query; got %v", titles)
		}
	}
}

// TestSearchSynonymsMultiTermAndSemanticsPreserved proves that combining a
// synonym with another required term still uses AND semantics: 'classic scifi'
// must keep returning only rows that contain both 'classic' and a scifi-group
// term, not all rows that contain any scifi-group term.
func TestSearchSynonymsMultiTermAndSemanticsPreserved(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	insertSearchSynonymGroup(t, ctx, pg, "public", "posts", "scifi", "science fiction")

	// Add a row that contains 'scifi' but NOT 'classic'; multi-term AND must
	// keep it out of results.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status)
		VALUES ('Lone Scifi', 'just a scifi reference', 1, 'published')
	`)
	testutil.NoError(t, err)

	titles := searchTitles(t, srv, "search=classic+scifi")
	got := map[string]bool{}
	for _, title := range titles {
		got[title] = true
	}
	for _, want := range []string{"Cyberpunk", "Foundation"} {
		if !got[want] {
			t.Fatalf("multi-term AND with synonym expansion missing %q; got %v", want, titles)
		}
	}
	if got["Lone Scifi"] {
		t.Fatalf("multi-term AND should have excluded 'Lone Scifi' (no 'classic'); got %v", titles)
	}
}

// TestSearchSynonymsQuotedPhrasePreserved proves that quoted-phrase syntax
// keeps websearch_to_tsquery phrase semantics on the literal input; the synonym
// expansion must not broaden quoted phrases into unrelated rows that happen to
// contain the phrase's words separately.
func TestSearchSynonymsQuotedPhrasePreserved(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	insertSearchSynonymGroup(t, ctx, pg, "public", "posts", "scifi", "science fiction")

	// Row whose body contains 'science' and 'fiction' as separate tokens but
	// NOT adjacent, so it should fail the quoted-phrase predicate even after
	// synonym expansion.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status)
		VALUES ('Mixed Words', 'pure science is a discipline; fiction is a genre', 1, 'published')
	`)
	testutil.NoError(t, err)

	// %22 = " quote
	titles := searchTitles(t, srv, "search=%22science+fiction%22")
	got := map[string]bool{}
	for _, title := range titles {
		got[title] = true
	}
	if got["Mixed Words"] {
		t.Fatalf("quoted-phrase search must not match non-adjacent words; got %v", titles)
	}
	if !got["Foundation"] {
		t.Fatalf("quoted-phrase search should still match literal 'science fiction'; got %v", titles)
	}
	if !got["Cyberpunk"] {
		t.Fatalf("quoted-phrase search should expand 'science fiction' synonym to scifi; got %v", titles)
	}
}

// TestSearchSynonymsBaselineUnchanged proves that without any synonym rows
// configured, search behavior matches the existing baseline counts.
func TestSearchSynonymsBaselineUnchanged(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	// Sanity: confirm the synonyms table exists but is empty for this test.
	var count int
	err := sharedPG.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_search_synonyms`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)

	// Reuse the baseline TestSearchBasic assertions inline: searching 'Bob'
	// returns exactly one row.
	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=Bob", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "Bob Post", jsonStr(t, items[0]["title"]))
}

// TestSearchSynonymsHighlight proves that synonym-matched rows can still return
// a properly escaped _highlight field; the highlight selector must not corrupt
// the stored text or leak unescaped markup.
func TestSearchSynonymsHighlight(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	insertSearchSynonymGroup(t, ctx, pg, "public", "posts", "scifi", "science fiction")

	// Insert a row that matches only via synonym expansion AND contains
	// dangerous HTML in its stored text.
	_, err := pg.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status)
		VALUES ('Markup', 'science fiction <script>alert(1)</script>', 1, 'published')
	`)
	testutil.NoError(t, err)

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=scifi&highlight=true", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	items := jsonItems(t, body)

	foundMarkup := false
	for _, item := range items {
		if jsonStr(t, item["title"]) == "Markup" {
			foundMarkup = true
			highlight := jsonStr(t, item["_highlight"])
			testutil.Contains(t, highlight, "&lt;script&gt;alert(1)&lt;/script&gt;")
			if strings.Contains(highlight, "<script>") {
				t.Fatalf("synonym-expanded highlight leaked raw <script>: %q", highlight)
			}
		}
	}
	if !foundMarkup {
		titles := make([]string, len(items))
		for i, item := range items {
			titles[i] = jsonStr(t, item["title"])
		}
		t.Fatalf("expected synonym expansion to include 'Markup' row; got %v", titles)
	}
}

// TestSearchSynonymsFacetCountsReflectExpansion proves that facet buckets are
// computed over the expanded result set, not the literal-match-only set.
func TestSearchSynonymsFacetCountsReflectExpansion(t *testing.T) {
	ctx := context.Background()
	srv, pg := setupSearchSynonymServer(t, ctx)
	insertSearchSynonymGroup(t, ctx, pg, "public", "posts", "scifi", "science fiction")

	w := doRequest(t, srv, "GET", "/api/collections/posts/?search=scifi&facets=author_id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	items := jsonItems(t, body)

	// Confirm both rows are present so the facet count is meaningful.
	if len(items) < 2 {
		titles := make([]string, len(items))
		for i, item := range items {
			titles[i] = jsonStr(t, item["title"])
		}
		t.Fatalf("expected at least 2 expanded matches; got %v", titles)
	}

	facets, ok := body["facets"].(map[string]any)
	testutil.True(t, ok, "expected facets object")
	authorRaw, ok := facets["author_id"].([]any)
	testutil.True(t, ok, "expected author_id facet bucket")

	// Both Cyberpunk and Foundation are author_id=1, so the bucket for author 1
	// must count at least 2.
	for _, raw := range authorRaw {
		bucket := raw.(map[string]any)
		if jsonNum(t, bucket["value"]) == 1.0 {
			if jsonNum(t, bucket["count"]) < 2.0 {
				t.Fatalf("expected author_id=1 facet count to include expanded rows; got %v", bucket["count"])
			}
			return
		}
	}
	t.Fatalf("expected an author_id=1 facet bucket; got %v", authorRaw)
}
