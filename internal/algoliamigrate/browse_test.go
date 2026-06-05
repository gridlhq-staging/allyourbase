package algoliamigrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestBrowseClientFollowsCursorAndSendsAlgoliaHeaders(t *testing.T) {
	t.Parallel()

	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/1/indexes/products/browse" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Algolia-Application-Id"); got != "APPID123" {
			t.Fatalf("application header = %q", got)
		}
		if got := r.Header.Get("X-Algolia-API-Key"); got != "secret-key" {
			t.Fatalf("api key header = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requests = append(requests, body)

		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"hits":[{"objectID":"one"}],"cursor":"next-page"}`))
		case 2:
			_, _ = w.Write([]byte(`{"hits":[{"objectID":"two"},{"objectID":"three"}]}`))
		default:
			t.Fatalf("unexpected extra browse request %d", len(requests))
		}
	}))
	defer server.Close()

	client, err := NewBrowseClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewBrowseClient: %v", err)
	}

	result, err := client.Browse(context.Background())
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}

	if got := len(result.Records); got != 3 {
		t.Fatalf("record count = %d, want 3", got)
	}
	if got := result.Requests; got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
	if _, ok := requests[0]["cursor"]; ok {
		t.Fatalf("first request unexpectedly sent cursor: %#v", requests[0])
	}
	if got := requests[1]["cursor"]; got != "next-page" {
		t.Fatalf("second request cursor = %#v, want next-page", got)
	}
}

func TestBrowseClientDoesNotLeakCredentialsInErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream rejected secret-key", http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewBrowseClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewBrowseClient: %v", err)
	}

	_, err = client.Browse(context.Background())
	if err == nil {
		t.Fatal("Browse unexpectedly succeeded")
	}
	if msg := err.Error(); strings.Contains(msg, "secret-key") || strings.Contains(msg, "APPID123") {
		t.Fatalf("error leaked credential: %s", msg)
	}
}

func TestBrowseClientRejectsAppIDHostInjection(t *testing.T) {
	t.Parallel()

	for _, appID := range []string{
		"attacker.example/path",
		"APPID123@attacker.example",
		"APPID123:443",
	} {
		_, err := NewBrowseClient(BrowseConfig{
			AppID:     appID,
			APIKey:    "secret-key",
			IndexName: "products",
		})
		if err == nil {
			t.Fatalf("NewBrowseClient accepted app ID %q that can alter the default host", appID)
		}
		if msg := err.Error(); !strings.Contains(msg, "alphanumeric") {
			t.Fatalf("error for app ID %q = %q, want alphanumeric guidance", appID, msg)
		}
	}
}

func TestBrowseClientSkipsMalformedHitsAndCountsDecodedRecords(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[{"objectID":"one","price":12},null,"bad-hit",{"objectID":"two","price":19.5}]}`))
	}))
	defer server.Close()

	client, err := NewBrowseClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewBrowseClient: %v", err)
	}

	result, err := client.Browse(context.Background())
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if got := result.Requests; got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
	if got := len(result.Records); got != 2 {
		t.Fatalf("decoded record count = %d, want 2", got)
	}
	if _, ok := result.Records[0]["price"].(json.Number); !ok {
		t.Fatalf("first record price type = %T, want json.Number", result.Records[0]["price"])
	}
	if got := result.Records[1]["objectID"]; got != "two" {
		t.Fatalf("second record objectID = %#v, want two", got)
	}
}

func TestSynonymSearchClientPostsEmptyQueryFollowsPagesAndSendsAlgoliaHeaders(t *testing.T) {
	t.Parallel()

	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/1/indexes/products/synonyms/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Algolia-Application-Id"); got != "APPID123" {
			t.Fatalf("application header = %q", got)
		}
		if got := r.Header.Get("X-Algolia-API-Key"); got != "secret-key" {
			t.Fatalf("api key header = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requests = append(requests, body)

		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = w.Write([]byte(`{"hits":[{"objectID":"one","type":"synonym","synonyms":["a","b"]}],"page":0,"nbPages":2}`))
		case 2:
			_, _ = w.Write([]byte(`{"hits":[{"objectID":"two","type":"placeholder","placeholder":"<p>","replacements":["c"]}],"page":1,"nbPages":2}`))
		default:
			t.Fatalf("unexpected extra synonym request %d", len(requests))
		}
	}))
	defer server.Close()

	client, err := NewSynonymSearchClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewSynonymSearchClient: %v", err)
	}

	input, err := client.SearchSynonyms(context.Background())
	if err != nil {
		t.Fatalf("SearchSynonyms: %v", err)
	}

	if got := len(input.Hits); got != 2 {
		t.Fatalf("hit count = %d, want 2", got)
	}
	if input.Stats.Requests != 2 {
		t.Fatalf("requests = %d, want 2", input.Stats.Requests)
	}
	wantBodies := []map[string]any{
		{"query": ""},
		{"query": "", "page": float64(1)},
	}
	if !reflect.DeepEqual(requests, wantBodies) {
		t.Fatalf("request bodies = %#v, want %#v", requests, wantBodies)
	}
}

func TestSynonymSearchClientTreatsSettingsACLFailuresAsSkippedStats(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "settings ACL rejected secret-key", http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewSynonymSearchClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewSynonymSearchClient: %v", err)
	}

	input, err := client.SearchSynonyms(context.Background())
	if err != nil {
		t.Fatalf("SearchSynonyms returned record-import error for settings ACL failure: %v", err)
	}
	if input.Stats.SkippedSettingsACL != 1 {
		t.Fatalf("settings ACL skips = %d, want 1", input.Stats.SkippedSettingsACL)
	}
	if got := len(input.Hits); got != 0 {
		t.Fatalf("hits = %d, want none", got)
	}
}
