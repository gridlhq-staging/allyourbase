package algoliamigrate

import (
	"fmt"
	"strings"

	dbschema "github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsettings"
)

const (
	settingsSkipBlankAttribute       = "blank_attribute"
	settingsSkipMalformedAttribute   = "malformed_attribute"
	settingsSkipUnsupportedWrapper   = "unsupported_wrapper"
	settingsSkipNestedAttribute      = "nested_attribute"
	settingsSkipMissingColumn        = "missing_column"
	settingsSkipNonTextColumn        = "non_text_column"
	settingsSkipDuplicateAttribute   = "duplicate_attribute"
	settingsSkipOverAttributeCap     = "over_attribute_cap"
	settingsSkipBlankCustomRanking   = "blank_custom_ranking"
	settingsSkipMalformedRanking     = "malformed_custom_ranking"
	settingsSkipUnsupportedOrder     = "unsupported_ranking_order"
	settingsSkipDuplicateRanking     = "duplicate_custom_ranking"
	settingsSkipRankingMissingColumn = "ranking_missing_column"
	settingsSkipNonRankableColumn    = "non_rankable_column"
	settingsSkipOverRankingCap       = "over_custom_ranking_cap"
	settingsSkipFacetAdvisoryOnly    = "facet_advisory_only"
	maxMappedSearchableAttributes    = 32
	maxMappedCustomRanking           = 32
	unorderedSearchableAttributeCall = "unordered("
)

var settingsSkipMessages = map[string]string{
	settingsSkipBlankAttribute:       "blank searchable attribute",
	settingsSkipMalformedAttribute:   "malformed searchable attribute syntax",
	settingsSkipUnsupportedWrapper:   "unsupported searchable attribute wrapper",
	settingsSkipNestedAttribute:      "nested searchable attributes are not supported",
	settingsSkipMissingColumn:        "searchable attribute column was not inferred",
	settingsSkipNonTextColumn:        "searchable attribute column is not searchable text",
	settingsSkipDuplicateAttribute:   "duplicate searchable attribute",
	settingsSkipOverAttributeCap:     "searchable attribute exceeds 32 attribute cap",
	settingsSkipBlankCustomRanking:   "blank custom ranking entry",
	settingsSkipMalformedRanking:     "malformed custom ranking syntax",
	settingsSkipUnsupportedOrder:     "unsupported custom ranking order",
	settingsSkipDuplicateRanking:     "duplicate custom ranking column",
	settingsSkipRankingMissingColumn: "custom ranking column was not inferred",
	settingsSkipNonRankableColumn:    "custom ranking column is not rankable",
	settingsSkipOverRankingCap:       "custom ranking entry exceeds 32 entry cap",
	settingsSkipFacetAdvisoryOnly:    "attributesForFaceting is advisory-only during import",
}

var algoliaWeightBuckets = []searchsettings.Weight{
	searchsettings.WeightHigh,
	searchsettings.WeightMedium,
	searchsettings.WeightLow,
	searchsettings.WeightLowest,
}

// MapAlgoliaSearchableAttributes maps Algolia searchableAttributes to AYB weights.
func MapAlgoliaSearchableAttributes(settings AlgoliaSettings, inferred Schema) SettingsPlan {
	settings.CustomRanking = nil
	settings.AttributesForFaceting = nil
	return MapAlgoliaSettings(settings, inferred)
}

// MapAlgoliaSettings maps supported Algolia settings into AYB search settings.
func MapAlgoliaSettings(settings AlgoliaSettings, inferred Schema) SettingsPlan {
	mapper := searchableAttributeMapper{
		columns: columnLookup(inferred),
		table:   searchsettingsTable(inferred),
		seen:    map[string]struct{}{},
	}
	groups := mapper.acceptedGroups(settings.SearchableAttributes)
	rankingMapper := customRankingMapper{
		columns: mapper.columns,
		table:   mapper.table,
		seen:    map[string]struct{}{},
		stats:   mapper.stats,
	}
	customRanking := rankingMapper.acceptedRankings(settings.CustomRanking)
	rankingMapper.skipFacets(settings.AttributesForFaceting)

	normalized, err := searchsettings.ValidateForTable(mapper.table, searchsettings.Settings{
		Attributes:    attributesForAcceptedGroups(groups),
		CustomRanking: customRanking,
	})
	if err != nil {
		return SettingsPlan{Stats: rankingMapper.stats}
	}
	return SettingsPlan{
		Settings: normalized,
		Stats:    rankingMapper.stats,
	}
}

