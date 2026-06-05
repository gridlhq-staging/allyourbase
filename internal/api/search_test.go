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

	search, err := buildSearchSQL(tbl, "hello world", 1, defaultSearchOptions(false))
	testutil.NoError(t, err)
	// args: search term, then the schema/table names consumed by the synonym
	// expansion subquery built by buildSearchQueryExpression.
	testutil.SliceLen(t, search.args, 3)
	testutil.Equal(t, "hello world", search.args[0].(string))
	testutil.Equal(t, "public", search.args[1].(string))
	testutil.Equal(t, "posts", search.args[2].(string))

	// WHERE should contain tsvector @@ ts_rewrite(...) with the original websearch tsquery inside.
	testutil.Contains(t, search.whereSQL, "to_tsvector('english'::regconfig")
	testutil.Contains(t, search.whereSQL, "ts_rewrite(websearch_to_tsquery('english'::regconfig, $1)")
	testutil.Contains(t, search.whereSQL, "@@")
	testutil.Contains(t, search.whereSQL, `coalesce("title", '')`)
	testutil.Contains(t, search.whereSQL, `coalesce("body", '')`)
	testutil.Contains(t, search.whereSQL, `coalesce("status", '')`)
	testutil.Contains(t, search.whereSQL, "quote_literal($2)")
	testutil.Contains(t, search.whereSQL, "quote_literal($3)")

	// Rank should use ts_rank.
	testutil.Contains(t, search.rankSQL, "ts_rank(")
	testutil.Contains(t, search.rankSQL, "ts_rewrite(websearch_to_tsquery('english'::regconfig, $1)")
	testutil.Equal(t, "", search.highlightSelect)
	testutil.Equal(t, "", search.highlightAlias)
}

func TestBuildSearchSQLUsesConfiguredTextSearchConfig(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	search, err := buildSearchSQL(tbl, "hello world", 1, searchOptions{
		textSearchConfig: "simple",
		typoThreshold:    defaultTypoThreshold,
		highlight:        true,
	})
	testutil.NoError(t, err)

	testutil.Contains(t, search.whereSQL, "to_tsvector('simple'::regconfig")
	testutil.Contains(t, search.whereSQL, "websearch_to_tsquery('simple'::regconfig, $1)")
	testutil.Contains(t, search.whereSQL, "phraseto_tsquery(''simple''::regconfig, term)")
	testutil.Contains(t, search.whereSQL, "websearch_to_tsquery(''simple''::regconfig, s1.term)")
	testutil.Contains(t, search.rankSQL, "ts_rank(to_tsvector('simple'::regconfig")
	testutil.Contains(t, search.highlightSelect, "ts_headline('simple'::regconfig")
	if strings.Contains(search.whereSQL, "DROP TABLE") || strings.Contains(search.highlightSelect, "DROP TABLE") {
		t.Fatalf("search SQL should only contain validated regconfig literals, got where=%s highlight=%s", search.whereSQL, search.highlightSelect)
	}
}

