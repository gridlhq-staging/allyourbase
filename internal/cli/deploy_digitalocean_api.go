package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DigitalOceanClient defines the interface for interacting with the DigitalOcean API.
type DigitalOceanClient interface {
	CreateDroplet(ctx context.Context, req DigitalOceanDropletCreateRequest) (DigitalOceanDroplet, error)
	GetDroplet(ctx context.Context, id int) (DigitalOceanDroplet, error)
	CreateFirewall(ctx context.Context, req DigitalOceanFirewallCreateRequest) (DigitalOceanFirewall, error)
	WaitForDropletActive(ctx context.Context, id int) (DigitalOceanDroplet, error)
	WaitForDropletPublicIP(ctx context.Context, id int) (string, error)
}

// digitalOceanHTTPClient implements DigitalOceanClient with HTTP API calls.
type digitalOceanHTTPClient struct {
	client  *http.Client
	token   string
	baseURL string
}

func newDigitalOceanHTTPClient(token string) *digitalOceanHTTPClient {
	return &digitalOceanHTTPClient{
		client:  &http.Client{Timeout: 30 * time.Second},
		token:   strings.TrimSpace(token),
		baseURL: doDefaultBaseURL,
	}
}

// DigitalOceanDroplet represents a droplet response from DigitalOcean API.
type DigitalOceanDroplet struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	PublicIP string `json:"public_ip_address,omitempty"`
	Networks struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks,omitempty"`
}

// DigitalOceanFirewall represents a firewall response from DigitalOcean API.
type DigitalOceanFirewall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DigitalOceanFirewallTargets struct {
	Addresses []string `json:"addresses,omitempty"`
}

// DigitalOceanInboundFirewallRule represents an inbound firewall rule for DigitalOcean.
type DigitalOceanInboundFirewallRule struct {
	Protocol  string                      `json:"protocol"`
	PortRange string                      `json:"ports"`
	Sources   DigitalOceanFirewallTargets `json:"sources"`
}

// DigitalOceanOutboundFirewallRule represents an outbound firewall rule for DigitalOcean.
type DigitalOceanOutboundFirewallRule struct {
	Protocol     string                      `json:"protocol"`
	PortRange    string                      `json:"ports"`
	Destinations DigitalOceanFirewallTargets `json:"destinations"`
}

// DigitalOceanDropletCreateRequest represents a request to create a droplet.
type DigitalOceanDropletCreateRequest struct {
	Name     string   `json:"name"`
	Region   string   `json:"region"`
	Size     string   `json:"size"`
	Image    string   `json:"image"`
	SSHKeys  []string `json:"ssh_keys,omitempty"`
	UserData string   `json:"user_data,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// DigitalOceanFirewallCreateRequest represents a request to create a firewall.
type DigitalOceanFirewallCreateRequest struct {
	Name          string                             `json:"name"`
	InboundRules  []DigitalOceanInboundFirewallRule  `json:"inbound_rules"`
	OutboundRules []DigitalOceanOutboundFirewallRule `json:"outbound_rules"`
	DropletIDs    []int                              `json:"droplet_ids"`
}

// doJSON performs an authenticated JSON HTTP request to the DigitalOcean API.
func (c *digitalOceanHTTPClient) doJSON(ctx context.Context, method, path string, reqBody, out any) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal digitalocean payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create digitalocean request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform digitalocean request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read digitalocean response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return buildDigitalOceanAPIError(resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode digitalocean response: %w", err)
		}
	}
	return nil
}

// buildDigitalOceanAPIError constructs a DeployAPIError from an HTTP status code and response body.
func buildDigitalOceanAPIError(statusCode int, body []byte) *DeployAPIError {
	return buildDeployAPIError("digitalocean", statusCode, body, "message", "error")
}

func (c *digitalOceanHTTPClient) CreateDroplet(ctx context.Context, req DigitalOceanDropletCreateRequest) (DigitalOceanDroplet, error) {
	var resp struct {
		Droplet DigitalOceanDroplet `json:"droplet"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/droplets", req, &resp); err != nil {
		return DigitalOceanDroplet{}, err
	}
	return resp.Droplet, nil
}

// GetDroplet retrieves a droplet by ID from the DigitalOcean API.
func (c *digitalOceanHTTPClient) GetDroplet(ctx context.Context, id int) (DigitalOceanDroplet, error) {
	path := fmt.Sprintf("/droplets/%d", id)
	var resp struct {
		Droplet DigitalOceanDroplet `json:"droplet"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return DigitalOceanDroplet{}, err
	}

	if resp.Droplet.PublicIP == "" {
		for _, n := range resp.Droplet.Networks.V4 {
			if n.Type == "public" {
				resp.Droplet.PublicIP = strings.TrimSpace(n.IPAddress)
				break
			}
		}
	}
	return resp.Droplet, nil
}

func (c *digitalOceanHTTPClient) CreateFirewall(ctx context.Context, req DigitalOceanFirewallCreateRequest) (DigitalOceanFirewall, error) {
	var resp struct {
		Firewall DigitalOceanFirewall `json:"firewall"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/firewalls", req, &resp); err != nil {
		return DigitalOceanFirewall{}, err
	}
	return resp.Firewall, nil
}

func (c *digitalOceanHTTPClient) WaitForDropletActive(ctx context.Context, id int) (DigitalOceanDroplet, error) {
	for {
		droplet, err := c.GetDroplet(ctx, id)
		if err != nil {
			return DigitalOceanDroplet{}, err
		}
		if droplet.Status == "active" {
			return droplet, nil
		}
		if err := sleepWithContext(ctx, doPollInterval); err != nil {
			return DigitalOceanDroplet{}, err
		}
	}
}

// WaitForDropletPublicIP polls until the droplet has a public IP address assigned.
func (c *digitalOceanHTTPClient) WaitForDropletPublicIP(ctx context.Context, id int) (string, error) {
	for {
		droplet, err := c.GetDroplet(ctx, id)
		if err != nil {
			return "", err
		}
		if ip := strings.TrimSpace(droplet.PublicIP); ip != "" {
			return ip, nil
		}
		for _, n := range droplet.Networks.V4 {
			if n.Type == "public" && strings.TrimSpace(n.IPAddress) != "" {
				return strings.TrimSpace(n.IPAddress), nil
			}
		}
		if err := sleepWithContext(ctx, doPollInterval); err != nil {
			return "", err
		}
	}
}
