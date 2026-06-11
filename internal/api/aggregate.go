// Package api.
package api

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

const maxAggregateLen = 1000 // max characters in aggregate expression

var sqlParamRefPattern = regexp.MustCompile(`\$(\d+)`)

// AggregateExpr represents a parsed aggregate function call.
// Column is empty for bare "count" (which maps to COUNT(*)).
type AggregateExpr struct {
	Func   string // count, count_distinct, sum, avg, min, max
	Column string // target column name (empty for bare count)
}

// allowedAggregateFuncs is the set of supported aggregate function names.
var allowedAggregateFuncs = map[string]bool{
	"count":          true,
	"count_distinct": true,
	"sum":            true,
	"avg":            true,
	"min":            true,
	"max":            true,
	"bbox":           true,
	"centroid":       true,
}

// numericTypeNames is the set of PostgreSQL type names that sum/avg can operate on.
var numericTypeNames = map[string]bool{
	"integer":          true,
	"bigint":           true,
	"smallint":         true,
	"numeric":          true,
	"decimal":          true,
	"real":             true,
	"double precision": true,
	"int2":             true,
	"int4":             true,
	"int8":             true,
	"float4":           true,
	"float8":           true,
	"money":            true,
}

// requiresNumericColumn returns true for aggregate functions that only work on numeric types.
func requiresNumericColumn(funcName string) bool {
	return funcName == "sum" || funcName == "avg"
}

func requiresSpatialColumn(funcName string) bool {
	return funcName == "bbox" || funcName == "centroid"
}

// isNumericType checks if a column type is numeric (for sum/avg validation).
func isNumericType(typeName string) bool {
	base := normalizedColumnTypeName(typeName)
	return numericTypeNames[base]
}

func normalizedColumnTypeName(typeName string) string {
	base := strings.ToLower(typeName)
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return base
}

// parseAggregate parses a comma-separated aggregate expression string into validated expressions.
// Example input: "count,sum(price),avg(quantity)"
func parseAggregate(tbl *schema.Table, input string) ([]AggregateExpr, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty aggregate expression")
	}
	if len(input) > maxAggregateLen {
		return nil, fmt.Errorf("aggregate expression too long (max %d characters)", maxAggregateLen)
	}

	parts := splitAggregateExprs(input)
	exprs := make([]AggregateExpr, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		expr, err := parseSingleAggregate(tbl, part)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}

	if len(exprs) == 0 {
		return nil, fmt.Errorf("empty aggregate expression")
	}

	return exprs, nil
}

// splitAggregateExprs splits on commas that are NOT inside parentheses.
func splitAggregateExprs(input string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range input {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, input[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, input[start:])
	return parts
}

// parseSingleAggregate parses one aggregate expression like "count", "sum(price)", "count_distinct(col)".
func parseSingleAggregate(tbl *schema.Table, expr string) (AggregateExpr, error) {
	parenIdx := strings.Index(expr, "(")
	if parenIdx < 0 {
		// Bare function name (e.g. "count").
		funcName := strings.TrimSpace(expr)
		if !allowedAggregateFuncs[funcName] {
			return AggregateExpr{}, fmt.Errorf("unknown aggregate function: %q", funcName)
		}
		if funcName != "count" {
			return AggregateExpr{}, fmt.Errorf("aggregate function %q requires a column argument", funcName)
		}
		return AggregateExpr{Func: "count"}, nil
	}

	// Function with column argument: "func(col)"
	if !strings.HasSuffix(expr, ")") {
		return AggregateExpr{}, fmt.Errorf("malformed aggregate expression: missing closing parenthesis in %q", expr)
	}

	funcName := strings.TrimSpace(expr[:parenIdx])
	colName := strings.TrimSpace(expr[parenIdx+1 : len(expr)-1])

	if funcName == "" {
		return AggregateExpr{}, fmt.Errorf("empty function name in aggregate expression")
	}
	if !allowedAggregateFuncs[funcName] {
		return AggregateExpr{}, fmt.Errorf("unknown aggregate function: %q", funcName)
	}
	if colName == "" {
		return AggregateExpr{}, fmt.Errorf("empty column name in %s()", funcName)
	}

	col := tbl.ColumnByName(colName)
	if col == nil {
		return AggregateExpr{}, fmt.Errorf("unknown column %q in aggregate %s()", colName, funcName)
	}

	if requiresNumericColumn(funcName) && !isNumericType(col.TypeName) {
		return AggregateExpr{}, fmt.Errorf("aggregate function %s requires a numeric column, but %q has type %q", funcName, colName, col.TypeName)
	}
	if requiresSpatialColumn(funcName) && !col.IsGeometry && !col.IsGeography {
		return AggregateExpr{}, fmt.Errorf("aggregate function %s requires a spatial column", funcName)
	}

	return AggregateExpr{Func: funcName, Column: colName}, nil
}

// parseGroupColumns parses and validates a comma-separated list of group-by column names.
// Returns the validated column names (unquoted). Quoting is done by the SQL builder.
func parseGroupColumns(tbl *schema.Table, input string) ([]string, error) {
	return parseColumnList(tbl, input, "group", false)
}

func parseFacetColumns(tbl *schema.Table, input string) ([]string, error) {
	return parseFacetColumnsParam(tbl, input, "facets")
}

func parseDisjunctiveFacetColumns(tbl *schema.Table, input string) ([]string, error) {
	return parseFacetColumnsParam(tbl, input, "disjunctiveFacets")
}

func parseFacetColumnsParam(tbl *schema.Table, input, paramName string) ([]string, error) {
	cols, err := parseColumnList(tbl, input, paramName, true)
	if err != nil {
		return nil, err
	}
	for _, name := range cols {
		col := tbl.ColumnByName(name)
		if col == nil {
			return nil, fmt.Errorf("unknown column %q in %s parameter", name, paramName)
		}
		if !isFacetColumnTypeSupported(col) {
			return nil, fmt.Errorf("unsupported facet column %q with type %q", name, col.TypeName)
		}
	}
	return cols, nil
}

func parseColumnList(tbl *schema.Table, input, paramName string, dedupe bool) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	parts := strings.Split(input, ",")
	cols := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if tbl.ColumnByName(p) == nil {
			return nil, fmt.Errorf("unknown column %q in %s parameter", p, paramName)
		}
		if dedupe {
			if _, exists := seen[p]; exists {
				continue
			}
			seen[p] = struct{}{}
		}
		cols = append(cols, p)
	}

	return cols, nil
}

