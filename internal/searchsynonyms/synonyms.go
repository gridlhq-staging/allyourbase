// Package searchsynonyms Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun05_pm_2_algolia_importer_cli/allyourbase_dev/internal/searchsynonyms/synonyms.go.
package searchsynonyms

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxSearchSynonymTermLength = 128
	maxSearchSynonymGroupTerms = 8
)

type Group struct {
	Terms []string
}

type Groups []Group

func NormalizeGroups(groups Groups) (Groups, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("groups is required")
	}

	seen := make(map[string]struct{})
	normalized := make(Groups, 0, len(groups))
	for _, group := range groups {
		terms, err := normalizeTerms(group.Terms, seen)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, Group{Terms: terms})
	}
	sortGroups(normalized)
	return normalized, nil
}

func normalizeTerms(terms []string, seen map[string]struct{}) ([]string, error) {
	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		if len(term) > maxSearchSynonymTermLength {
			return nil, fmt.Errorf("synonym terms must be 128 characters or fewer")
		}
		if _, ok := seen[term]; ok {
			return nil, fmt.Errorf("duplicate synonym term: %s", term)
		}
		seen[term] = struct{}{}
		normalized = append(normalized, term)
	}
	if len(normalized) < 2 {
		return nil, fmt.Errorf("each synonym group must include at least two terms")
	}
	if len(normalized) > maxSearchSynonymGroupTerms {
		return nil, fmt.Errorf("synonym groups may include at most 8 terms")
	}
	sort.Strings(normalized)
	return normalized, nil
}

func sortGroups(groups Groups) {
	sort.Slice(groups, func(i, j int) bool {
		return strings.Join(groups[i].Terms, "\x00") < strings.Join(groups[j].Terms, "\x00")
	})
}
