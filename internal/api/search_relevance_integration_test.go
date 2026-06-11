//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsettings"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSearchRelevanceWeightedAttributesChangeOrdering(t *testing.T) {
	ctx := context.Background()
	srv, _ := setupTestServer(t, ctx)

	store := searchsettings.NewStore(sharedPG.Pool)
	savedSettings := searchsettings.Settings{
		Attributes: []searchsettings.Attribute{
			{Column: "title", Weight: searchsettings.WeightHigh},
			{Column: "body", Weight: searchsettings.WeightLow},
		},
	}
	testutil.NoError(t, store.Save(ctx, "public", "posts", savedSettings))
	_, err := sharedPG.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status) VALUES
			('weightedneedle', 'ordinary body', 1, 'published'),
			('ordinary title', 'weightedneedle', 1, 'published')
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	testutil.NoError(t, ch.ReloadWait(ctx))
	tbl := ch.Get().TableByName("posts")
	testutil.NotNil(t, tbl)

	loadedSettings, err := store.Load(ctx, "public", "posts")
	testutil.NoError(t, err)
	_, rankSQL, args, err := api.BuildSearchSQLPartsForIntegrationTest(tbl, "weightedneedle", 1, loadedSettings)
	testutil.NoError(t, err)

	rows, err := sharedPG.Pool.Query(ctx, fmt.Sprintf(`
		SELECT title, %s AS rank
		FROM posts
		WHERE title IN ('weightedneedle', 'ordinary title')
		ORDER BY id
	`, rankSQL), args...)
	testutil.NoError(t, err)
	defer rows.Close()

	var titleRank float64
	var bodyRank float64
	for rows.Next() {
		var title string
		var rank float64
		testutil.NoError(t, rows.Scan(&title, &rank))
		if title == "weightedneedle" {
			titleRank = rank
		}
		if title == "ordinary title" {
			bodyRank = rank
		}
	}
	testutil.NoError(t, rows.Err())
	testutil.True(t, titleRank > bodyRank, "expected title match rank %v to exceed body match rank %v", titleRank, bodyRank)

	w := doRequest(t, srv, http.MethodGet, "/api/collections/posts/?search=weightedneedle", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	items := jsonItems(t, parseJSON(t, w))
	testutil.Equal(t, "weightedneedle", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, "ordinary title", jsonStr(t, items[1]["title"]))
}

func TestSearchRelevanceWithoutSettingsKeepsEqualRanks(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status) VALUES
			('plainneedle', 'ordinary body', 1, 'published'),
			('ordinary title', 'plainneedle', 1, 'published')
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	tbl := ch.Get().TableByName("posts")
	testutil.NotNil(t, tbl)

	whereSQL, rankSQL, args, err := api.BuildSearchSQLPartsForIntegrationTest(tbl, "plainneedle", 1, searchsettings.Settings{})
	testutil.NoError(t, err)

	rows, err := sharedPG.Pool.Query(ctx,
		fmt.Sprintf(`SELECT title, %s AS rank FROM posts WHERE %s ORDER BY id`, rankSQL, whereSQL),
		args...,
	)
	testutil.NoError(t, err)
	defer rows.Close()

	ranks := map[string]float64{}
	for rows.Next() {
		var title string
		var rank float64
		testutil.NoError(t, rows.Scan(&title, &rank))
		ranks[title] = rank
	}
	testutil.NoError(t, rows.Err())
	testutil.Equal(t, ranks["plainneedle"], ranks["ordinary title"])
}

