package api

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func disjunctiveFacetTestTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "products",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid"},
			{Name: "email", TypeName: "text"},
			{Name: "name", TypeName: "text", IsNullable: true},
			{Name: "category", TypeName: "text"},
			{Name: "brand", TypeName: "text", IsNullable: true},
			{Name: "location", TypeName: "geometry(Point,4326)", IsGeometry: true, SRID: 4326},
		},
		PrimaryKey: []string{"id"},
	}
}

func disjunctiveFacetTestSchema() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users":    disjunctiveFacetTestTable(),
			"public.products": disjunctiveFacetTestTable(),
		},
		Schemas: []string{"public"},
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertAnySlice(t *testing.T, got, want []any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Fatalf("%q unexpectedly contains %q", s, substr)
	}
}

func TestDisjunctiveFacetColumnsParser(t *testing.T) {
	t.Parallel()
	tbl := disjunctiveFacetTestTable()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "preserves_order", input: "email,name", want: []string{"email", "name"}},
		{name: "deduplicates", input: "name,email,name,email", want: []string{"name", "email"}},
		{name: "empty", input: "", want: nil},
		{name: "whitespace", input: "  \t  ", want: nil},
		{name: "skips_empty_segments", input: " name, ,email,", want: []string{"name", "email"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDisjunctiveFacetColumns(tbl, tc.input)
			testutil.NoError(t, err)
			assertStringSlice(t, got, tc.want)
		})
	}
}

func TestDisjunctiveFacetColumnsParserRejectsUnknownColumn(t *testing.T) {
	t.Parallel()
	_, err := parseDisjunctiveFacetColumns(disjunctiveFacetTestTable(), "missing")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "disjunctiveFacets parameter")
}

func TestDisjunctiveFacetColumnsParserRejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	_, err := parseDisjunctiveFacetColumns(disjunctiveFacetTestTable(), "location")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "unsupported facet column")
	testutil.Contains(t, err.Error(), "location")
}

func TestHandleList_InvalidDisjunctiveFacetColumnReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(disjunctiveFacetTestSchema())
	w := doRequest(h, "GET", "/collections/users?facets=name&disjunctiveFacets=missing", "")

	testutil.Equal(t, 400, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "disjunctiveFacets parameter")
}

func TestHandleList_UnsupportedDisjunctiveFacetTypeReturns400(t *testing.T) {
	t.Parallel()
	h := testHandler(disjunctiveFacetTestSchema())
	w := doRequest(h, "GET", "/collections/users?facets=name&disjunctiveFacets=location", "")

	testutil.Equal(t, 400, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "unsupported facet column")
	testutil.Contains(t, resp.Message, "location")
}

func TestParseListBaseOpts_DisjunctiveFacetAndRawFilter(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(disjunctiveFacetTestSchema()), nil, nil, nil, nil)
	tbl := disjunctiveFacetTestTable()
	req := httptest.NewRequest("GET", "/collections/users?facets=name&disjunctiveFacets=name&filter=name%3D'alice'", nil)
	w := httptest.NewRecorder()

	fs, ok := h.parseFilterAndSpatial(w, tbl, req.URL.Query())
	testutil.Equal(t, true, ok)
	opts, ok := h.parseListBaseOpts(w, tbl, req.URL.Query(), nil, 20, fs)
	testutil.Equal(t, true, ok)

	assertStringSlice(t, opts.facetCols, []string{"name"})
	assertStringSlice(t, opts.disjunctiveFacetCols, []string{"name"})
	testutil.Equal(t, "name='alice'", opts.rawFilter)
}

func TestParseListBaseOpts_DisjunctiveFacetAndRawFilterAbsentOrEmpty(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(disjunctiveFacetTestSchema()), nil, nil, nil, nil)
	tbl := disjunctiveFacetTestTable()

	tests := []struct {
		name string
		path string
	}{
		{name: "absent", path: "/collections/users?facets=name"},
		{name: "empty", path: "/collections/users?facets=name&disjunctiveFacets=&filter="},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			fs, ok := h.parseFilterAndSpatial(w, tbl, req.URL.Query())
			testutil.Equal(t, true, ok)
			opts, ok := h.parseListBaseOpts(w, tbl, req.URL.Query(), nil, 20, fs)
			testutil.Equal(t, true, ok)

			assertStringSlice(t, opts.facetCols, []string{"name"})
			assertStringSlice(t, opts.disjunctiveFacetCols, nil)
			testutil.Equal(t, "", opts.rawFilter)
		})
	}
}

func TestBuildFacetCountQueriesDisjunctiveFacetDropsOwnEqualityOnly(t *testing.T) {
	t.Parallel()
	tbl := disjunctiveFacetTestTable()
	opts := listOpts{
		rawFilter:            `category='books' && brand='acme'`,
		filterSQL:            `("category" = $1 AND "brand" = $2)`,
		filterArgs:           []any{"books", "acme"},
		spatialSQL:           `ST_Intersects("location", ST_MakeEnvelope($3, $4, $5, $6, 4326))`,
		spatialArgs:          []any{-1.0, -2.0, 3.0, 4.0},
		searchSQL:            `to_tsvector('simple', "name") @@ websearch_to_tsquery('simple', $7)`,
		searchArgs:           []any{"widget"},
		facetCols:            []string{"category", "brand"},
		disjunctiveFacetCols: []string{"category"},
	}

	queries := buildFacetCountQueries(tbl, opts)
	categoryQuery := queries["category"]
	assertNotContains(t, categoryQuery.sql, `"category" =`)
	testutil.Contains(t, categoryQuery.sql, `"brand" = $1`)
	testutil.Contains(t, categoryQuery.sql, `ST_MakeEnvelope($2, $3, $4, $5, 4326)`)
	testutil.Contains(t, categoryQuery.sql, `websearch_to_tsquery('simple', $6)`)
	assertAnySlice(t, categoryQuery.args, []any{"acme", -1.0, -2.0, 3.0, 4.0, "widget"})

	brandQuery := queries["brand"]
	testutil.Contains(t, brandQuery.sql, `("category" = $1 AND "brand" = $2)`)
	testutil.Contains(t, brandQuery.sql, `ST_MakeEnvelope($3, $4, $5, $6, 4326)`)
	testutil.Contains(t, brandQuery.sql, `websearch_to_tsquery('simple', $7)`)
	assertAnySlice(t, brandQuery.args[:2], opts.filterArgs)
}

func TestBuildFacetCountQueriesDisjunctiveFacetDropsOwnNullCheck(t *testing.T) {
	t.Parallel()
	tbl := disjunctiveFacetTestTable()
	opts := listOpts{
		rawFilter:            `brand=null && category='books'`,
		filterSQL:            `("brand" IS NULL AND "category" = $1)`,
		filterArgs:           []any{"books"},
		facetCols:            []string{"brand"},
		disjunctiveFacetCols: []string{"brand"},
	}

	queries := buildFacetCountQueries(tbl, opts)
	query := queries["brand"]
	assertNotContains(t, query.sql, `"brand" IS NULL`)
	testutil.Contains(t, query.sql, `"category" = $1`)
	assertAnySlice(t, query.args, []any{"books"})
}

func TestBuildFilterExcludingFacetColumnLeavesUnlistedFilterIntact(t *testing.T) {
	t.Parallel()
	sql, args, err := buildFilterExcludingFacetColumn(disjunctiveFacetTestTable(), `category='books' && brand='acme'`, "category")
	testutil.NoError(t, err)
	testutil.Equal(t, `"brand" = $1`, sql)
	assertAnySlice(t, args, []any{"acme"})
}
