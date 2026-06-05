package api

import "github.com/allyourbase/ayb/internal/schema"

// BuildSearchSQLForIntegrationTest exposes the unexported buildSearchSQL owner
// so integration tests in package api_test can drive EXPLAIN-based regressions
// off the exact predicate the runtime handler generates, instead of a helper
// that re-derives the search SQL locally. Keep test-only.
func BuildSearchSQLForIntegrationTest(tbl *schema.Table, searchTerm string, argOffset int) (whereSQL string, args []any, err error) {
	res, err := buildSearchSQL(tbl, searchTerm, argOffset, defaultSearchOptions(false))
	if err != nil {
		return "", nil, err
	}
	return res.whereSQL, res.args, nil
}