func TestSearchCustomRankingBreaksRelevanceTiesButNotPrimaryRelevance(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		ALTER TABLE posts ADD COLUMN popularity INTEGER NOT NULL DEFAULT 0;
		INSERT INTO posts (title, body, author_id, status, popularity) VALUES
			('needle needle needle', 'same body', 1, 'published', 1),
			('needle', 'same body', 1, 'published', 99),
			('needle', 'same body', 1, 'published', 10)
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(config.Default(), logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, http.MethodGet, "/api/collections/posts/?search=needle&sort=-popularity&perPage=2&cursor=", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.Equal(t, "needle needle needle", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, 99.0, jsonNum(t, items[1]["popularity"]))

	cursor := jsonStr(t, body["nextCursor"])
	w = doRequest(t, srv, http.MethodGet, "/api/collections/posts/?search=needle&sort=-popularity&perPage=2&cursor="+cursor, nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	items = jsonItems(t, parseJSON(t, w))
	testutil.Equal(t, 10.0, jsonNum(t, items[0]["popularity"]))
}

func TestSearchCustomRankingAscendingSortBreaksRelevanceTies(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		ALTER TABLE posts ADD COLUMN popularity INTEGER NOT NULL DEFAULT 0;
		INSERT INTO posts (title, body, author_id, status, popularity) VALUES
			('ascneedle ascneedle ascneedle', 'same body', 1, 'published', 99),
			('ascneedle', 'same body', 1, 'published', 20),
			('ascneedle', 'same body', 1, 'published', 5)
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(config.Default(), logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, http.MethodGet, "/api/collections/posts/?search=ascneedle&sort=popularity", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	items := jsonItems(t, parseJSON(t, w))
	testutil.Equal(t, "ascneedle ascneedle ascneedle", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, 5.0, jsonNum(t, items[1]["popularity"]))
	testutil.Equal(t, 20.0, jsonNum(t, items[2]["popularity"]))
}

func TestSearchCustomRankingSearchOnlyUsesRelevanceThenDefaultIDCursorOrder(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		ALTER TABLE posts ADD COLUMN popularity INTEGER NOT NULL DEFAULT 0;
		INSERT INTO posts (title, body, author_id, status, popularity) VALUES
			('onlyneedle onlyneedle onlyneedle', 'same body', 1, 'published', 1),
			('onlyneedle', 'same body', 1, 'published', 99),
			('onlyneedle', 'same body', 1, 'published', 10)
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(config.Default(), logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, http.MethodGet, "/api/collections/posts/?search=onlyneedle&perPage=3&cursor=", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	items := jsonItems(t, parseJSON(t, w))
	testutil.Equal(t, "onlyneedle onlyneedle onlyneedle", jsonStr(t, items[0]["title"]))
	testutil.Equal(t, 99.0, jsonNum(t, items[1]["popularity"]))
	testutil.Equal(t, 10.0, jsonNum(t, items[2]["popularity"]))
}

func TestWeightedSearchPlanUsesSearchIndexAfterReload(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	store := searchsettings.NewStore(sharedPG.Pool)
	savedSettings := searchsettings.Settings{
		Attributes: []searchsettings.Attribute{
			{Column: "title", Weight: searchsettings.WeightHigh},
			{Column: "body", Weight: searchsettings.WeightLow},
		},
	}
	testutil.NoError(t, store.Save(ctx, "public", "posts", savedSettings))
	_, err := sharedPG.Pool.Exec(ctx, `
		INSERT INTO posts (title, body, author_id, status)
		SELECT 'Weighted filler ' || gs, 'search filler body ' || gs, 1, 'published'
		FROM generate_series(1, 2500) AS gs;
		INSERT INTO posts (title, body, author_id, status) VALUES
			('weightedplanneedle', 'ordinary body', 1, 'published'),
			('ordinary title', 'weightedplanneedle', 1, 'published');
		ANALYZE posts;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, config.Default().API, logger)
	testutil.NoError(t, ch.Load(ctx))
	testutil.NoError(t, ch.ReloadWait(ctx))

	tbl := ch.Get().TableByName("posts")
	testutil.NotNil(t, tbl)
	loadedSettings, err := store.Load(ctx, "public", "posts")
	testutil.NoError(t, err)
	whereSQL, _, args, err := api.BuildSearchSQLPartsForIntegrationTest(tbl, "weightedplanneedle", 1, loadedSettings)
	testutil.NoError(t, err)

	query := fmt.Sprintf(`EXPLAIN (FORMAT JSON) SELECT id FROM posts WHERE %s`, whereSQL)
	var raw []byte
	err = sharedPG.Pool.QueryRow(ctx, query, args...).Scan(&raw)
	testutil.NoError(t, err)

	var explain []map[string]any
	testutil.NoError(t, json.Unmarshal(raw, &explain))
	if len(explain) == 0 || !searchPlanUsesIndex(explain[0]["Plan"]) {
		t.Fatalf("expected weighted search plan to use an index; predicate=%s plan: %s", whereSQL, string(raw))
	}
}
