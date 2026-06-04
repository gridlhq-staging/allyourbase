package api

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// textColumnTypes are PostgreSQL type names that should be included in full-text search.
var textColumnTypes = map[string]bool{
	"text":              true,
	"varchar":           true,
	"character varying": true,
	"char":              true,
	"character":         true,
	"name":              true,
	"citext":            true,
}

const defaultTypoThreshold = 0.2
const searchHighlightSQLAlias = "__ayb_search_highlight"
const searchHighlightResponseField = "_highlight"

type searchOptions struct {
	fuzzy         bool
	typoThreshold float64
	highlight     bool
}

type searchSQLResult struct {
	whereSQL        string
	rankSQL         string
	args            []any
	highlightSelect string
	highlightAlias  string
}

func defaultSearchOptions(fuzzy bool) searchOptions {
	return searchOptions{
		fuzzy:         fuzzy,
		typoThreshold: defaultTypoThreshold,
	}
}

// isTextColumn returns true if a column is a text type suitable for FTS.
func isTextColumn(col *schema.Column) bool {
	if col.IsJSON || col.IsArray || col.IsEnum {
		return false
	}
	// Normalize: strip modifiers like (255).
	base := strings.ToLower(col.TypeName)
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return textColumnTypes[base]
}

// textColumns returns the names of all text columns in a table.
func textColumns(tbl *schema.Table) []string {
	var cols []string
	for _, c := range tbl.Columns {
		if isTextColumn(c) {
			cols = append(cols, c.Name)
		}
	}
	return cols
}

func buildSearchDocumentExpression(cols []string) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = fmt.Sprintf("coalesce(%s, '')", sqlutil.QuoteIdent(col))
	}
	return strings.Join(parts, " || ' ' || ")
}

func buildSearchQueryExpression(paramRef string) string {
	return fmt.Sprintf("websearch_to_tsquery('simple', %s)", paramRef)
}

func buildSearchHTMLEscapedExpression(docExpr string) string {
	return fmt.Sprintf("replace(replace(replace(%s, '&', '&amp;'), '<', '&lt;'), '>', '&gt;')", docExpr)
}

func searchHighlightAliasForTable(tbl *schema.Table) string {
	if tbl.ColumnByName(searchHighlightSQLAlias) == nil {
		return searchHighlightSQLAlias
	}
	for suffix := 1; ; suffix++ {
		alias := fmt.Sprintf("%s_%d", searchHighlightSQLAlias, suffix)
		if tbl.ColumnByName(alias) == nil {
			return alias
		}
	}
}

func buildSearchHighlightSelect(docExpr, tsvector, tsquery, alias string) string {
	escapedDocExpr := buildSearchHTMLEscapedExpression(docExpr)
	return fmt.Sprintf("CASE WHEN %s @@ %s THEN ts_headline('simple', %s, %s, 'StartSel=<b>,StopSel=</b>') ELSE %s END AS %s",
		tsvector,
		tsquery,
		escapedDocExpr,
		tsquery,
		escapedDocExpr,
		sqlutil.QuoteIdent(alias),
	)
}

// buildSearchSQL generates a FTS WHERE clause and an ORDER BY expression for ranking.
// It uses websearch_to_tsquery (Postgres 11+) for user-friendly search syntax.
//
// argOffset is the starting parameter index (e.g., if filters already used $1-$3, pass 4).
//
// Returns:
//   - whereSQL: the WHERE condition, e.g. `to_tsvector('simple', ...) @@ websearch_to_tsquery('simple', $4)`
//   - rankSQL: the ORDER BY expression, e.g. `ts_rank(to_tsvector('simple', ...), websearch_to_tsquery('simple', $4))`
//   - args: the query parameter values (just the search term)
//   - error: if no searchable text columns exist
func buildSearchSQL(tbl *schema.Table, searchTerm string, argOffset int, opts searchOptions) (searchSQLResult, error) {
	cols := textColumns(tbl)
	if len(cols) == 0 {
		return searchSQLResult{}, fmt.Errorf("table %q has no text columns to search", tbl.Name)
	}

	args := []any{searchTerm}
	paramRef := fmt.Sprintf("$%d", argOffset)
	docExpr := buildSearchDocumentExpression(cols)
	tsvector := fmt.Sprintf("to_tsvector('simple', %s)", docExpr)
	tsquery := buildSearchQueryExpression(paramRef)

	result := searchSQLResult{
		whereSQL: fmt.Sprintf("%s @@ %s", tsvector, tsquery),
		rankSQL:  fmt.Sprintf("ts_rank(%s, %s)", tsvector, tsquery),
		args:     args,
	}
	if opts.highlight {
		if tbl.ColumnByName(searchHighlightResponseField) != nil {
			return searchSQLResult{}, fmt.Errorf("highlight cannot be used on table %q because it has a %q column", tbl.Name, searchHighlightResponseField)
		}
		highlightAlias := searchHighlightAliasForTable(tbl)
		result.highlightSelect = buildSearchHighlightSelect(docExpr, tsvector, tsquery, highlightAlias)
		result.highlightAlias = highlightAlias
	}
	if opts.fuzzy {
		typoThreshold := fmt.Sprintf("%g", opts.typoThreshold)
		trigramPredicates := make([]string, 0, len(cols))
		trigramRanks := make([]string, 0, len(cols))
		tokenPredicates := make([]string, 0, len(strings.Fields(searchTerm)))
		for _, col := range cols {
			columnExpr := fmt.Sprintf("coalesce(%s, '')", sqlutil.QuoteIdent(col))
			trigramPredicates = append(trigramPredicates, fmt.Sprintf("similarity(%s, %s) > %s", columnExpr, paramRef, typoThreshold))
			trigramRanks = append(trigramRanks, fmt.Sprintf("similarity(%s, %s)", columnExpr, paramRef))
		}
		for _, token := range strings.Fields(searchTerm) {
			tokenParam := fmt.Sprintf("$%d", argOffset+len(args))
			args = append(args, token)
			tokenMatch := make([]string, 0, len(cols))
			for _, col := range cols {
				columnExpr := fmt.Sprintf("coalesce(%s, '')", sqlutil.QuoteIdent(col))
				tokenMatch = append(tokenMatch, fmt.Sprintf("strict_word_similarity(lower(%s), lower(%s)) >= %s", tokenParam, columnExpr, typoThreshold))
			}
			tokenPredicates = append(tokenPredicates, fmt.Sprintf("(%s)", strings.Join(tokenMatch, " OR ")))
		}
		fuzzyMatchSQL := fmt.Sprintf("(%s)", strings.Join(trigramPredicates, " OR "))
		if len(tokenPredicates) > 0 {
			fuzzyMatchSQL = fmt.Sprintf("(%s AND %s)", fuzzyMatchSQL, strings.Join(tokenPredicates, " AND "))
		}
		result.whereSQL = fmt.Sprintf("(%s OR %s)", result.whereSQL, fuzzyMatchSQL)
		result.rankSQL = fmt.Sprintf("GREATEST(%s, %s)", result.rankSQL, strings.Join(trigramRanks, ", "))
		result.args = args
	}

	return result, nil
}
