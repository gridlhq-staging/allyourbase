// Package api.
package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	defaultMaxFacetHits = 10
	maxFacetHits        = 100
	maxFacetPrefixLen   = 1000
)

// facetValueSearchHit is one entry in the facetHits response array.
type facetValueSearchHit struct {
	Value       string `json:"value"`
	Highlighted string `json:"highlighted"`
	Count       int64  `json:"count"`
}

// facetValueSearchResponse is the JSON body returned by handleFacetValueSearch.
type facetValueSearchResponse struct {
	FacetHits             []facetValueSearchHit `json:"facetHits"`
	ExhaustiveFacetsCount bool                  `json:"exhaustiveFacetsCount"`
}

// handleFacetValueSearch serves GET /collections/{table}/facets/{column}/search.
// It returns facet values matching an optional `q` prefix, scoped by the same
// filter/search predicates the list endpoint accepts.
func (h *Handler) handleFacetValueSearch(w http.ResponseWriter, r *http.Request) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return
	}

	column := chi.URLParam(r, "column")
	if _, err := parseFacetColumnsParam(tbl, column, "facets"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	col := tbl.ColumnByName(column)
	if col == nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown column %q in facets parameter", column))
		return
	}
	if !isTextColumn(col) {
		writeError(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("facet value search requires a text column, but %q has type %q", column, col.TypeName),
		)
		return
	}

	q := r.URL.Query()

	maxHits, err := parseMaxFacetHits(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fs, ok := h.parseFilterAndSpatial(w, tbl, q)
	if !ok {
		return
	}

	search, ok := h.parseSearchParam(w, tbl, q, len(fs.filterArgs)+len(fs.spatialArgs)+1)
	if !ok {
		return
	}

	prefix := q.Get("q")
	if len(prefix) > maxFacetPrefixLen {
		writeError(w, http.StatusBadRequest, "facet prefix too long")
		return
	}

	sqlText, args := buildFacetValueSearchQuery(tbl, column, prefix, maxHits, fs, search)

	querier, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := querier.Query(r.Context(), sqlText, args...)
	if err != nil {
		_ = done(err)
		if !mapPGError(w, err) {
			h.logger.Error("facet value search query error", "error", err, "table", tbl.Name, "column", column)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	hits, err := scanFacetValueSearchRows(rows, prefix, maxHits)
	rows.Close()
	if err != nil {
		_ = done(err)
		h.logger.Error("facet value search scan error", "error", err, "table", tbl.Name, "column", column)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	exhaustive := len(hits) <= maxHits
	if !exhaustive {
		hits = hits[:maxHits]
	}

	writeJSON(w, http.StatusOK, facetValueSearchResponse{
		FacetHits:             hits,
		ExhaustiveFacetsCount: exhaustive,
	})
}

// parseMaxFacetHits returns the requested maxFacetHits, defaulting to 10. Values
// <= 0 (including invalid integers) are rejected with the contract-test message.
func parseMaxFacetHits(q url.Values) (int, error) {
	raw := strings.TrimSpace(q.Get("maxFacetHits"))
	if raw == "" {
		return defaultMaxFacetHits, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("maxFacetHits must be greater than 0")
	}
	if n > maxFacetHits {
		return 0, fmt.Errorf("maxFacetHits must be less than or equal to %d", maxFacetHits)
	}
	return n, nil
}

// buildFacetValueSearchQuery assembles the GROUP BY / ORDER BY query for the
// facet-value-search endpoint, threading filter, spatial, and search predicates
// alongside the optional ILIKE prefix clause.
func buildFacetValueSearchQuery(
	tbl *schema.Table,
	column, prefix string,
	maxHits int,
	fs filterSpatialResult,
	search searchParamResult,
) (string, []any) {
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)
	quotedCol := sqlutil.QuoteIdent(column)

	conditions := []sqlCondition{
		{clause: fmt.Sprintf("%s IS NOT NULL", quotedCol)},
		{clause: fs.filterSQL, args: fs.filterArgs},
		{clause: fs.spatialSQL, args: fs.spatialArgs},
		{clause: search.searchSQL, args: search.searchArgs},
	}

	if prefix != "" {
		nextArg := len(fs.filterArgs) + len(fs.spatialArgs) + len(search.searchArgs) + 1
		conditions = append(conditions, sqlCondition{
			clause: fmt.Sprintf(`%s ILIKE $%d || '%%' ESCAPE '\'`, quotedCol, nextArg),
			args:   []any{escapeLikePrefix(prefix)},
		})
	}

	predicate, args := combineSQLConditions(conditions...)
	whereClause := ""
	if predicate != "" {
		whereClause = " WHERE " + predicate
	}

	limitPlaceholder := len(args) + 1
	args = append(args, maxHits+1)

	sqlText := fmt.Sprintf(
		"SELECT %s AS value, COUNT(*)::bigint AS count FROM %s%s GROUP BY %s ORDER BY count DESC, %s ASC LIMIT $%d",
		quotedCol, ref, whereClause, quotedCol, quotedCol, limitPlaceholder,
	)
	return sqlText, args
}

// scanFacetValueSearchRows reads up to maxHits+1 rows, producing highlighted
// hits. The caller uses the returned slice length to detect truncation.
func scanFacetValueSearchRows(rows pgx.Rows, prefix string, maxHits int) ([]facetValueSearchHit, error) {
	hits := make([]facetValueSearchHit, 0, maxHits+1)
	prefixRunes := utf8.RuneCountInString(prefix)
	for rows.Next() {
		var value string
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		hits = append(hits, facetValueSearchHit{
			Value:       value,
			Highlighted: highlightPrefix(value, prefixRunes),
			Count:       count,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hits, nil
}

// highlightPrefix wraps the leading prefixRunes runes of value in <mark>...</mark>.
// When prefixRunes is 0 the value is returned unchanged. When prefixRunes exceeds
// the value's rune count, the whole value is wrapped.
func highlightPrefix(value string, prefixRunes int) string {
	if prefixRunes <= 0 {
		return value
	}
	byteOffset := 0
	seen := 0
	for byteOffset < len(value) && seen < prefixRunes {
		_, size := utf8.DecodeRuneInString(value[byteOffset:])
		byteOffset += size
		seen++
	}
	return "<mark>" + value[:byteOffset] + "</mark>" + value[byteOffset:]
}

// escapeLikePrefix escapes the LIKE/ILIKE metacharacters %, _, and \ so the
// supplied string matches literally under ESCAPE '\'.
func escapeLikePrefix(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '%', '_':
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
