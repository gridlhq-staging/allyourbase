//go:build integration

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

type facetValueSearchHit struct {
	Value       string
	Highlighted string
	Count       float64
}

type facetValueSearchResponse struct {
	Status                int
	Hits                  []facetValueSearchHit
	ExhaustiveFacetsCount bool
}

func setupFacetValueSearchServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndSeedDB(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			brand TEXT NOT NULL,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb
		);
		INSERT INTO products (name, category, brand, metadata) VALUES
			('Acme Guide Basics', 'books', 'Acme', '{"tier":"core"}'),
			('Acme Cookbook Guide', 'books', 'Acme', '{"tier":"core"}'),
			('Acme Phone Guide', 'Electronics', 'Acme', '{"tier":"core"}'),
			('Ace Field Guide', 'books', 'ace', '{"tier":"alt"}'),
			('Ace Pocket Guide', 'books', 'ace', '{"tier":"alt"}'),
			('Acre Camera Guide', 'Electronics', 'ACRE', '{"tier":"alt"}'),
			('Axiom Reference', 'books', 'Axiom', '{"tier":"alt"}'),
			('Beta Book Guide', 'books', 'Beta', '{"tier":"other"}'),
			('Zenith Phone Guide', 'Electronics', 'Zenith', '{"tier":"other"}'),
			('Percent Manual', 'books', '%Percent', '{"tier":"literal"}'),
			('Underscore Manual', 'books', '_Underscore', '{"tier":"literal"}'),
			('Backslash Manual', 'Electronics', '\Backslash', '{"tier":"literal"}');
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	cfg := config.Default()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	api.RegisterSearchIndexPostReloadHook(ch, sharedPG.Pool, cfg.API, logger)
	testutil.NoError(t, ch.Load(ctx))
	return server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
}

func parseFacetValueSearchResponse(t *testing.T, w *httptest.ResponseRecorder) facetValueSearchResponse {
	t.Helper()
	body := parseJSON(t, w)
	rawHits, ok := body["facetHits"].([]any)
	if !ok {
		t.Fatalf("expected facetHits array, got %T in body %v", body["facetHits"], body)
	}
	hits := make([]facetValueSearchHit, len(rawHits))
	for i, rawHit := range rawHits {
		hit := jsonMap(t, rawHit)
		hits[i] = facetValueSearchHit{
			Value:       jsonStr(t, hit["value"]),
			Highlighted: jsonStr(t, hit["highlighted"]),
			Count:       jsonNum(t, hit["count"]),
		}
	}
	exhaustive, ok := body["exhaustiveFacetsCount"].(bool)
	if !ok {
		t.Fatalf("expected exhaustiveFacetsCount bool, got %T in body %v", body["exhaustiveFacetsCount"], body)
	}
	return facetValueSearchResponse{
		Status:                w.Code,
		Hits:                  hits,
		ExhaustiveFacetsCount: exhaustive,
	}
}

func assertFacetValueHits(t *testing.T, got, want []facetValueSearchHit) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facetHits mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func assertFacetValueError(t *testing.T, w *httptest.ResponseRecorder, status int, wantMessage string) {
	t.Helper()
	testutil.StatusCode(t, status, w.Code)
	body := parseJSON(t, w)
	msg := jsonStr(t, body["message"])
	testutil.Contains(t, msg, wantMessage)
}

func facetValueSearchPath(column, rawQuery string) string {
	if rawQuery == "" {
		return "/api/collections/products/facets/" + column + "/search"
	}
	return "/api/collections/products/facets/" + column + "/search?" + rawQuery
}

func TestFacetValueSearch_PrefixMatch(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=ac"), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	assertFacetValueHits(t, resp.Hits, []facetValueSearchHit{
		{Value: "Acme", Highlighted: "<mark>Ac</mark>me", Count: 3},
		{Value: "ace", Highlighted: "<mark>ac</mark>e", Count: 2},
		{Value: "ACRE", Highlighted: "<mark>AC</mark>RE", Count: 1},
	})
	testutil.True(t, resp.ExhaustiveFacetsCount, "expected prefix results to be exhaustive")
}

func TestFacetValueSearch_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	lower := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=ac"), nil)
	upper := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=AC"), nil)
	testutil.StatusCode(t, http.StatusOK, lower.Code)
	testutil.StatusCode(t, http.StatusOK, upper.Code)

	lowerResp := parseFacetValueSearchResponse(t, lower)
	upperResp := parseFacetValueSearchResponse(t, upper)
	assertFacetValueHits(t, upperResp.Hits, lowerResp.Hits)
}

func TestFacetValueSearch_EmptyQ(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q="), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	if len(resp.Hits) < 2 {
		t.Fatalf("expected at least 2 facet hits, got %d", len(resp.Hits))
	}
	assertFacetValueHits(t, resp.Hits[:3], []facetValueSearchHit{
		{Value: "Acme", Highlighted: "Acme", Count: 3},
		{Value: "ace", Highlighted: "ace", Count: 2},
		{Value: "%Percent", Highlighted: "%Percent", Count: 1},
	})
}

