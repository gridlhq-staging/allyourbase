package api

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/searchsettings"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildSearchSQLUsesWeightedVectorWhenSettingsConfigured(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "body", Weight: searchsettings.WeightLow},
	}}

	search, err := buildSearchSQL(tbl, "needle", 1, searchOptions{
		typoThreshold:    defaultTypoThreshold,
		textSearchConfig: "simple",
		settings:         settings,
	})
	testutil.NoError(t, err)

	weightedVector := `setweight(to_tsvector('simple'::regconfig, coalesce("title", '')), 'A') || setweight(to_tsvector('simple'::regconfig, coalesce("body", '')), 'C') || setweight(to_tsvector('simple'::regconfig, coalesce("status", '')), 'D')`
	testutil.Contains(t, search.whereSQL, weightedVector)
	testutil.Contains(t, search.rankSQL, "ts_rank_cd("+weightedVector)
}

func TestBuildSearchSQLWithoutSettingsKeepsEqualWeightRanking(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()

	search, err := buildSearchSQL(tbl, "needle", 1, searchOptions{
		typoThreshold:    defaultTypoThreshold,
		textSearchConfig: "simple",
	})
	testutil.NoError(t, err)

	docExpr := `coalesce("title", '') || ' ' || coalesce("body", '') || ' ' || coalesce("status", '')`
	testutil.Contains(t, search.whereSQL, "to_tsvector('simple'::regconfig, "+docExpr+")")
	testutil.Contains(t, search.rankSQL, "ts_rank(to_tsvector('simple'::regconfig, "+docExpr+")")
	if search.rankSQL == "" {
		t.Fatal("expected rank SQL")
	}
	if containsWeightedSearchSQL(search.whereSQL) || containsWeightedSearchSQL(search.rankSQL) {
		t.Fatalf("expected unweighted search SQL, got where=%s rank=%s", search.whereSQL, search.rankSQL)
	}
}

func TestBuildSearchSQLRejectsNonSearchableConfiguredColumn(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "views", Weight: searchsettings.WeightHigh},
	}}

	_, err := buildSearchSQL(tbl, "needle", 1, searchOptions{
		typoThreshold:    defaultTypoThreshold,
		textSearchConfig: "simple",
		settings:         settings,
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `search setting attribute column "views" is not a searchable text column on table "posts"`)
}

func containsWeightedSearchSQL(sql string) bool {
	return strings.Contains(sql, "setweight(") || strings.Contains(sql, "ts_rank_cd(")
}
