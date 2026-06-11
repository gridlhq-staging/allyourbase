// Package algoliamigrate.
package algoliamigrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SynonymSearchClient enumerates Algolia synonyms through the settings endpoint.
type SynonymSearchClient struct {
	endpoint   string
	appID      string
	apiKey     string
	indexName  string
	httpClient *http.Client
}

// NewSynonymSearchClient validates configuration for synonym enumeration.
func NewSynonymSearchClient(cfg BrowseConfig) (*SynonymSearchClient, error) {
	browseClient, err := NewBrowseClient(cfg)
	if err != nil {
		return nil, err
	}
	return &SynonymSearchClient{
		endpoint:   browseClient.endpoint,
		appID:      browseClient.appID,
		apiKey:     browseClient.apiKey,
		indexName:  browseClient.indexName,
		httpClient: browseClient.httpClient,
	}, nil
}

// SearchSynonyms follows Algolia synonym search pages. Settings ACL failures are
// reportable synonym skips so record import can still proceed.
func (c *SynonymSearchClient) SearchSynonyms(ctx context.Context) (*SynonymInput, error) {
	input := &SynonymInput{}
	for page := 0; ; page++ {
		envelope, aclSkipped, err := c.searchSynonymPage(ctx, page)
		input.Stats.Requests++
		if aclSkipped {
			input.Stats.SkippedSettingsACL++
			return input, nil
		}
		if err != nil {
			return nil, err
		}
		input.Hits = append(input.Hits, envelope.Hits...)
		if envelope.NbPages == 0 || envelope.Page >= envelope.NbPages-1 {
			return input, nil
		}
	}
}

func (c *SynonymSearchClient) searchSynonymPage(ctx context.Context, page int) (*synonymSearchEnvelope, bool, error) {
	body := map[string]any{"query": ""}
	if page > 0 {
		body["page"] = page
	}

	resp, err := algoliaPostJSON(ctx, algoliaJSONRequest{
		httpClient: c.httpClient,
		endpoint:   c.searchURL(),
		appID:      c.appID,
		apiKey:     c.apiKey,
		body:       body,
		operation:  "synonym",
	})
	if err != nil {
		return nil, false, redactAlgoliaError(err, c.appID, c.apiKey)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, true, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, false, redactAlgoliaError(algoliaStatusError(resp, "synonym search"), c.appID, c.apiKey)
	}

	envelope, err := DecodeSynonymSearchResponseReader(resp.Body)
	if err != nil {
		return nil, false, redactAlgoliaError(fmt.Errorf("decoding algolia synonym response: %w", err), c.appID, c.apiKey)
	}
	return envelope, false, nil
}

func (c *SynonymSearchClient) searchURL() string {
	return c.endpoint + "/1/indexes/" + url.PathEscape(c.indexName) + "/synonyms/search"
}

type synonymSearchEnvelope struct {
	Hits    []AlgoliaSynonymHit
	Page    int
	NbPages int
}

// DecodeSynonymSearchResponseReader decodes Algolia's synonym search envelope.
func DecodeSynonymSearchResponseReader(r io.Reader) (*synonymSearchEnvelope, error) {
	dec := json.NewDecoder(r)
	var raw struct {
		Hits    []json.RawMessage `json:"hits"`
		Page    int               `json:"page,omitempty"`
		NbPages int               `json:"nbPages,omitempty"`
	}
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	envelope := &synonymSearchEnvelope{Page: raw.Page, NbPages: raw.NbPages}
	for _, rawHit := range raw.Hits {
		var hit AlgoliaSynonymHit
		if err := json.Unmarshal(rawHit, &hit); err != nil {
			envelope.Hits = append(envelope.Hits, AlgoliaSynonymHit{})
			continue
		}
		envelope.Hits = append(envelope.Hits, hit)
	}
	return envelope, nil
}