func TestBuildSearchSQLWithHighlightUsesCanonicalDocument(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	search, err := buildSearchSQL(tbl, "hello", 1, searchOptions{
		highlight:     true,
		typoThreshold: defaultTypoThreshold,
	})
	testutil.NoError(t, err)

	docExpr := `coalesce("title", '') || ' ' || coalesce("body", '') || ' ' || coalesce("status", '')`
	// The tsquery expression now goes through ts_rewrite to support synonym
	// expansion; the same expression must be reused by where, rank, and the
	// highlight CASE/ELSE branches so all three score against the same query.
	tsqueryPrefix := "(SELECT ts_rewrite(websearch_to_tsquery('english'::regconfig, $1),"
	testutil.Contains(t, search.whereSQL, "to_tsvector('english'::regconfig, "+docExpr+") @@ "+tsqueryPrefix)
	testutil.Contains(t, search.rankSQL, "ts_rank(to_tsvector('english'::regconfig, "+docExpr+"), "+tsqueryPrefix)
	testutil.Contains(t, search.highlightSelect, "CASE WHEN to_tsvector('english'::regconfig, "+docExpr+") @@ "+tsqueryPrefix)
	escapedDocExpr := "replace(replace(replace(" + docExpr + ", '&', '&amp;'), '<', '&lt;'), '>', '&gt;')"
	testutil.Contains(t, search.highlightSelect, "ts_headline('english'::regconfig, "+escapedDocExpr+", "+tsqueryPrefix)
	testutil.Contains(t, search.highlightSelect, "ELSE "+escapedDocExpr)
	testutil.Contains(t, search.highlightSelect, `AS "`+searchHighlightSQLAlias+`"`)
	testutil.Equal(t, searchHighlightSQLAlias, search.highlightAlias)
	testutil.Contains(t, search.highlightResultSelect, `jsonb_build_object('title', jsonb_build_object('value', CASE WHEN to_tsvector('english'::regconfig, coalesce("title", '')) @@ `+tsqueryPrefix)
	testutil.Contains(t, search.highlightResultSelect, `ts_headline('english'::regconfig, replace(replace(replace(coalesce("body", ''), '&', '&amp;'), '<', '&lt;'), '>', '&gt;'), `+tsqueryPrefix)
	testutil.Contains(t, search.highlightResultSelect, `'matchLevel', CASE WHEN to_tsvector('english'::regconfig, coalesce("status", '')) @@ `+tsqueryPrefix)
	testutil.Contains(t, search.highlightResultSelect, `AS "`+searchHighlightResultSQLAlias+`"`)
	testutil.Equal(t, searchHighlightResultSQLAlias, search.highlightResultAlias)
}

func TestBuildSearchSQLWithHighlightAvoidsSchemaAliasCollision(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	tbl.Columns = append(tbl.Columns,
		&schema.Column{Name: "__search_highlight", Position: 5, TypeName: "text"},
		&schema.Column{Name: searchHighlightSQLAlias, Position: 6, TypeName: "text"},
	)

	search, err := buildSearchSQL(tbl, "hello", 1, searchOptions{
		highlight:     true,
		typoThreshold: defaultTypoThreshold,
	})
	testutil.NoError(t, err)

	testutil.Equal(t, searchHighlightSQLAlias+"_1", search.highlightAlias)
	testutil.Contains(t, search.highlightSelect, `AS "`+searchHighlightSQLAlias+`_1"`)
	testutil.Equal(t, searchHighlightResultSQLAlias, search.highlightResultAlias)
}

func TestBuildSearchSQLWithHighlightRejectsResponseFieldCollision(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{Name: searchHighlightResponseField, Position: 8, TypeName: "text"})

	_, err := buildSearchSQL(tbl, "hello", 1, searchOptions{
		highlight:     true,
		typoThreshold: defaultTypoThreshold,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `highlight cannot be used on table "posts" because it has a "_highlight" column`)
}

func TestBuildSearchSQLWithHighlightRejectsHighlightResultCollision(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{Name: searchHighlightResultResponseField, Position: 8, TypeName: "jsonb", IsJSON: true})

	_, err := buildSearchSQL(tbl, "hello", 1, searchOptions{
		highlight:     true,
		typoThreshold: defaultTypoThreshold,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `"`+searchHighlightResultResponseField+`" column`)
}

func TestBuildSearchSQLWithOffset(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// Simulate filter already using $1, $2. The search now claims $3 (term),
	// $4 (schema), and $5 (table) for its FTS+synonym expansion.
	search, err := buildSearchSQL(tbl, "test", 3, defaultSearchOptions(false))
	testutil.NoError(t, err)
	testutil.SliceLen(t, search.args, 3)
	testutil.Equal(t, "test", search.args[0].(string))
	testutil.Equal(t, "public", search.args[1].(string))
	testutil.Equal(t, "posts", search.args[2].(string))
	testutil.Contains(t, search.whereSQL, "$3")
	testutil.Contains(t, search.whereSQL, "quote_literal($4)")
	testutil.Contains(t, search.whereSQL, "quote_literal($5)")
	testutil.Contains(t, search.rankSQL, "$3")

	// Must NOT contain $1 or $2 — those belong to the filter.
	if strings.Contains(search.whereSQL, "$1") || strings.Contains(search.whereSQL, "$2") {
		t.Errorf("whereSQL should not use $1/$2 (reserved for filter), got: %s", search.whereSQL)
	}
	if strings.Contains(search.rankSQL, "$1") || strings.Contains(search.rankSQL, "$2") {
		t.Errorf("rankSQL should not use $1/$2 (reserved for filter), got: %s", search.rankSQL)
	}
}