type searchableAttributeMapper struct {
	columns map[string]Column
	table   *dbschema.Table
	seen    map[string]struct{}
	stats   SettingsStats
}

func (m *searchableAttributeMapper) acceptedGroups(searchableAttributes []string) [][]string {
	var groups [][]string
	for _, group := range searchableAttributes {
		accepted := m.acceptedGroup(strings.Split(group, ","))
		if len(accepted) > 0 {
			groups = append(groups, accepted)
		}
	}
	m.stats.SupportedAttributes = acceptedAttributeCount(groups)
	return groups
}

func (m *searchableAttributeMapper) acceptedGroup(rawMembers []string) []string {
	var accepted []string
	for _, rawMember := range rawMembers {
		columnName, skipReason := normalizeSearchableAttribute(rawMember)
		if skipReason != "" {
			m.skip(skipReason)
			continue
		}
		if reason := m.skipReasonForColumn(columnName); reason != "" {
			m.skip(reason)
			continue
		}
		if len(m.seen) >= maxMappedSearchableAttributes {
			m.skip(settingsSkipOverAttributeCap)
			continue
		}
		m.seen[columnName] = struct{}{}
		accepted = append(accepted, columnName)
	}
	return accepted
}

func (m *searchableAttributeMapper) skipReasonForColumn(columnName string) string {
	if strings.Contains(columnName, ".") {
		return settingsSkipNestedAttribute
	}
	if _, ok := m.columns[columnName]; !ok {
		return settingsSkipMissingColumn
	}
	if _, ok := m.seen[columnName]; ok {
		return settingsSkipDuplicateAttribute
	}
	_, err := searchsettings.ValidateForTable(m.table, searchsettings.Settings{
		Attributes: []searchsettings.Attribute{{Column: columnName, Weight: searchsettings.WeightHigh}},
	})
	if err != nil {
		return settingsSkipNonTextColumn
	}
	return ""
}

func (m *searchableAttributeMapper) skip(reason string) {
	m.stats.SkippedAttributes++
	incrementSkip(&m.stats, reason)
}

func normalizeSearchableAttribute(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", settingsSkipBlankAttribute
	}
	if strings.HasPrefix(trimmed, unorderedSearchableAttributeCall) {
		if !strings.HasSuffix(trimmed, ")") {
			return "", settingsSkipMalformedAttribute
		}
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, unorderedSearchableAttributeCall), ")"))
		if inner == "" || strings.ContainsAny(inner, "()") {
			return "", settingsSkipMalformedAttribute
		}
		return inner, ""
	}
	if strings.ContainsAny(trimmed, "()") {
		if isWrappedAttribute(trimmed) {
			return "", settingsSkipUnsupportedWrapper
		}
		return "", settingsSkipMalformedAttribute
	}
	return trimmed, ""
}

type customRankingMapper struct {
	columns map[string]Column
	table   *dbschema.Table
	seen    map[string]struct{}
	stats   SettingsStats
}

func (m *customRankingMapper) acceptedRankings(rawRankings []string) []searchsettings.CustomRanking {
	rankings := make([]searchsettings.CustomRanking, 0, len(rawRankings))
	for _, rawRanking := range rawRankings {
		ranking, skipReason := normalizeCustomRanking(rawRanking)
		if skipReason != "" {
			m.skip(skipReason)
			continue
		}
		if reason := m.skipReasonForRanking(ranking); reason != "" {
			m.skip(reason)
			continue
		}
		if len(m.seen) >= maxMappedCustomRanking {
			m.skip(settingsSkipOverRankingCap)
			continue
		}
		m.seen[ranking.Column] = struct{}{}
		rankings = append(rankings, ranking)
	}
	m.stats.SupportedCustomRanking = len(rankings)
	return rankings
}

func (m *customRankingMapper) skipReasonForRanking(ranking searchsettings.CustomRanking) string {
	if _, ok := m.seen[ranking.Column]; ok {
		return settingsSkipDuplicateRanking
	}
	if _, ok := m.columns[ranking.Column]; !ok {
		return settingsSkipRankingMissingColumn
	}
	_, err := searchsettings.ValidateForTable(m.table, searchsettings.Settings{
		CustomRanking: []searchsettings.CustomRanking{ranking},
	})
	if err != nil {
		return settingsSkipNonRankableColumn
	}
	return ""
}

func (m *customRankingMapper) skipFacets(rawFacets []string) {
	for range rawFacets {
		m.stats.SkippedFacets++
		incrementSkip(&m.stats, settingsSkipFacetAdvisoryOnly)
	}
}

