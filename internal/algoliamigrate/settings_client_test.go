package algoliamigrate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeSettingsResponseReaderParsesAllThreeArrays(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "algolia_settings_sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	settings, err := DecodeSettingsResponseReader(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("DecodeSettingsResponseReader: %v", err)
	}

	wantSearchable := []string{"title", "unordered(subtitle)", "tags"}
	if !reflect.DeepEqual(settings.SearchableAttributes, wantSearchable) {
		t.Fatalf("SearchableAttributes = %#v, want %#v", settings.SearchableAttributes, wantSearchable)
	}
	wantCustom := []string{"desc(inventory_count)", "asc(price)"}
	if !reflect.DeepEqual(settings.CustomRanking, wantCustom) {
		t.Fatalf("CustomRanking = %#v, want %#v", settings.CustomRanking, wantCustom)
	}
	wantFaceting := []string{"tags", "searchable(published)"}
	if !reflect.DeepEqual(settings.AttributesForFaceting, wantFaceting) {
		t.Fatalf("AttributesForFaceting = %#v, want %#v", settings.AttributesForFaceting, wantFaceting)
	}
}

func TestDecodeSettingsResponseReaderLegacyAttributesToIndexAlias(t *testing.T) {
	t.Parallel()

	body := `{
		"attributesToIndex": ["title", "unordered(subtitle)"],
		"customRanking": ["desc(price)"],
		"attributesForFaceting": ["tags"]
	}`

	settings, err := DecodeSettingsResponseReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSettingsResponseReader: %v", err)
	}

	wantSearchable := []string{"title", "unordered(subtitle)"}
	if !reflect.DeepEqual(settings.SearchableAttributes, wantSearchable) {
		t.Fatalf("SearchableAttributes = %#v, want %#v", settings.SearchableAttributes, wantSearchable)
	}
	wantCustom := []string{"desc(price)"}
	if !reflect.DeepEqual(settings.CustomRanking, wantCustom) {
		t.Fatalf("CustomRanking = %#v, want %#v", settings.CustomRanking, wantCustom)
	}
	wantFaceting := []string{"tags"}
	if !reflect.DeepEqual(settings.AttributesForFaceting, wantFaceting) {
		t.Fatalf("AttributesForFaceting = %#v, want %#v", settings.AttributesForFaceting, wantFaceting)
	}
}

func TestDecodeSettingsResponseReaderPrefersSearchableAttributesOverLegacyAlias(t *testing.T) {
	t.Parallel()

	body := `{
		"searchableAttributes": ["title", "unordered(subtitle)"],
		"attributesToIndex": ["legacy_title", "legacy_subtitle"],
		"customRanking": ["desc(price)"],
		"attributesForFaceting": ["tags"]
	}`

	settings, err := DecodeSettingsResponseReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSettingsResponseReader: %v", err)
	}

	wantSearchable := []string{"title", "unordered(subtitle)"}
	if !reflect.DeepEqual(settings.SearchableAttributes, wantSearchable) {
		t.Fatalf("SearchableAttributes = %#v, want %#v", settings.SearchableAttributes, wantSearchable)
	}
}

func TestDecodeSettingsResponseReaderIgnoresUnknownKeysAndMissingArrays(t *testing.T) {
	t.Parallel()

	body := `{
		"queryType": "prefixLast",
		"hitsPerPage": 20,
		"unknownKey": {"nested": true},
		"customRanking": ["asc(name)"]
	}`

	settings, err := DecodeSettingsResponseReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSettingsResponseReader: %v", err)
	}

	if settings.SearchableAttributes != nil {
		t.Fatalf("SearchableAttributes = %#v, want nil", settings.SearchableAttributes)
	}
	if settings.AttributesForFaceting != nil {
		t.Fatalf("AttributesForFaceting = %#v, want nil", settings.AttributesForFaceting)
	}
	wantCustom := []string{"asc(name)"}
	if !reflect.DeepEqual(settings.CustomRanking, wantCustom) {
		t.Fatalf("CustomRanking = %#v, want %#v", settings.CustomRanking, wantCustom)
	}
}

