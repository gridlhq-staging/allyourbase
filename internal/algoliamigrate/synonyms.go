// Package algoliamigrate.
package algoliamigrate

import (
	"fmt"
	"sort"

	"github.com/allyourbase/ayb/internal/searchsynonyms"
)

const (
	algoliaSynonymTypeRegular        = "synonym"
	algoliaSynonymTypeOneWay         = "oneWaySynonym"
	algoliaSynonymTypeAltCorrection1 = "altCorrection1"
	algoliaSynonymTypeAltCorrection2 = "altCorrection2"
	algoliaSynonymTypePlaceholder    = "placeholder"
)

var unsupportedSynonymReasons = map[string]string{
	algoliaSynonymTypeOneWay:         "unsupported directional synonym",
	algoliaSynonymTypeAltCorrection1: "unsupported alternative correction",
	algoliaSynonymTypeAltCorrection2: "unsupported alternative correction",
	algoliaSynonymTypePlaceholder:    "unsupported placeholder synonym",
}

// MapAlgoliaSynonyms converts supported Algolia synonym hits into AYB groups.
func MapAlgoliaSynonyms(input SynonymInput) SynonymPlan {
	stats := cloneSynonymStats(input.Stats)
	candidates := make(searchsynonyms.Groups, 0, len(input.Hits))

	for _, hit := range input.Hits {
		switch hit.Type {
		case algoliaSynonymTypeRegular:
			group, ok := normalizeSingleSynonymGroup(hit.Synonyms, &stats)
			if ok {
				candidates = appendSynonymCandidate(candidates, group, &stats)
			}
		case "":
			stats.SkippedMalformedHits++
		default:
			recordUnsupportedSynonym(hit.Type, &stats)
		}
	}

	if len(candidates) == 0 {
		return SynonymPlan{Stats: stats}
	}
	stats.SupportedGroups = len(candidates)
	return SynonymPlan{Groups: candidates, Stats: stats}
}

func normalizeSingleSynonymGroup(terms []string, stats *SynonymStats) (searchsynonyms.Group, bool) {
	groups, err := searchsynonyms.NormalizeGroups(searchsynonyms.Groups{{Terms: terms}})
	if err != nil {
		stats.SkippedInvalidGroups++
		recordInvalidSynonymReason(err.Error(), stats)
		return searchsynonyms.Group{}, false
	}
	return groups[0], true
}

func appendSynonymCandidate(candidates searchsynonyms.Groups, group searchsynonyms.Group, stats *SynonymStats) searchsynonyms.Groups {
	next := append(append(searchsynonyms.Groups(nil), candidates...), group)
	groups, err := searchsynonyms.NormalizeGroups(next)
	if err != nil {
		stats.SkippedInvalidGroups++
		recordInvalidSynonymReason(err.Error(), stats)
		return candidates
	}
	return groups
}

func recordUnsupportedSynonym(synonymType string, stats *SynonymStats) {
	reason, ok := unsupportedSynonymReasons[synonymType]
	if !ok {
		stats.SkippedMalformedHits++
		return
	}
	if stats.SkippedUnsupportedTypes == nil {
		stats.SkippedUnsupportedTypes = map[string]int{}
	}
	if stats.SkippedUnsupportedReasons == nil {
		stats.SkippedUnsupportedReasons = map[string]string{}
	}
	stats.SkippedUnsupportedTypes[synonymType]++
	stats.SkippedUnsupportedReasons[synonymType] = reason
}

func recordInvalidSynonymReason(reason string, stats *SynonymStats) {
	if stats.SkippedInvalidReasons == nil {
		stats.SkippedInvalidReasons = map[string]int{}
	}
	stats.SkippedInvalidReasons[reason]++
}

func cloneSynonymStats(stats SynonymStats) SynonymStats {
	stats.SkippedUnsupportedTypes = cloneIntMap(stats.SkippedUnsupportedTypes)
	stats.SkippedUnsupportedReasons = cloneStringMap(stats.SkippedUnsupportedReasons)
	stats.SkippedInvalidReasons = cloneIntMap(stats.SkippedInvalidReasons)
	return stats
}

func cloneIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func synonymWarningMessages(stats SynonymStats) []string {
	var warnings []string
	for _, synonymType := range sortedSynonymTypes(stats.SkippedUnsupportedTypes) {
		reason := stats.SkippedUnsupportedReasons[synonymType]
		warnings = append(warnings, pluralWarning(stats.SkippedUnsupportedTypes[synonymType],
			"unsupported "+synonymType+" synonyms skipped: "+reason))
	}
	if stats.SkippedInvalidGroups > 0 {
		warnings = append(warnings, pluralWarning(stats.SkippedInvalidGroups, "invalid synonym groups skipped"))
	}
	if stats.SkippedSettingsACL > 0 {
		warnings = append(warnings, pluralWarning(stats.SkippedSettingsACL,
			"synonym enumeration skipped due to missing settings ACL"))
	}
	if stats.SkippedMalformedHits > 0 {
		warnings = append(warnings, pluralWarning(stats.SkippedMalformedHits, "malformed synonym hits skipped"))
	}
	return warnings
}

func sortedSynonymTypes(counts map[string]int) []string {
	preferred := []string{
		algoliaSynonymTypeOneWay,
		algoliaSynonymTypeAltCorrection1,
		algoliaSynonymTypeAltCorrection2,
		algoliaSynonymTypePlaceholder,
	}
	seen := map[string]bool{}
	var out []string
	for _, synonymType := range preferred {
		if counts[synonymType] > 0 {
			out = append(out, synonymType)
			seen[synonymType] = true
		}
	}
	var unknown []string
	for synonymType, count := range counts {
		if count > 0 && !seen[synonymType] {
			unknown = append(unknown, synonymType)
		}
	}
	sort.Strings(unknown)
	out = append(out, unknown...)
	return out
}

func pluralWarning(count int, suffix string) string {
	return fmt.Sprintf("%d %s", count, suffix)
}
