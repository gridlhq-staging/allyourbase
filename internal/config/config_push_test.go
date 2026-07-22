package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPushUseLogProviderConfigContract(t *testing.T) {
	t.Parallel()

	cfg := Default()
	testutil.False(t, cfg.Push.UseLogProvider)

	value, err := GetValue(cfg, "push.use_log_provider")
	testutil.NoError(t, err)
	testutil.Equal(t, false, value.(bool))

	parsed, err := ParseTOML([]byte(`
[jobs]
enabled = true

[push]
enabled = true
use_log_provider = true
`))
	testutil.NoError(t, err)
	testutil.True(t, parsed.Push.UseLogProvider)
}

func TestPushUseLogProviderEnvironmentContract(t *testing.T) {
	t.Setenv("AYB_JOBS_ENABLED", "true")
	t.Setenv("AYB_PUSH_ENABLED", "true")
	t.Setenv("AYB_PUSH_USE_LOG_PROVIDER", "true")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"), nil)
	testutil.NoError(t, err)
	testutil.True(t, cfg.Push.UseLogProvider)
}

func TestPushUseLogProviderSetValueContract(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "ayb.toml")
	testutil.NoError(t, SetValue(configPath, "push.use_log_provider", "true"))

	cfg, err := Load(configPath, nil)
	testutil.NoError(t, err)
	testutil.True(t, cfg.Push.UseLogProvider)
}

func TestPushUseLogProviderValidationContract(t *testing.T) {
	t.Parallel()

	t.Run("log_provider_still_requires_jobs", func(t *testing.T) {
		cfg := Default()
		cfg.Push.Enabled = true
		cfg.Push.UseLogProvider = true

		testutil.ErrorContains(t, cfg.Validate(), "push.enabled requires jobs.enabled")
	})

	t.Run("log_provider_satisfies_provider_presence", func(t *testing.T) {
		cfg := Default()
		cfg.Jobs.Enabled = true
		cfg.Push.Enabled = true
		cfg.Push.UseLogProvider = true

		testutil.NoError(t, cfg.Validate())
	})

	t.Run("log_provider_is_rejected_on_non_loopback_host", func(t *testing.T) {
		cfg := Default()
		cfg.Jobs.Enabled = true
		cfg.Push.Enabled = true
		cfg.Push.UseLogProvider = true
		cfg.Server.Host = "0.0.0.0"

		testutil.ErrorContains(t, cfg.Validate(), "push.use_log_provider is limited to local loopback runtimes")
	})

	t.Run("log_provider_is_rejected_with_remote_site_url", func(t *testing.T) {
		cfg := Default()
		cfg.Jobs.Enabled = true
		cfg.Push.Enabled = true
		cfg.Push.UseLogProvider = true
		cfg.Server.SiteURL = "https://push.example.com"

		testutil.ErrorContains(t, cfg.Validate(), "push.use_log_provider is limited to local loopback runtimes")
	})

	t.Run("normal_mode_still_requires_configured_provider", func(t *testing.T) {
		cfg := Default()
		cfg.Jobs.Enabled = true
		cfg.Push.Enabled = true

		testutil.ErrorContains(t, cfg.Validate(), "push.enabled requires at least one provider")
	})
}

func TestPushUseLogProviderInvalidInputsFailClosed(t *testing.T) {
	t.Run("invalid_toml_boolean", func(t *testing.T) {
		_, err := ParseTOML([]byte(`
[push]
use_log_provider = "true"
`))
		testutil.ErrorContains(t, err, "UseLogProvider")
	})

	t.Run("invalid_environment_boolean", func(t *testing.T) {
		t.Setenv("AYB_PUSH_USE_LOG_PROVIDER", "yes")
		cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"), nil)
		testutil.ErrorContains(t, err, "AYB_PUSH_USE_LOG_PROVIDER")
		if cfg != nil {
			testutil.False(t, cfg.Push.UseLogProvider)
		}
	})

	t.Run("invalid_set_value_boolean", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "ayb.toml")
		err := SetValue(configPath, "push.use_log_provider", "yes")
		testutil.ErrorContains(t, err, "push.use_log_provider")
		_, statErr := os.Stat(configPath)
		testutil.True(t, os.IsNotExist(statErr), "invalid SetValue must not create config file")
	})
}
