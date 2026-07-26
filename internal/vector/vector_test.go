package vector

import (
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Operator selection ---

func TestDistanceOperator(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"cosine", "<=>"},
		{"l2", "<->"},
		{"inner_product", "<#>"},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			op, err := DistanceOperator(tt.metric)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if op != tt.want {
				t.Errorf("DistanceOperator(%q) = %q, want %q", tt.metric, op, tt.want)
			}
		})
	}
}

func TestDistanceOperatorInvalid(t *testing.T) {
	_, err := DistanceOperator("euclidean")
	testutil.ErrorContains(t, err, `unsupported distance metric "euclidean"`)
}

func TestValidMetrics(t *testing.T) {
	metrics := ValidMetrics()
	if len(metrics) != 3 {
		t.Fatalf("expected 3 valid metrics, got %d", len(metrics))
	}
	seen := map[string]bool{}
	for _, m := range metrics {
		seen[m] = true
	}
	for _, want := range []string{"cosine", "l2", "inner_product"} {
		if !seen[want] {
			t.Errorf("missing metric %q in ValidMetrics()", want)
		}
	}
}

// --- SQL generation for nearest query ---

func TestBuildNearestQuery(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "documents",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", JSONType: "string", IsPrimaryKey: true},
			{Name: "title", TypeName: "text", JSONType: "string"},
			{Name: "embedding", TypeName: "vector(3)", JSONType: "array", IsVector: true, VectorDim: 3},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := `SELECT "id", "title", "embedding", "embedding" <=> $1 AS _distance FROM "public"."documents" ORDER BY "embedding" <=> $1 ASC LIMIT $2`
	if sql != wantSQL {
		t.Fatalf("SQL mismatch\ngot:  %s\nwant: %s", sql, wantSQL)
	}

	wantArgs := []any{"[0.1,0.2,0.3]", 10}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args mismatch\ngot:  %#v\nwant: %#v", args, wantArgs)
	}
}

func TestBuildNearestQueryL2(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "items",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", JSONType: "integer", IsPrimaryKey: true},
			{Name: "vec", TypeName: "vector(2)", JSONType: "array", IsVector: true, VectorDim: 2},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "vec",
		QueryVector:  []float64{1.0, 2.0},
		Metric:       "l2",
		Limit:        5,
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := `SELECT "id", "vec", "vec" <-> $1 AS _distance FROM "public"."items" ORDER BY "vec" <-> $1 ASC LIMIT $2`
	if sql != wantSQL {
		t.Fatalf("SQL mismatch\ngot:  %s\nwant: %s", sql, wantSQL)
	}

	wantArgs := []any{"[1,2]", 5}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args mismatch\ngot:  %#v\nwant: %#v", args, wantArgs)
	}
}

func TestBuildNearestQueryInvalidMetric(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "manhattan",
		Limit:        10,
	})
	testutil.ErrorContains(t, err, `unsupported distance metric "manhattan"`)
}

func TestBuildNearestQueryDimensionMismatch(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "embedding", TypeName: "vector(3)", IsVector: true, VectorDim: 3},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2},
		Metric:       "cosine",
		Limit:        10,
	})
	testutil.ErrorContains(t, err, `dimension mismatch: query vector has 2 dimensions, column "embedding" expects 3`)
}

func TestBuildNearestQueryUnknownColumn(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "title", TypeName: "text"},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	})
	testutil.ErrorContains(t, err, `column "embedding" not found in table "docs"`)
}

func TestBuildNearestQueryNonVectorColumn(t *testing.T) {
	tbl := &schema.Table{
		Schema: "public", Name: "docs",
		Columns: []*schema.Column{
			{Name: "title", TypeName: "text", JSONType: "string"},
		},
	}
	_, _, err := BuildNearestQuery(NearestParams{
		Table:        tbl,
		VectorColumn: "title",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
	})
	testutil.ErrorContains(t, err, `column "title" is not a vector column`)
}

func TestBuildNearestQueryWithFilter(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "documents",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", JSONType: "string", IsPrimaryKey: true},
			{Name: "status", TypeName: "text", JSONType: "string"},
			{Name: "embedding", TypeName: "vector(3)", JSONType: "array", IsVector: true, VectorDim: 3},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
		FilterSQL:    `"status" = $1`,
		FilterArgs:   []any{"active"},
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := `SELECT "id", "status", "embedding", "embedding" <=> $2 AS _distance FROM "public"."documents" WHERE "status" = $1 ORDER BY "embedding" <=> $2 ASC LIMIT $3`
	if sql != wantSQL {
		t.Fatalf("SQL mismatch\ngot:  %s\nwant: %s", sql, wantSQL)
	}

	wantArgs := []any{"active", "[0.1,0.2,0.3]", 10}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args mismatch\ngot:  %#v\nwant: %#v", args, wantArgs)
	}
}

