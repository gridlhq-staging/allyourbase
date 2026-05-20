// Package graphql contains SQL-building helpers for GraphQL table queries.
package graphql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// operatorSQL maps WhereInput operator keys to SQL operators.
var operatorSQL = map[string]string{
	"_eq":    "=",
	"_neq":   "!=",
	"_gt":    ">",
	"_gte":   ">=",
	"_lt":    "<",
	"_lte":   "<=",
	"_like":  "LIKE",
	"_ilike": "ILIKE",
}

func resolveWhere(args map[string]interface{}, tbl *schema.Table, paramIdx int) (string, []any, error) {
	if len(args) == 0 {
		return "", nil, nil
	}

	var parts []string
	var allArgs []any
	idx := paramIdx

	for _, key := range sortedMapKeys(args) {
		val := args[key]

		switch key {
		case "_and", "_or":
			sql, subArgs, nextIdx, err := resolveLogicalClause(key, val, tbl, idx)
			if err != nil {
				return "", nil, err
			}
			parts, allArgs = appendResolvedClause(parts, allArgs, sql, subArgs)
			idx = nextIdx

		case "_not":
			sub, ok := val.(map[string]interface{})
			if !ok {
				return "", nil, fmt.Errorf("_not must be an object")
			}
			sql, subArgs, nextIdx, err := resolveLogicalNot(sub, tbl, idx)
			if err != nil {
				return "", nil, err
			}
			parts, allArgs = appendResolvedClause(parts, allArgs, sql, subArgs)
			idx = nextIdx

		default:
			col := tbl.ColumnByName(key)
			if col == nil {
				return "", nil, fmt.Errorf("unknown column: %s", key)
			}

			ops, ok := val.(map[string]interface{})
			if !ok {
				return "", nil, fmt.Errorf("column %s filter must be an object", key)
			}

			columnParts, columnArgs, nextIdx, err := resolveColumnFilter(key, ops, idx)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, columnParts...)
			allArgs = append(allArgs, columnArgs...)
			idx = nextIdx
		}
	}

	return strings.Join(parts, " AND "), allArgs, nil
}

func sortedMapKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendResolvedClause(parts []string, args []any, sql string, subArgs []any) ([]string, []any) {
	if sql == "" {
		return parts, args
	}
	return append(parts, sql), append(args, subArgs...)
}

func resolveLogicalClause(key string, value interface{}, tbl *schema.Table, idx int) (string, []any, int, error) {
	list, ok := value.([]interface{})
	if !ok {
		return "", nil, idx, fmt.Errorf("%s must be a list", key)
	}

	joiner := " AND "
	if key == "_or" {
		joiner = " OR "
	}
	return resolveLogicalCombinator(key, joiner, list, tbl, idx)
}

func resolveLogicalCombinator(key, joiner string, list []interface{}, tbl *schema.Table, idx int) (string, []any, int, error) {
	var combinedParts []string
	var allArgs []any

	for _, item := range list {
		sub, ok := item.(map[string]interface{})
		if !ok {
			return "", nil, idx, fmt.Errorf("%s items must be objects", key)
		}

		sql, subArgs, err := resolveWhere(sub, tbl, idx)
		if err != nil {
			return "", nil, idx, err
		}
		if sql == "" {
			continue
		}
		combinedParts = append(combinedParts, sql)
		allArgs = append(allArgs, subArgs...)
		idx += len(subArgs)
	}

	if len(combinedParts) == 0 {
		return "", nil, idx, nil
	}
	return "(" + strings.Join(combinedParts, joiner) + ")", allArgs, idx, nil
}

func resolveLogicalNot(sub map[string]interface{}, tbl *schema.Table, idx int) (string, []any, int, error) {
	sql, subArgs, err := resolveWhere(sub, tbl, idx)
	if err != nil {
		return "", nil, idx, err
	}
	if sql == "" {
		return "", nil, idx, nil
	}
	return "NOT (" + sql + ")", subArgs, idx + len(subArgs), nil
}

func resolveColumnFilter(key string, ops map[string]interface{}, idx int) ([]string, []any, int, error) {
	opKeys := sortedMapKeys(ops)
	parts := make([]string, 0, len(opKeys))
	var allArgs []any
	for _, opKey := range opKeys {
		opVal := ops[opKey]

		if opKey == "_is_null" {
			boolVal, ok := opVal.(bool)
			if !ok {
				return nil, nil, idx, fmt.Errorf("_is_null must be boolean")
			}
			if boolVal {
				parts = append(parts, sqlutil.QuoteIdent(key)+" IS NULL")
			} else {
				parts = append(parts, sqlutil.QuoteIdent(key)+" IS NOT NULL")
			}
			continue
		}

		if opKey == "_in" {
			list, ok := opVal.([]interface{})
			if !ok {
				return nil, nil, idx, fmt.Errorf("_in must be a list")
			}
			placeholders := make([]string, len(list))
			for i, value := range list {
				placeholders[i] = fmt.Sprintf("$%d", idx)
				allArgs = append(allArgs, value)
				idx++
			}
			parts = append(parts, sqlutil.QuoteIdent(key)+" IN ("+strings.Join(placeholders, ", ")+")")
			continue
		}

		sqlOp, ok := operatorSQL[opKey]
		if !ok {
			return nil, nil, idx, fmt.Errorf("unknown operator: %s", opKey)
		}
		parts = append(parts, fmt.Sprintf("%s %s $%d", sqlutil.QuoteIdent(key), sqlOp, idx))
		allArgs = append(allArgs, opVal)
		idx++
	}

	return parts, allArgs, idx, nil
}

