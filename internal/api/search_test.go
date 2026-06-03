package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func searchableTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", Position: 2, TypeName: "text"},
			{Name: "body", Position: 3, TypeName: "text"},
			{Name: "status", Position: 4, TypeName: "varchar"},
			{Name: "views", Position: 5, TypeName: "integer"},
			{Name: "metadata", Position: 6, TypeName: "jsonb", IsJSON: true},
			{Name: "tags", Position: 7, TypeName: "text[]", IsArray: true},
		},
		PrimaryKey: []string{"id"},
	}
}

func noTextTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "counters",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "count", Position: 2, TypeName: "bigint"},
		},
		PrimaryKey: []string{"id"},
	}
}

func TestIsTextColumn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		col    *schema.Column
		expect bool
	}{
		{&schema.Column{TypeName: "text"}, true},
		{&schema.Column{TypeName: "varchar"}, true},
		{&schema.Column{TypeName: "varchar(255)"}, true},
		{&schema.Column{TypeName: "character varying"}, true},
		{&schema.Column{TypeName: "character varying(100)"}, true},
		{&schema.Column{TypeName: "char"}, true},
		{&schema.Column{TypeName: "character"}, true},
		{&schema.Column{TypeName: "citext"}, true},
		{&schema.Column{TypeName: "name"}, true},
		// Uppercase variants (Postgres reports types in various cases).
		{&schema.Column{TypeName: "TEXT"}, true},
		{&schema.Column{TypeName: "VARCHAR(255)"}, true},
		{&schema.Column{TypeName: "CHARACTER VARYING(100)"}, true},
		{&schema.Column{TypeName: "integer"}, false},
		{&schema.Column{TypeName: "boolean"}, false},
		{&schema.Column{TypeName: "jsonb", IsJSON: true}, false},
		{&schema.Column{TypeName: "text[]", IsArray: true}, false},
		{&schema.Column{TypeName: "uuid"}, false},
		{&schema.Column{TypeName: "timestamp"}, false},
	}

	for _, tc := range tests {
		result := isTextColumn(tc.col)
		if result != tc.expect {
			t.Errorf("isTextColumn(%q) = %v, want %v (isJSON=%v, isArray=%v)",
				tc.col.TypeName, result, tc.expect, tc.col.IsJSON, tc.col.IsArray)
		}
	}
}

func TestTextColumns(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	cols := textColumns(tbl)
	// Should include title, body, status but not id, views, metadata, tags
	testutil.SliceLen(t, cols, 3)
	testutil.Equal(t, "title", cols[0])
	testutil.Equal(t, "body", cols[1])
	testutil.Equal(t, "status", cols[2])
}

func TestBuildSearchSQL(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	whereSQL, rankSQL, args, err := buildSearchSQL(tbl, "hello world", 1, false)
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "hello world", args[0].(string))

	// WHERE should contain tsvector @@ tsquery
	testutil.Contains(t, whereSQL, "to_tsvector('simple'")
	testutil.Contains(t, whereSQL, "websearch_to_tsquery('simple', $1)")
	testutil.Contains(t, whereSQL, "@@")
	testutil.Contains(t, whereSQL, `coalesce("title", '')`)
	testutil.Contains(t, whereSQL, `coalesce("body", '')`)
	testutil.Contains(t, whereSQL, `coalesce("status", '')`)

	// Rank should use ts_rank
	testutil.Contains(t, rankSQL, "ts_rank(")
	testutil.Contains(t, rankSQL, "websearch_to_tsquery('simple', $1)")
}

func TestBuildSearchSQLWithOffset(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// Simulate filter already using $1, $2
	whereSQL, rankSQL, args, err := buildSearchSQL(tbl, "test", 3, false)
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 1)
	testutil.Contains(t, whereSQL, "$3")
	testutil.Contains(t, rankSQL, "$3")

	// Must NOT contain $1 or $2 — those belong to the filter.
	if strings.Contains(whereSQL, "$1") || strings.Contains(whereSQL, "$2") {
		t.Errorf("whereSQL should only use $3, got: %s", whereSQL)
	}
	if strings.Contains(rankSQL, "$1") || strings.Contains(rankSQL, "$2") {
		t.Errorf("rankSQL should only use $3, got: %s", rankSQL)
	}
}

func TestBuildSearchSQLEmptyTerm(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// Empty search term should still produce valid SQL (handler guards against this,
	// but buildSearchSQL itself should not panic or produce broken SQL).
	whereSQL, rankSQL, args, err := buildSearchSQL(tbl, "", 1, false)
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, "", args[0].(string))
	testutil.Contains(t, whereSQL, "@@")
	testutil.Contains(t, rankSQL, "ts_rank(")
}

