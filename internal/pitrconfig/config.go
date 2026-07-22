package pitrconfig

import "fmt"

// Config contains the shared PITR validation contract used by config loading
// and backup runtime wiring.
type Config struct {
	Enabled                  bool
	ArchiveBucket            string
	ArchivePrefix            string
	WALRetentionDays         int
	BaseBackupRetentionDays  int
	ComplianceSnapshotMonths int
	EnvironmentClass         string
	KMSKeyID                 string
	RetentionSchedule        string
	RPOMinutes               int
	StorageBudgetBytes       int64
	ShadowMode               bool
	BaseBackupSchedule       string
	VerifySchedule           string
}

// Validate checks that PITR config is usable. Disabled configs always pass.
func (p *Config) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.ArchiveBucket == "" {
		return fmt.Errorf("pitr: archive_bucket is required when enabled")
	}
	if p.RPOMinutes <= 0 {
		return fmt.Errorf("pitr: rpo_minutes must be > 0")
	}
	if p.WALRetentionDays < 1 {
		return fmt.Errorf("pitr: wal_retention_days must be >= 1")
	}
	if p.RetentionSchedule == "" {
		return fmt.Errorf("pitr: retention_schedule is required when enabled")
	}
	if p.BaseBackupRetentionDays < 1 {
		return fmt.Errorf("pitr: base_backup_retention_days must be >= 1")
	}
	if p.ComplianceSnapshotMonths < 0 {
		return fmt.Errorf("pitr: compliance_snapshot_months must be >= 0")
	}
	if p.StorageBudgetBytes < 0 {
		return fmt.Errorf("pitr: storage_budget_bytes must be >= 0")
	}
	if p.EnvironmentClass == "" {
		p.EnvironmentClass = "non-prod"
	}
	return nil
}
