package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type deployJSONError struct {
	Provider string `json:"provider"`
	Error    string `json:"error"`
}

// outputDeployError formats and outputs a deployment error as JSON or plain text.
func outputDeployError(cmd *cobra.Command, provider string, err error) error {
	if err == nil {
		return nil
	}
	if outputFormat(cmd) == "json" {
		providerLabel := deployProviderTokenUnknown
		if provider != "" {
			providerLabel = normalizeDeployProviderName(provider)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		if encodeErr := enc.Encode(deployJSONError{Provider: providerLabel, Error: err.Error()}); encodeErr != nil {
			return encodeErr
		}
		return nil
	}
	return err
}

// outputDeployResult formats and outputs a successful deployment result.
func outputDeployResult(cmd *cobra.Command, result DeployResult) error {
	if outputFormat(cmd) == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(result)
	}

	if result.Provider != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Provider: %s\n", result.Provider)
	}
	if result.AppURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "App URL: %s\n", result.AppURL)
	}
	if result.DashboardURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Dashboard URL: %s\n", result.DashboardURL)
	}
	if len(result.NextSteps) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
		for _, step := range result.NextSteps {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", step)
		}
	}
	return nil
}

// deriveAppName derives a provider-safe app name from a domain or creates a fallback name.
func deriveAppName(domain, defaultSuffix string) string {
	if strings.TrimSpace(domain) != "" {
		name := sanitizeFlyAppName(normalizeDomainForAppName(domain))
		if name != "" {
			return name
		}
	}
	if strings.TrimSpace(defaultSuffix) == "" {
		defaultSuffix = "ayb"
	}
	if clean := sanitizeFlyAppName(defaultSuffix); clean != "" {
		defaultSuffix = clean
	}
	return defaultSuffix + randomAlphaNum(4)
}

// randomAlphaNum generates a random alphanumeric string of specified length.
func randomAlphaNum(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return strings.Repeat("0", length)
	}
	for i, v := range raw {
		b[i] = charset[int(v)%len(charset)]
	}
	return string(b)
}

// mergeDeployEnv combines deploy config with additional environment mappings.
func mergeDeployEnv(cfg DeployConfig) (map[string]string, error) {
	mergedEnv := make(map[string]string)
	for k, v := range cfg.Env {
		mergedEnv[k] = v
	}
	if cfg.PostgresURL != "" {
		mergedEnv["AYB_DATABASE_URL"] = cfg.PostgresURL
	}
	if _, hasJWTSecret := mergedEnv["AYB_AUTH_JWT_SECRET"]; !hasJWTSecret {
		jwtSecret, err := generateJWTSecret()
		if err != nil {
			return nil, err
		}
		mergedEnv["AYB_AUTH_JWT_SECRET"] = jwtSecret
	}
	return mergedEnv, nil
}

// generateJWTSecret creates a JWT secret for the application.
func generateJWTSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hasDeployDatabaseConfig(cfg DeployConfig) bool {
	if strings.TrimSpace(cfg.PostgresURL) != "" {
		return true
	}
	return strings.TrimSpace(cfg.Env["AYB_DATABASE_URL"]) != ""
}

func warnMissingDatabaseConfig(cfg DeployConfig) {
	if hasDeployDatabaseConfig(cfg) {
		return
	}
	fmt.Fprintln(os.Stderr, "warning: no --postgres-url provided and AYB_DATABASE_URL not found in --env; AYB requires a database to function")
}

func resolveProviderTimeout(override, fallback time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	return fallback
}
