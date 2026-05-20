// Package cli implements deployment to Fly.io via HTTP API, including app and machine creation, secrets management, and configuration generation.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	flyDefaultBaseURL    = "https://api.machines.dev"
	flyDefaultRegion     = "iad"
	flyDefaultVMSize     = "shared-cpu-1x"
	flyDefaultVolumeSize = 1
	flyDefaultWaitTime   = 30 * time.Second
)

type FlyClient interface {
	CreateApp(ctx context.Context, req FlyCreateAppRequest) (FlyApp, error)
	GetApp(ctx context.Context, appName string) (FlyApp, error)
	CreateVolume(ctx context.Context, appName string, req FlyCreateVolumeRequest) (FlyVolume, error)
	SetSecrets(ctx context.Context, appName string, req FlySetSecretsRequest) error
	CreateMachine(ctx context.Context, appName string, req FlyCreateMachineRequest) (FlyMachine, error)
	WaitForMachineState(ctx context.Context, appName, machineID, state string, timeout time.Duration) error
}

type flyProvider struct {
	client        FlyClient
	jwtSecretFunc func() (string, error)
	nowFunc       func() time.Time
	waitTimeout   time.Duration
}

func (p flyProvider) Name() string {
	return deployProviderFly
}

func (p flyProvider) Validate(cfg DeployConfig) error {
	image := strings.TrimSpace(cfg.ProviderOptions[flyOptionImage])
	if image == "" {
		return errors.New("--image is required. Build and push an image first, then pass --image <registry>/<repo>:<tag>")
	}
	warnMissingDatabaseConfig(cfg)
	return nil
}

// Deploy creates and configures a complete application deployment on Fly.io, provisioning an app, storage volume, setting secrets, and launching a virtual machine. It waits for the machine to reach the started state and returns deployment metadata and next-step instructions.
func (p flyProvider) Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error) {
	options := resolveFlyOptions(cfg)
	appName := options.AppName
	if appName == "" {
		appName = deriveFlyAppName(cfg.Domain, p.now())
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = flyDefaultRegion
	}

	client := p.client
	if client == nil {
		client = newFlyHTTPClient(cfg.Token, "")
	}

	if err := ensureFlyAppExists(ctx, client, appName); err != nil {
		return DeployResult{}, err
	}

	volumeName := flyVolumeName(appName)
	if _, err := client.CreateVolume(ctx, appName, FlyCreateVolumeRequest{
		Name:   volumeName,
		Region: region,
		SizeGB: options.VolumeSize,
	}); err != nil {
		return DeployResult{}, err
	}

	secrets, err := p.buildFlySecrets(cfg)
	if err != nil {
		return DeployResult{}, err
	}
	if err := client.SetSecrets(ctx, appName, FlySetSecretsRequest{Secrets: secrets}); err != nil {
		return DeployResult{}, err
	}

	machine, err := client.CreateMachine(ctx, appName, buildFlyMachineRequest(appName, volumeName, options))
	if err != nil {
		return DeployResult{}, err
	}

	if err := client.WaitForMachineState(ctx, appName, machine.ID, "started", p.timeout()); err != nil {
		return DeployResult{}, err
	}

	return buildFlyDeployResult(cfg, appName, region, volumeName, machine.ID), nil
}

func (p flyProvider) buildFlySecrets(cfg DeployConfig) (map[string]string, error) {
	secrets, err := mergeDeployEnv(cfg)
	if err != nil {
		return nil, err
	}
	// Allow deterministic secrets in tests while keeping mergeDeployEnv as the shared source.
	if p.jwtSecretFunc != nil {
		jwtSecret, err := p.newJWTSecret()
		if err != nil {
			return nil, err
		}
		secrets["AYB_AUTH_JWT_SECRET"] = jwtSecret
	}
	return secrets, nil
}