func TestBuildSearchSQLEmptyTerm(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// Empty search term should still produce valid SQL (handler guards against this,
	// but buildSearchSQL itself should not panic or produce broken SQL).
	search, err := buildSearchSQL(tbl, "", 1, defaultSearchOptions(false))
	testutil.NoError(t, err)
	testutil.SliceLen(t, search.args, 3)
	testutil.Equal(t, "", search.args[0].(string))
	testutil.Equal(t, "public", search.args[1].(string))
	testutil.Equal(t, "posts", search.args[2].(string))
	testutil.Contains(t, search.whereSQL, "@@")
	testutil.Contains(t, search.rankSQL, "ts_rank(")
}

func TestBuildSearchSQLNoTextColumns(t *testing.T) {
	t.Parallel()
	tbl := noTextTable()

	_, err := buildSearchSQL(tbl, "hello", 1, defaultSearchOptions(false))
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "no text columns")
}

func TestBuildSearchSQLWithFuzzy(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	// argOffset=2: $2=term, $3=schema, $4=table, then fuzzy token args $5/$6.
	search, err := buildSearchSQL(tbl, "helo wrld", 2, defaultSearchOptions(true))
	testutil.NoError(t, err)
	testutil.SliceLen(t, search.args, 5)
	testutil.Equal(t, "helo wrld", search.args[0].(string))
	testutil.Equal(t, "public", search.args[1].(string))
	testutil.Equal(t, "posts", search.args[2].(string))
	testutil.Equal(t, "helo", search.args[3].(string))
	testutil.Equal(t, "wrld", search.args[4].(string))
	testutil.Contains(t, search.whereSQL, "websearch_to_tsquery('english'::regconfig, $2)")
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("title", ''), $2) > 0.2`)
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("body", ''), $2) > 0.2`)
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("status", ''), $2) > 0.2`)
	testutil.Contains(t, search.whereSQL, `strict_word_similarity(lower($5), lower(coalesce("title", ''))) >= 0.2`)
	testutil.Contains(t, search.whereSQL, `strict_word_similarity(lower($6), lower(coalesce("title", ''))) >= 0.2`)
	testutil.Contains(t, search.whereSQL, "AND")
	testutil.Contains(t, search.rankSQL, "GREATEST(")
	testutil.Contains(t, search.rankSQL, `similarity(coalesce("title", ''), $2)`)
	testutil.Contains(t, search.rankSQL, `similarity(coalesce("body", ''), $2)`)
	testutil.Contains(t, search.rankSQL, `similarity(coalesce("status", ''), $2)`)
}

func TestBuildSearchSQLWithFuzzyTypoThreshold(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	search, err := buildSearchSQL(tbl, "helo wrld", 2, searchOptions{
		fuzzy:         true,
		typoThreshold: 0.1,
	})
	testutil.NoError(t, err)
	testutil.SliceLen(t, search.args, 5)
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("title", ''), $2) > 0.1`)
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("body", ''), $2) > 0.1`)
	testutil.Contains(t, search.whereSQL, `similarity(coalesce("status", ''), $2) > 0.1`)
	testutil.Contains(t, search.whereSQL, `strict_word_similarity(lower($5), lower(coalesce("title", ''))) >= 0.1`)
	testutil.Contains(t, search.whereSQL, `strict_word_similarity(lower($6), lower(coalesce("title", ''))) >= 0.1`)
	if strings.Contains(search.whereSQL, "> 0.2") || strings.Contains(search.whereSQL, ">= 0.2") {
		t.Fatalf("expected caller-provided fuzzy threshold, got whereSQL: %s", search.whereSQL)
	}
	testutil.Contains(t, search.rankSQL, `similarity(coalesce("title", ''), $2)`)
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

	// Explicit legacy sort must break ties after relevance, not replace it.
	opts := listOpts{
		page:       1,
		perPage:    20,
		sortSQL:    `"title" ASC`,
		searchSQL:  `to_tsvector('simple', coalesce("title", '')) @@ websearch_to_tsquery('simple', $1)`,
		searchRank: `ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $1))`,
		searchArgs: []any{"hello"},
	}

	dataQ, _, _, _ := buildList(tbl, opts)

	testutil.Contains(t, dataQ, `ORDER BY ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $1)) DESC, "title" ASC`)
}

func TestBuildListSearchWithStructuredSortAppendsAfterRank(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	parsedSort, err := parseStructuredSort(tbl, "-title", true)
	testutil.NoError(t, err)

	opts := listOpts{
		page:       1,
		perPage:    20,
		sort:       parsedSort,
		searchSQL:  `to_tsvector('simple', coalesce("title", '')) @@ websearch_to_tsquery('simple', $1)`,
		searchRank: `ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $1))`,
		searchArgs: []any{"hello"},
	}

	dataQ, _, _, _ := buildList(tbl, opts)

	testutil.Contains(t, dataQ, `ORDER BY ts_rank(to_tsvector('simple', coalesce("title", '')), websearch_to_tsquery('simple', $1)) DESC, "title" DESC`)
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

			search, ok := h.parseSearchParam(w, tbl, q, 1)
			testutil.Equal(t, true, ok)
			testutil.Equal(t, http.StatusOK, w.Code)
			testutil.Contains(t, search.searchSQL, "websearch_to_tsquery")
			// Every searchArgs slice now includes schema/table refs in addition
			// to the term (and any fuzzy tokens). For the searchableTable
			// fixture, schema=public, table=posts.
			if tc.wantFuzzy {
				testutil.SliceLen(t, search.searchArgs, 4)
				testutil.Equal(t, "post", search.searchArgs[0].(string))
				testutil.Equal(t, "public", search.searchArgs[1].(string))
				testutil.Equal(t, "posts", search.searchArgs[2].(string))
				testutil.Equal(t, "post", search.searchArgs[3].(string))
				testutil.Contains(t, search.searchSQL, "similarity(")
				testutil.Contains(t, search.searchSQL, "strict_word_similarity(")
				testutil.Contains(t, search.searchRank, "similarity(")
			} else {
				testutil.SliceLen(t, search.searchArgs, 3)
				testutil.Equal(t, "post", search.searchArgs[0].(string))
				testutil.Equal(t, "public", search.searchArgs[1].(string))
				testutil.Equal(t, "posts", search.searchArgs[2].(string))
				if strings.Contains(search.searchSQL, "similarity(") {
					t.Fatalf("expected exact search SQL for fuzzy=false, got: %s", search.searchSQL)
				}
				if strings.Contains(search.searchRank, "similarity(") {
					t.Fatalf("expected exact rank SQL for fuzzy=false, got: %s", search.searchRank)
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

	_, ok := h.parseSearchParam(w, tbl, q, 1)
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

	_, ok := h.parseSearchParam(w, tbl, q, 1)
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

			_, ok := h.parseSearchParam(w, tbl, q, 1)
			testutil.Equal(t, false, ok)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, strings.ToLower(w.Body.String()), "fuzzy")
			testutil.Contains(t, strings.ToLower(w.Body.String()), "search")
		})
	}
}

func TestParseSearchParamTypoThreshold(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{HasPgTrgm: true}), nil, nil, nil, nil)
	tbl := searchableTable()

	tests := []struct {
		name        string
		query       url.Values
		wantOK      bool
		wantSnippet string
	}{
		{
			name: "omitted_threshold_uses_default",
			query: url.Values{
				"search": []string{"post"},
				"fuzzy":  []string{"true"},
			},
			wantOK:      true,
			wantSnippet: "> 0.2",
		},
		{
			name: "fuzzy_true_with_threshold_succeeds",
			query: url.Values{
				"search":         []string{"post"},
				"fuzzy":          []string{"true"},
				"typo_threshold": []string{"0.1"},
			},
			wantOK:      true,
			wantSnippet: "> 0.1",
		},
		{
			name: "threshold_without_fuzzy_fails",
			query: url.Values{
				"search":         []string{"post"},
				"typo_threshold": []string{"0.1"},
			},
			wantOK:      false,
			wantSnippet: "fuzzy",
		},
		{
			name: "threshold_not_number_fails",
			query: url.Values{
				"search":         []string{"post"},
				"fuzzy":          []string{"true"},
				"typo_threshold": []string{"not-a-number"},
			},
			wantOK:      false,
			wantSnippet: "typo_threshold",
		},
		{
			name: "threshold_below_zero_fails",
			query: url.Values{
				"search":         []string{"post"},
				"fuzzy":          []string{"true"},
				"typo_threshold": []string{"-0.01"},
			},
			wantOK:      false,
			wantSnippet: "typo_threshold",
		},
		{
			name: "threshold_above_one_fails",
			query: url.Values{
				"search":         []string{"post"},
				"fuzzy":          []string{"true"},
				"typo_threshold": []string{"1.01"},
			},
			wantOK:      false,
			wantSnippet: "typo_threshold",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()

			search, ok := h.parseSearchParam(w, tbl, tc.query, 1)
			testutil.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				testutil.Equal(t, http.StatusOK, w.Code)
				testutil.Contains(t, search.searchSQL, tc.wantSnippet)
				return
			}
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, strings.ToLower(w.Body.String()), strings.ToLower(tc.wantSnippet))
		})
	}
}

func TestParseSearchParamHighlight(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{HasPgTrgm: true}), nil, nil, nil, nil)
	tbl := searchableTable()

	tests := []struct {
		name          string
		query         url.Values
		wantOK        bool
		wantHighlight bool
		wantSnippet   string
	}{
		{
			name: "highlight_true",
			query: url.Values{
				"search":    []string{"post"},
				"highlight": []string{"true"},
			},
			wantOK:        true,
			wantHighlight: true,
			wantSnippet:   "ts_headline",
		},
		{
			name: "highlight_false",
			query: url.Values{
				"search":    []string{"post"},
				"highlight": []string{"false"},
			},
			wantOK:        true,
			wantHighlight: false,
		},
		{
			name: "highlight_invalid_boolean",
			query: url.Values{
				"search":    []string{"post"},
				"highlight": []string{"sometimes"},
			},
			wantOK:      false,
			wantSnippet: "highlight",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()

			search, ok := h.parseSearchParam(w, tbl, tc.query, 1)
			testutil.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				testutil.Equal(t, http.StatusBadRequest, w.Code)
				testutil.Contains(t, strings.ToLower(w.Body.String()), strings.ToLower(tc.wantSnippet))
				return
			}
			testutil.Equal(t, http.StatusOK, w.Code)
			if tc.wantHighlight {
				testutil.Contains(t, search.highlightSelect, tc.wantSnippet)
				testutil.Equal(t, searchHighlightSQLAlias, search.highlightAlias)
				testutil.Contains(t, search.highlightResultSelect, searchHighlightResultSQLAlias)
				testutil.Equal(t, searchHighlightResultSQLAlias, search.highlightResultAlias)
				return
			}
			testutil.Equal(t, "", search.highlightSelect)
			testutil.Equal(t, "", search.highlightAlias)
			testutil.Equal(t, "", search.highlightResultSelect)
			testutil.Equal(t, "", search.highlightResultAlias)
		})
	}
}

func TestParseSearchParamHighlightRejectsHighlightResultCollision(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(&schema.SchemaCache{HasPgTrgm: true}), nil, nil, nil, nil)
	tbl := searchableTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{Name: searchHighlightResultResponseField, Position: 8, TypeName: "jsonb", IsJSON: true})
	w := httptest.NewRecorder()

	q := url.Values{
		"search":    []string{"post"},
		"highlight": []string{"true"},
	}

	_, ok := h.parseSearchParam(w, tbl, q, 1)
	testutil.Equal(t, false, ok)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), searchHighlightResultResponseField)
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
