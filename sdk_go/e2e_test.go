package allyourbase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type sharedListSearchSeedContract struct {
	HighlightSearch     string         `json:"highlightSearch"`
	HighlightedTitle    string         `json:"highlightedTitle"`
	FuzzySearch         string         `json:"fuzzySearch"`
	FuzzyTypoThreshold  float64        `json:"fuzzyTypoThreshold"`
	FuzzyMatchTitle     string         `json:"fuzzyMatchTitle"`
	FacetColumn         string         `json:"facetColumn"`
	ExpectedFacetCounts map[string]int `json:"expectedFacetCounts"`
}

func TestSharedListSearchSeedContractFixture(t *testing.T) {
	contract := mustLoadSharedListSearchSeedContract(t)
	if contract.HighlightSearch == "" {
		t.Fatalf("highlight search must be populated: %+v", contract)
	}
	if contract.HighlightedTitle == "" || contract.FuzzyMatchTitle == "" {
		t.Fatalf("expected titles must be populated: %+v", contract)
	}
	if contract.FuzzySearch == "" || contract.FuzzyTypoThreshold <= 0 {
		t.Fatalf("fuzzy search contract must include query and positive threshold: %+v", contract)
	}
	if contract.FacetColumn == "" || len(contract.ExpectedFacetCounts) == 0 {
		t.Fatalf("facet contract must include column and buckets: %+v", contract)
	}
}

func TestE2EContract(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}
	collection := os.Getenv("AYB_TEST_COLLECTION")
	if collection == "" {
		t.Skip("AYB_TEST_COLLECTION not set")
	}

	c := NewClient(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Auth.SignInAnonymously(ctx); err != nil {
		t.Fatalf("sign in anonymously: %v", err)
	}

	contract := mustLoadSharedListSearchSeedContract(t)
	highlighted, err := c.Records.List(ctx, collection, ListParams{
		Search:    contract.HighlightSearch,
		Highlight: true,
	})
	if err != nil {
		t.Fatalf("list highlighted search results: %v", err)
	}
	if !listHasHighlightedTitle(highlighted, contract.HighlightedTitle) {
		t.Fatalf("expected shared seed highlight for %q, got %+v", contract.HighlightedTitle, highlighted.Items)
	}

	fuzzy, err := c.Records.List(ctx, collection, ListParams{
		Search:        contract.FuzzySearch,
		Fuzzy:         true,
		TypoThreshold: ptrFloat64(contract.FuzzyTypoThreshold),
	})
	if err != nil {
		t.Fatalf("list fuzzy search results: %v", err)
	}
	if !listHasTitle(fuzzy, contract.FuzzyMatchTitle) {
		t.Fatalf("expected shared seed fuzzy match for %q, got %+v", contract.FuzzyMatchTitle, fuzzy.Items)
	}

	faceted, err := c.Records.List(ctx, collection, ListParams{
		Facets: []string{contract.FacetColumn},
	})
	if err != nil {
		t.Fatalf("list faceted results: %v", err)
	}
	assertFacetCounts(t, faceted.Facets[contract.FacetColumn], contract.ExpectedFacetCounts)
}

func listHasHighlightedTitle(res *ListResponse, title string) bool {
	for _, item := range res.Items {
		if itemString(item, "title") == title && itemString(item, "_highlight") != "" {
			return true
		}
	}
	return false
}

func listHasTitle(res *ListResponse, title string) bool {
	for _, item := range res.Items {
		if itemString(item, "title") == title {
			return true
		}
	}
	return false
}

func itemString(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func mustLoadSharedListSearchSeedContract(t *testing.T) sharedListSearchSeedContract {
	t.Helper()
	path := filepath.Join("..", "tests", "contract", "fixtures", "sdk_contract", "list_search_seed_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared list-search seed contract: %v", err)
	}
	var contract sharedListSearchSeedContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("decode shared list-search seed contract: %v", err)
	}
	return contract
}

func assertFacetCounts(t *testing.T, actual []FacetValueCount, expected map[string]int) {
	t.Helper()
	remaining := make(map[string]int, len(expected))
	for value, count := range expected {
		remaining[value] = count
	}
	if len(actual) != len(expected) {
		t.Fatalf("expected %d facet buckets, got %d: %+v", len(expected), len(actual), actual)
	}
	for _, bucket := range actual {
		value, ok := bucket.Value.(string)
		if !ok {
			t.Fatalf("expected string facet value, got %T in %+v", bucket.Value, bucket)
		}
		count, ok := remaining[value]
		if !ok {
			t.Fatalf("unexpected facet bucket %+v; expected buckets %+v", bucket, expected)
		}
		if bucket.Count != count {
			t.Fatalf("facet bucket %q count=%d, expected %d", value, bucket.Count, count)
		}
		delete(remaining, value)
	}
	if len(remaining) > 0 {
		t.Fatalf("missing facet buckets: %+v", remaining)
	}
}
