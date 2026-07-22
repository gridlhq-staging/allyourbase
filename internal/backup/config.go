// Package backup This file defines backup configuration structures for full backups and point-in-time recovery, including Config for general backup settings and PITRConfig for WAL archiving.
package backup

import (
	"fmt"

	"github.com/allyourbase/ayb/internal/pitrconfig"
)

// Config mirrors config.BackupConfig for the backup package.
// The wiring layer (start.go) converts config.BackupConfig → backup.Config.
type Config struct {
	Enabled        bool
	Bucket         string
	Region         string
	Prefix         string
	Schedule       string
	RetentionCount int
	RetentionDays  int
	Encryption     string // "" | "AES256" | "aws:kms"
	Endpoint       string // custom endpoint for LocalStack / MinIO
	AccessKey      string
	SecretKey      string
	PITR           PITRConfig
}

// PITRConfig configures point-in-time recovery and the WAL archival settings AYB
// expects operators or future managed-Postgres wiring to supply.
// When Enabled is true, AYB takes physical base backups and exposes the WAL
// archiving/shadow-mode configuration used by the PITR pipeline.
// ShadowMode (default true) refuses restore cutover requests; base backups still
// run, and WAL shipping continues only if Postgres is configured to invoke the
// archive command.
type PITRConfig struct {
	Enabled                  bool
	ArchiveBucket            string
	ArchivePrefix            string // optional namespace prefix; empty means paths start with projects/
	WALRetentionDays         int    // default 14
	BaseBackupRetentionDays  int    // default 35
	ComplianceSnapshotMonths int    // default 12
	EnvironmentClass         string // e.g. "prod", "staging"
	KMSKeyID                 string
	RetentionSchedule        string // cron expression, default "0 4 * * *" (daily 4 AM)
	RPOMinutes               int    // default 5; must be > 0
	StorageBudgetBytes       int64  // default 0 (unlimited)
	ShadowMode               bool   // default true
	BaseBackupSchedule       string // cron expression, default "0 3 * * *"
	VerifySchedule           string // cron expression, default "0 */6 * * *" (every 6 hours)
}

// DefaultPITR returns a PITRConfig with sensible defaults.
func DefaultPITR() PITRConfig {
	return PITRConfig{
		WALRetentionDays:         14,
		BaseBackupRetentionDays:  35,
		ComplianceSnapshotMonths: 12,
		RetentionSchedule:        "0 4 * * *",
		RPOMinutes:               5,
		StorageBudgetBytes:       0,
		ShadowMode:               true,
		BaseBackupSchedule:       "0 3 * * *",
		VerifySchedule:           "0 */6 * * *",
	}
}

// Validate checks that PITRConfig is usable. Disabled configs always pass.
func (p *PITRConfig) Validate() error {
	shared := p.sharedConfig()
	if err := shared.Validate(); err != nil {
		return err
	}
	p.EnvironmentClass = shared.EnvironmentClass
	return nil
}

func (p *PITRConfig) sharedConfig() pitrconfig.Config {
	return pitrconfig.Config{
		Enabled:                  p.Enabled,
		ArchiveBucket:            p.ArchiveBucket,
		ArchivePrefix:            p.ArchivePrefix,
		WALRetentionDays:         p.WALRetentionDays,
		BaseBackupRetentionDays:  p.BaseBackupRetentionDays,
		ComplianceSnapshotMonths: p.ComplianceSnapshotMonths,
		EnvironmentClass:         p.EnvironmentClass,
		KMSKeyID:                 p.KMSKeyID,
		RetentionSchedule:        p.RetentionSchedule,
		RPOMinutes:               p.RPOMinutes,
		StorageBudgetBytes:       p.StorageBudgetBytes,
		ShadowMode:               p.ShadowMode,
		BaseBackupSchedule:       p.BaseBackupSchedule,
		VerifySchedule:           p.VerifySchedule,
	}
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Region:   "us-east-1",
		Prefix:   "backups",
		Schedule: "0 2 * * *", // daily at 2 AM UTC
		PITR:     DefaultPITR(),
	}
}

// Validate checks that a Config is usable. Disabled configs always pass.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Bucket == "" {
		return fmt.Errorf("backup: bucket is required when enabled")
	}
	if c.Region == "" {
		return fmt.Errorf("backup: region is required when enabled")
	}
	if c.RetentionCount < 0 {
		return fmt.Errorf("backup: retention_count must be >= 0")
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("backup: retention_days must be >= 0")
	}
	switch c.Encryption {
	case "", "AES256", "aws:kms":
		// valid
	default:
		return fmt.Errorf("backup: invalid encryption %q (must be \"\", \"AES256\", or \"aws:kms\")", c.Encryption)
	}
	if err := c.PITR.Validate(); err != nil {
		return err
	}
	return nil
}
