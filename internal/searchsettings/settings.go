// Package searchsettings stores and validates per-table search ranking settings.
package searchsettings

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

const (
	maxAttributes    = 32
	maxCustomRanking = 32
)

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

const (
	RankingOrderAsc  RankingOrder = "asc"
	RankingOrderDesc RankingOrder = "desc"
)

type Weight string

type RankingOrder string

type Attribute struct {
	Column string `json:"column"`
	Weight Weight `json:"weight"`
}

type CustomRanking struct {
	Column string       `json:"column"`
	Order  RankingOrder `json:"order"`
}

type Settings struct {
	Attributes    []Attribute     `json:"attributes"`
	CustomRanking []CustomRanking `json:"customRanking,omitempty"`
}

func Validate(settings Settings) (Settings, error) {
	if len(settings.Attributes) > maxAttributes {
		return Settings{}, fmt.Errorf("search settings may include at most %d attributes", maxAttributes)
	}
	if len(settings.CustomRanking) > maxCustomRanking {
		return Settings{}, fmt.Errorf("search settings may include at most %d custom ranking entries", maxCustomRanking)
	}

	attributes, err := normalizeAttributes(settings.Attributes)
	if err != nil {
		return Settings{}, err
	}
	customRanking, err := normalizeCustomRanking(settings.CustomRanking)
	if err != nil {
		return Settings{}, err
	}
	return Settings{Attributes: attributes, CustomRanking: customRanking}, nil
}

func normalizeAttributes(attributes []Attribute) ([]Attribute, error) {
	seen := make(map[string]struct{}, len(attributes))
	normalized := make([]Attribute, 0, len(attributes))
	for _, attr := range attributes {
		column := strings.TrimSpace(attr.Column)
		if column == "" {
			return nil, fmt.Errorf("search setting attribute column is required")
		}
		if _, ok := seen[column]; ok {
			return nil, fmt.Errorf("duplicate search setting attribute column: %s", column)
		}
		weight := Weight(strings.TrimSpace(string(attr.Weight)))
		if _, err := PostgresWeightLabel(weight); err != nil {
			return nil, err
		}
		seen[column] = struct{}{}
		normalized = append(normalized, Attribute{Column: column, Weight: weight})
	}
	return normalized, nil
}

func normalizeCustomRanking(customRanking []CustomRanking) ([]CustomRanking, error) {
	if len(customRanking) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(customRanking))
	normalized := make([]CustomRanking, 0, len(customRanking))
	for _, ranking := range customRanking {
		column := strings.TrimSpace(ranking.Column)
		if column == "" {
			return nil, fmt.Errorf("search setting custom ranking column is required")
		}
		if _, ok := seen[column]; ok {
			return nil, fmt.Errorf("duplicate search setting custom ranking column: %s", column)
		}
		order, err := normalizeRankingOrder(ranking.Order)
		if err != nil {
			return nil, err
		}
		seen[column] = struct{}{}
		normalized = append(normalized, CustomRanking{Column: column, Order: order})
	}
	return normalized, nil
}

func normalizeRankingOrder(order RankingOrder) (RankingOrder, error) {
	normalized := RankingOrder(strings.ToLower(strings.TrimSpace(string(order))))
	switch normalized {
	case RankingOrderAsc, RankingOrderDesc:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown search setting custom ranking order: %s", order)
	}
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
	for _, ranking := range normalized.CustomRanking {
		if err := validateCustomRankingColumn(tbl, ranking.Column); err != nil {
			return Settings{}, err
		}
	}
	return normalized, nil
}

func validateCustomRankingColumn(tbl *schema.Table, columnName string) error {
	col := tbl.ColumnByName(columnName)
	if col == nil {
		return fmt.Errorf("search setting custom ranking column %q was not found on table %q", columnName, tbl.Name)
	}
	if !IsRankableColumn(col) {
		return fmt.Errorf("search setting custom ranking column %q is not rankable on table %q", columnName, tbl.Name)
	}
	return nil
}

func IsRankableColumn(col *schema.Column) bool {
	return !col.IsJSON && !col.IsArray && !col.IsEnum
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
