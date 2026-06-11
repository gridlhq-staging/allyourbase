// Package api.
package api

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5"
)

func executeFacetQueries(ctx context.Context, querier Querier, tbl *schema.Table, opts listOpts) (FacetCounts, FacetStats, error) {
	facets, err := executeFacetCountQueries(ctx, querier, buildFacetCountQueries(tbl, opts))
	if err != nil {
		return nil, nil, err
	}
	stats, err := executeFacetStatsQueries(ctx, querier, buildFacetStatsQueries(tbl, opts))
	if err != nil {
		return nil, nil, err
	}
	return facets, stats, nil
}

func executeFacetCountQueries(ctx context.Context, querier Querier, queries map[string]facetCountQuery) (FacetCounts, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	facets := make(FacetCounts, len(queries))
	for facetCol, query := range queries {
		rows, err := querier.Query(ctx, query.sql, query.args...)
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

func executeFacetStatsQueries(ctx context.Context, querier Querier, queries map[string]facetStatsQuery) (FacetStats, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	stats := make(FacetStats, len(queries))
	for facetCol, query := range queries {
		bounds, ok, err := scanFacetStatsRow(querier.QueryRow(ctx, query.sql, query.args...))
		if err != nil {
			return nil, err
		}
		if ok {
			stats[facetCol] = bounds
		}
	}
	if len(stats) == 0 {
		return nil, nil
	}
	return stats, nil
}

func scanFacetStatsRow(row pgx.Row) (FacetMinMax, bool, error) {
	var min sql.NullString
	var max sql.NullString
	if err := row.Scan(&min, &max); err != nil {
		return FacetMinMax{}, false, err
	}
	if !min.Valid || !max.Valid {
		return FacetMinMax{}, false, nil
	}
	return FacetMinMax{Min: json.Number(min.String), Max: json.Number(max.String)}, true, nil
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
