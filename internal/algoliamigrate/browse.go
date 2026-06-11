// Package algoliamigrate.
package algoliamigrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// BrowseClient enumerates records from the Algolia v1 browse endpoint.
type BrowseClient struct {
	endpoint   string
	appID      string
	apiKey     string
	indexName  string
	httpClient *http.Client
}

// NewBrowseClient validates the browse configuration and returns a reusable client.
func NewBrowseClient(cfg BrowseConfig) (*BrowseClient, error) {
	if strings.TrimSpace(cfg.AppID) == "" {
		return nil, errors.New("algolia app ID is required")
	}
	if !isAlgoliaAppID(cfg.AppID) {
		return nil, errors.New("algolia app ID must be alphanumeric")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("algolia API key is required")
	}
	if strings.TrimSpace(cfg.IndexName) == "" {
		return nil, errors.New("algolia index name is required")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = "https://" + strings.ToLower(cfg.AppID) + "-dsn.algolia.net"
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return nil, fmt.Errorf("invalid algolia endpoint: %w", err)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &BrowseClient{
		endpoint:   endpoint,
		appID:      cfg.AppID,
		apiKey:     cfg.APIKey,
		indexName:  cfg.IndexName,
		httpClient: httpClient,
	}, nil
}

func isAlgoliaAppID(appID string) bool {
	for _, r := range appID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// Browse follows Algolia browse cursors until the final envelope omits cursor.
func (c *BrowseClient) Browse(ctx context.Context) (*BrowseResult, error) {
	result := &BrowseResult{}
	var cursor string

	for {
		page, err := c.browsePage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		result.Requests++
		result.Records = append(result.Records, page.Hits...)
		if page.Cursor == "" {
			return result, nil
		}
		cursor = page.Cursor
	}
}

func (c *BrowseClient) browsePage(ctx context.Context, cursor string) (*BrowseResponse, error) {
	body := map[string]string{}
	if cursor != "" {
		body["cursor"] = cursor
	}

	resp, err := algoliaPostJSON(ctx, algoliaJSONRequest{
		httpClient: c.httpClient,
		endpoint:   c.browseURL(),
		appID:      c.appID,
		apiKey:     c.apiKey,
		body:       body,
		operation:  "browse",
	})
	if err != nil {
		return nil, c.redactError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, c.redactError(algoliaStatusError(resp, "browse"))
	}

	page, err := DecodeBrowseResponseReader(resp.Body)
	if err != nil {
		return nil, c.redactError(fmt.Errorf("decoding algolia browse response: %w", err))
	}
	return page, nil
}

func (c *BrowseClient) browseURL() string {
	return c.endpoint + "/1/indexes/" + url.PathEscape(c.indexName) + "/browse"
}

func (c *BrowseClient) redactError(err error) error {
	return redactAlgoliaError(err, c.appID, c.apiKey)
}

func setAlgoliaHeaders(req *http.Request, appID, apiKey string) {
	req.Header.Set("X-Algolia-Application-Id", appID)
	req.Header.Set("X-Algolia-API-Key", apiKey)
}

type algoliaJSONRequest struct {
	httpClient *http.Client
	endpoint   string
	appID      string
	apiKey     string
	body       any
	operation  string
}

func algoliaPostJSON(ctx context.Context, algoliaReq algoliaJSONRequest) (*http.Response, error) {
	payload, err := json.Marshal(algoliaReq.body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, algoliaReq.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building algolia %s request: %w", algoliaReq.operation, err)
	}
	req.Header.Set("Content-Type", "application/json")
	setAlgoliaHeaders(req, algoliaReq.appID, algoliaReq.apiKey)

	resp, err := algoliaReq.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("algolia %s request failed: %w", algoliaReq.operation, err)
	}
	return resp, nil
}

func algoliaStatusError(resp *http.Response, operation string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("algolia %s returned %s: %s", operation, resp.Status, strings.TrimSpace(string(raw)))
}

func redactAlgoliaError(err error, secrets ...string) error {
	msg := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	return errors.New(msg)
}

// DecodeBrowseResponse decodes a browse envelope while preserving JSON numbers.
func DecodeBrowseResponse(raw []byte) (*BrowseResponse, error) {
	return DecodeBrowseResponseReader(bytes.NewReader(raw))
}

// DecodeBrowseResponseReader decodes a browse envelope while preserving JSON numbers.
func DecodeBrowseResponseReader(r io.Reader) (*BrowseResponse, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var envelope struct {
		Hits   []json.RawMessage `json:"hits"`
		Cursor string            `json:"cursor,omitempty"`
	}
	if err := dec.Decode(&envelope); err != nil {
		return nil, err
	}

	resp := &BrowseResponse{
		Hits:   make([]Record, 0, len(envelope.Hits)),
		Cursor: envelope.Cursor,
	}
	for _, rawHit := range envelope.Hits {
		hit, err := decodeBrowseHit(rawHit)
		if err != nil {
			continue
		}
		resp.Hits = append(resp.Hits, hit)
	}
	return resp, nil
}

func decodeBrowseHit(raw []byte) (Record, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var hit Record
	if err := dec.Decode(&hit); err != nil {
		return nil, err
	}
	if hit == nil {
		return nil, errors.New("browse hit must be a JSON object")
	}
	return hit, nil
}
