// Package cli builds the managed-Postgres WAL archive_command wired at startup.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allyourbase/ayb/internal/config"
)

// managedPostgresArchiveCommand returns the archive_command for managed
// PostgreSQL, or "" when backups or PITR are disabled. An unusable PITR
// configuration is an error rather than a silently disabled archive.
func managedPostgresArchiveCommand(cfg *config.Config, effectiveConfigPath string) (string, error) {
	if !cfg.Backup.Enabled || !cfg.Backup.PITR.Enabled {
		return "", nil
	}
	backupCfg := backupConfigFromRuntimeConfig(cfg)
	if err := backupCfg.PITR.Validate(); err != nil {
		return "", err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving executable for WAL archive command: %w", err)
	}
	return BuildManagedPostgresArchiveCommand(executablePath, effectiveConfigPath)
}

// BuildManagedPostgresArchiveCommand constructs the archive command shared by
// managed startup and integration harnesses that supply an explicit AYB binary.
func BuildManagedPostgresArchiveCommand(executablePath, effectiveConfigPath string) (string, error) {
	absConfigPath, err := filepath.Abs(effectiveConfigPath)
	if err != nil {
		return "", fmt.Errorf("resolving config path for WAL archive command: %w", err)
	}
	return shellSingleQuote(executablePath) +
		" wal-ship --config " + shellSingleQuote(absConfigPath) +
		" " + shellSingleQuote("%p") +
		" " + shellSingleQuote("%f"), nil
}
