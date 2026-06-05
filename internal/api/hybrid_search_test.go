package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func hybridTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "articles",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", Position: 2, TypeName: "text"},
			{Name: "body", Position: 3, TypeName: "text"},
			{Name: "embedding", Position: 4, TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
		PrimaryKey: []string{"id"},
	}
}

func vectorNoTextTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "metrics",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "score", Position: 2, TypeName: "bigint"},
			{Name: "embedding", Position: 3, TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
		PrimaryKey: []string{"id"},
	}
}

func textOnlyTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "notes",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", Position: 1, TypeName: "integer", IsPrimaryKey: true},
			{Name: "content", Position: 2, TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}
}

func hybridSchemaCache() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.articles": hybridTable(),
			"public.metrics":  vectorNoTextTable(),
			"public.notes":    textOnlyTable(),
			"public.multi":    multiVectorTable(),
		},
		HasPgVector: true,
		Schemas:     []string{"public"},
	}
}

func testHandlerForHybrid(sc *schema.SchemaCache, fn EmbedFunc) http.Handler {
	ch := testCacheHolder(sc)
	h := NewHandler(nil, ch, nil, nil, nil, nil)
	h.ApplyOptions(WithEmbedder(fn))
	return h.Routes()
}

func TestExecuteFTSQuery_NilPoolBuildsSearchAndFilter(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(hybridSchemaCache()), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/collections/articles", nil)
	_, err := h.executeFTSQuery(req, hybridTable(), "hello", 5, "\"id\" = $1", []any{123})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database pool is not configured")

	q, args, qerr := buildFTSHybridQuery(hybridTable(), "hello", 5, "\"id\" = $1", []any{123})
	testutil.NoError(t, qerr)
	testutil.Contains(t, q, " AS _fts_rank")
	testutil.Contains(t, q, "ORDER BY _fts_rank DESC")
	testutil.Contains(t, q, "\"id\" = $1")
	testutil.Contains(t, q, "to_tsvector('english'::regconfig")
	testutil.Contains(t, q, "websearch_to_tsquery('english'::regconfig, $2)")
	// Args now include the synonym-expansion schema/table refs after the
	// search term: [filter..., term, schema, table, limit].
	testutil.Equal(t, 5, len(args))
	testutil.Equal(t, 123, args[0])
	testutil.Equal(t, "hello", args[1])
	testutil.Equal(t, hybridTable().Schema, args[2].(string))
	testutil.Equal(t, hybridTable().Name, args[3].(string))
	testutil.Equal(t, 5, args[4])

	customQ, _, customErr := buildFTSHybridQuery(hybridTable(), "hello", 5, "", nil, "simple")
	testutil.NoError(t, customErr)
	testutil.Contains(t, customQ, "to_tsvector('simple'::regconfig")
	testutil.Contains(t, customQ, "websearch_to_tsquery('simple'::regconfig, $1)")
}

func TestExecuteVectorQuery_NilPoolBuildsFilterArgs(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, testCacheHolder(hybridSchemaCache()), nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/collections/articles", nil)
	col, err := findVectorColumn(hybridTable(), "embedding")
	testutil.NoError(t, err)

	_, err = h.executeVectorQuery(req, hybridTable(), col, []float64{0.1, 0.2, 0.3}, "cosine", 5, "\"id\" = $1", []any{123})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "database pool is not configured")

	q, args, qerr := buildVectorHybridQuery(hybridTable(), col, []float64{0.1, 0.2, 0.3}, "cosine", 5, "\"id\" = $1", []any{123})
	testutil.NoError(t, qerr)
	testutil.Contains(t, q, " AS _distance")
	testutil.Contains(t, q, "\"id\" = $1")
	testutil.Equal(t, 3, len(args))
	testutil.Equal(t, 123, args[0])
	testutil.Equal(t, 5, args[2])
}

func TestHybridSearch_NoEmbedder(t *testing.T) {
	t.Parallel()
	h := testHandler(hybridSchemaCache())
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusNotImplemented, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "not configured")
}

func TestHybridSearch_NoPgVector(t *testing.T) {
	t.Parallel()
	sc := hybridSchemaCache()
	sc.HasPgVector = false
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) { return nil, nil }
	h := testHandlerForHybrid(sc, embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "pgvector")
}

func TestHybridSearch_NoTextColumns(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/metrics?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "no text columns")
}

func TestHybridSearch_NoVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/notes?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "no vector columns")
}

func TestHybridSearch_MutualExclusionWithNearest(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&nearest=[0.1,0.2,0.3]", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "cannot combine")
}