func TestBuildNearestQueryWithMultiArgFilter(t *testing.T) {
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "documents",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", JSONType: "string", IsPrimaryKey: true},
			{Name: "status", TypeName: "text", JSONType: "string"},
			{Name: "kind", TypeName: "text", JSONType: "string"},
			{Name: "embedding", TypeName: "vector(3)", JSONType: "array", IsVector: true, VectorDim: 3},
		},
	}

	params := NearestParams{
		Table:        tbl,
		VectorColumn: "embedding",
		QueryVector:  []float64{0.1, 0.2, 0.3},
		Metric:       "cosine",
		Limit:        10,
		FilterSQL:    `"status" = $1 AND "kind" = $2`,
		FilterArgs:   []any{"active", "news"},
	}

	sql, args, err := BuildNearestQuery(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := `SELECT "id", "status", "kind", "embedding", "embedding" <=> $3 AS _distance FROM "public"."documents" WHERE "status" = $1 AND "kind" = $2 ORDER BY "embedding" <=> $3 ASC LIMIT $4`
	if sql != wantSQL {
		t.Fatalf("SQL mismatch\ngot:  %s\nwant: %s", sql, wantSQL)
	}

	wantArgs := []any{"active", "news", "[0.1,0.2,0.3]", 10}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args mismatch\ngot:  %#v\nwant: %#v", args, wantArgs)
	}
}

func TestFormatVectorLiteralAvoidsExponentNotation(t *testing.T) {
	got := FormatVectorLiteral([]float64{0.0000001, -2.5, 3})
	want := "[0.0000001,-2.5,3]"
	if got != want {
		t.Fatalf("FormatVectorLiteral() = %q, want %q", got, want)
	}
}

// --- Vector column detection ---

func TestIsVectorColumn(t *testing.T) {
	tests := []struct {
		typeName string
		want     bool
	}{
		{"vector(3)", true},
		{"vector(1536)", true},
		{"vector", true},
		{"text", false},
		{"integer", false},
		{"jsonb", false},
		{"geometry", false},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := IsVectorType(tt.typeName)
			if got != tt.want {
				t.Errorf("IsVectorType(%q) = %v, want %v", tt.typeName, got, tt.want)
			}
		})
	}
}

func TestParseVectorDimension(t *testing.T) {
	tests := []struct {
		typeName string
		want     int
	}{
		{"vector(3)", 3},
		{"vector(1536)", 1536},
		{"VECTOR(4)", 4},
		{"text", 0},
		{"vector", 0},
		{"vector(3", 0},
		{"vector()", 0},
		{"vector(x)", 0},
		{"vector(12.5)", 0},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got := ParseVectorDim(tt.typeName)
			if got != tt.want {
				t.Errorf("ParseVectorDim(%q) = %d, want %d", tt.typeName, got, tt.want)
			}
		})
	}
}

// --- Vector index SQL ---

func TestBuildCreateIndexSQL(t *testing.T) {
	tests := []struct {
		name    string
		params  IndexParams
		wantSQL string
	}{
		{
			name: "hnsw cosine",
			params: IndexParams{
				Schema:    "public",
				Table:     "documents",
				Column:    "embedding",
				Method:    "hnsw",
				Metric:    "cosine",
				IndexName: "idx_documents_embedding",
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_documents_embedding" ON "public"."documents" USING hnsw ("embedding" vector_cosine_ops)`,
		},
		{
			name: "ivfflat l2",
			params: IndexParams{
				Schema:    "public",
				Table:     "documents",
				Column:    "embedding",
				Method:    "ivfflat",
				Metric:    "l2",
				IndexName: "idx_documents_embedding_ivf",
				Lists:     100,
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_documents_embedding_ivf" ON "public"."documents" USING ivfflat ("embedding" vector_l2_ops) WITH (lists = 100)`,
		},
		{
			name: "hnsw inner_product",
			params: IndexParams{
				Schema:    "public",
				Table:     "items",
				Column:    "vec",
				Method:    "hnsw",
				Metric:    "inner_product",
				IndexName: "idx_items_vec",
			},
			wantSQL: `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_items_vec" ON "public"."items" USING hnsw ("vec" vector_ip_ops)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, err := BuildCreateIndexSQL(tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("got:\n  %s\nwant:\n  %s", sql, tt.wantSQL)
			}
		})
	}
}

func TestBuildCreateIndexSQLInvalidMethod(t *testing.T) {
	_, err := BuildCreateIndexSQL(IndexParams{
		Schema: "public", Table: "docs", Column: "vec",
		Method: "btree", Metric: "cosine", IndexName: "idx",
	})
	testutil.ErrorContains(t, err, `unsupported index method "btree"`)
}

func TestBuildCreateIndexSQLInvalidMetric(t *testing.T) {
	_, err := BuildCreateIndexSQL(IndexParams{
		Schema: "public", Table: "docs", Column: "vec",
		Method: "hnsw", Metric: "hamming", IndexName: "idx",
	})
	testutil.ErrorContains(t, err, `unsupported metric "hamming" for index`)
}
