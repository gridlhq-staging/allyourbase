package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// RPC calls a PostgreSQL function with JSON-encoded arguments.
func (c *Client) RPC(ctx context.Context, name string, args any) (json.RawMessage, error) {
	response, err := c.doJSON(ctx, http.MethodPost, "/api/rpc/"+url.PathEscape(name), nil, args)
	if err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, nil
	}
	return json.RawMessage(response), nil
}
