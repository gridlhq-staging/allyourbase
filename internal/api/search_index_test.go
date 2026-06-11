package api

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsettings"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildSearchIndexSpecUsesWeightedVectorWhenSettingsConfigured(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "body", Weight: searchsettings.WeightLow},
	}}

	spec, ok, err := buildSearchIndexSpec(tbl, "simple", settings)
	testutil.NoError(t, err)
	testutil.Equal(t, true, ok)

	regConfig, err := buildSearchRegConfigSQL("simple")
	testutil.NoError(t, err)
	weightedVector, err := buildSearchVectorExpression(textColumns(tbl), regConfig, settings)
	testutil.NoError(t, err)
	testutil.Contains(t, spec.createSQL, "("+weightedVector+")")
}

func TestBuildSearchIndexSpecLongUnicodeNameKeepsHashedNameWithinLimit(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema: strings.Repeat("a", 36),
		Name:   "測" + strings.Repeat("b", 32),
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "body", Position: 1, TypeName: "text"},
		},
	}

	spec, ok, err := buildSearchIndexSpec(tbl, "english", searchsettings.Settings{})
	testutil.NoError(t, err)
	testutil.Equal(t, true, ok)
	if len(spec.name) > 63 {
		t.Fatalf("search index name length = %d bytes, want <= 63", len(spec.name))
	}
	prefix := truncateSearchIndexNamePrefix(buildSearchIndexNamePrefix(tbl))
	if !strings.HasPrefix(spec.name, prefix) {
		t.Fatalf("search index name %q must keep truncated prefix %q", spec.name, prefix)
	}
	hash := strings.TrimPrefix(spec.name, prefix)
	if len(hash) != searchIndexHashLength {
		t.Fatalf("search index name hash length = %d, want %d", len(hash), searchIndexHashLength)
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("search index hash %q must be lowercase hex", hash)
		}
	}
}

func TestBuildSearchIndexSpecRejectsNonSearchableConfiguredColumn(t *testing.T) {
	t.Parallel()
	tbl := searchableTable()
	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "views", Weight: searchsettings.WeightHigh},
	}}

	_, _, err := buildSearchIndexSpec(tbl, "english", settings)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `search setting attribute column "views" is not a searchable text column on table "posts"`)
}
