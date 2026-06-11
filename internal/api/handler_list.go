// Package api.
package api

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5"
)

// applyPagination parses page, perPage, and skipTotal from query parameters,
// clamping values to safe ranges. Returns an error if cursor and page are
// used together.
func applyPagination(q url.Values) (page, perPage int, skipTotal bool, err error) {
	pageStr := q.Get("page")
	page, _ = strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	perPage, _ = strconv.Atoi(q.Get("perPage"))
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 500 {
		perPage = 500
	}
	// Mutual exclusion: cursor + explicit page (>1) is invalid.
	cursorMode := q.Has("cursor")
	if cursorMode && pageStr != "" && page > 1 {
		return 0, 0, false, fmt.Errorf("cursor and page parameters are mutually exclusive")
	}
	skipTotal = q.Get("skipTotal") == "true"
	return page, perPage, skipTotal, nil
}

// handleList handles GET /collections/{table}
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	tbl := h.resolveTable(w, r)
	if tbl == nil {
		return
	}

	q := r.URL.Query()

	// Detect aggregate mode early — branches to a separate code path.
	aggregateParam := q.Get("aggregate")
	groupParam := q.Get("group")

	if groupParam != "" && aggregateParam == "" {
		writeError(w, http.StatusBadRequest, "group parameter requires aggregate parameter")
		return
	}

	// Parse filter and spatial filters.
	fs, ok := h.parseFilterAndSpatial(w, tbl, q)
	if !ok {
		return
	}

	if aggregateParam != "" {
		if !h.effectiveAPIConfig().AggregateEnabled {
			writeError(w, http.StatusForbidden, "aggregate queries are disabled")
			return
		}
		h.handleAggregate(w, r, tbl, aggregateParam, groupParam, fs)
		return
	}

	// Parse pagination.
	page, perPage, skipTotal, pgErr := applyPagination(q)
	if pgErr != nil {
		writeError(w, http.StatusBadRequest, pgErr.Error())
		return
	}

	fields := parseFields(r)

	// Vector nearest-neighbor paths short-circuit the normal list flow.
	vectorSQL, vectorArgs := combineSQLConditions(
		sqlCondition{clause: fs.filterSQL, args: fs.filterArgs},
		sqlCondition{clause: fs.spatialSQL, args: fs.spatialArgs},
	)
	if h.dispatchVectorPaths(w, r, tbl, page, perPage, sqlCondition{clause: vectorSQL, args: vectorArgs}) {
		return
	}

	baseOpts, ok := h.parseListBaseOpts(w, tbl, q, fields, perPage, fs)
	if !ok {
		return
	}

	sc := h.schema.Get()
	hasPostGIS := sc != nil && sc.HasPostGIS
	parsedSort, err := parseStructuredSort(tbl, q.Get("sort"), hasPostGIS)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Cursor pagination path.
	cursorParam, cursorMode := q.Get("cursor"), q.Has("cursor")
	if cursorMode {
		baseOpts.sort = ensureStructuredSortPKTiebreaker(tbl, parsedSort)
		h.handleCursorList(w, r, tbl, cursorParam, baseOpts)
		return
	}

	// Offset pagination path.
	baseOpts.page = page
	baseOpts.skipTotal = skipTotal
	if len(parsedSort.Terms) > 0 {
		baseOpts.sort = ensureStructuredSortPKTiebreaker(tbl, parsedSort)
	}
	h.handleOffsetList(w, r, tbl, baseOpts)
}

