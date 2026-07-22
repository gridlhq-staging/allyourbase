package testutil

const (
	// BackupConfigPathEnv exposes a disposable effective config to integration
	// tests and the real wal-ship subprocess invoked by managed PostgreSQL.
	BackupConfigPathEnv = "TEST_BACKUP_CONFIG_PATH"
	// ArchiveProjectIDEnv is inherited by PostgreSQL's archive_command process.
	ArchiveProjectIDEnv = "AYB_PROJECT_ID"
)
