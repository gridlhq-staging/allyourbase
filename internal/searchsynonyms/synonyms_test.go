package searchsynonyms

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeGroups(t *testing.T) {
	tests := []struct {
		name    string
		groups  Groups
		want    Groups
		wantErr string
	}{
		{
			name: "trims lowercases_ignores_blanks_and_sorts",
			groups: Groups{
				{Terms: []string{" Zebra ", "", "alpha", "  "}},
				{Terms: []string{"Beta", "ALPHA TWO"}},
			},
			want: Groups{
				{Terms: []string{"alpha", "zebra"}},
				{Terms: []string{"alpha two", "beta"}},
			},
		},
		{
			name:    "rejects_missing_groups",
			groups:  nil,
			wantErr: "groups is required",
		},
		{
			name:    "rejects_empty_groups",
			groups:  Groups{},
			wantErr: "groups is required",
		},
		{
			name:    "rejects_group_with_only_blank_terms",
			groups:  Groups{{Terms: []string{" ", ""}}},
			wantErr: "each synonym group must include at least two terms",
		},
		{
			name:    "rejects_group_with_one_normalized_term",
			groups:  Groups{{Terms: []string{"scifi", " "}}},
			wantErr: "each synonym group must include at least two terms",
		},
		{
			name:    "rejects_duplicate_terms_in_group",
			groups:  Groups{{Terms: []string{" SciFi ", "scifi"}}},
			wantErr: "duplicate synonym term: scifi",
		},
		{
			name: "rejects_duplicate_terms_across_groups",
			groups: Groups{
				{Terms: []string{"scifi", "science fiction"}},
				{Terms: []string{"ai", " SCIFI "}},
			},
			wantErr: "duplicate synonym term: scifi",
		},
		{
			name:    "rejects_terms_over_128_characters",
			groups:  Groups{{Terms: []string{"scifi", strings.Repeat("a", 129)}}},
			wantErr: "synonym terms must be 128 characters or fewer",
		},
		{
			name:    "allows_terms_at_128_characters",
			groups:  Groups{{Terms: []string{"scifi", strings.Repeat("a", 128)}}},
			want:    Groups{{Terms: []string{strings.Repeat("a", 128), "scifi"}}},
			wantErr: "",
		},
		{
			name: "rejects_more_than_8_terms",
			groups: Groups{{
				Terms: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
			}},
			wantErr: "synonym groups may include at most 8 terms",
		},
		{
			name: "allows_8_terms",
			groups: Groups{{
				Terms: []string{"h", "g", "f", "e", "d", "c", "b", "a"},
			}},
			want: Groups{{Terms: []string{"a", "b", "c", "d", "e", "f", "g", "h"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeGroups(tt.groups)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeGroups error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("NormalizeGroups error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeGroups unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("NormalizeGroups mismatch:\nwant: %#v\n got: %#v", tt.want, got)
			}
		})
	}
}