// parseListBaseOpts parses the non-vector list inputs (search, facets) and
// assembles the shared listOpts the offset and cursor paths consume. It returns
// (opts, false) and writes the HTTP error response when validation fails.
func (h *Handler) parseListBaseOpts(w http.ResponseWriter, tbl *schema.Table, q url.Values, fields []string, perPage int, fs filterSpatialResult) (listOpts, bool) {
	search, ok := h.parseSearchParam(w, tbl, q, len(fs.filterArgs)+len(fs.spatialArgs)+1)
	if !ok {
		return listOpts{}, false
	}
	facetCols, err := parseFacetColumns(tbl, q.Get("facets"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return listOpts{}, false
	}
	disjunctiveFacetCols, err := parseDisjunctiveFacetColumns(tbl, q.Get("disjunctiveFacets"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return listOpts{}, false
	}
	return listOpts{
		table:                 tbl,
		perPage:               perPage,
		fields:                fields,
		rawFilter:             fs.rawFilter,
		filterSQL:             fs.filterSQL,
		filterArgs:            fs.filterArgs,
		spatialSQL:            fs.spatialSQL,
		spatialArgs:           fs.spatialArgs,
		searchSQL:             search.searchSQL,
		searchRank:            search.searchRank,
		searchArgs:            search.searchArgs,
		highlightSelect:       search.highlightSelect,
		highlightAlias:        search.highlightAlias,
		highlightResultSelect: search.highlightResultSelect,
		highlightResultAlias:  search.highlightResultAlias,
		facetCols:             facetCols,
		disjunctiveFacetCols:  disjunctiveFacetCols,
	}, true
}

// dispatchVectorPaths checks for nearest/semantic/hybrid query parameters and
// dispatches to the appropriate vector handler. Returns true if a vector path
// was handled (including error responses), false if the caller should continue
// with the standard list flow.
func (h *Handler) dispatchVectorPaths(w http.ResponseWriter, r *http.Request, tbl *schema.Table, page, perPage int, filter sqlCondition) bool {
	q := r.URL.Query()
	nearestRaw := q.Get("nearest")
	semanticQuery := q.Get("semantic_query")
	semanticFlag := q.Get("semantic") == "true"
	searchStr := strings.TrimSpace(q.Get("search"))

	if nearestRaw == "" && semanticQuery == "" && !semanticFlag {
		return false
	}

	if nearestRaw != "" && semanticQuery != "" {
		writeError(w, http.StatusBadRequest, "cannot use both 'nearest' and 'semantic_query' parameters")
		return true
	}
	if semanticFlag && (nearestRaw != "" || semanticQuery != "") {
		writeError(w, http.StatusBadRequest, "cannot combine semantic=true with nearest or semantic_query")
		return true
	}
	if semanticFlag {
		if rejectUnsupportedSearchParams(w, q, unsupportedVectorSearchParams) {
			return true
		}
	}
	if semanticFlag && searchStr != "" {
		if len(searchStr) > maxSearchLen {
			writeErrorWithDoc(w, http.StatusBadRequest, "search term too long", docURL("/guide/api-reference#full-text-search"))
			return true
		}
		h.handleHybridSearch(w, r, tbl, searchStr, q.Get("vector_column"), q.Get("distance"), page, perPage, filter.clause, filter.args)
		return true
	}
	if nearestRaw != "" {
		if rejectUnsupportedSearchParams(w, q, unsupportedVectorSearchParams) {
			return true
		}
		h.handleNearest(w, r, tbl, nearestRaw, q.Get("vector_column"), q.Get("distance"), perPage, filter.clause, filter.args)
		return true
	}
	if semanticQuery != "" {
		if rejectUnsupportedSearchParams(w, q, unsupportedVectorSearchParams) {
			return true
		}
		h.handleSemanticQuery(w, r, tbl, semanticQuery, q.Get("vector_column"), q.Get("distance"), perPage, filter.clause, filter.args)
		return true
	}
	return false
}

// handleOffsetList executes the offset-based pagination list query path.
func (h *Handler) handleOffsetList(w http.ResponseWriter, r *http.Request, tbl *schema.Table, opts listOpts) {
	dataQuery, dataArgs, countQuery, countArgs := buildList(tbl, opts)

	querier, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	totalItems := -1
	totalPages := -1
	if !opts.skipTotal {
		err := querier.QueryRow(r.Context(), countQuery, countArgs...).Scan(&totalItems)
		if err != nil {
			done(err)
			h.logger.Error("count error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		totalPages = int(math.Ceil(float64(totalItems) / float64(opts.perPage)))
	}

	rows, err := querier.Query(r.Context(), dataQuery, dataArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("list error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	items, err := scanListItems(rows, opts.highlightAlias, opts.highlightResultAlias)
	if err != nil {
		done(err)
		h.logger.Error("scan error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.decryptListItems(tbl, items); err != nil {
		done(err)
		h.logger.Error("decrypt response record error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.expandListItems(r, querier, tbl, items)

	facets, facetStats, err := executeFacetQueries(r.Context(), querier, tbl, opts)
	if err != nil {
		done(err)
		h.logger.Error("facet count error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, ListResponse{
		Page:       opts.page,
		PerPage:    opts.perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Items:      items,
		Facets:     facets,
		FacetStats: facetStats,
	})
}

func resolveCursorListSort(opts listOpts) (listOpts, error) {
	resolvedSort, err := resolveStructuredSort(opts.sort, len(opts.filterArgs)+len(opts.spatialArgs)+len(opts.searchArgs)+1)
	if err != nil {
		return opts, err
	}

	sortFields := prependSearchRankCursorSort(opts.searchRank, resolvedSort.Fields)
	cursorProjection := prepareCursorSortProjection(opts.table, opts.fields, sortFields)
	opts.sortFields = cursorProjection.Fields
	opts.cursorSelects = cursorProjection.Selects
	opts.cursorHelperColumns = cursorProjection.HelperColumns
	opts.sortArgs = resolvedSort.Args
	opts.distanceSelect = resolvedSort.DistanceSelect
	return opts, nil
}

// decodeCursorListPredicate decodes the opaque cursor token into a keyset WHERE clause and args for the next page. Returns empty strings when cursorParam is empty (first page).
func decodeCursorListPredicate(opts listOpts, cursorParam string) (string, []any, error) {
	if cursorParam == "" {
		return "", nil, nil
	}

	values, err := decodeCursor(cursorParam)
	if err != nil {
		return "", nil, err
	}

	argOffset := len(opts.filterArgs) + len(opts.spatialArgs) + len(opts.searchArgs) + len(opts.sortArgs) + 1
	cursorWhere, cursorArgs, err := buildCursorWhere(opts.sortFields, values, argOffset)
	if err != nil {
		return "", nil, fmt.Errorf("invalid cursor: %w", err)
	}

	return cursorWhere, cursorArgs, nil
}

func scanListItems(rows pgx.Rows, highlightAlias, highlightResultAlias string) ([]map[string]any, error) {
	items, err := scanRows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	renameListHighlightAlias(items, highlightAlias, highlightResultAlias)
	return items, nil
}

func renameListHighlightAlias(items []map[string]any, highlightAlias, highlightResultAlias string) {
	if highlightAlias == "" && highlightResultAlias == "" {
		return
	}
	for _, item := range items {
		if highlightAlias != "" {
			if highlight, ok := item[highlightAlias]; ok {
				delete(item, highlightAlias)
				item[searchHighlightResponseField] = highlight
			}
		}
		if highlightResultAlias != "" {
			if highlightResult, ok := item[highlightResultAlias]; ok {
				delete(item, highlightResultAlias)
				item[searchHighlightResultResponseField] = highlightResult
			}
		}
	}
}

func (h *Handler) decryptListItems(tbl *schema.Table, items []map[string]any) error {
	if h.fieldEncryptor != nil {
		for _, item := range items {
			if err := h.fieldEncryptor.DecryptRecord(tbl.Name, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) expandListItems(r *http.Request, querier Querier, tbl *schema.Table, items []map[string]any) {
	expandParam := r.URL.Query().Get("expand")
	if expandParam == "" || len(items) == 0 {
		return
	}

	sc := h.schema.Get()
	if sc != nil {
		expandRecords(r.Context(), querier, sc, tbl, items, expandParam, h.logger)
	}
}

// handleCursorList handles cursor-based pagination for list requests.
func (h *Handler) handleCursorList(
	w http.ResponseWriter, r *http.Request, tbl *schema.Table, cursorParam string, opts listOpts,
) {
	var err error
	opts, err = resolveCursorListSort(opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Decode cursor if provided (empty string = first page).
	cursorWhere, cursorArgs, err := decodeCursorListPredicate(opts, cursorParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	dataQuery, dataArgs := buildListWithCursor(tbl, opts, opts.sortFields, cursorWhere, cursorArgs)

	querier, done, err := h.withRLS(r)
	if err != nil {
		h.logger.Error("rls setup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := querier.Query(r.Context(), dataQuery, dataArgs...)
	if err != nil {
		done(err)
		if !mapPGError(w, err) {
			h.logger.Error("cursor list error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	items, err := scanListItems(rows, opts.highlightAlias, opts.highlightResultAlias)
	if err != nil {
		done(err)
		h.logger.Error("scan error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.decryptListItems(tbl, items); err != nil {
		done(err)
		h.logger.Error("decrypt response record error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.expandListItems(r, querier, tbl, items)

	facets, facetStats, err := executeFacetQueries(r.Context(), querier, tbl, opts)
	if err != nil {
		done(err)
		h.logger.Error("facet count error", "error", err, "table", tbl.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := done(nil); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Detect next page: if we got more than perPage rows, there are more.
	var nextCursor string
	if len(items) > opts.perPage {
		items = items[:opts.perPage]
		lastItem := items[len(items)-1]
		nextCursor = encodeCursor(extractCursorValues(opts.sortFields, lastItem))
	}
	stripCursorHelperFields(items, opts.cursorHelperColumns)

	writeJSON(w, http.StatusOK, CursorListResponse{
		PerPage:    opts.perPage,
		NextCursor: nextCursor,
		Items:      items,
		Facets:     facets,
		FacetStats: facetStats,
	})
}
