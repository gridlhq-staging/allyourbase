// Package searchsettings Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun09_pm_4_search_relevance_weighting_and_custom_ranking/allyourbase_dev/internal/searchsettings/settings.go.
package searchsettings

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

const maxAttributes = 32

var searchableTextColumnTypes = map[string]struct{}{
	"text":              {},
	"varchar":           {},
	"character varying": {},
	"char":              {},
	"character":         {},
	"name":              {},
	"citext":            {},
}

const (
	WeightHigh   Weight = "high"
	WeightMedium Weight = "medium"
	WeightLow    Weight = "low"
	WeightLowest Weight = "lowest"
)

type Weight string

type Attribute struct {
	Column string `json:"column"`
	Weight Weight `json:"weight"`
}

type Settings struct {
	Attributes []Attribute `json:"attributes"`
}

func Validate(settings Settings) (Settings, error) {
	if len(settings.Attributes) > maxAttributes {
		return Settings{}, fmt.Errorf("search settings may include at most %d attributes", maxAttributes)
	}

	seen := make(map[string]struct{}, len(settings.Attributes))
	normalized := make([]Attribute, 0, len(settings.Attributes))
	for _, attr := range settings.Attributes {
		column := strings.TrimSpace(attr.Column)
		if column == "" {
			return Settings{}, fmt.Errorf("search setting attribute column is required")
		}
		if _, ok := seen[column]; ok {
			return Settings{}, fmt.Errorf("duplicate search setting attribute column: %s", column)
		}
		weight := Weight(strings.TrimSpace(string(attr.Weight)))
		if _, err := PostgresWeightLabel(weight); err != nil {
			return Settings{}, err
		}
		seen[column] = struct{}{}
		normalized = append(normalized, Attribute{Column: column, Weight: weight})
	}
	return Settings{Attributes: normalized}, nil
}

func ValidateForTable(tbl *schema.Table, settings Settings) (Settings, error) {
	normalized, err := Validate(settings)
	if err != nil {
		return Settings{}, err
	}
	if tbl == nil {
		return Settings{}, fmt.Errorf("search settings table is required")
	}

	allowed := make(map[string]struct{}, len(tbl.Columns))
	for _, column := range SearchableTextColumns(tbl) {
		allowed[column] = struct{}{}
	}
	for _, attr := range normalized.Attributes {
		if _, ok := allowed[attr.Column]; ok {
			continue
		}
		return Settings{}, fmt.Errorf("search setting attribute column %q is not a searchable text column on table %q", attr.Column, tbl.Name)
	}
	return normalized, nil
}

func IsSearchableTextColumn(col *schema.Column) bool {
	if col.IsJSON || col.IsArray || col.IsEnum {
		return false
	}
	base := strings.ToLower(col.TypeName)
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	_, ok := searchableTextColumnTypes[base]
	return ok
}

func SearchableTextColumns(tbl *schema.Table) []string {
	var cols []string
	for _, col := range tbl.Columns {
		if IsSearchableTextColumn(col) {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

func PostgresWeightLabel(weight Weight) (string, error) {
	switch weight {
	case WeightHigh:
		return "A", nil
	case WeightMedium:
		return "B", nil
	case WeightLow:
		return "C", nil
	case WeightLowest:
		return "D", nil
	default:
		return "", fmt.Errorf("unknown search setting attribute weight: %s", weight)
	}
}
