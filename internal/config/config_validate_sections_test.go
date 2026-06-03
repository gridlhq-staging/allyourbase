package config

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateSectionExtractionsExist(t *testing.T) {
	_ = validateServerConfig
	_ = validateDatabaseConfig
	_ = validateAuthConfig
	_ = validateBillingConfig
	_ = validateGraphQLConfig
	_ = validateSMSConfig
	_ = validateOAuthConfig
	_ = validateOIDCConfig
	_ = validateSAMLConfig
	_ = validateOAuthModeConfig
	_ = validateEmailConfig
	_ = validateStorageConfig
	_ = validateEdgeFuncConfig
	_ = validateLoggingConfig
	_ = validateMetricsConfig
	_ = validateTelemetryConfig
	_ = validateJobsConfig
	_ = validateStatusConfig
	_ = validatePushConfig
	_ = validateBackupConfig
	_ = validateAIConfig
	_ = validateRealtimeConfig
}

func TestValidateJobsConfigJobRunsRetentionDays(t *testing.T) {
	cfg := Default()
	cfg.Jobs.Enabled = true
	cfg.Jobs.JobRunsRetentionDays = 0

	err := validateJobsConfig(cfg)
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "jobs.job_runs_retention_days must be positive")
}
