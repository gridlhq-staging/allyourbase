//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminSQLMoviesSearchRPCAndSeedIdempotency(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	execSQL := func(t *testing.T, query string) map[string]any {
		t.Helper()
		body := `{"query":` + jsonString(query) + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		var payload map[string]any
		testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		return payload
	}

	moviesSchemaSQL, err := fs.ReadFile(examples.FS, "movies/schema.sql")
	testutil.NoError(t, err)
	execSQL(t, string(moviesSchemaSQL))

	moviesSeedSQL, err := fs.ReadFile(examples.FS, "movies/seed.sql")
	testutil.NoError(t, err)
	execSQL(t, string(moviesSeedSQL))
	execSQL(t, string(moviesSeedSQL))

	countPayload := execSQL(t, "SELECT COUNT(*) FROM movies")
	rows, ok := countPayload["rows"].([]any)
	testutil.True(t, ok, "expected rows in count response")
	testutil.True(t, len(rows) == 1, "expected exactly one count row")
	countRow := rows[0].([]any)
	countValue, ok := countRow[0].(float64)
	testutil.True(t, ok, "expected numeric movie count")
	testutil.True(t, countValue >= 250, "expected at least 250 seeded movies")

	emptyGenrePayload := execSQL(t, "SELECT COUNT(*) FROM movies WHERE primary_genre = ''")
	emptyGenreRows, ok := emptyGenrePayload["rows"].([]any)
	testutil.True(t, ok, "expected rows in primary_genre count response")
	testutil.True(t, len(emptyGenreRows) == 1, "expected exactly one primary_genre count row")
	emptyGenreCount, ok := emptyGenreRows[0].([]any)[0].(float64)
	testutil.True(t, ok, "expected numeric primary_genre count")
	testutil.Equal(t, float64(0), emptyGenreCount)

	notesPolicyPayload := execSQL(t, `SELECT COUNT(*) FROM pg_policies
		WHERE schemaname = 'public' AND tablename = 'movies_notes'`)
	notesPolicyRows, ok := notesPolicyPayload["rows"].([]any)
	testutil.True(t, ok, "expected rows in movies_notes policy response")
	testutil.True(t, len(notesPolicyRows) == 1, "expected exactly one movies_notes policy count row")
	notesPolicyCount, ok := notesPolicyRows[0].([]any)[0].(float64)
	testutil.True(t, ok, "expected numeric movies_notes policy count")
	// User-generated notes are written through the server-owned embed endpoint,
	// so the table should not expose a blanket public-read policy.
	testutil.Equal(t, float64(0), notesPolicyCount)

	searchPayload := execSQL(t, "SELECT slug, title FROM search_movies('dreams heist', '[0.90,0.10,0.20]'::vector(3), 3)")
	searchRows, ok := searchPayload["rows"].([]any)
	testutil.True(t, ok, "expected rows in search response")
	if len(searchRows) == 0 {
		t.Fatal("expected at least one row from search_movies")
	}
	firstRow := searchRows[0].([]any)
	testutil.Equal(t, "inception", firstRow[0])
}