func (p flyProvider) newJWTSecret() (string, error) {
	if p.jwtSecretFunc != nil {
		return p.jwtSecretFunc()
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate AYB_AUTH_JWT_SECRET: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (p flyProvider) now() time.Time {
	if p.nowFunc != nil {
		return p.nowFunc()
	}
	return time.Now().UTC()
}

func (p flyProvider) timeout() time.Duration {
	return resolveProviderTimeout(p.waitTimeout, flyDefaultWaitTime)
}

func ensureFlyAppExists(ctx context.Context, client FlyClient, appName string) error {
	if _, err := client.CreateApp(ctx, FlyCreateAppRequest{Name: appName}); err != nil {
		if !IsDeployStatusCode(err, http.StatusConflict) {
			return err
		}
		if _, getErr := client.GetApp(ctx, appName); getErr != nil {
			return getErr
		}
	}
	return nil
}

func buildFlyMachineRequest(appName, volumeName string, options flyOptions) FlyCreateMachineRequest {
	return FlyCreateMachineRequest{
		Name: appName + "-vm",
		Config: FlyMachineConfig{
			Image: options.Image,
			Mounts: []FlyMachineMount{
				{Volume: volumeName, Path: "/data"},
			},
			Services: []FlyMachineService{
				{
					Protocol:     "tcp",
					InternalPort: 8090,
					Ports: []FlyServicePort{
						{Port: 80, Handlers: []string{"http"}},
						{Port: 443, Handlers: []string{"tls", "http"}},
					},
				},
			},
			Checks: map[string]FlyCheck{
				"http": {
					Type:        "http",
					GracePeriod: "20s",
					Interval:    "15s",
					Timeout:     "10s",
					HTTP: FlyHTTPCheck{
						Method: "GET",
						Path:   "/health",
						Port:   8090,
					},
				},
			},
			Guest: FlyMachineGuest{Size: options.VMSize},
			Restart: &FlyMachineRestartPolicy{
				Policy: "always",
			},
		},
	}
}

func buildFlyDeployResult(cfg DeployConfig, appName, region, volumeName, machineID string) DeployResult {
	appURL := fmt.Sprintf("https://%s.fly.dev", appName)
	nextSteps := []string{
		fmt.Sprintf("Set an admin password: ayb admin reset-password --url %s", appURL),
		fmt.Sprintf("View logs: flyctl logs -a %s", appName),
	}
	if strings.TrimSpace(cfg.Domain) != "" {
		nextSteps = append(nextSteps, fmt.Sprintf("Create CNAME record: %s -> %s.fly.dev", strings.TrimSpace(cfg.Domain), appName))
	}

	return DeployResult{
		Provider:     deployProviderFly,
		AppURL:       appURL,
		DashboardURL: fmt.Sprintf("https://fly.io/apps/%s", appName),
		NextSteps:    nextSteps,
		Metadata: map[string]any{
			"app_name":    appName,
			"region":      region,
			"volume_name": volumeName,
			"machine_id":  machineID,
			"fly_toml":    generateFlyToml(appName, region),
			"dockerfile":  generateDockerfile(),
		},
	}
}

type flyOptions struct {
	AppName    string
	Image      string
	VMSize     string
	VolumeSize int
}

// resolveFlyOptions extracts and normalizes Fly deployment options from the configuration, applying defaults for VM size and volume size.
func resolveFlyOptions(cfg DeployConfig) flyOptions {
	appName := sanitizeFlyAppName(cfg.ProviderOptions[flyOptionAppName])
	image := strings.TrimSpace(cfg.ProviderOptions[flyOptionImage])
	vmSize := strings.TrimSpace(cfg.ProviderOptions[flyOptionVMSize])
	if vmSize == "" {
		vmSize = flyDefaultVMSize
	}
	volumeSize := flyDefaultVolumeSize
	if raw := strings.TrimSpace(cfg.ProviderOptions[flyOptionVolumeSize]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			volumeSize = parsed
		}
	}
	return flyOptions{
		AppName:    appName,
		Image:      image,
		VMSize:     vmSize,
		VolumeSize: volumeSize,
	}
}

// deriveFlyAppName generates a valid Fly app name by normalizing the provided domain, applying sanitization, and using a timestamp-based fallback if the domain is empty or invalid. The returned name is at most 63 characters.
func deriveFlyAppName(domain string, now time.Time) string {
	candidate := normalizeDomainForAppName(domain)
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	candidate = sanitizeFlyAppName(candidate)
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	if len(candidate) > 63 {
		candidate = strings.Trim(candidate[:63], "-")
	}
	if candidate == "" {
		candidate = fmt.Sprintf("ayb-%d", now.Unix())
	}
	return candidate
}

func normalizeDomainForAppName(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	if idx := strings.Index(domain, "/"); idx >= 0 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, ":"); idx >= 0 {
		domain = domain[:idx]
	}
	return domain
}

// sanitizeFlyAppName converts a string into a valid Fly application name by converting to lowercase, replacing special characters with hyphens, collapsing consecutive hyphens, and truncating to 63 characters.
func sanitizeFlyAppName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	value = replacer.Replace(value)

	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if r == '-' {
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	clean := strings.Trim(b.String(), "-")
	if len(clean) > 63 {
		clean = strings.Trim(clean[:63], "-")
	}
	return clean
}

func flyVolumeName(appName string) string {
	return appName + "-data"
}

// generateFlyToml generates fly.toml configuration file content for the given app name and region, including HTTP service, health checks, and volume mount settings.
func generateFlyToml(appName, region string) string {
	if strings.TrimSpace(region) == "" {
		region = flyDefaultRegion
	}
	volumeName := flyVolumeName(appName)
	return fmt.Sprintf(`app = "%s"
primary_region = "%s"

[http_service]
  internal_port = 8090
  force_https = true

[checks]
  [checks.http]
    type = "http"
    path = "/health"
    interval = "15s"
    timeout = "10s"

[[mounts]]
  source = "%s"
  destination = "/data"
`, appName, region, volumeName)
}

func generateDockerfile() string {
	return `FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY ayb /usr/local/bin/ayb
EXPOSE 8090
ENTRYPOINT ["ayb"]
CMD ["start"]
`
}
