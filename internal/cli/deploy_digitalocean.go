package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

const (
	doDefaultBaseURL      = "https://api.digitalocean.com/v2"
	doDefaultRegion       = "nyc1"
	doDefaultDropletSize  = "s-1vcpu-1gb"
	doDefaultDropletImage = "ubuntu-22-04-x64"
	doDefaultWaitTimeout  = 2 * time.Minute
	doPollInterval        = 2 * time.Second

	doOptionDropletSize  = "droplet_size"
	doOptionDropletImage = "droplet_image"
	doOptionSSHKeyID     = "ssh_key_id"
	doOptionFirewallName = "firewall_name"
	doOptionBinaryURL    = "binary_url"
)

// digitalOceanProviderOption holds DigitalOcean-specific provider options.
type digitalOceanProviderOption struct {
	DropletSize  string
	DropletImage string
	SSHKeyIDs    []string
	FirewallName string
	BinaryURL    string
}

// resolveDigitalOceanOptions extracts DigitalOcean-specific deployment options from the configuration, applying default values for droplet size and image if not specified.
func resolveDigitalOceanOptions(cfg DeployConfig) digitalOceanProviderOption {
	opts := digitalOceanProviderOption{
		DropletSize:  strings.TrimSpace(cfg.ProviderOptions[doOptionDropletSize]),
		DropletImage: strings.TrimSpace(cfg.ProviderOptions[doOptionDropletImage]),
		FirewallName: strings.TrimSpace(cfg.ProviderOptions[doOptionFirewallName]),
		BinaryURL:    strings.TrimSpace(cfg.ProviderOptions[doOptionBinaryURL]),
		SSHKeyIDs:    splitCommaOption(strings.TrimSpace(cfg.ProviderOptions[doOptionSSHKeyID])),
	}
	if opts.DropletSize == "" {
		opts.DropletSize = doDefaultDropletSize
	}
	if opts.DropletImage == "" {
		opts.DropletImage = doDefaultDropletImage
	}
	return opts
}

// digitalOceanProvider implements the DeployProvider interface for DigitalOcean.
type digitalOceanProvider struct {
	client      DigitalOceanClient
	waitTimeout time.Duration
}

func (p digitalOceanProvider) Name() string {
	return deployProviderDigitalOcean
}

func (p digitalOceanProvider) Validate(cfg DeployConfig) error {
	opts := resolveDigitalOceanOptions(cfg)
	if opts.BinaryURL == "" {
		return fmt.Errorf("DigitalOcean deployment requires --binary-url (URL to pre-built AYB binary tarball)\n  Example: --binary-url https://github.com/allyourbase/ayb/releases/download/v0.1.0/ayb_0.1.0_linux_amd64.tar.gz")
	}
	warnMissingDatabaseConfig(cfg)
	return nil
}

// Deploy creates a DigitalOcean droplet with the specified configuration, waits for it to become active, retrieves its public IP, creates and attaches a firewall, and returns deployment details including the app URL and dashboard link.
func (p digitalOceanProvider) Deploy(ctx context.Context, cfg DeployConfig) (DeployResult, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	opts := resolveDigitalOceanOptions(cfg)
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = doDefaultRegion
	}

	mergedEnv, err := mergeDeployEnv(cfg)
	if err != nil {
		return DeployResult{}, fmt.Errorf("merge deploy env: %w", err)
	}

	appName := deriveAppName(cfg.Domain, "ayb-do-")
	userData, err := generateDigitalOceanCloudInit(cfg, opts, mergedEnv)
	if err != nil {
		return DeployResult{}, fmt.Errorf("generate cloud-init: %w", err)
	}

	client := p.client
	if client == nil {
		client = newDigitalOceanHTTPClient(cfg.Token)
	}

	droplet, err := client.CreateDroplet(waitCtx, buildDigitalOceanDropletRequest(appName, region, opts, userData))
	if err != nil {
		return DeployResult{}, fmt.Errorf("creating droplet: %w", err)
	}

	if _, err := client.WaitForDropletActive(waitCtx, droplet.ID); err != nil {
		return DeployResult{}, fmt.Errorf("waiting for droplet active state: %w", err)
	}

	publicIP, err := client.WaitForDropletPublicIP(waitCtx, droplet.ID)
	if err != nil {
		return DeployResult{}, fmt.Errorf("waiting for droplet public IP: %w", err)
	}

	firewallName := opts.FirewallName
	if firewallName == "" {
		firewallName = fmt.Sprintf("%s-fw", appName)
	}

	firewall, err := client.CreateFirewall(waitCtx, buildDigitalOceanFirewallRequest(firewallName, droplet.ID))
	if err != nil {
		return DeployResult{}, fmt.Errorf("creating firewall: %w", err)
	}

	return buildDigitalOceanDeployResult(cfg, droplet.ID, firewall.ID, publicIP), nil
}

func (p digitalOceanProvider) timeout() time.Duration {
	return resolveProviderTimeout(p.waitTimeout, doDefaultWaitTimeout)
}

func buildDigitalOceanDropletRequest(appName, region string, opts digitalOceanProviderOption, userData string) DigitalOceanDropletCreateRequest {
	return DigitalOceanDropletCreateRequest{
		Name:     appName,
		Region:   region,
		Size:     opts.DropletSize,
		Image:    opts.DropletImage,
		SSHKeys:  opts.SSHKeyIDs,
		UserData: userData,
		Tags:     []string{"ayb-deployment", appName},
	}
}

