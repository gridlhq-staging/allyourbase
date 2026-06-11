package searchsettings

import (
	"reflect"
	"strings"
	"testing"

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
