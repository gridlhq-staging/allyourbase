package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestRecordsListQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		params   ListParams
		expected map[string]string
	}{
		{
			name: "existing pagination and skip total params",
			params: ListParams{
				PerPage:   10,
				SkipTotal: true,
			},
			expected: map[string]string{
				"perPage":   "10",
				"skipTotal": "true",
			},
		},
		{
			name: "list search params match JavaScript query keys",
			params: ListParams{
				Search:        "banan",
				Fuzzy:         true,
				TypoThreshold: ptrFloat64(0.2),
				Highlight:     true,
				Facets:        []string{"category"},
				Semantic:      true,
				SemanticQuery: "banana semantic",
			},
			expected: map[string]string{
				"search":         "banan",
				"fuzzy":          "true",
				"typo_threshold": "0.2",
				"highlight":      "true",
				"facets":         "category",
				"semantic":       "true",
				"semantic_query": "banana semantic",
			},
		},
		{
			name: "false boolean and empty slice params are omitted",
			params: ListParams{
				Fuzzy:     false,
				Highlight: false,
				Facets:    []string{},
				Semantic:  false,
			},
			expected: map[string]string{},
		},
		{
			name: "explicit zero typo threshold is encoded",
			params: ListParams{
				TypoThreshold: ptrFloat64(0),
			},
			expected: map[string]string{
				"typo_threshold": "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actual := map[string]string{}
				for key, values := range r.URL.Query() {
					if len(values) != 1 {
						t.Fatalf("query key %q has %d values in raw query %q", key, len(values), r.URL.RawQuery)
					}
					actual[key] = values[0]
				}
				if !reflect.DeepEqual(actual, tt.expected) {
					t.Fatalf("bad query for raw query %q\nactual:   %#v\nexpected: %#v", r.URL.RawQuery, actual, tt.expected)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items":      []map[string]any{{"id": "rec_1"}},
					"page":       1,
					"perPage":    10,
					"totalItems": 1,
					"totalPages": 1,
				})
			}))
			defer ts.Close()

			c := NewClient(ts.URL)
			_, err := c.Records.List(context.Background(), "posts", tt.params)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecordsListParsesCursorEnvelope(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":      []map[string]any{{"id": "rec_1"}},
			"perPage":    10,
			"nextCursor": "cursor_2",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Records.List(context.Background(), "posts", ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res.NextCursor == nil || *res.NextCursor != "cursor_2" {
		t.Fatalf("expected next cursor to be parsed, got %+v", res.NextCursor)
	}
	if res.Page != 0 || res.TotalItems != 0 || res.TotalPages != 0 {
		t.Fatalf("cursor envelope should leave offset fields at zero values, got %+v", res)
	}
	if len(res.Items) != 1 || res.Items[0]["id"] != "rec_1" {
		t.Fatalf("unexpected cursor envelope items: %+v", res.Items)
	}
}

func TestRecordsListEscapesCollectionPathSegment(t *testing.T) {
	collection := "posts?admin=true"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("admin") {
			t.Fatalf("collection name injected query params into request: %q", r.URL.RawQuery)
		}
		wantPath := "/api/collections/" + url.PathEscape(collection)
		if r.URL.EscapedPath() != wantPath {
			t.Fatalf("escaped path = %q, want %q", r.URL.EscapedPath(), wantPath)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":      []map[string]any{{"id": "rec_1"}},
			"page":       1,
			"perPage":    10,
			"totalItems": 1,
			"totalPages": 1,
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Records.List(context.Background(), collection, ListParams{}); err != nil {
		t.Fatal(err)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func TestRecordsGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/collections/posts/rec_1" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Records.Get(context.Background(), "posts", "rec_1", GetParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res["id"] != "rec_1" {
		t.Fatalf("unexpected record")
	}
}

func TestRecordsCreate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1", "title": "ok"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Records.Create(context.Background(), "posts", map[string]any{"title": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if res["id"] != "rec_1" {
		t.Fatalf("unexpected record: %+v", res)
	}
}

func TestRecordsUpdateDeleteBatch(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch step {
		case 0:
			if r.Method != http.MethodPatch {
				t.Fatalf("method=%s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1", "title": "updated"})
		case 1:
			if r.Method != http.MethodDelete {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		case 2:
			if r.URL.Path != "/api/collections/posts/batch" {
				t.Fatalf("path=%s", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"index": 0, "status": 201, "body": map[string]any{"id": "rec_2"}}})
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	updated, err := c.Records.Update(context.Background(), "posts", "rec_1", map[string]any{"title": "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if updated["title"] != "updated" {
		t.Fatalf("bad update")
	}
	if err := c.Records.Delete(context.Background(), "posts", "rec_1"); err != nil {
		t.Fatal(err)
	}
	res, err := c.Records.Batch(context.Background(), "posts", []BatchOperation{{Method: "create", Body: map[string]any{"title": "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Status != 201 {
		t.Fatalf("bad batch result")
	}
}
