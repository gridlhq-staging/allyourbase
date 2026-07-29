package allyourbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func TestE2EGoRPCLive(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}
	adminToken := os.Getenv("AYB_TEST_ADMIN_TOKEN")
	if adminToken == "" {
		t.Fatal("AYB_TEST_ADMIN_TOKEN not set")
	}

	requestFixture := mustLoadContractFixture(t, "rpc_request.json")
	var args map[string]int
	if err := json.Unmarshal(requestFixture, &args); err != nil {
		t.Fatalf("decode rpc request fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := NewClient(baseURL, WithAPIKey(adminToken)).RPC(ctx, "sdk_contract_add", args)
	if err != nil {
		t.Fatalf("RPC sdk_contract_add: %v", err)
	}
	expected := mustLoadContractFixture(t, "rpc_response.json")
	if !bytes.Equal(got, expected) {
		t.Fatalf("RPC response = %q, want fixture %q", got, expected)
	}
}

func TestE2ERealtimeCreateEventLive(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}
	collection := os.Getenv("AYB_TEST_COLLECTION")
	if collection == "" {
		t.Skip("AYB_TEST_COLLECTION not set")
	}

	c := NewClient(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := c.Auth.SignInAnonymously(ctx); err != nil {
		t.Fatalf("sign in anonymously: %v", err)
	}

	events, unsubscribe, err := c.Realtime().Subscribe(ctx, collection, SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe to realtime collection %q: %v", collection, err)
	}
	defer func() {
		if err := unsubscribe(); err != nil {
			t.Errorf("unsubscribe from realtime collection %q: %v", collection, err)
		}
	}()

	contract := mustLoadSharedListSearchSeedContract(t)
	row := map[string]any{
		"id":       fmt.Sprintf("go_realtime_%d", time.Now().UnixNano()),
		"title":    "Go SDK realtime live create",
		"category": realtimeWritableCategory(contract),
	}

	created, err := c.Records.Create(ctx, collection, row)
	if err != nil {
		t.Fatalf("create realtime fixture row %q in %q: %v", row["id"], collection, err)
	}
	createdID, _ := row["id"].(string)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = c.Records.Delete(cleanupCtx, collection, createdID)
	})
	assertRecordContainsFixtureFields(t, created, row)

	event := receiveRealtimeCreateEvent(t, ctx, events, collection, createdID)
	if event.OldRecord != nil {
		t.Fatalf("expected create websocket event without OldRecord, got %#v", event.OldRecord)
	}
	assertRecordContainsFixtureFields(t, event.Record, row)
}

func listHasHighlightedTitle(res *ListResponse, title string) bool {
	for _, item := range res.Items {
		if itemString(item, "title") == title && itemString(item, "_highlight") != "" {
			return true
		}
	}
	return false
}

func realtimeWritableCategory(contract sharedListSearchSeedContract) string {
	if _, ok := contract.ExpectedFacetCounts["docs"]; ok {
		return "docs"
	}
	categories := make([]string, 0, len(contract.ExpectedFacetCounts))
	for category := range contract.ExpectedFacetCounts {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	if len(categories) == 0 {
		return "live"
	}
	return categories[0]
}

func receiveRealtimeCreateEvent(t *testing.T, ctx context.Context, events <-chan Event, collection, createdID string) Event {
	t.Helper()

	var lastObserved *Event
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("realtime event channel closed before create event for row %q; last observed: %+v", createdID, lastObserved)
			}
			lastObserved = &event
			if event.Table == collection && itemString(event.Record, "id") == createdID {
				if event.Action != "create" {
					t.Fatalf("event action for row %q = %q, want create; event: %+v", createdID, event.Action, event)
				}
				return event
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for create event for row %q; last observed: %+v", createdID, lastObserved)
		}
	}
}

func assertRecordContainsFixtureFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for _, key := range []string{"id", "title", "category"} {
		if got[key] != want[key] {
			t.Fatalf("record field %q = %#v, want %#v in record %#v", key, got[key], want[key], got)
		}
	}
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

