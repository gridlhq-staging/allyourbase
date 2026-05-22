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
	testutil.Equal(t, float64(3), countValue)

	searchPayload := execSQL(t, "SELECT slug, title FROM search_movies('dreams heist', '[0.90,0.10,0.20]'::vector(3), 3)")
	searchRows, ok := searchPayload["rows"].([]any)
	testutil.True(t, ok, "expected rows in search response")
	if len(searchRows) == 0 {
		t.Fatal("expected at least one row from search_movies")
	}
	firstRow := searchRows[0].([]any)
	testutil.Equal(t, "inception", firstRow[0])
}
