package algoliamigrate

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/searchsettings"
)

func TestMapAlgoliaSearchableAttributesAssignsExactWeights(t *testing.T) {
	tests := []struct {
		name        string
		groupCount  int
		wantWeights []searchsettings.Weight
	}{
		{name: "one", groupCount: 1, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
		}},
		{name: "two", groupCount: 2, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightLowest,
		}},
		{name: "three", groupCount: 3, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightMedium,
			searchsettings.WeightLowest,
		}},
		{name: "four", groupCount: 4, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightMedium,
			searchsettings.WeightLow,
			searchsettings.WeightLowest,
		}},
		{name: "five", groupCount: 5, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightHigh,
			searchsettings.WeightMedium,
			searchsettings.WeightLow,
			searchsettings.WeightLowest,
		}},
		{name: "six", groupCount: 6, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightHigh,
			searchsettings.WeightMedium,
			searchsettings.WeightMedium,
			searchsettings.WeightLow,
			searchsettings.WeightLowest,
		}},
		{name: "seven", groupCount: 7, wantWeights: []searchsettings.Weight{
			searchsettings.WeightHigh,
			searchsettings.WeightHigh,
			searchsettings.WeightMedium,
			searchsettings.WeightMedium,
			searchsettings.WeightLow,
			searchsettings.WeightLow,
			searchsettings.WeightLowest,
		}},
		{name: "thirty two", groupCount: 32, wantWeights: thirtyTwoExpectedWeights()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := AlgoliaSettings{SearchableAttributes: numberedAttributeNames(tt.groupCount)}
			plan := MapAlgoliaSearchableAttributes(settings, numberedTextSchema(tt.groupCount))

			assertAttributes(t, plan.Settings.Attributes, weightedNumberedAttributes(tt.wantWeights))
			assertSkippedReasons(t, plan.Stats.SkippedReasons, nil)
		})
	}
}

func TestMapAlgoliaSearchableAttributesPreservesCommaJoinedGroups(t *testing.T) {
	settings := AlgoliaSettings{SearchableAttributes: []string{
		"title,subtitle",
		"tags",
	}}

	plan := MapAlgoliaSearchableAttributes(settings, browseFixtureSchema())

	assertAttributes(t, plan.Settings.Attributes, []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "subtitle", Weight: searchsettings.WeightHigh},
		{Column: "tags", Weight: searchsettings.WeightLowest},
	})
	assertSkippedReasons(t, plan.Stats.SkippedReasons, nil)
}

func TestMapAlgoliaSearchableAttributesNormalizesWrappers(t *testing.T) {
	settings := AlgoliaSettings{SearchableAttributes: []string{
		"unordered(title)",
		"ordered(subtitle)",
		"unordered(tags",
		"tags)",
		"tags",
	}}

	plan := MapAlgoliaSearchableAttributes(settings, browseFixtureSchema())

	assertAttributes(t, plan.Settings.Attributes, []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "tags", Weight: searchsettings.WeightLowest},
	})
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"unsupported_wrapper": 1,
		"malformed_attribute": 2,
	})
}

func TestMapAlgoliaSearchableAttributesTracksAdvisorySkips(t *testing.T) {
	settings := AlgoliaSettings{SearchableAttributes: []string{
		" ",
		"title",
		"unordered(title)",
		"author.name",
		"missing_column",
		"inventory_count",
		"price",
		"published",
		"metadata",
		"subtitle",
		",tags",
	}}

	plan := MapAlgoliaSearchableAttributes(settings, browseFixtureSchema())

	assertAttributes(t, plan.Settings.Attributes, []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "subtitle", Weight: searchsettings.WeightMedium},
		{Column: "tags", Weight: searchsettings.WeightLowest},
	})
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"blank_attribute":     2,
		"duplicate_attribute": 1,
		"nested_attribute":    1,
		"missing_column":      1,
		"non_text_column":     4,
	})
}

func TestMapAlgoliaSearchableAttributesEnforcesCap(t *testing.T) {
	settings := AlgoliaSettings{SearchableAttributes: numberedAttributeNames(34)}
	plan := MapAlgoliaSearchableAttributes(settings, numberedTextSchema(34))

	if got, want := len(plan.Settings.Attributes), 32; got != want {
		t.Fatalf("len(attributes) = %d, want %d", got, want)
	}
	if got, want := plan.Settings.Attributes[31], (searchsettings.Attribute{Column: "field_32", Weight: searchsettings.WeightLowest}); got != want {
		t.Fatalf("attribute[31] = %#v, want %#v", got, want)
	}
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"over_attribute_cap": 2,
	})
}

func TestMapAlgoliaSettingsParsesCustomRankingInOrder(t *testing.T) {
	settings := AlgoliaSettings{
		SearchableAttributes: []string{"title"},
		CustomRanking:        []string{"desc(price)", "asc(inventory_count)", " desc( published ) "},
	}

	plan := MapAlgoliaSettings(settings, browseFixtureSchema())

	assertAttributes(t, plan.Settings.Attributes, []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
	})
	assertCustomRanking(t, plan.Settings.CustomRanking, []searchsettings.CustomRanking{
		{Column: "price", Order: searchsettings.RankingOrderDesc},
		{Column: "inventory_count", Order: searchsettings.RankingOrderAsc},
		{Column: "published", Order: searchsettings.RankingOrderDesc},
	})
	if got, want := plan.Stats.SupportedCustomRanking, 3; got != want {
		t.Fatalf("SupportedCustomRanking = %d, want %d", got, want)
	}
	assertSkippedReasons(t, plan.Stats.SkippedReasons, nil)
}

