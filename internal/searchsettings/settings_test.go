package searchsettings

import (
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidatePreservesOrderedAttributes(t *testing.T) {
	settings := Settings{Attributes: []Attribute{
		{Column: "title", Weight: WeightHigh},
		{Column: "body", Weight: WeightLow},
	}}

	got, err := Validate(settings)
	testutil.NoError(t, err)
	want := Settings{Attributes: []Attribute{
		{Column: "title", Weight: "high"},
		{Column: "body", Weight: "low"},
	}}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestValidateRejectsInvalidAttributes(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		want     string
	}{
		{
			name:     "empty_column",
			settings: Settings{Attributes: []Attribute{{Column: "", Weight: WeightHigh}}},
			want:     "search setting attribute column is required",
		},
		{
			name:     "blank_column",
			settings: Settings{Attributes: []Attribute{{Column: "  ", Weight: WeightHigh}}},
			want:     "search setting attribute column is required",
		},
		{
			name: "duplicate_column",
			settings: Settings{Attributes: []Attribute{
				{Column: "title", Weight: WeightHigh},
				{Column: "title", Weight: WeightLow},
			}},
			want: "duplicate search setting attribute column: title",
		},
		{
			name:     "unknown_weight",
			settings: Settings{Attributes: []Attribute{{Column: "title", Weight: "heavy"}}},
			want:     "unknown search setting attribute weight: heavy",
		},
		{
			name:     "too_many_attributes",
			settings: Settings{Attributes: manyAttributes(maxAttributes + 1)},
			want:     "search settings may include at most 32 attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(tt.settings)
			testutil.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateNormalizesCustomRankingOrder(t *testing.T) {
	settings := Settings{CustomRanking: []CustomRanking{
		{Column: " created_at ", Order: " DESC "},
		{Column: "score", Order: " asc"},
		{Column: "price", Order: RankingOrderDesc},
	}}

	got, err := Validate(settings)
	testutil.NoError(t, err)
	want := Settings{Attributes: []Attribute{}, CustomRanking: []CustomRanking{
		{Column: "created_at", Order: RankingOrderDesc},
		{Column: "score", Order: RankingOrderAsc},
		{Column: "price", Order: RankingOrderDesc},
	}}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestValidateRejectsInvalidCustomRanking(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		want     string
	}{
		{
			name:     "empty_column",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "", Order: RankingOrderAsc}}},
			want:     "search setting custom ranking column is required",
		},
		{
			name:     "blank_column",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "  ", Order: RankingOrderAsc}}},
			want:     "search setting custom ranking column is required",
		},
		{
			name: "duplicate_column_after_trim",
			settings: Settings{CustomRanking: []CustomRanking{
				{Column: "score", Order: RankingOrderAsc},
				{Column: " score ", Order: RankingOrderDesc},
			}},
			want: "duplicate search setting custom ranking column: score",
		},
		{
			name:     "unknown_order",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "score", Order: "newest"}}},
			want:     "unknown search setting custom ranking order: newest",
		},
		{
			name:     "too_many_rankings",
			settings: Settings{CustomRanking: manyCustomRankings(maxCustomRanking + 1)},
			want:     "search settings may include at most 32 custom ranking entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(tt.settings)
			testutil.ErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateForTableValidatesAttributesAndCustomRanking(t *testing.T) {
	tbl := &schema.Table{
		Name: "products",
		Columns: []*schema.Column{
			{Name: "title", TypeName: "text"},
			{Name: "price", TypeName: "numeric"},
			{Name: "created_at", TypeName: "timestamp with time zone"},
			{Name: "metadata", TypeName: "jsonb", IsJSON: true},
			{Name: "tags", TypeName: "text[]", IsArray: true},
			{Name: "status", TypeName: "product_status", IsEnum: true},
		},
	}

	tests := []struct {
		name     string
		settings Settings
		want     Settings
		wantErr  string
	}{
		{
			name: "attributes_still_require_searchable_text",
			settings: Settings{Attributes: []Attribute{
				{Column: "price", Weight: WeightHigh},
			}},
			wantErr: `search setting attribute column "price" is not a searchable text column on table "products"`,
		},
		{
			name: "custom_ranking_accepts_rankable_non_text_columns",
			settings: Settings{CustomRanking: []CustomRanking{
				{Column: "price", Order: RankingOrderDesc},
				{Column: "created_at", Order: RankingOrderAsc},
			}},
			want: Settings{Attributes: []Attribute{}, CustomRanking: []CustomRanking{
				{Column: "price", Order: RankingOrderDesc},
				{Column: "created_at", Order: RankingOrderAsc},
			}},
		},
		{
			name:     "custom_ranking_rejects_missing_columns",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "missing", Order: RankingOrderAsc}}},
			wantErr:  `search setting custom ranking column "missing" was not found on table "products"`,
		},
		{
			name:     "custom_ranking_rejects_json_columns",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "metadata", Order: RankingOrderAsc}}},
			wantErr:  `search setting custom ranking column "metadata" is not rankable on table "products"`,
		},
		{
			name:     "custom_ranking_rejects_array_columns",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "tags", Order: RankingOrderAsc}}},
			wantErr:  `search setting custom ranking column "tags" is not rankable on table "products"`,
		},
		{
			name:     "custom_ranking_rejects_enum_columns",
			settings: Settings{CustomRanking: []CustomRanking{{Column: "status", Order: RankingOrderAsc}}},
			wantErr:  `search setting custom ranking column "status" is not rankable on table "products"`,
		},
		{
			name:     "custom_ranking_rejects_too_many_entries",
			settings: Settings{CustomRanking: manyCustomRankings(maxCustomRanking + 1)},
			wantErr:  "search settings may include at most 32 custom ranking entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateForTable(tbl, tt.settings)
			if tt.wantErr != "" {
				testutil.ErrorContains(t, err, tt.wantErr)
				return
			}
			testutil.NoError(t, err)
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", tt.want, got)
			}
		})
	}
}

func TestPostgresWeightLabel(t *testing.T) {
	tests := []struct {
		weight Weight
		want   string
	}{
		{weight: WeightHigh, want: "A"},
		{weight: WeightMedium, want: "B"},
		{weight: WeightLow, want: "C"},
		{weight: WeightLowest, want: "D"},
	}

	for _, tt := range tests {
		t.Run(string(tt.weight), func(t *testing.T) {
			got, err := PostgresWeightLabel(tt.weight)
			testutil.NoError(t, err)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestPostgresWeightLabelRejectsUnknownWeight(t *testing.T) {
	_, err := PostgresWeightLabel("heavy")
	testutil.ErrorContains(t, err, "unknown search setting attribute weight: heavy")
}

func manyAttributes(count int) []Attribute {
	attrs := make([]Attribute, count)
	for i := range attrs {
		attrs[i] = Attribute{Column: "column_" + strings.Repeat("x", i+1), Weight: WeightMedium}
	}
	return attrs
}

func manyCustomRankings(count int) []CustomRanking {
	rankings := make([]CustomRanking, count)
	for i := range rankings {
		rankings[i] = CustomRanking{Column: "column_" + strings.Repeat("x", i+1), Order: RankingOrderAsc}
	}
	return rankings
}