func resolveOrderBy(args map[string]interface{}, tbl *schema.Table) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	var parts []string
	for _, key := range sortedMapKeys(args) {
		col := tbl.ColumnByName(key)
		if col == nil {
			return "", fmt.Errorf("unknown column: %s", key)
		}

		dir, ok := args[key].(string)
		if !ok {
			return "", fmt.Errorf("order_by value for %s must be ASC or DESC", key)
		}
		dirUpper := strings.ToUpper(dir)
		if dirUpper != "ASC" && dirUpper != "DESC" {
			return "", fmt.Errorf("order_by value for %s must be ASC or DESC, got %s", key, dir)
		}
		parts = append(parts, sqlutil.QuoteIdent(key)+" "+dirUpper)
	}

	return strings.Join(parts, ", "), nil
}

// buildSelectQuery constructs a SELECT query from table metadata and GraphQL arguments.
// Exported for testing. Returns the SQL string, args, and any error.
func buildSelectQuery(tbl *schema.Table, where map[string]interface{}, orderBy map[string]interface{}, limit, offset int) (string, []any, error) {
	return buildSelectQueryWithSpatial(tbl, where, nil, orderBy, limit, offset)
}

// buildSelectQueryWithSpatial constructs a parameterized SELECT query from table metadata, optional WHERE/spatial/ORDER BY clauses, and limit/offset pagination.
func buildSelectQueryWithSpatial(
	tbl *schema.Table,
	where map[string]interface{},
	spatialFilters []spatial.Filter,
	orderBy map[string]interface{},
	limit, offset int,
) (string, []any, error) {
	var b strings.Builder
	var allArgs []any
	paramIdx := 1

	b.WriteString("SELECT ")
	b.WriteString(buildGraphQLProjection(tbl))
	b.WriteString(" FROM ")
	b.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))

	whereParts := make([]string, 0, 1+len(spatialFilters))
	if len(where) > 0 {
		whereSQL, whereArgs, err := resolveWhere(where, tbl, paramIdx)
		if err != nil {
			return "", nil, err
		}
		if whereSQL != "" {
			whereParts = append(whereParts, whereSQL)
			allArgs = append(allArgs, whereArgs...)
			paramIdx += len(whereArgs)
		}
	}

	for _, filter := range spatialFilters {
		if filter == nil {
			continue
		}
		filterSQL, filterArgs, err := filter.WhereClause(paramIdx)
		if err != nil {
			return "", nil, err
		}
		if filterSQL == "" {
			continue
		}
		whereParts = append(whereParts, filterSQL)
		allArgs = append(allArgs, filterArgs...)
		paramIdx += len(filterArgs)
	}

	if len(whereParts) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(whereParts, " AND "))
	}

	if len(orderBy) > 0 {
		orderSQL, err := resolveOrderBy(orderBy, tbl)
		if err != nil {
			return "", nil, err
		}
		if orderSQL != "" {
			b.WriteString(" ORDER BY ")
			b.WriteString(orderSQL)
		}
	}

	effectiveLimit := DefaultMaxLimit
	if limit > 0 && limit < DefaultMaxLimit {
		effectiveLimit = limit
	}
	b.WriteString(fmt.Sprintf(" LIMIT $%d", paramIdx))
	allArgs = append(allArgs, effectiveLimit)
	paramIdx++

	if offset > 0 {
		b.WriteString(fmt.Sprintf(" OFFSET $%d", paramIdx))
		allArgs = append(allArgs, offset)
	}

	return b.String(), allArgs, nil
}

// buildGraphQLProjection returns the SELECT column list, converting geometry/geography columns to GeoJSON via ST_AsGeoJSON and selecting all other columns by name.
func buildGraphQLProjection(tbl *schema.Table) string {
	if tbl == nil || !tbl.HasGeometry() {
		return "*"
	}

	selectExprs := make([]string, 0, len(tbl.Columns))
	for _, col := range tbl.Columns {
		if col.IsGeometry || col.IsGeography {
			selectExprs = append(selectExprs, fmt.Sprintf("ST_AsGeoJSON(%s)::jsonb AS %s", sqlutil.QuoteIdent(col.Name), sqlutil.QuoteIdent(col.Name)))
			continue
		}
		selectExprs = append(selectExprs, sqlutil.QuoteIdent(col.Name))
	}
	if len(selectExprs) == 0 {
		return "*"
	}
	return strings.Join(selectExprs, ", ")
}