func TestBuildSearchSQLNoTextColumns(t *testing.T) {
	t.Parallel()
	tbl := noTextTable()

	_, _, _, err := buildSearchSQL(tbl, "hello", 1, false)
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "no text columns")
}

func TestBuildSearchSQLWithFuzzy(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	whereSQL, rankSQL, args, err := buildSearchSQL(tbl, "helo wrld", 2, true)
	testutil.NoError(t, err)
	testutil.SliceLen(t, args, 3)
	testutil.Equal(t, "helo wrld", args[0].(string))
	testutil.Equal(t, "helo", args[1].(string))
	testutil.Equal(t, "wrld", args[2].(string))
	testutil.Contains(t, whereSQL, "websearch_to_tsquery('simple', $2)")
	testutil.Contains(t, whereSQL, `similarity(coalesce("title", ''), $2) > 0.2`)
	testutil.Contains(t, whereSQL, `similarity(coalesce("body", ''), $2) > 0.2`)
	testutil.Contains(t, whereSQL, `similarity(coalesce("status", ''), $2) > 0.2`)
	testutil.Contains(t, whereSQL, `strict_word_similarity(lower($3), lower(coalesce("title", ''))) >= 0.2`)
	testutil.Contains(t, whereSQL, `strict_word_similarity(lower($4), lower(coalesce("title", ''))) >= 0.2`)
	testutil.Contains(t, whereSQL, "AND")
	testutil.Contains(t, rankSQL, "GREATEST(")
	testutil.Contains(t, rankSQL, `similarity(coalesce("title", ''), $2)`)
	testutil.Contains(t, rankSQL, `similarity(coalesce("body", ''), $2)`)
	testutil.Contains(t, rankSQL, `similarity(coalesce("status", ''), $2)`)
}

func TestBuildListWithSearch(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	opts := listOpts{
		page:       1,
		perPage:    20,
		searchSQL:  `to_tsvector('simple', coalesce("title", '') || ' ' || coalesce("body", '')) @@ websearch_to_tsquery('simple', $1)`,
		searchRank: `ts_rank(to_tsvector('simple', coalesce("title", '') || ' ' || coalesce("body", '')), websearch_to_tsquery('simple', $1))`,
		searchArgs: []any{"hello"},
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)

	// Data query should have WHERE and ORDER BY rank
	testutil.Contains(t, dataQ, "WHERE")
	testutil.Contains(t, dataQ, "@@")
	testutil.Contains(t, dataQ, "ORDER BY ts_rank(")
	testutil.Contains(t, dataQ, "DESC")
	testutil.Contains(t, dataQ, "LIMIT $2")
	testutil.Contains(t, dataQ, "OFFSET $3")
	testutil.SliceLen(t, dataArgs, 3) // search arg + limit + offset
	testutil.Equal(t, "hello", dataArgs[0].(string))
	testutil.Equal(t, 20, dataArgs[1].(int))
	testutil.Equal(t, 0, dataArgs[2].(int))

	// Count query should also have WHERE
	testutil.Contains(t, countQ, "WHERE")
	testutil.SliceLen(t, countArgs, 1)
}

func TestBuildListWithFilterAndSearch(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	opts := listOpts{
		page:       1,
		perPage:    10,
		filterSQL:  `"status" = $1`,
		filterArgs: []any{"published"},
		searchSQL:  `to_tsvector('simple', coalesce("title", '')) @@ websearch_to_tsquery('simple', $2)`,
		searchRank: `ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $2))`,
		searchArgs: []any{"hello"},
	}

	dataQ, dataArgs, countQ, countArgs := buildList(tbl, opts)

	// Should combine filter AND search
	testutil.Contains(t, dataQ, "WHERE")
	testutil.Contains(t, dataQ, `"status" = $1`)
	testutil.Contains(t, dataQ, "AND")
	testutil.Contains(t, dataQ, "@@")
	testutil.Contains(t, dataQ, "LIMIT $3")
	testutil.Contains(t, dataQ, "OFFSET $4")
	testutil.SliceLen(t, dataArgs, 4) // filter arg + search arg + limit + offset
	testutil.Equal(t, "published", dataArgs[0].(string))
	testutil.Equal(t, "hello", dataArgs[1].(string))
	testutil.Equal(t, 10, dataArgs[2].(int)) // perPage
	testutil.Equal(t, 0, dataArgs[3].(int))  // offset (page 1)

	// Count query should combine filter AND search, with args in same order.
	testutil.Contains(t, countQ, "AND")
	testutil.SliceLen(t, countArgs, 2)
	testutil.Equal(t, "published", countArgs[0].(string))
	testutil.Equal(t, "hello", countArgs[1].(string))
}

