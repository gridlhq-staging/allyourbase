//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func setupDisjunctiveFacetServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndSeedDB(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			brand TEXT NOT NULL,
			price NUMERIC,
			discount NUMERIC
		);
		INSERT INTO products (name, category, brand, price, discount) VALUES
			('Guide Book One', 'books', 'acme', 10.50, NULL),
			('Guide Book Two', 'books', 'acme', 20.25, NULL),
			('Guide Book Three', 'books', 'beta', 30.75, NULL),
			('Guide Phone', 'electronics', 'acme', 100.00, NULL),
			('Guide Tablet', 'electronics', 'zenith', 200.25, NULL);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	cfg := config.Default()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, cfg.API, logger)
	testutil.NoError(t, ch.Load(ctx))
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
}

func facetCounts(t *testing.T, body map[string]any, column string) map[string]float64 {
	t.Helper()
	facets := jsonMap(t, body["facets"])
	rawBuckets, ok := facets[column].([]any)
	if !ok {
		t.Fatalf("expected %s facet buckets, got %T", column, facets[column])
	}
	counts := make(map[string]float64, len(rawBuckets))
	for _, rawBucket := range rawBuckets {
		bucket := rawBucket.(map[string]any)
		counts[facetCountKey(bucket["value"])] = jsonNum(t, bucket["count"])
	}
	return counts
}

func facetCountKey(value any) string {
	return fmt.Sprint(value)
}

func assertFacetCounts(t *testing.T, got, want map[string]float64) {
	t.Helper()
	testutil.Equal(t, len(want), len(got))
	for value, count := range want {
		testutil.Equal(t, count, got[value])
	}
}

func assertItemsCategory(t *testing.T, body map[string]any, category string) {
	t.Helper()
	items := jsonItems(t, body)
	if len(items) == 0 {
		t.Fatal("expected filtered items")
	}
	for _, item := range items {
		testutil.Equal(t, category, jsonStr(t, item["category"]))
	}
}

func facetStats(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	return jsonMap(t, body["facetStats"])
}

func assertFacetStat(t *testing.T, stats map[string]any, column string, wantMin, wantMax float64) {
	t.Helper()
	raw, ok := stats[column]
	if !ok {
		t.Fatalf("expected %s facet stats", column)
	}
	bounds := jsonMap(t, raw)
	testutil.Equal(t, wantMin, jsonNum(t, bounds["min"]))
	testutil.Equal(t, wantMax, jsonNum(t, bounds["max"]))
}

func parseJSONWithNumbers(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	dec := json.NewDecoder(w.Body)
	dec.UseNumber()
	var result map[string]any
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("parsing JSON response: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

func assertFacetStatJSONNumber(t *testing.T, body map[string]any, column, wantMin, wantMax string) {
	t.Helper()
	stats := jsonMap(t, body["facetStats"])
	bounds := jsonMap(t, stats[column])
	testutil.Equal(t, wantMin, fmt.Sprint(bounds["min"]))
	testutil.Equal(t, wantMax, fmt.Sprint(bounds["max"]))
}

func TestDisjunctiveFacetCountsOffsetList(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?search=guide&filter=category%3D'books'&facets=category,brand&disjunctiveFacets=category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetCounts(t, facetCounts(t, body, "category"), map[string]float64{"books": 3, "electronics": 2})
	assertFacetCounts(t, facetCounts(t, body, "brand"), map[string]float64{"acme": 2, "beta": 1})
}

func TestCursorDisjunctiveFacetCounts(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?cursor=&perPage=10&sort=id&search=guide&filter=category%3D'books'&facets=category,brand&disjunctiveFacets=category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetCounts(t, facetCounts(t, body, "category"), map[string]float64{"books": 3, "electronics": 2})
	assertFacetCounts(t, facetCounts(t, body, "brand"), map[string]float64{"acme": 2, "beta": 1})
}

func TestDisjunctiveFacetDefaultRemainsConjunctive(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?filter=category%3D'books'&facets=category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetCounts(t, facetCounts(t, body, "category"), map[string]float64{"books": 3})
}

func TestDisjunctiveFacetEachColumnDropsOnlyOwnPredicate(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?filter=category%3D'books'%20%26%26%20brand%3D'acme'&facets=category,brand&disjunctiveFacets=category,brand", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetCounts(t, facetCounts(t, body, "category"), map[string]float64{"books": 2, "electronics": 1})
	assertFacetCounts(t, facetCounts(t, body, "brand"), map[string]float64{"acme": 2, "beta": 1})
}

func TestFacetStatsNumericBoundsOffsetList(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?facets=price,category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	stats := facetStats(t, body)
	assertFacetStat(t, stats, "price", 10.50, 200.25)
	if _, ok := stats["category"]; ok {
		t.Fatal("expected non-numeric category facet stats to be omitted")
	}
}

func TestFacetStatsScopedOffsetList(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?filter=category%3D'books'&facets=price,category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetCounts(t, facetCounts(t, body, "price"), map[string]float64{"10.5": 1, "20.25": 1, "30.75": 1})
	assertFacetStat(t, facetStats(t, body), "price", 10.50, 30.75)
}

func TestCursorFacetStatsScopedList(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?cursor=&sort=id&filter=category%3D'books'&facets=price,category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetStat(t, facetStats(t, body), "price", 10.50, 30.75)
}

func TestFacetStatsDisjunctiveFacetDropsOwnPredicate(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?filter=category%3D'books'%20%26%26%20price%3D20.25&facets=price,category&disjunctiveFacets=price", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	assertItemsCategory(t, body, "books")
	assertFacetStat(t, facetStats(t, body), "price", 10.50, 30.75)
}

func TestFacetStatsOmitsAllNullNumericFacet(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/products/?facets=discount,category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSON(t, w)
	if _, ok := body["facetStats"]; ok {
		t.Fatal("expected facetStats to be omitted when every requested facet stat is omitted")
	}
}

func TestFacetStatsPreservesExactNumericBounds(t *testing.T) {
	ctx := context.Background()
	srv := setupDisjunctiveFacetServer(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		INSERT INTO products (name, category, brand, price, discount) VALUES
			('Guide Exact Low', 'books', 'precise', 12345678901234567890.123456789012345678, NULL),
			('Guide Exact High', 'books', 'precise', 12345678901234567890.123456789012345679, NULL);
	`)
	testutil.NoError(t, err)

	w := doRequest(t, srv, "GET", "/api/collections/products/?filter=brand%3D'precise'&facets=price", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSONWithNumbers(t, w)
	assertFacetStatJSONNumber(t, body, "price", "12345678901234567890.123456789012345678", "12345678901234567890.123456789012345679")
}

func TestFacetStatsSupportsMoneyBounds(t *testing.T) {
	ctx := context.Background()
	resetAndSeedDB(t, ctx)
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE priced_services (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			fee MONEY
		);
		INSERT INTO priced_services (name, category, fee) VALUES
			('basic', 'support', 1.23::money),
			('premium', 'support', 9876543.21::money);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	cfg := config.Default()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, cfg.API, logger)
	testutil.NoError(t, ch.Load(ctx))
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	w := doRequest(t, srv, "GET", "/api/collections/priced_services/?facets=fee,category", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body := parseJSONWithNumbers(t, w)
	assertFacetStatJSONNumber(t, body, "fee", "1.23", "9876543.21")
	if _, ok := jsonMap(t, body["facetStats"])["category"]; ok {
		t.Fatal("expected non-numeric category facet stats to be omitted")
	}
}