func TestFacetValueSearch_FilterScoped(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	q := url.Values{}
	q.Set("filter", "category='books'")
	q.Set("q", "a")
	w := doRequest(t, srv, "GET", facetValueSearchPath("brand", q.Encode()), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	assertFacetValueHits(t, resp.Hits, []facetValueSearchHit{
		{Value: "Acme", Highlighted: "<mark>A</mark>cme", Count: 2},
		{Value: "ace", Highlighted: "<mark>a</mark>ce", Count: 2},
		{Value: "Axiom", Highlighted: "<mark>A</mark>xiom", Count: 1},
	})
}

func TestFacetValueSearch_SearchScoped(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	q := url.Values{}
	q.Set("search", "guide")
	q.Set("q", "a")
	w := doRequest(t, srv, "GET", facetValueSearchPath("brand", q.Encode()), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	assertFacetValueHits(t, resp.Hits, []facetValueSearchHit{
		{Value: "Acme", Highlighted: "<mark>A</mark>cme", Count: 3},
		{Value: "ace", Highlighted: "<mark>a</mark>ce", Count: 2},
		{Value: "ACRE", Highlighted: "<mark>A</mark>CRE", Count: 1},
	})
}

func TestFacetValueSearch_InvalidColumn(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", facetValueSearchPath("nonexistent", "q=a"), nil)
	assertFacetValueError(t, w, http.StatusBadRequest, `unknown column "nonexistent" in facets parameter`)
}

func TestFacetValueSearch_UnknownCollection(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", "/api/collections/missing/facets/brand/search?q=a", nil)
	assertFacetValueError(t, w, http.StatusNotFound, "collection not found: missing")
}

func TestFacetValueSearch_UnsupportedColumnType(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", facetValueSearchPath("metadata", "q=a"), nil)
	assertFacetValueError(t, w, http.StatusBadRequest, `unsupported facet column "metadata" with type "jsonb"`)
}

func TestFacetValueSearch_MaxFacetHits(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	limited := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=a&maxFacetHits=2"), nil)
	testutil.StatusCode(t, http.StatusOK, limited.Code)
	limitedResp := parseFacetValueSearchResponse(t, limited)
	assertFacetValueHits(t, limitedResp.Hits, []facetValueSearchHit{
		{Value: "Acme", Highlighted: "<mark>A</mark>cme", Count: 3},
		{Value: "ace", Highlighted: "<mark>a</mark>ce", Count: 2},
	})

	zero := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=a&maxFacetHits=0"), nil)
	assertFacetValueError(t, zero, http.StatusBadRequest, "maxFacetHits must be greater than 0")

	negative := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=a&maxFacetHits=-1"), nil)
	assertFacetValueError(t, negative, http.StatusBadRequest, "maxFacetHits must be greater than 0")
}

func TestFacetValueSearch_Highlighting(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	w := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=ac"), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	wantHighlights := map[string]string{
		"Acme": "<mark>Ac</mark>me",
		"ace":  "<mark>ac</mark>e",
		"ACRE": "<mark>AC</mark>RE",
	}
	for _, hit := range resp.Hits {
		want, ok := wantHighlights[hit.Value]
		if !ok {
			t.Fatalf("unexpected highlighted hit %q", hit.Value)
		}
		testutil.Equal(t, want, hit.Highlighted)
	}
}

func TestFacetValueSearch_LiteralEscaping(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	tests := []struct {
		name string
		q    string
		want facetValueSearchHit
	}{
		{name: "percent", q: "%", want: facetValueSearchHit{Value: "%Percent", Highlighted: "<mark>%</mark>Percent", Count: 1}},
		{name: "underscore", q: "_", want: facetValueSearchHit{Value: "_Underscore", Highlighted: "<mark>_</mark>Underscore", Count: 1}},
		{name: "backslash", q: `\`, want: facetValueSearchHit{Value: `\Backslash`, Highlighted: `<mark>\</mark>Backslash`, Count: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("q", tt.q)
			w := doRequest(t, srv, "GET", facetValueSearchPath("brand", q.Encode()), nil)
			testutil.StatusCode(t, http.StatusOK, w.Code)
			resp := parseFacetValueSearchResponse(t, w)
			assertFacetValueHits(t, resp.Hits, []facetValueSearchHit{tt.want})
		})
	}
}

func TestFacetValueSearch_ExhaustiveFacetsCount(t *testing.T) {
	ctx := context.Background()
	srv := setupFacetValueSearchServer(t, ctx)

	exhaustive := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=ac&maxFacetHits=10"), nil)
	testutil.StatusCode(t, http.StatusOK, exhaustive.Code)
	exhaustiveResp := parseFacetValueSearchResponse(t, exhaustive)
	testutil.True(t, exhaustiveResp.ExhaustiveFacetsCount, "expected untruncated facet hits to be exhaustive")

	truncated := doRequest(t, srv, "GET", facetValueSearchPath("brand", "q=a&maxFacetHits=2"), nil)
	testutil.StatusCode(t, http.StatusOK, truncated.Code)
	truncatedResp := parseFacetValueSearchResponse(t, truncated)
	testutil.Equal(t, false, truncatedResp.ExhaustiveFacetsCount)
	testutil.Equal(t, 2, len(truncatedResp.Hits))
}

func TestFacetValueSearch_RLSEnforced(t *testing.T) {
	ctx := context.Background()
	srv := setupRLSTestServer(t, ctx)
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-alice",
		},
	}

	w := doRequestWithClaims(t, srv, "GET", "/api/collections/rls_test_docs/facets/owner_id/search?q=user", nil, claims)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseFacetValueSearchResponse(t, w)

	assertFacetValueHits(t, resp.Hits, []facetValueSearchHit{
		{Value: "user-alice", Highlighted: "<mark>user</mark>-alice", Count: 2},
	})
	if strings.Contains(w.Body.String(), "user-bob") {
		t.Fatal("RLS leak: bob owner_id bucket visible in facet value search for Alice")
	}
}