func TestSettingsClientGetSettingsSendsAuthHeadersAndDecodesResponse(t *testing.T) {
	t.Parallel()

	requestErrs := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := validateSettingsRequest(r); err != nil {
			requestErrs <- err
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"searchableAttributes":["title"],"customRanking":["desc(price)"],"attributesForFaceting":["tags"]}`))
	}))
	defer server.Close()

	client, err := NewSettingsClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewSettingsClient: %v", err)
	}

	settings, err := client.GetSettings(context.Background())
	if requestErr := receiveSettingsRequestError(requestErrs); requestErr != nil {
		t.Fatalf("request contract: %v", requestErr)
	}
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !reflect.DeepEqual(settings.SearchableAttributes, []string{"title"}) {
		t.Fatalf("SearchableAttributes = %#v", settings.SearchableAttributes)
	}
	if !reflect.DeepEqual(settings.CustomRanking, []string{"desc(price)"}) {
		t.Fatalf("CustomRanking = %#v", settings.CustomRanking)
	}
	if !reflect.DeepEqual(settings.AttributesForFaceting, []string{"tags"}) {
		t.Fatalf("AttributesForFaceting = %#v", settings.AttributesForFaceting)
	}
}

func validateSettingsRequest(r *http.Request) error {
	if r.Method != http.MethodGet {
		return fmt.Errorf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != "/1/indexes/products/settings" {
		return fmt.Errorf("path = %s, want /1/indexes/products/settings", r.URL.Path)
	}
	if got := r.Header.Get("X-Algolia-Application-Id"); got != "APPID123" {
		return fmt.Errorf("application header = %q", got)
	}
	if got := r.Header.Get("X-Algolia-API-Key"); got != "secret-key" {
		return fmt.Errorf("api key header = %q", got)
	}
	return nil
}

func receiveSettingsRequestError(requestErrs <-chan error) error {
	select {
	case err := <-requestErrs:
		return err
	default:
		return nil
	}
}

func TestSettingsClientGetSettingsRedactsCredentialsOnError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"access denied for key secret-key"}`))
	}))
	defer server.Close()

	client, err := NewSettingsClient(BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewSettingsClient: %v", err)
	}

	_, err = client.GetSettings(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error leaks api key: %v", err)
	}
}

func TestNewSettingsClientRejectsRemotePlaintextEndpoints(t *testing.T) {
	t.Parallel()

	_, err := NewSettingsClient(BrowseConfig{
		Endpoint:  "http://example.com",
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err == nil {
		t.Fatal("expected insecure remote http endpoint to be rejected")
	}
	if !strings.Contains(err.Error(), "remote endpoints must use https") {
		t.Fatalf("error = %v, want remote https validation", err)
	}
}

func TestNewSettingsClientRejectsNonAlgoliaHTTPSHosts(t *testing.T) {
	t.Parallel()

	_, err := NewSettingsClient(BrowseConfig{
		Endpoint:  "https://example.com",
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err == nil {
		t.Fatal("expected non-Algolia https endpoint to be rejected")
	}
	if !strings.Contains(err.Error(), "official Algolia endpoint or loopback") {
		t.Fatalf("error = %v, want host allowlist validation", err)
	}
}

func TestNewSettingsClientAllowsLoopbackFixtureEndpoints(t *testing.T) {
	t.Parallel()

	client, err := NewSettingsClient(BrowseConfig{
		Endpoint:  "http://127.0.0.1:8123",
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	})
	if err != nil {
		t.Fatalf("NewSettingsClient: %v", err)
	}
	if client.endpoint != "http://127.0.0.1:8123" {
		t.Fatalf("endpoint = %q, want loopback fixture endpoint", client.endpoint)
	}
}

func TestFetchSettingsFallsBackToFixtureWithoutCredentials(t *testing.T) {
	t.Parallel()

	fixture := AlgoliaSettings{
		SearchableAttributes:  []string{"title"},
		CustomRanking:         []string{"desc(price)"},
		AttributesForFaceting: []string{"tags"},
	}
	got, err := FetchSettings(context.Background(), BrowseConfig{}, fixture)
	if err != nil {
		t.Fatalf("FetchSettings: %v", err)
	}
	if !reflect.DeepEqual(got, fixture) {
		t.Fatalf("FetchSettings = %#v, want %#v", got, fixture)
	}
}

func TestFetchSettingsUsesLiveCredentialsWhenConfigured(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"searchableAttributes":["live_title"],"customRanking":["asc(rank)"],"attributesForFaceting":["live_tags"]}`))
	}))
	defer server.Close()

	got, err := FetchSettings(context.Background(), BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	}, AlgoliaSettings{SearchableAttributes: []string{"fixture_only"}})
	if err != nil {
		t.Fatalf("FetchSettings: %v", err)
	}
	wantSearchable := []string{"live_title"}
	if !reflect.DeepEqual(got.SearchableAttributes, wantSearchable) {
		t.Fatalf("SearchableAttributes = %#v, want %#v", got.SearchableAttributes, wantSearchable)
	}
}