func isFacetColumnTypeSupported(col *schema.Column) bool {
	return !col.IsGeometry && !col.IsGeography && !col.IsVector && !col.IsArray && !col.IsJSON && !col.IsRaster
}

type facetCountQuery struct {
	sql  string
	args []any
}

type facetStatsQuery struct {
	sql  string
	args []any
}

func buildFacetCountQueries(tbl *schema.Table, opts listOpts) map[string]facetCountQuery {
	if len(opts.facetCols) == 0 {
		return nil
	}
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)
	queries := make(map[string]facetCountQuery, len(opts.facetCols))
	for _, facetCol := range opts.facetCols {
		predicate, args := buildFacetCountPredicate(tbl, opts, facetCol)
		quotedCol := sqlutil.QuoteIdent(facetCol)
		whereClause := ""
		if predicate != "" {
			whereClause = " WHERE " + predicate
		}
		queries[facetCol] = facetCountQuery{
			sql: fmt.Sprintf(
				"SELECT %s AS value, COUNT(*)::bigint AS count FROM %s%s GROUP BY %s ORDER BY COUNT(*) DESC, %s ASC",
				quotedCol, ref, whereClause, quotedCol, quotedCol,
			),
			args: args,
		}
	}
	return queries
}

func buildFacetStatsQueries(tbl *schema.Table, opts listOpts) map[string]facetStatsQuery {
	if len(opts.facetCols) == 0 {
		return nil
	}
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)
	queries := make(map[string]facetStatsQuery, len(opts.facetCols))
	for _, facetCol := range opts.facetCols {
		col := tbl.ColumnByName(facetCol)
		if col == nil || !isNumericType(col.TypeName) {
			continue
		}
		predicate, args := buildFacetCountPredicate(tbl, opts, facetCol)
		whereClause := ""
		if predicate != "" {
			whereClause = " WHERE " + predicate
		}
		minExpr, maxExpr := facetStatsTextExpressions(col)
		queries[facetCol] = facetStatsQuery{
			sql: fmt.Sprintf(
				"SELECT %s AS min, %s AS max FROM %s%s",
				minExpr, maxExpr, ref, whereClause,
			),
			args: args,
		}
	}
	return queries
}

func facetStatsTextExpressions(col *schema.Column) (string, string) {
	quotedCol := sqlutil.QuoteIdent(col.Name)
	if normalizedColumnTypeName(col.TypeName) == "money" {
		return fmt.Sprintf("(MIN(%s)::numeric)::text", quotedCol), fmt.Sprintf("(MAX(%s)::numeric)::text", quotedCol)
	}
	return fmt.Sprintf("(MIN(%s))::text", quotedCol), fmt.Sprintf("(MAX(%s))::text", quotedCol)
}

