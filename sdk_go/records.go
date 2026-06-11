// Package allyourbase.
package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ListParams struct {
	Page      int
	PerPage   int
	Sort      string
	Filter    string
	Search    string
	Fields    string
	Expand    string
	SkipTotal bool

	Fuzzy         bool
	TypoThreshold *float64
	Highlight     bool
	Facets        []string
	Semantic      bool
	SemanticQuery string
}

type GetParams struct {
	Fields string
	Expand string
}

type RecordsClient struct {
	client *Client
}

func recordsCollectionPath(collection string) string {
	return "/api/collections/" + url.PathEscape(collection)
}

func recordItemPath(collection, id string) string {
	return recordsCollectionPath(collection) + "/" + url.PathEscape(id)
}

// List returns a paginated list of records from the given collection, filtered and sorted by the provided parameters.
func (r *RecordsClient) List(ctx context.Context, collection string, params ListParams) (*ListResponse, error) {
	q := url.Values{}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.PerPage > 0 {
		q.Set("perPage", strconv.Itoa(params.PerPage))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}
	if params.Filter != "" {
		q.Set("filter", params.Filter)
	}
	if params.Search != "" {
		q.Set("search", params.Search)
	}
	if params.Fields != "" {
		q.Set("fields", params.Fields)
	}
	if params.Expand != "" {
		q.Set("expand", params.Expand)
	}
	if params.SkipTotal {
		q.Set("skipTotal", "true")
	}
	if params.Fuzzy {
		q.Set("fuzzy", "true")
	}
	if params.TypoThreshold != nil {
		q.Set("typo_threshold", strconv.FormatFloat(*params.TypoThreshold, 'f', -1, 64))
	}
	if params.Highlight {
		q.Set("highlight", "true")
	}
	if len(params.Facets) > 0 {
		q.Set("facets", strings.Join(params.Facets, ","))
	}
	if params.Semantic {
		q.Set("semantic", "true")
	}
	if params.SemanticQuery != "" {
		q.Set("semantic_query", params.SemanticQuery)
	}
	body, err := r.client.doJSON(ctx, http.MethodGet, recordsCollectionPath(collection), q, nil)
	if err != nil {
		return nil, err
	}
	var out ListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get retrieves a single record by ID from the given collection with optional field selection and relation expansion.
func (r *RecordsClient) Get(ctx context.Context, collection, id string, params GetParams) (map[string]any, error) {
	q := url.Values{}
	if params.Fields != "" {
		q.Set("fields", params.Fields)
	}
	if params.Expand != "" {
		q.Set("expand", params.Expand)
	}
	body, err := r.client.doJSON(ctx, http.MethodGet, recordItemPath(collection, id), q, nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RecordsClient) Create(ctx context.Context, collection string, data map[string]any) (map[string]any, error) {
	return r.writeRecord(ctx, http.MethodPost, recordsCollectionPath(collection), data)
}

func (r *RecordsClient) Update(ctx context.Context, collection, id string, data map[string]any) (map[string]any, error) {
	return r.writeRecord(ctx, http.MethodPatch, recordItemPath(collection, id), data)
}

func (r *RecordsClient) Delete(ctx context.Context, collection, id string) error {
	_, err := r.client.doJSON(ctx, http.MethodDelete, recordItemPath(collection, id), nil, nil)
	return err
}

func (r *RecordsClient) Batch(ctx context.Context, collection string, operations []BatchOperation) ([]BatchResult, error) {
	body, err := r.client.doJSON(ctx, http.MethodPost, recordsCollectionPath(collection)+"/batch", nil, map[string]any{"operations": operations})
	if err != nil {
		return nil, err
	}
	var out []BatchResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RecordsClient) writeRecord(ctx context.Context, method, path string, data map[string]any) (map[string]any, error) {
	body, err := r.client.doJSON(ctx, method, path, nil, data)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
