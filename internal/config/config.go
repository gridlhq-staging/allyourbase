// Package config Config types and functions for loading, validating, and managing AYB configuration from TOML files, environment variables, and CLI flags with comprehensive defaults and utilities.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level AYB configuration struct containing all settings for the server, database, authentication, billing, logging, storage, edge functions, and other subsystems.
type Config struct {
	Server           ServerConfig            `toml:"server"`
	Database         DatabaseConfig          `toml:"database"`
	ManagedPG        ManagedPGConfig         `toml:"managed_pg"`
	Admin            AdminConfig             `toml:"admin"`
	Auth             AuthConfig              `toml:"auth"`
	Billing          BillingConfig           `toml:"billing"`
	Support          SupportConfig           `toml:"support"`
	RateLimit        RateLimitConfig         `toml:"rate_limit"`
	API              APIConfig               `toml:"api"`
	Vault            VaultConfig             `toml:"vault"`
	Email            EmailConfig             `toml:"email"`
	Storage          StorageConfig           `toml:"storage"`
	EdgeFunctions    EdgeFuncConfig          `toml:"edge_functions"`
	Logging          LoggingConfig           `toml:"logging"`
	Metrics          MetricsConfig           `toml:"metrics"`
	Telemetry        TelemetryConfig         `toml:"telemetry"`
	Jobs             JobsConfig              `toml:"jobs"`
	Status           StatusConfig            `toml:"status"`
	Push             PushConfig              `toml:"push"`
	Audit            AuditConfig             `toml:"audit"`
	AI               AIConfig                `toml:"ai"`
	DashboardAI      DashboardAIConfig       `toml:"dashboard_ai"`
	Backup           BackupConfig            `toml:"backup"`
	GraphQL          GraphQLConfig           `toml:"graphql"`
	Realtime         RealtimeConfig          `toml:"realtime"`
	EncryptedColumns []EncryptedColumnConfig `toml:"encrypted_columns"`
}

func Default() *Config {
	return &Config{
		Server:        defaultServerConfig(),
		Database:      defaultDatabaseConfig(),
		ManagedPG:     defaultManagedPGConfig(),
		Admin:         defaultAdminConfig(),
		Auth:          defaultAuthConfig(),
		Billing:       defaultBillingConfig(),
		Support:       defaultSupportConfig(),
		RateLimit:     defaultRateLimitConfig(),
		API:           defaultAPIConfig(),
		Vault:         defaultVaultConfig(),
		Email:         defaultEmailConfig(),
		Storage:       defaultStorageConfig(),
		EdgeFunctions: defaultEdgeFunctionsConfig(),
		Logging:       defaultLoggingConfig(),
		Metrics:       defaultMetricsConfig(),
		Telemetry:     defaultTelemetryConfig(),
		Jobs:          defaultJobsConfig(),
		Status:        defaultStatusConfig(),
		Push:          defaultPushConfig(),
		Audit:         defaultAuditConfig(),
		AI:            defaultAIConfig(),
		DashboardAI:   defaultDashboardAIConfig(),
		Backup:        defaultBackupConfig(),
		GraphQL:       defaultGraphQLConfig(),
		Realtime:      defaultRealtimeConfig(),
	}
}

// Load reads configuration with priority: defaults → ayb.toml → env vars → CLI flags.
// The flags parameter allows CLI flag overrides to be passed in.
func Load(configPath string, flags map[string]string) (*Config, error) {
	cfg := Default()

	// Load from TOML file if it exists.
	if configPath == "" {
		configPath = "ayb.toml"
	}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", configPath, err)
		}
	}

	// Apply environment variables.
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	// Apply CLI flag overrides.
	applyFlags(cfg, flags)

	// Validate.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// ParseTOML parses raw TOML bytes into a validated Config.
func ParseTOML(data []byte) (*Config, error) {
	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks the configuration for invalid values.

// Address returns the host:port string for the server to listen on.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// PublicBaseURL returns the public base URL for email action links (password reset,
// magic links, etc.). If server.site_url is configured, it is used as-is (with
// trailing slashes stripped). Otherwise, a URL is constructed from host:port,
// replacing the bind-all address 0.0.0.0 with localhost so links work in browsers.
func (c *Config) PublicBaseURL() string {
	if c.Server.SiteURL != "" {
		return strings.TrimRight(c.Server.SiteURL, "/")
	}
	if c.Server.TLSEnabled && c.Server.TLSDomain != "" {
		return fmt.Sprintf("https://%s", c.Server.TLSDomain)
	}
	host := c.Server.Host
	if host == "0.0.0.0" || host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, c.Server.Port)
}

// GenerateDefault writes a commented default ayb.toml to the given path.
func GenerateDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultTOML), 0o600)
}

// ToTOML returns the config serialized as TOML.
func (c *Config) ToTOML() (string, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