func buildFacetCountPredicate(tbl *schema.Table, opts listOpts, facetCol string) (string, []any) {
	if !isDisjunctiveFacetColumn(opts.disjunctiveFacetCols, facetCol) || strings.TrimSpace(opts.rawFilter) == "" {
		return combineSQLConditions(
			sqlCondition{clause: opts.filterSQL, args: opts.filterArgs},
			sqlCondition{clause: opts.spatialSQL, args: opts.spatialArgs},
			sqlCondition{clause: opts.searchSQL, args: opts.searchArgs},
		)
	}

	filterSQL, filterArgs, err := buildFilterExcludingFacetColumn(tbl, opts.rawFilter, facetCol)
	if err != nil {
		return combineSQLConditions(
			sqlCondition{clause: opts.filterSQL, args: opts.filterArgs},
			sqlCondition{clause: opts.spatialSQL, args: opts.spatialArgs},
			sqlCondition{clause: opts.searchSQL, args: opts.searchArgs},
		)
	}
	nextArg := len(filterArgs) + 1
	spatialSQL := rebaseSQLParamRefs(opts.spatialSQL, nextArg-len(opts.filterArgs)-1)
	nextArg += len(opts.spatialArgs)
	searchSQL := rebaseSQLParamRefs(opts.searchSQL, nextArg-len(opts.filterArgs)-len(opts.spatialArgs)-1)

	return combineSQLConditions(
		sqlCondition{clause: filterSQL, args: filterArgs},
		sqlCondition{clause: spatialSQL, args: opts.spatialArgs},
		sqlCondition{clause: searchSQL, args: opts.searchArgs},
	)
}

func isDisjunctiveFacetColumn(disjunctiveFacetCols []string, facetCol string) bool {
	for _, col := range disjunctiveFacetCols {
		if col == facetCol {
			return true
		}
	}
	return false
}

func rebaseSQLParamRefs(sql string, delta int) string {
	if sql == "" || delta == 0 {
		return sql
	}
	return sqlParamRefPattern.ReplaceAllStringFunc(sql, func(match string) string {
		n, err := strconv.Atoi(match[1:])
		if err != nil {
			return match
		}
		return fmt.Sprintf("$%d", n+delta)
	})
}

// aggregateSelectExpr returns the SQL select expression and alias for an AggregateExpr.
func aggregateSelectExpr(tbl *schema.Table, expr AggregateExpr) (string, error) {
	col := tbl.ColumnByName(expr.Column)

	switch expr.Func {
	case "count":
		if expr.Column == "" {
			return `COUNT(*) AS "count"`, nil
		}
		return fmt.Sprintf(`COUNT(%s) AS "count_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "count_distinct":
		return fmt.Sprintf(`COUNT(DISTINCT %s) AS "count_distinct_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "sum":
		return fmt.Sprintf(`SUM(%s) AS "sum_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "avg":
		return fmt.Sprintf(`AVG(%s) AS "avg_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "min":
		return fmt.Sprintf(`MIN(%s) AS "min_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "max":
		return fmt.Sprintf(`MAX(%s) AS "max_%s"`, sqlutil.QuoteIdent(expr.Column), expr.Column), nil
	case "bbox":
		if col == nil {
			return "", fmt.Errorf("unknown column %q in aggregate bbox()", expr.Column)
		}
		return fmt.Sprintf(`%s AS "bbox_%s"`, spatial.BBoxAggregateExpr(col), expr.Column), nil
	case "centroid":
		if col == nil {
			return "", fmt.Errorf("unknown column %q in aggregate centroid()", expr.Column)
		}
		return fmt.Sprintf(`%s AS "centroid_%s"`, spatial.CentroidAggregateExpr(col), expr.Column), nil
	default:
		return "", fmt.Errorf("unknown aggregate function: %q", expr.Func)
	}
}

// buildAggregate generates a SELECT query with aggregate functions.
// Group columns are added to the SELECT and GROUP BY clauses.
// Filter and search from opts compose as WHERE conditions.
func buildAggregate(tbl *schema.Table, exprs []AggregateExpr, opts listOpts, groupCols []string) (string, []any, error) {
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)

	// Build SELECT clause: group columns first, then aggregate expressions.
	selectParts := make([]string, 0, len(groupCols)+len(exprs))
	for _, col := range groupCols {
		selectParts = append(selectParts, sqlutil.QuoteIdent(col))
	}
	for _, expr := range exprs {
		selectExpr, err := aggregateSelectExpr(tbl, expr)
		if err != nil {
			return "", nil, err
		}
		selectParts = append(selectParts, selectExpr)
	}

	combinedPredicate, allArgs := combineSQLConditions(
		sqlCondition{clause: opts.filterSQL, args: opts.filterArgs},
		sqlCondition{clause: opts.spatialSQL, args: opts.spatialArgs},
		sqlCondition{clause: opts.searchSQL, args: opts.searchArgs},
	)
	whereClause := ""
	if combinedPredicate != "" {
		whereClause = " WHERE " + combinedPredicate
	}

	// Build GROUP BY clause.
	groupByClause := ""
	if len(groupCols) > 0 {
		quotedCols := make([]string, len(groupCols))
		for i, col := range groupCols {
			quotedCols[i] = sqlutil.QuoteIdent(col)
		}
		groupByClause = " GROUP BY " + strings.Join(quotedCols, ", ")
	}

	q := fmt.Sprintf("SELECT %s FROM %s%s%s",
		strings.Join(selectParts, ", "),
		ref,
		whereClause,
		groupByClause,
	)

	return q, allArgs, nil
}
