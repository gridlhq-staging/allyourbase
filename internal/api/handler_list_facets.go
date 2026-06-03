// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun02_pm_1_facets_search_coherence/allyourbase_dev/internal/api/handler_list_facets.go.
package api

import (
	"context"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5"
)

func executeFacetQueries(ctx context.Context, querier Querier, tbl *schema.Table, opts listOpts) (FacetCounts, error) {
	queries, allArgs := buildFacetCountQueries(tbl, opts)
	if len(queries) == 0 {
		return nil, nil
	}

	facets := make(FacetCounts, len(queries))
	for facetCol, query := range queries {
		rows, err := querier.Query(ctx, query, allArgs...)
		if err != nil {
			return nil, err
		}
		values, err := scanFacetRows(rows)
		if err != nil {
			return nil, err
		}
		facets[facetCol] = values
	}
	return facets, nil
}

func scanFacetRows(rows pgx.Rows) ([]FacetValueCount, error) {
	defer rows.Close()

	values := []FacetValueCount{}
	for rows.Next() {
		var value any
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		values = append(values, FacetValueCount{Value: value, Count: int(count)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
