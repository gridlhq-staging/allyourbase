package api

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// filterSearchResult holds parsed filter and search SQL fragments.
type filterSearchResult struct {
	searchParamResult
	filterSQL  string
	filterArgs []any
}

type searchParamResult struct {
	searchSQL       string
	searchRank      string
	searchArgs      []any
	highlightSelect string
	highlightAlias  string
}

// filterSpatialResult holds parsed filter and spatial SQL fragments.
type filterSpatialResult struct {
	filterSQL   string
	filterArgs  []any
	spatialSQL  string
	spatialArgs []any
}

var unsupportedVectorSearchParams = []string{"fuzzy", "facets", "typo_threshold", "highlight"}

const typoThresholdParam = "typo_threshold"

func findUnsupportedSearchParam(q url.Values, unsupportedParams []string) string {
	for _, param := range unsupportedParams {
		if q.Has(param) {
			return param
		}
	}
	return ""
}

func rejectUnsupportedSearchParams(w http.ResponseWriter, q url.Values, unsupportedParams []string) bool {
	param := findUnsupportedSearchParam(q, unsupportedParams)
	if param == "" {
		return false
	}
	writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported parameter %q", param))
	return true
}

func parseFuzzyParam(q url.Values, searchStr string) (bool, error) {
	if !q.Has("fuzzy") {
		return false, nil
	}
	if searchStr == "" {
		return false, fmt.Errorf("fuzzy parameter requires non-empty search")
	}
	switch strings.ToLower(strings.TrimSpace(q.Get("fuzzy"))) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("fuzzy parameter must be a boolean")
	}
}

func parseTypoThresholdParam(q url.Values, fuzzy bool) (float64, error) {
	if !q.Has(typoThresholdParam) {
		return defaultTypoThreshold, nil
	}
	if !fuzzy {
		return 0, fmt.Errorf("%s requires fuzzy=true", typoThresholdParam)
	}

	threshold, err := strconv.ParseFloat(strings.TrimSpace(q.Get(typoThresholdParam)), 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return 0, fmt.Errorf("%s must be a number between 0 and 1", typoThresholdParam)
	}
	return threshold, nil
}

func parseHighlightParam(q url.Values) (bool, error) {
	if !q.Has("highlight") {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(q.Get("highlight"))) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("highlight parameter must be a boolean")
	}
}

func hasPgTrgm(sc *schema.SchemaCache) bool {
	return sc != nil && sc.HasPgTrgm
}

// parseFilterParam validates and parses the filter query parameter.
// On validation failure it writes the error response and returns false.
func (h *Handler) parseFilterParam(w http.ResponseWriter, tbl *schema.Table, q url.Values) (string, []any, bool) {
	filterStr := q.Get("filter")
	if filterStr == "" {
		return "", nil, true
	}
	if len(filterStr) > maxFilterLen {
		writeErrorWithDoc(w, http.StatusBadRequest, "filter expression too long", docURL("/guide/api-reference#filter-syntax"))
		return "", nil, false
	}
	if h.fieldEncryptor != nil {
		if err := h.fieldEncryptor.ValidateFilter(tbl.Name, filterStr); err != nil {
			writeErrorWithDoc(w, http.StatusBadRequest, "invalid filter: "+err.Error(), docURL("/guide/api-reference#filter-syntax"))
			return "", nil, false
		}
	}
	sql, args, err := parseFilter(tbl, filterStr)
	if err != nil {
		writeErrorWithDoc(w, http.StatusBadRequest, "invalid filter: "+err.Error(), docURL("/guide/api-reference#filter-syntax"))
		return "", nil, false
	}
	return sql, args, true
}

// parseFilterAndSpatial validates and parses filter and spatial query parameters.
// On validation failure it writes the error response and returns false.
func (h *Handler) parseFilterAndSpatial(w http.ResponseWriter, tbl *schema.Table, q url.Values) (filterSpatialResult, bool) {
	var res filterSpatialResult
	var (
		ok  bool
		err error
	)
	res.filterSQL, res.filterArgs, ok = h.parseFilterParam(w, tbl, q)
	if !ok {
		return res, false
	}

	sc := h.schema.Get()
	res.spatialSQL, res.spatialArgs, err = parseSpatialParams(tbl, q, sc, len(res.filterArgs)+1)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return res, false
	}

	return res, true
}

// parseSearchParam validates and parses the search query parameter into full-text
// search SQL fragments. argOffset controls the starting $N placeholder index.
// On validation failure it writes the error response and returns false.
func (h *Handler) parseSearchParam(w http.ResponseWriter, tbl *schema.Table, q url.Values, argOffset int) (searchParamResult, bool) {
	var res searchParamResult
	searchStr := strings.TrimSpace(q.Get("search"))
	fuzzy, err := parseFuzzyParam(q, searchStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return res, false
	}
	typoThreshold, err := parseTypoThresholdParam(q, fuzzy)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return res, false
	}
	highlight, err := parseHighlightParam(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return res, false
	}
	if fuzzy && !hasPgTrgm(h.schema.Get()) {
		writeError(w, http.StatusBadRequest, "fuzzy search is unavailable because pg_trgm is not installed")
		return res, false
	}
	if searchStr == "" {
		return res, true
	}
	if len(searchStr) > maxSearchLen {
		writeErrorWithDoc(w, http.StatusBadRequest, "search term too long", docURL("/guide/api-reference#full-text-search"))
		return res, false
	}
	search, err := buildSearchSQL(tbl, searchStr, argOffset, searchOptions{
		fuzzy:         fuzzy,
		typoThreshold: typoThreshold,
		highlight:     highlight,
	})
	if err != nil {
		writeErrorWithDoc(w, http.StatusBadRequest, "search not supported: "+err.Error(), docURL("/guide/api-reference#full-text-search"))
		return res, false
	}
	res.searchSQL = search.whereSQL
	res.searchRank = search.rankSQL
	res.searchArgs = search.args
	res.highlightSelect = search.highlightSelect
	res.highlightAlias = search.highlightAlias
	return res, true
}

// parseFilterAndSearch validates and parses filter and search query parameters.
// On validation failure it writes the error response and returns false.
func (h *Handler) parseFilterAndSearch(w http.ResponseWriter, tbl *schema.Table, q url.Values) (filterSearchResult, bool) {
	var res filterSearchResult
	var ok bool
	res.filterSQL, res.filterArgs, ok = h.parseFilterParam(w, tbl, q)
	if !ok {
		return res, false
	}
	search, ok := h.parseSearchParam(w, tbl, q, len(res.filterArgs)+1)
	if !ok {
		return res, false
	}
	res.searchParamResult = search
	return res, true
}