func buildDigitalOceanFirewallRequest(firewallName string, dropletID int) DigitalOceanFirewallCreateRequest {
	return DigitalOceanFirewallCreateRequest{
		Name: firewallName,
		InboundRules: []DigitalOceanInboundFirewallRule{
			{Protocol: "tcp", PortRange: "22", Sources: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "tcp", PortRange: "8090", Sources: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
		},
		OutboundRules: []DigitalOceanOutboundFirewallRule{
			{Protocol: "tcp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "udp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
			{Protocol: "icmp", PortRange: "all", Destinations: DigitalOceanFirewallTargets{Addresses: []string{"0.0.0.0/0", "::/0"}}},
		},
		DropletIDs: []int{dropletID},
	}
}

func buildDigitalOceanDeployResult(cfg DeployConfig, dropletID int, firewallID, publicIP string) DeployResult {
	appURL := fmt.Sprintf("http://%s:8090", publicIP)
	if strings.TrimSpace(cfg.Domain) != "" {
		appURL = "https://" + strings.TrimSpace(cfg.Domain)
	}

	nextSteps := []string{
		fmt.Sprintf("SSH into the host: ssh root@%s", publicIP),
		"Inspect service logs: sudo journalctl -u ayb.service -f",
		fmt.Sprintf("View droplet in dashboard: https://cloud.digitalocean.com/droplets/%d", dropletID),
	}
	if strings.TrimSpace(cfg.Domain) != "" {
		nextSteps = append(nextSteps, fmt.Sprintf("Create DNS A record: %s -> %s", strings.TrimSpace(cfg.Domain), publicIP))
	}

	return DeployResult{
		Provider:     deployProviderDigitalOcean,
		AppURL:       appURL,
		DashboardURL: fmt.Sprintf("https://cloud.digitalocean.com/droplets/%d", dropletID),
		NextSteps:    nextSteps,
		Metadata: map[string]any{
			"droplet_id":  dropletID,
			"firewall_id": firewallID,
			"public_ip":   publicIP,
		},
	}
}

// generateDigitalOceanCloudInit generates cloud-init user data for AYB binary installation.
func generateDigitalOceanCloudInit(_ DeployConfig, opts digitalOceanProviderOption, mergedEnv map[string]string) (string, error) {
	if strings.TrimSpace(opts.BinaryURL) == "" {
		return "", errors.New("binary URL is required for cloud-init generation")
	}

	envKeys := make([]string, 0, len(mergedEnv))
	for k := range mergedEnv {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	envLines := make([]string, 0, len(envKeys))
	for _, k := range envKeys {
		envLines = append(envLines, fmt.Sprintf("%s=%s", k, shellSingleQuote(mergedEnv[k])))
	}

	data := struct {
		BinaryURLQuoted string
		EnvLines        []string
		SystemdUnit     string
	}{
		BinaryURLQuoted: shellSingleQuote(strings.TrimSpace(opts.BinaryURL)),
		EnvLines:        envLines,
		SystemdUnit:     generateAYBSystemdUnit(),
	}

	const cloudInitTemplate = `#cloud-config
package_update: true
package_upgrade: true
packages:
  - curl
  - ca-certificates

write_files:
  - path: /etc/ayb/.env
    owner: root:root
    permissions: '0644'
    content: |
{{- range .EnvLines }}
      {{ . }}
{{- end }}
  - path: /etc/systemd/system/ayb.service
    owner: root:root
    permissions: '0644'
    content: |
{{ indent .SystemdUnit 6 }}

runcmd:
  - mkdir -p /etc/ayb /usr/local/bin
  - |
      if [ ! -x /usr/local/bin/ayb ]; then
        curl -fsSL {{ .BinaryURLQuoted }} -o /tmp/ayb.tgz
        rm -rf /tmp/ayb-extract
        mkdir -p /tmp/ayb-extract
        tar -xzf /tmp/ayb.tgz -C /tmp/ayb-extract
        AYB_BIN="$(find /tmp/ayb-extract -type f -name 'ayb' | head -n1)"
        if [ -z "$AYB_BIN" ]; then
          echo "ayb binary not found in archive"
          exit 1
        fi
        install -m 0755 "$AYB_BIN" /usr/local/bin/ayb
      fi
  - systemctl daemon-reload
  - systemctl enable ayb.service
  - systemctl restart ayb.service
`

	tmpl, err := template.New("do-cloud-init").Funcs(template.FuncMap{
		"indent": func(s string, spaces int) string {
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i := range lines {
				lines[i] = pad + lines[i]
			}
			return strings.Join(lines, "\n")
		},
	}).Parse(cloudInitTemplate)
	if err != nil {
		return "", fmt.Errorf("parse cloud-init template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render cloud-init template: %w", err)
	}
	return buf.String(), nil
}

// generateAYBSystemdUnit returns the AYB service unit content.
func generateAYBSystemdUnit() string {
	return `[Unit]
Description=Allyourbase Server
After=network.target

[Service]
Type=simple
User=root
Group=root
EnvironmentFile=/etc/ayb/.env
ExecStart=/usr/local/bin/ayb start
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`
}

func splitCommaOption(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func shellSingleQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		d = time.Second
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