func TestMapAlgoliaSettingsTracksCustomRankingSkips(t *testing.T) {
	settings := AlgoliaSettings{
		CustomRanking: []string{
			"",
			"price",
			"sum(price)",
			"desc(price",
			"desc(price)",
			"asc(price)",
			"desc(metadata)",
			"asc(missing)",
		},
	}

	plan := MapAlgoliaSettings(settings, browseFixtureSchema())

	assertCustomRanking(t, plan.Settings.CustomRanking, []searchsettings.CustomRanking{
		{Column: "price", Order: searchsettings.RankingOrderDesc},
	})
	if got, want := plan.Stats.SupportedCustomRanking, 1; got != want {
		t.Fatalf("SupportedCustomRanking = %d, want %d", got, want)
	}
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"blank_custom_ranking":      1,
		"malformed_custom_ranking":  2,
		"unsupported_ranking_order": 1,
		"duplicate_custom_ranking":  1,
		"non_rankable_column":       1,
		"ranking_missing_column":    1,
	})
}

func TestMapAlgoliaSettingsEnforcesCustomRankingCap(t *testing.T) {
	settings := AlgoliaSettings{CustomRanking: numberedCustomRankings(34)}
	plan := MapAlgoliaSettings(settings, numberedIntegerSchema(34))

	if got, want := len(plan.Settings.CustomRanking), 32; got != want {
		t.Fatalf("len(customRanking) = %d, want %d", got, want)
	}
	if got, want := plan.Settings.CustomRanking[31], (searchsettings.CustomRanking{Column: "field_32", Order: searchsettings.RankingOrderDesc}); got != want {
		t.Fatalf("customRanking[31] = %#v, want %#v", got, want)
	}
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"over_custom_ranking_cap": 2,
	})
}

func TestMapAlgoliaSettingsAttributesForFacetingAreAdvisoryOnly(t *testing.T) {
	settings := AlgoliaSettings{
		SearchableAttributes:  []string{"title"},
		CustomRanking:         []string{"desc(price)"},
		AttributesForFaceting: []string{"tags", "filterOnly(published)", "searchable(metadata)"},
	}

	plan := MapAlgoliaSettings(settings, browseFixtureSchema())

	assertAttributes(t, plan.Settings.Attributes, []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
	})
	assertCustomRanking(t, plan.Settings.CustomRanking, []searchsettings.CustomRanking{
		{Column: "price", Order: searchsettings.RankingOrderDesc},
	})
	if got, want := plan.Stats.SkippedFacets, 3; got != want {
		t.Fatalf("SkippedFacets = %d, want %d", got, want)
	}
	assertSkippedReasons(t, plan.Stats.SkippedReasons, map[string]int{
		"facet_advisory_only": 3,
	})
}

func browseFixtureSchema() Schema {
	return Schema{Columns: []Column{
		{Name: "title", Type: ColumnTypeText},
		{Name: "subtitle", Type: ColumnTypeText},
		{Name: "inventory_count", Type: ColumnTypeInteger},
		{Name: "price", Type: ColumnTypeDouble},
		{Name: "published", Type: ColumnTypeBoolean},
		{Name: "tags", Type: ColumnTypeText},
		{Name: "metadata", Type: ColumnTypeJSONB},
	}}
}

func numberedTextSchema(count int) Schema {
	columns := make([]Column, 0, count)
	for i := 1; i <= count; i++ {
		columns = append(columns, Column{
			Name: fmt.Sprintf("field_%02d", i),
			Type: ColumnTypeText,
		})
	}
	return Schema{Columns: columns}
}

func numberedIntegerSchema(count int) Schema {
	columns := make([]Column, 0, count)
	for i := 1; i <= count; i++ {
		columns = append(columns, Column{
			Name: fmt.Sprintf("field_%02d", i),
			Type: ColumnTypeInteger,
		})
	}
	return Schema{Columns: columns}
}

func numberedAttributeNames(count int) []string {
	attributes := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		attributes = append(attributes, fmt.Sprintf("field_%02d", i))
	}
	return attributes
}

func numberedCustomRankings(count int) []string {
	rankings := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		rankings = append(rankings, fmt.Sprintf("desc(field_%02d)", i))
	}
	return rankings
}

func weightedNumberedAttributes(weights []searchsettings.Weight) []searchsettings.Attribute {
	attributes := make([]searchsettings.Attribute, 0, len(weights))
	for i, weight := range weights {
		attributes = append(attributes, searchsettings.Attribute{
			Column: fmt.Sprintf("field_%02d", i+1),
			Weight: weight,
		})
	}
	return attributes
}

func thirtyTwoExpectedWeights() []searchsettings.Weight {
	return []searchsettings.Weight{
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightHigh,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightMedium,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLow,
		searchsettings.WeightLowest,
	}
}

func assertAttributes(t *testing.T, got, want []searchsettings.Attribute) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attributes = %#v, want %#v", got, want)
	}
}

func assertSkippedReasons(t *testing.T, got, want map[string]int) {
	t.Helper()
	if len(got) == 0 {
		got = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skipped reasons = %#v, want %#v", got, want)
	}
}

func assertCustomRanking(t *testing.T, got, want []searchsettings.CustomRanking) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("customRanking = %#v, want %#v", got, want)
	}
}
