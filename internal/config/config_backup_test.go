package config

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateBackupPITRRequiresUsableArchiveConfig(t *testing.T) {
	cfg := Default()
	cfg.Backup.Enabled = true
	cfg.Backup.Bucket = "logical-backups"
	cfg.Backup.Region = "us-east-1"
	cfg.Backup.AccessKey = "access-key"
	cfg.Backup.SecretKey = "secret-key"
	cfg.Backup.PITR.Enabled = true
	cfg.Backup.PITR.ArchiveBucket = ""

	err := cfg.Validate()
	testutil.ErrorContains(t, err, "pitr: archive_bucket is required when enabled")
}
