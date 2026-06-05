package api

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
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
const searchHighlightResultSQLAlias = "__ayb_search_highlight_result"
const searchHighlightResponseField = "_highlight"
const searchHighlightResultResponseField = "_highlightResult"

type searchOptions struct {
	fuzzy            bool
	typoThreshold    float64
	highlight        bool
	textSearchConfig string
}

type searchSQLResult struct {
	whereSQL              string
	rankSQL               string
	args                  []any
	highlightSelect       string
	highlightAlias        string
	highlightResultSelect string
	highlightResultAlias  string
}

type searchRegConfigSQL struct {
	literal        string
	rewriteLiteral string
}

func defaultSearchOptions(fuzzy bool) searchOptions {
	return searchOptions{
		fuzzy:            fuzzy,
		typoThreshold:    defaultTypoThreshold,
		textSearchConfig: config.Default().API.TextSearchConfig,
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

func effectiveSearchOptions(opts searchOptions) searchOptions {
	if opts.typoThreshold == 0 {
		opts.typoThreshold = defaultTypoThreshold
	}
	if opts.textSearchConfig == "" {
		opts.textSearchConfig = config.Default().API.TextSearchConfig
	}
	return opts
}

func buildSearchRegConfigSQL(textSearchConfig string) (searchRegConfigSQL, error) {
	if !config.IsValidTextSearchConfigName(textSearchConfig) {
		return searchRegConfigSQL{}, fmt.Errorf("invalid text search config %q", textSearchConfig)
	}
	quoted := strings.ReplaceAll(textSearchConfig, "'", "''")
	return searchRegConfigSQL{
		literal:        "'" + quoted + "'::regconfig",
		rewriteLiteral: "''" + quoted + "''::regconfig",
	}, nil
}

// buildSearchQueryExpression builds the FTS tsquery expression for the user's
// search term, expanded by any per-collection synonym groups configured in
// _ayb_search_synonyms for (schemaRef, tableRef).
//
// Mechanism: ts_rewrite substitutes each matched synonym term in-place,
// preserving any AND structure from the surrounding user query. For each
// synonym group, the substitute is the OR of phraseto_tsquery() over all
// group members, so multi-word terms become phrase predicates and special
// tsquery characters in stored terms are escaped safely. Each group emits
// both a phraseto_tsquery target (matches quoted-phrase user input) and a
// websearch_to_tsquery target (matches plain user input) so expansion fires
// regardless of how the user typed the trigger term. The rewrite SELECT text
// is built dynamically with quote_literal() on schema/table names rather than
// outer-query parameters, because ts_rewrite's text variant executes the
// SELECT in a separate planning context that does not see outer $N args.
func buildSearchQueryExpression(paramRef, schemaRef, tableRef string, regConfig searchRegConfigSQL) string {
	rewriteSQL := "" +
		"'WITH grp AS (' " +
		"|| 'SELECT schema_name, table_name, group_id, ' " +
		"|| '       (string_agg(''('' || phraseto_tsquery(" + regConfig.rewriteLiteral + ", term)::text || '')'', '' | ''))::tsquery AS expanded ' " +
		"|| 'FROM _ayb_search_synonyms ' " +
		"|| 'WHERE schema_name = ' || quote_literal(" + schemaRef + ") || ' AND table_name = ' || quote_literal(" + tableRef + ") || ' ' " +
		"|| 'GROUP BY schema_name, table_name, group_id) ' " +
		"|| 'SELECT phraseto_tsquery(" + regConfig.rewriteLiteral + ", s1.term), grp.expanded ' " +
		"|| 'FROM _ayb_search_synonyms s1 JOIN grp USING (schema_name, table_name, group_id) ' " +
		"|| 'WHERE s1.schema_name = ' || quote_literal(" + schemaRef + ") || ' AND s1.table_name = ' || quote_literal(" + tableRef + ") || ' ' " +
		"|| 'UNION ' " +
		"|| 'SELECT websearch_to_tsquery(" + regConfig.rewriteLiteral + ", s1.term), grp.expanded ' " +
		"|| 'FROM _ayb_search_synonyms s1 JOIN grp USING (schema_name, table_name, group_id) ' " +
		"|| 'WHERE s1.schema_name = ' || quote_literal(" + schemaRef + ") || ' AND s1.table_name = ' || quote_literal(" + tableRef + ")"
	return fmt.Sprintf("(SELECT ts_rewrite(websearch_to_tsquery(%s, %s), %s))", regConfig.literal, paramRef, rewriteSQL)
}

func buildSearchHTMLEscapedExpression(docExpr string) string {
	return fmt.Sprintf("replace(replace(replace(%s, '&', '&amp;'), '<', '&lt;'), '>', '&gt;')", docExpr)
}

func searchAliasForTable(tbl *schema.Table, baseAlias string) string {
	if tbl.ColumnByName(baseAlias) == nil {
		return baseAlias
	}
	for suffix := 1; ; suffix++ {
		alias := fmt.Sprintf("%s_%d", baseAlias, suffix)
		if tbl.ColumnByName(alias) == nil {
			return alias
		}
	}
}

func searchHighlightAliasForTable(tbl *schema.Table) string {
	return searchAliasForTable(tbl, searchHighlightSQLAlias)
}

func searchHighlightResultAliasForTable(tbl *schema.Table) string {
	return searchAliasForTable(tbl, searchHighlightResultSQLAlias)
}

func buildSearchHeadlineExpression(docExpr, tsvector, tsquery string, regConfig searchRegConfigSQL) string {
	escapedDocExpr := buildSearchHTMLEscapedExpression(docExpr)
	return fmt.Sprintf("CASE WHEN %s @@ %s THEN ts_headline(%s, %s, %s, 'StartSel=<b>,StopSel=</b>') ELSE %s END",
		tsvector,
		tsquery,
		regConfig.literal,
		escapedDocExpr,
		tsquery,
		escapedDocExpr,
	)
}

func buildSearchHighlightSelect(docExpr, tsvector, tsquery, alias string, regConfig searchRegConfigSQL) string {
	return fmt.Sprintf("%s AS %s", buildSearchHeadlineExpression(docExpr, tsvector, tsquery, regConfig), sqlutil.QuoteIdent(alias))
}

func quoteSearchHighlightResultKey(columnName string) string {
	return "'" + strings.ReplaceAll(columnName, "'", "''") + "'"
}

func buildSearchHighlightResultSelect(cols []string, tsquery, alias string, regConfig searchRegConfigSQL) string {
	entries := make([]string, 0, len(cols))
	for _, col := range cols {
		columnExpr := fmt.Sprintf("coalesce(%s, '')", sqlutil.QuoteIdent(col))
		columnVector := fmt.Sprintf("to_tsvector(%s, %s)", regConfig.literal, columnExpr)
		headlineExpr := buildSearchHeadlineExpression(columnExpr, columnVector, tsquery, regConfig)
		matchExpr := fmt.Sprintf("CASE WHEN %s @@ %s THEN 'full' ELSE 'none' END", columnVector, tsquery)
		entries = append(entries, fmt.Sprintf("%s, jsonb_build_object('value', %s, 'matchLevel', %s)", quoteSearchHighlightResultKey(col), headlineExpr, matchExpr))
	}
	return fmt.Sprintf("jsonb_build_object(%s) AS %s", strings.Join(entries, ", "), sqlutil.QuoteIdent(alias))
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
	opts = effectiveSearchOptions(opts)
	regConfig, err := buildSearchRegConfigSQL(opts.textSearchConfig)
	if err != nil {
		return searchSQLResult{}, err
	}
	cols := textColumns(tbl)
	if len(cols) == 0 {
		return searchSQLResult{}, fmt.Errorf("table %q has no text columns to search", tbl.Name)
	}

	// Three positional args: the search term, the collection schema, and the
	// collection table. The schema/table refs are consumed by the synonym
	// subquery in buildSearchQueryExpression to scope expansion to this
	// collection only.
	args := []any{searchTerm, tbl.Schema, tbl.Name}
	paramRef := fmt.Sprintf("$%d", argOffset)
	schemaRef := fmt.Sprintf("$%d", argOffset+1)
	tableRef := fmt.Sprintf("$%d", argOffset+2)
	docExpr := buildSearchDocumentExpression(cols)
	tsvector := fmt.Sprintf("to_tsvector(%s, %s)", regConfig.literal, docExpr)
	tsquery := buildSearchQueryExpression(paramRef, schemaRef, tableRef, regConfig)

	result := searchSQLResult{
		whereSQL: fmt.Sprintf("%s @@ %s", tsvector, tsquery),
		rankSQL:  fmt.Sprintf("ts_rank(%s, %s)", tsvector, tsquery),
		args:     args,
	}
	if opts.highlight {
		if tbl.ColumnByName(searchHighlightResponseField) != nil {
			return searchSQLResult{}, fmt.Errorf("highlight cannot be used on table %q because it has a %q column", tbl.Name, searchHighlightResponseField)
		}
		if tbl.ColumnByName(searchHighlightResultResponseField) != nil {
			return searchSQLResult{}, fmt.Errorf("highlight cannot be used on table %q because it has a %q column", tbl.Name, searchHighlightResultResponseField)
		}
		highlightAlias := searchHighlightAliasForTable(tbl)
		highlightResultAlias := searchHighlightResultAliasForTable(tbl)
		result.highlightSelect = buildSearchHighlightSelect(docExpr, tsvector, tsquery, highlightAlias, regConfig)
		result.highlightAlias = highlightAlias
		result.highlightResultSelect = buildSearchHighlightResultSelect(cols, tsquery, highlightResultAlias, regConfig)
		result.highlightResultAlias = highlightResultAlias
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
