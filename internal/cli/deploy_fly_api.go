package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type FlyCreateAppRequest struct {
	Name string
}

type FlyApp struct {
	Name string
}

type FlyCreateVolumeRequest struct {
	Name   string `json:"name,omitempty"`
	Region string `json:"region"`
	SizeGB int    `json:"size_gb"`
}

type FlyVolume struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FlySetSecretsRequest struct {
	Secrets map[string]string
}

type FlySecretEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type FlyCreateMachineRequest struct {
	Name   string           `json:"name,omitempty"`
	Config FlyMachineConfig `json:"config"`
}

type FlyMachineConfig struct {
	Image    string                   `json:"image"`
	Mounts   []FlyMachineMount        `json:"mounts,omitempty"`
	Services []FlyMachineService      `json:"services,omitempty"`
	Checks   map[string]FlyCheck      `json:"checks,omitempty"`
	Guest    FlyMachineGuest          `json:"guest"`
	Env      map[string]string        `json:"env,omitempty"`
	Metadata map[string]string        `json:"metadata,omitempty"`
	Restart  *FlyMachineRestartPolicy `json:"restart,omitempty"`
}

type FlyMachineRestartPolicy struct {
	Policy string `json:"policy"`
}

type FlyMachineMount struct {
	Volume string `json:"volume"`
	Path   string `json:"path"`
}

type FlyMachineService struct {
	Protocol     string           `json:"protocol"`
	InternalPort int              `json:"internal_port"`
	Ports        []FlyServicePort `json:"ports,omitempty"`
}

type FlyServicePort struct {
	Port     int      `json:"port"`
	Handlers []string `json:"handlers,omitempty"`
}

type FlyCheck struct {
	Type        string       `json:"type"`
	GracePeriod string       `json:"grace_period,omitempty"`
	Interval    string       `json:"interval,omitempty"`
	Timeout     string       `json:"timeout,omitempty"`
	HTTP        FlyHTTPCheck `json:"http_service"`
}

type FlyHTTPCheck struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Port   int    `json:"port"`
}

type FlyMachineGuest struct {
	Size string `json:"size"`
}

type FlyMachine struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
}

type flyHTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newFlyHTTPClient(token, baseURL string) *flyHTTPClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = flyDefaultBaseURL
	}
	return &flyHTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateApp sends an HTTP POST request to create an application with the given name.
func (c *flyHTTPClient) CreateApp(ctx context.Context, req FlyCreateAppRequest) (FlyApp, error) {
	payload := struct {
		AppName string `json:"app_name"`
	}{
		AppName: req.Name,
	}
	var resp struct {
		Name string `json:"name"`
		App  struct {
			Name string `json:"name"`
		} `json:"app"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apps", payload, &resp); err != nil {
		return FlyApp{}, err
	}
	if resp.Name == "" {
		resp.Name = resp.App.Name
	}
	return FlyApp{Name: resp.Name}, nil
}

// GetApp retrieves application details from the Fly API by name.
func (c *flyHTTPClient) GetApp(ctx context.Context, appName string) (FlyApp, error) {
	var resp struct {
		Name string `json:"name"`
		App  struct {
			Name string `json:"name"`
		} `json:"app"`
	}
	path := "/v1/apps/" + url.PathEscape(appName)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return FlyApp{}, err
	}
	if resp.Name == "" {
		resp.Name = resp.App.Name
	}
	return FlyApp{Name: resp.Name}, nil
}

func (c *flyHTTPClient) CreateVolume(ctx context.Context, appName string, req FlyCreateVolumeRequest) (FlyVolume, error) {
	var resp FlyVolume
	path := "/v1/apps/" + url.PathEscape(appName) + "/volumes"
	if err := c.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return FlyVolume{}, err
	}
	return resp, nil
}

// SetSecrets sends environment variables to the Fly API for the given application.
func (c *flyHTTPClient) SetSecrets(ctx context.Context, appName string, req FlySetSecretsRequest) error {
	entries := make([]FlySecretEntry, 0, len(req.Secrets))
	keys := make([]string, 0, len(req.Secrets))
	for k := range req.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entries = append(entries, FlySecretEntry{Key: key, Value: req.Secrets[key]})
	}
	payload := struct {
		Secrets []FlySecretEntry `json:"secrets"`
	}{Secrets: entries}
	path := "/v1/apps/" + url.PathEscape(appName) + "/secrets"
	return c.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func (c *flyHTTPClient) CreateMachine(ctx context.Context, appName string, req FlyCreateMachineRequest) (FlyMachine, error) {
	var resp FlyMachine
	path := "/v1/apps/" + url.PathEscape(appName) + "/machines"
	if err := c.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return FlyMachine{}, err
	}
	return resp, nil
}

func (c *flyHTTPClient) WaitForMachineState(ctx context.Context, appName, machineID, state string, timeout time.Duration) error {
	query := url.Values{}
	if strings.TrimSpace(state) != "" {
		query.Set("state", state)
	}
	if timeout > 0 {
		query.Set("timeout", strconv.Itoa(int(timeout.Seconds())))
	}
	path := "/v1/apps/" + url.PathEscape(appName) + "/machines/" + url.PathEscape(machineID) + "/wait"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return c.doJSON(ctx, http.MethodGet, path, nil, nil)
}

// doJSON performs an HTTP request to the Fly API with JSON serialization and deserialization.
func (c *flyHTTPClient) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal fly payload: %w", err)
		}
		body = strings.NewReader(string(raw))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create fly request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform fly request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read fly response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return buildFlyAPIError(resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode fly response: %w", err)
		}
	}
	return nil
}

// buildFlyAPIError parses an HTTP error response from the Fly API and constructs a DeployAPIError.
func buildFlyAPIError(statusCode int, body []byte) *DeployAPIError {
	return buildDeployAPIError("fly", statusCode, body, "error", "message")
}
