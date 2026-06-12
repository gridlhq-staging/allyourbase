// Package algoliamigrate.
package algoliamigrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SettingsClient fetches Algolia index settings through the v1 getSettings endpoint.
type SettingsClient struct {
	endpoint   string
	appID      string
	apiKey     string
	indexName  string
	httpClient *http.Client
}

// NewSettingsClient validates configuration and composes the browse client seam.
func NewSettingsClient(cfg BrowseConfig) (*SettingsClient, error) {
	browseClient, err := NewBrowseClient(cfg)
	if err != nil {
		return nil, err
	}
	return &SettingsClient{
		endpoint:   browseClient.endpoint,
		appID:      browseClient.appID,
		apiKey:     browseClient.apiKey,
		indexName:  browseClient.indexName,
		httpClient: browseClient.httpClient,
	}, nil
}

// GetSettings returns the decoded Algolia index settings for the configured index.
func (c *SettingsClient) GetSettings(ctx context.Context) (AlgoliaSettings, error) {
	resp, err := algoliaGetJSON(ctx, algoliaJSONRequest{
		httpClient: c.httpClient,
		endpoint:   c.settingsURL(),
		appID:      c.appID,
		apiKey:     c.apiKey,
		operation:  "getSettings",
	})
	if err != nil {
		return AlgoliaSettings{}, c.redactError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AlgoliaSettings{}, c.redactError(algoliaStatusError(resp, "getSettings"))
	}

	settings, err := DecodeSettingsResponseReader(resp.Body)
	if err != nil {
		return AlgoliaSettings{}, c.redactError(fmt.Errorf("decoding algolia getSettings response: %w", err))
	}
	return settings, nil
}

func (c *SettingsClient) settingsURL() string {
	return c.endpoint + "/1/indexes/" + url.PathEscape(c.indexName) + "/settings"
}

func (c *SettingsClient) redactError(err error) error {
	return redactAlgoliaError(err, c.appID, c.apiKey)
}

// DecodeSettingsResponse decodes a getSettings envelope from raw bytes.
func DecodeSettingsResponse(raw []byte) (AlgoliaSettings, error) {
	return DecodeSettingsResponseReader(bytes.NewReader(raw))
}

// DecodeSettingsResponseReader decodes a getSettings envelope, accepting the
// legacy attributesToIndex alias for searchableAttributes and ignoring unknown
// keys so the source model stays scoped to Stage 1 fields.
func DecodeSettingsResponseReader(r io.Reader) (AlgoliaSettings, error) {
	var raw struct {
		SearchableAttributes  []string `json:"searchableAttributes"`
		AttributesToIndex     []string `json:"attributesToIndex"`
		CustomRanking         []string `json:"customRanking"`
		AttributesForFaceting []string `json:"attributesForFaceting"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return AlgoliaSettings{}, err
	}
	searchable := raw.SearchableAttributes
	if searchable == nil {
		searchable = raw.AttributesToIndex
	}
	return AlgoliaSettings{
		SearchableAttributes:  searchable,
		CustomRanking:         raw.CustomRanking,
		AttributesForFaceting: raw.AttributesForFaceting,
	}, nil
}

// FetchSettings uses live Algolia credentials when present and otherwise returns
// the supplied fixture so normal tests never need secrets, mirroring
// CheckRecordParity in migrator.go.
func FetchSettings(ctx context.Context, cfg BrowseConfig, fixture AlgoliaSettings) (AlgoliaSettings, error) {
	if !hasLiveBrowseCredentials(cfg) {
		return fixture, nil
	}
	client, err := NewSettingsClient(cfg)
	if err != nil {
		return AlgoliaSettings{}, err
	}
	return client.GetSettings(ctx)
}