func (m *customRankingMapper) skip(reason string) {
	m.stats.SkippedCustomRanking++
	incrementSkip(&m.stats, reason)
}

func normalizeCustomRanking(raw string) (searchsettings.CustomRanking, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return searchsettings.CustomRanking{}, settingsSkipBlankCustomRanking
	}
	open := strings.Index(trimmed, "(")
	close := strings.LastIndex(trimmed, ")")
	if open <= 0 || close != len(trimmed)-1 || strings.ContainsAny(trimmed[open+1:close], "()") {
		return searchsettings.CustomRanking{}, settingsSkipMalformedRanking
	}
	order, ok := normalizeCustomRankingOrder(trimmed[:open])
	if !ok {
		return searchsettings.CustomRanking{}, settingsSkipUnsupportedOrder
	}
	column := strings.TrimSpace(trimmed[open+1 : close])
	if column == "" {
		return searchsettings.CustomRanking{}, settingsSkipMalformedRanking
	}
	return searchsettings.CustomRanking{Column: column, Order: order}, ""
}

func normalizeCustomRankingOrder(raw string) (searchsettings.RankingOrder, bool) {
	switch strings.TrimSpace(raw) {
	case string(searchsettings.RankingOrderAsc):
		return searchsettings.RankingOrderAsc, true
	case string(searchsettings.RankingOrderDesc):
		return searchsettings.RankingOrderDesc, true
	default:
		return "", false
	}
}

func isWrappedAttribute(attribute string) bool {
	open := strings.Index(attribute, "(")
	close := strings.LastIndex(attribute, ")")
	return open > 0 &&
		close == len(attribute)-1 &&
		!strings.ContainsAny(attribute[:open], "()") &&
		!strings.ContainsAny(attribute[open+1:close], "()")
}

func attributesForAcceptedGroups(groups [][]string) []searchsettings.Attribute {
	attributes := make([]searchsettings.Attribute, 0, acceptedAttributeCount(groups))
	for groupIndex, group := range groups {
		weight := weightForGroup(groupIndex, len(groups))
		for _, columnName := range group {
			attributes = append(attributes, searchsettings.Attribute{
				Column: columnName,
				Weight: weight,
			})
		}
	}
	return attributes
}

func weightForGroup(groupIndex, groupCount int) searchsettings.Weight {
	if groupCount <= 1 {
		return searchsettings.WeightHigh
	}
	// Algolia group position is projected into four AYB buckets with floor(i/(n-1)*3).
	bucket := groupIndex * (len(algoliaWeightBuckets) - 1) / (groupCount - 1)
	return algoliaWeightBuckets[bucket]
}

func columnLookup(inferred Schema) map[string]Column {
	columns := make(map[string]Column, len(inferred.Columns))
	for _, column := range inferred.Columns {
		columns[column.Name] = column
	}
	return columns
}

func searchsettingsTable(inferred Schema) *dbschema.Table {
	columns := make([]*dbschema.Column, 0, len(inferred.Columns))
	for _, column := range inferred.Columns {
		columns = append(columns, searchsettingsColumn(column))
	}
	return &dbschema.Table{Name: "algolia_import", Columns: columns}
}

func searchsettingsColumn(column Column) *dbschema.Column {
	return &dbschema.Column{
		Name:     column.Name,
		TypeName: string(column.Type),
		IsJSON:   column.Type == ColumnTypeJSONB,
	}
}

func acceptedAttributeCount(groups [][]string) int {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	return count
}

func settingsWarningMessages(stats SettingsStats) []string {
	var warnings []string
	if stats.SkippedFacets > 0 {
		warnings = append(warnings, fmt.Sprintf("%d facet attributes skipped (advisory only)", stats.SkippedFacets))
	}
	if stats.SkippedAttributes > 0 {
		warnings = append(warnings, fmt.Sprintf("%d searchableAttributes skipped", stats.SkippedAttributes))
	}
	if stats.SkippedCustomRanking > 0 {
		warnings = append(warnings, fmt.Sprintf("%d customRanking entries skipped", stats.SkippedCustomRanking))
	}
	return warnings
}

func incrementSkip(stats *SettingsStats, reason string) {
	if stats.SkippedReasons == nil {
		stats.SkippedReasons = map[string]int{}
	}
	if stats.SkippedMessages == nil {
		stats.SkippedMessages = map[string]string{}
	}
	stats.SkippedReasons[reason]++
	stats.SkippedMessages[reason] = settingsSkipMessages[reason]
}
