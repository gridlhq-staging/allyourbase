package api

import (
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsettings"
)

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

// BuildSearchSQLPartsForIntegrationTest exposes the exact WHERE and ranking SQL
// owners so integration tests can assert numeric rank behavior and index-plan
// parity without reconstructing SQL outside the package.
func BuildSearchSQLPartsForIntegrationTest(tbl *schema.Table, searchTerm string, argOffset int, settings searchsettings.Settings) (whereSQL, rankSQL string, args []any, err error) {
	res, err := buildSearchSQL(tbl, searchTerm, argOffset, searchOptions{
		typoThreshold:    defaultTypoThreshold,
		textSearchConfig: config.Default().API.TextSearchConfig,
		settings:         settings,
	})
	if err != nil {
		return "", "", nil, err
	}
	return res.whereSQL, res.rankSQL, res.args, nil
}