func TestE2EWebAuthnBeginLive(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}

	c := NewClient(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	email := fmt.Sprintf("probe-%d@example.com", time.Now().UnixNano())
	resp, err := c.Auth.BeginWebAuthnLogin(ctx, email)
	if err != nil {
		t.Fatalf("BeginWebAuthnLogin: %v", err)
	}
	if resp.ChallengeID == "" {
		t.Fatal("expected non-empty ChallengeID")
	}
	if resp.Options.Challenge == "" {
		t.Fatal("expected non-empty Options.Challenge")
	}
	if resp.Options.RPID == "" {
		t.Fatal("expected non-empty Options.RPID")
	}
	if resp.Options.AllowCredentials == nil {
		t.Fatal("expected non-nil Options.AllowCredentials")
	}
}

func TestE2EWebAuthnDiscoverableBeginLive(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}

	c := NewClient(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.Auth.BeginDiscoverableLogin(ctx)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}
	if resp.ChallengeID == "" {
		t.Fatal("expected non-empty ChallengeID")
	}
	if resp.Options.Challenge == "" {
		t.Fatal("expected non-empty Options.Challenge")
	}
	if resp.Options.RPID == "" {
		t.Fatal("expected non-empty Options.RPID")
	}
	if len(resp.Options.AllowCredentials) != 0 {
		t.Fatalf("expected absent or empty Options.AllowCredentials, got %+v", resp.Options.AllowCredentials)
	}
}

func TestE2ESynonymsRoundTripLive(t *testing.T) {
	baseURL := os.Getenv("AYB_TEST_URL")
	if baseURL == "" {
		t.Skip("AYB_TEST_URL not set")
	}
	adminToken := os.Getenv("AYB_TEST_ADMIN_TOKEN")
	if adminToken == "" {
		t.Skip("AYB_TEST_ADMIN_TOKEN not set")
	}
	collection := os.Getenv("AYB_TEST_COLLECTION")
	if collection == "" {
		t.Skip("AYB_TEST_COLLECTION not set")
	}

	c := NewClient(baseURL)
	c.SetTokens(adminToken, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseline, err := c.Records.GetSynonyms(ctx, collection)
	if err != nil {
		t.Fatalf("GetSynonyms (baseline): %v", err)
	}
	t.Cleanup(func() {
		restore := SearchSynonymsRequest{Groups: []SearchSynonymsGroup{}}
		if baseline != nil && len(baseline.Groups) > 0 {
			restore.Groups = make([]SearchSynonymsGroup, len(baseline.Groups))
			copy(restore.Groups, baseline.Groups)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = c.Records.SetSynonyms(cleanupCtx, collection, restore)
	})

	probeGroup := SearchSynonymsGroup{Terms: []string{"new york", "nyc"}}
	setReq := SearchSynonymsRequest{Groups: []SearchSynonymsGroup{probeGroup}}
	_, err = c.Records.SetSynonyms(ctx, collection, setReq)
	if err != nil {
		t.Fatalf("SetSynonyms: %v", err)
	}

	got, err := c.Records.GetSynonyms(ctx, collection)
	if err != nil {
		t.Fatalf("GetSynonyms (after set): %v", err)
	}
	assertSynonymGroupPresent(t, got.Groups, probeGroup.Terms)
}

func assertSynonymGroupPresent(t *testing.T, groups []SearchSynonymsGroup, wantTerms []string) {
	t.Helper()
	sorted := make([]string, len(wantTerms))
	copy(sorted, wantTerms)
	sort.Strings(sorted)

	for _, g := range groups {
		actual := make([]string, len(g.Terms))
		copy(actual, g.Terms)
		sort.Strings(actual)
		if len(actual) == len(sorted) {
			match := true
			for i := range actual {
				if actual[i] != sorted[i] {
					match = false
					break
				}
			}
			if match {
				return
			}
		}
	}
	t.Fatalf("expected synonym group containing %v, got groups %+v", wantTerms, groups)
}