func TestHybridSearch_MutualExclusionWithSemanticQuery(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&semantic_query=world", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "cannot combine")
}

func TestHybridSearch_RejectsUnsupportedSearchParams(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)

	tests := []string{"fuzzy", "facets", "typo_threshold", "highlight"}
	for _, param := range tests {
		t.Run(param, func(t *testing.T) {
			t.Parallel()
			w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&"+param+"=true", "")
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			resp := decodeError(t, w)
			testutil.Contains(t, strings.ToLower(resp.Message), "unsupported parameter")
			testutil.Contains(t, resp.Message, param)
		})
	}
}

func TestHybridSearch_RejectsUnsupportedSearchParamsWithBlankSearch(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)

	searchCases := []struct {
		name  string
		query string
	}{
		{name: "empty", query: "search="},
		{name: "whitespace", query: "search=+++"},
	}
	params := []string{"fuzzy", "facets", "typo_threshold", "highlight"}

	for _, searchCase := range searchCases {
		searchCase := searchCase
		for _, param := range params {
			param := param
			t.Run(searchCase.name+"_"+param, func(t *testing.T) {
				t.Parallel()
				url := "/collections/articles?" + searchCase.query + "&semantic=true&" + param + "=true"
				w := doRequest(h, "GET", url, "")
				testutil.Equal(t, http.StatusBadRequest, w.Code)
				resp := decodeError(t, w)
				testutil.Contains(t, strings.ToLower(resp.Message), "unsupported parameter")
				testutil.Contains(t, resp.Message, param)
			})
		}
	}
}

func TestHybridSearchRejectsHighlightForSemanticSearchWithBlankAndNonEmptySearch(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)

	for _, query := range []string{"search=hello", "search=", "search=+++"} {
		query := query
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			w := doRequest(h, "GET", "/collections/articles?"+query+"&semantic=true&highlight=true", "")
			testutil.Equal(t, http.StatusBadRequest, w.Code)
			resp := decodeError(t, w)
			testutil.Contains(t, strings.ToLower(resp.Message), "unsupported")
			testutil.Contains(t, resp.Message, "highlight")
		})
	}
}

func TestHybridSearch_SemanticFalseIsRegularFTS(t *testing.T) {
	t.Parallel()
	h := testHandler(hybridSchemaCache())
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=false", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

func TestHybridSearch_EmbedTimeout(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) { return nil, context.DeadlineExceeded }
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusGatewayTimeout, w.Code)
}

func TestHybridSearch_EmbedAuthError(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return nil, &ai.ProviderError{StatusCode: 401, Message: "invalid key", Provider: "openai"}
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHybridSearch_DimensionMismatch(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3, 0.4}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "dimension mismatch")
}

func TestHybridSearch_AutoSelectVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		testutil.Equal(t, 1, len(texts))
		testutil.Equal(t, "hello", texts[0])
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHybridSearch_ExplicitVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=hello&semantic=true&vector_column=embedding", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHybridSearch_AmbiguousVectorColumn(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/multi?search=hello&semantic=true", "")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, strings.ToLower(resp.Message), "multiple vector columns")
}

func TestHybridSearch_FullFlowMockProvider(t *testing.T) {
	t.Parallel()
	called := false
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		called = true
		if len(texts) == 0 {
			return nil, errors.New("no text")
		}
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
	h := testHandlerForHybrid(hybridSchemaCache(), embedFn)
	w := doRequest(h, "GET", "/collections/articles?search=find+similar&semantic=true&distance=l2", "")
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, true, called)
	resp := decodeError(t, w)
	testutil.Equal(t, "internal error", resp.Message)
}

func TestHybridSearch_EmptyResponseWhenBothSignalsEmpty(t *testing.T) {
	t.Parallel()
	merged := rrfMerge(nil, nil, []string{"id"}, defaultRRFConstant)
	testutil.Equal(t, 0, len(merged))
}

func TestHybridSearch_ResponseEnvelopeDefaults(t *testing.T) {
	t.Parallel()
	resp := ListResponse{Page: 1, PerPage: 20, TotalItems: 0, TotalPages: 1, Items: []map[string]any{}}
	b, err := json.Marshal(resp)
	testutil.NoError(t, err)
	testutil.Contains(t, string(b), "\"page\":1")
	testutil.Contains(t, string(b), "\"perPage\":20")
	testutil.Contains(t, string(b), "\"totalPages\":1")
}

type hybridFakeConn struct {
	queries int
}

