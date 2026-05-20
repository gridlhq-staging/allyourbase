// Package graphql resolve.go contains GraphQL resolver functions that execute SQL queries with Row-Level Security support, including utilities for building parameterized WHERE and ORDER BY clauses and managing RLS context from authentication claims.
package graphql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
)

// DefaultMaxLimit is the maximum number of rows a GraphQL query can return.
// Applied when no limit is specified or when the requested limit exceeds this value.
const DefaultMaxLimit = 1000

type queryRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type txContextKey struct{}

func ctxWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	if tx == nil {
		return ctx
	}
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) pgx.Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx
}

// withRLSQueryRunner executes fn with RLS (Row-Level Security) support based on the authenticated user's claims from the context. If a transaction is already present in ctx, it is reused. If no claims are found, the function runs directly against the pool. Otherwise, a new transaction is created, configured with RLS context, and committed after the function completes.
func withRLSQueryRunner(ctx context.Context, pool *pgxpool.Pool, fn func(q queryRunner) (interface{}, error)) (interface{}, error) {
	if tx := txFromContext(ctx); tx != nil {
		return fn(tx)
	}

	if pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}

	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return fn(pool)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		return nil, fmt.Errorf("set RLS context: %w", err)
	}

	result, err := fn(tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func queryAndScanRows(ctx context.Context, q queryRunner, sql string, args ...any) ([]map[string]any, int64, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}

	result, scanErr := scanRows(rows)
	affected := rows.CommandTag().RowsAffected()
	rows.Close()
	if scanErr != nil {
		return nil, 0, scanErr
	}
	return result, affected, nil
}

// scanRows scans pgx rows into a slice of maps.
func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	var result []map[string]any
	descs := rows.FieldDescriptions()
	for rows.Next() {
		values := make([]any, len(descs))
		ptrs := make([]any, len(descs))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		record := make(map[string]any, len(descs))
		for i, desc := range descs {
			record[desc.Name] = normalizeValue(values[i])
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// normalizeValue converts certain pgx return types to JSON-friendly Go types.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}

// resolveTable is the root-level resolver for a single table's query field.
// It builds and executes a SELECT query with RLS enforcement.
func resolveTable(ctx context.Context, tbl *schema.Table, pool *pgxpool.Pool, cache *schema.SchemaCache, args map[string]interface{}) (interface{}, error) {
	// Extract typed arguments
	var whereArg map[string]interface{}
	if w, ok := args["where"]; ok && w != nil {
		whereArg, _ = w.(map[string]interface{})
	}
	var orderByArg map[string]interface{}
	if o, ok := args["order_by"]; ok && o != nil {
		orderByArg, _ = o.(map[string]interface{})
	}
	var limit int
	if l, ok := args["limit"]; ok && l != nil {
		switch v := l.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		}
	}
	var offset int
	if o, ok := args["offset"]; ok && o != nil {
		switch v := o.(type) {
		case int:
			offset = v
		case float64:
			offset = int(v)
		}
	}

	spatialFilters, err := parseSpatialArgs(tbl, cache, args)
	if err != nil {
		return nil, err
	}

	sql, sqlArgs, err := buildSelectQueryWithSpatial(tbl, whereArg, spatialFilters, orderByArg, limit, offset)
	if err != nil {
		return nil, err
	}

	result, err := withRLSQueryRunner(ctx, pool, func(q queryRunner) (interface{}, error) {
		records, _, queryErr := queryAndScanRows(ctx, q, sql, sqlArgs...)
		if queryErr != nil {
			return nil, fmt.Errorf("query: %w", queryErr)
		}
		if dl := dataloaderFromCtx(ctx); dl != nil {
			primeRowsForTableRelationships(dl, tbl, records)
		}
		return records, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