func TestBuildListSearchWithExplicitSort(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// When user provides explicit sort, it should override search rank
	opts := listOpts{
		page:       1,
		perPage:    20,
		sortSQL:    `"title" ASC`,
		searchSQL:  `to_tsvector('simple', coalesce("title", '')) @@ websearch_to_tsquery('simple', $1)`,
		searchRank: `ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $1))`,
		searchArgs: []any{"hello"},
	}

	dataQ, _, _, _ := buildList(tbl, opts)

	// Should use user's sort, not rank.
	testutil.Contains(t, dataQ, `ORDER BY "title" ASC`)
	if strings.Contains(dataQ, "ts_rank") {
		t.Errorf("expected explicit sort to override rank, but ts_rank found in query: %s", dataQ)
	}
}

func TestParseSearchParamAllowsFuzzyWithNonEmptySearch(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{HasPgTrgm: true}), nil, nil, nil, nil)
	tbl := searchableTable()

	tests := []struct {
		name      string
		fuzzy     string
		wantFuzzy bool
	}{
		{name: "true", fuzzy: "true", wantFuzzy: true},
		{name: "false", fuzzy: "false", wantFuzzy: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			q := url.Values{
				"search": []string{"post"},
				"fuzzy":  []string{tc.fuzzy},
			}

			whereSQL, rankSQL, args, ok := h.parseSearchParam(w, tbl, q, 1)
			testutil.Equal(t, true, ok)
			testutil.Equal(t, http.StatusOK, w.Code)
			testutil.Contains(t, whereSQL, "websearch_to_tsquery")
			if tc.wantFuzzy {
				testutil.SliceLen(t, args, 2)
				testutil.Equal(t, "post", args[0].(string))
				testutil.Equal(t, "post", args[1].(string))
				testutil.Contains(t, whereSQL, "similarity(")
				testutil.Contains(t, whereSQL, "strict_word_similarity(")
				testutil.Contains(t, rankSQL, "similarity(")
			} else {
				testutil.SliceLen(t, args, 1)
				testutil.Equal(t, "post", args[0].(string))
				if strings.Contains(whereSQL, "similarity(") {
					t.Fatalf("expected exact search SQL for fuzzy=false, got: %s", whereSQL)
				}
				if strings.Contains(rankSQL, "similarity(") {
					t.Fatalf("expected exact rank SQL for fuzzy=false, got: %s", rankSQL)
				}
			}
		})
	}
}

func TestParseSearchParamRejectsFuzzyWhenPgTrgmUnavailable(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{HasPgTrgm: false}), nil, nil, nil, nil)
	tbl := searchableTable()
	w := httptest.NewRecorder()

	q := url.Values{
		"search": []string{"post"},
		"fuzzy":  []string{"true"},
	}

	_, _, _, ok := h.parseSearchParam(w, tbl, q, 1)
	testutil.Equal(t, false, ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, strings.ToLower(w.Body.String()), "pg_trgm")
	testutil.Contains(t, strings.ToLower(w.Body.String()), "unavailable")
}

func TestParseSearchParamRejectsInvalidFuzzyBoolean(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{}), nil, nil, nil, nil)
	tbl := searchableTable()
	w := httptest.NewRecorder()

	q := url.Values{
		"search": []string{"post"},
		"fuzzy":  []string{"notabool"},
	}

	_, _, _, ok := h.parseSearchParam(w, tbl, q, 1)
	testutil.Equal(t, false, ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, strings.ToLower(w.Body.String()), "fuzzy")
	testutil.Contains(t, strings.ToLower(w.Body.String()), "boolean")
}

func TestParseSearchParamRejectsFuzzyWithoutUsableSearch(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{}), nil, nil, nil, nil)
	tbl := searchableTable()

	searchCases := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "whitespace", value: "   "},
	}
	for _, searchCase := range searchCases {
		searchCase := searchCase
		t.Run(searchCase.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			q := url.Values{
				"search": []string{searchCase.value},
				"fuzzy":  []string{"true"},
			}

			_, _, _, ok := h.parseSearchParam(w, tbl, q, 1)
			testutil.Equal(t, false, ok)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, strings.ToLower(w.Body.String()), "fuzzy")
			testutil.Contains(t, strings.ToLower(w.Body.String()), "search")
		})
	}
}

func TestParseSearchParamRejectsTypoThreshold(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{}), nil, nil, nil, nil)
	tbl := searchableTable()

	w := httptest.NewRecorder()
	q := url.Values{
		"search":         []string{"post"},
		"typo_threshold": []string{"0.42"},
	}
	_, _, _, ok := h.parseSearchParam(w, tbl, q, 1)
	testutil.Equal(t, false, ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "unsupported parameter")
}

func TestHandleList_AllowsFacetsOnNonVectorPath(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?search=alice&facets=name", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

func TestHandleList_InvalidFacetColumnUsesParseError(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "GET", "/collections/users?search=alice&facets=missing", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Equal(t, `unknown column "missing" in facets parameter`, resp.Message)
}