func (c *hybridFakeConn) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	c.queries++
	if strings.Contains(sql, "_fts_rank") {
		return hybridFakeRows([]map[string]any{
			{"id": int32(1), "title": "Alpha", "body": "needle", "embedding": []float32{1, 0, 0}},
			{"id": int32(2), "title": "Bravo", "body": "needle", "embedding": []float32{0.9, 0, 0}},
			{"id": int32(3), "title": "Charlie", "body": "needle", "embedding": []float32{0.8, 0, 0}},
			{"id": int32(4), "title": "Delta", "body": "needle", "embedding": []float32{0.7, 0, 0}},
		}), nil
	}
	return hybridFakeRows([]map[string]any{
		{"id": int32(1), "title": "Alpha", "body": "needle", "embedding": []float32{1, 0, 0}},
		{"id": int32(2), "title": "Bravo", "body": "needle", "embedding": []float32{0.9, 0, 0}},
		{"id": int32(3), "title": "Charlie", "body": "needle", "embedding": []float32{0.8, 0, 0}},
		{"id": int32(4), "title": "Delta", "body": "needle", "embedding": []float32{0.7, 0, 0}},
	}), nil
}

func (c *hybridFakeConn) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (c *hybridFakeConn) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (c *hybridFakeConn) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unexpected transaction")
}

type hybridRows struct {
	fields []pgconn.FieldDescription
	rows   [][]any
	idx    int
	closed bool
}

func hybridFakeRows(records []map[string]any) *hybridRows {
	fields := []pgconn.FieldDescription{
		{Name: "id"},
		{Name: "title"},
		{Name: "body"},
		{Name: "embedding"},
	}
	rows := make([][]any, len(records))
	for i, record := range records {
		rows[i] = []any{record["id"], record["title"], record["body"], record["embedding"]}
	}
	return &hybridRows{fields: fields, rows: rows, idx: -1}
}

func (r *hybridRows) Close() {
	r.closed = true
}

func (r *hybridRows) Err() error {
	return nil
}

func (r *hybridRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *hybridRows) FieldDescriptions() []pgconn.FieldDescription {
	return r.fields
}

func (r *hybridRows) Next() bool {
	if r.idx+1 >= len(r.rows) {
		r.closed = true
		return false
	}
	r.idx++
	return true
}

func (r *hybridRows) Scan(dest ...any) error {
	if r.idx < 0 || r.idx >= len(r.rows) {
		return errors.New("scan before next")
	}
	for i := range dest {
		ptr, ok := dest[i].(*any)
		if !ok {
			return errors.New("hybridRows only supports *any destinations")
		}
		*ptr = r.rows[r.idx][i]
	}
	return nil
}

func (r *hybridRows) Values() ([]any, error) {
	if r.idx < 0 || r.idx >= len(r.rows) {
		return nil, errors.New("values before next")
	}
	return r.rows[r.idx], nil
}

func (r *hybridRows) RawValues() [][]byte {
	return nil
}

func (r *hybridRows) Conn() *pgx.Conn {
	return nil
}

func TestHybridSearchPaginationEnvelopeUsesRequestedPage(t *testing.T) {
	t.Parallel()
	embedFn := func(_ context.Context, texts []string) ([][]float64, error) {
		if len(texts) != 1 || texts[0] != "needle" {
			t.Fatalf("embedding input = %v, want [needle]", texts)
		}
		return [][]float64{{1, 0, 0}}, nil
	}
	h := NewHandler(nil, testCacheHolder(hybridSchemaCache()), nil, nil, nil, nil)
	h.ApplyOptions(WithEmbedder(embedFn))
	fakeConn := &hybridFakeConn{}
	req := httptest.NewRequest(http.MethodGet, "/collections/articles?search=needle&semantic=true&page=2&perPage=2", nil)
	req = req.WithContext(tenant.ContextWithRequestConn(req.Context(), fakeConn))
	w := httptest.NewRecorder()

	h.handleHybridSearch(w, req, hybridTable(), "needle", "", "", 2, 2, "", nil)

	testutil.Equal(t, http.StatusOK, w.Code)
	var resp ListResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 2, resp.Page)
	testutil.Equal(t, 2, resp.PerPage)
	testutil.Equal(t, 4, resp.TotalItems)
	testutil.Equal(t, 2, resp.TotalPages)
	testutil.Equal(t, 2, len(resp.Items))
	testutil.Equal(t, 3.0, resp.Items[0]["id"].(float64))
	testutil.Equal(t, 4.0, resp.Items[1]["id"].(float64))
	testutil.Equal(t, 2, fakeConn.queries)
}
