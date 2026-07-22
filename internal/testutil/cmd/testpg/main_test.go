package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewTestPGConfigUsesIsolatedState(t *testing.T) {
	root := t.TempDir()
	binRoot := t.TempDir()

	cfg := newTestPGConfig(root, binRoot, 15432, testutil.DiscardLogger())

	testutil.Equal(t, uint32(15432), cfg.Port)
	testutil.Equal(t, root, cfg.BaseDir)
	testutil.Equal(t, filepath.Join(root, "data"), cfg.DataDir)
	testutil.Equal(t, filepath.Join(root, "run"), cfg.RuntimeDir)
	testutil.Equal(t, filepath.Join(root, "pg.pid"), cfg.PIDFile)
}

// The binary cache and extracted binaries must not live under the disposable
// per-invocation root, or every integration run re-downloads Postgres.
func TestNewTestPGConfigReusesPersistentBinaries(t *testing.T) {
	root := t.TempDir()
	binRoot := t.TempDir()

	cfg := newTestPGConfig(root, binRoot, 15432, testutil.DiscardLogger())

	testutil.Equal(t, filepath.Join(binRoot, "pg"), cfg.BinCacheDir)
	testutil.Equal(t, filepath.Join(binRoot, "pgbin"), cfg.BinDir)
	if strings.HasPrefix(cfg.BinCacheDir, root+string(filepath.Separator)) {
		t.Fatalf("BinCacheDir %s is inside the disposable root %s", cfg.BinCacheDir, root)
	}
	if strings.HasPrefix(cfg.BinDir, root+string(filepath.Separator)) {
		t.Fatalf("BinDir %s is inside the disposable root %s", cfg.BinDir, root)
	}

	// Two invocations with different disposable roots must resolve the same
	// binary locations — that identity is what makes the download reusable.
	other := newTestPGConfig(t.TempDir(), binRoot, 15433, testutil.DiscardLogger())
	testutil.Equal(t, cfg.BinCacheDir, other.BinCacheDir)
	testutil.Equal(t, cfg.BinDir, other.BinDir)
}

func TestPersistentPGBinRootHonorsOverride(t *testing.T) {
	t.Setenv("TESTPG_PG_HOME", "/tmp/testpg-home")

	root, err := persistentPGBinRoot()

	testutil.NoError(t, err)
	testutil.Equal(t, "/tmp/testpg-home", root)
}

func TestPersistentPGBinRootDefaultsToAYBHome(t *testing.T) {
	t.Setenv("TESTPG_PG_HOME", "")

	root, err := persistentPGBinRoot()

	testutil.NoError(t, err)
	home, err := os.UserHomeDir()
	testutil.NoError(t, err)
	testutil.Equal(t, filepath.Join(home, ".ayb"), root)
}

func TestWithSharedPGBinaryLockSerializesCallers(t *testing.T) {
	binRoot := t.TempDir()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- withSharedPGBinaryLock(binRoot, func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- withSharedPGBinaryLock(binRoot, func() error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("second caller entered the shared managed Postgres binary section before the first released it")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	testutil.NoError(t, <-firstDone)
	testutil.NoError(t, <-secondDone)
	<-secondEntered
}

func TestNewChildCommandExposesManagedPostgresBinDir(t *testing.T) {
	t.Parallel()

	cmd := newChildCommand([]string{"go", "test"}, childCommandEnvironment{
		databaseURL:      "postgres://example",
		pgBinDir:         "/tmp/pgbin/bin",
		backupConfigPath: "/tmp/archive config/ayb.toml",
		projectID:        "project-roundtrip",
	})

	env := map[string]string{}
	for _, entry := range cmd.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	testutil.Equal(t, "postgres://example", env["TEST_DATABASE_URL"])
	testutil.Equal(t, "/tmp/pgbin/bin", env["TEST_PG_BIN_DIR"])
	testutil.Equal(t, "/tmp/archive config/ayb.toml", env["TEST_BACKUP_CONFIG_PATH"])
	testutil.Equal(t, "project-roundtrip", env["AYB_PROJECT_ID"])
}

func TestNewArchiveIdentityUsesPerRunState(t *testing.T) {
	t.Parallel()

	first := newArchiveIdentity("/tmp/ayb-testpg-run-one")
	second := newArchiveIdentity("/tmp/ayb-testpg-run-two")

	testutil.Equal(t, "ayb-testpg-run-one", first.containerName)
	testutil.Equal(t, "ayb-testpg-run-one", first.bucket)
	testutil.Equal(t, "roundtrip/ayb-testpg-run-one", first.archivePrefix)
	if first == second {
		t.Fatal("separate testpg roots resolved identical archive state")
	}
}

func TestNewArchiveRuntimeConfigUsesSharedDatabaseAndMinIO(t *testing.T) {
	t.Parallel()

	harness := &testutil.MinIOHarness{
		Endpoint:  "127.0.0.1:19000",
		Bucket:    "ayb-testpg-run-one",
		AccessKey: "access-key",
		SecretKey: "secret-key",
		Region:    "us-east-1",
		UseSSL:    false,
	}
	cfg := newArchiveRuntimeConfig(
		"postgresql://ayb:ayb@127.0.0.1:15432/postgres?sslmode=disable",
		harness,
		"roundtrip/ayb-testpg-run-one",
	)

	testutil.Equal(t, "postgresql://ayb:ayb@127.0.0.1:15432/postgres?sslmode=disable", cfg.Database.URL)
	testutil.Equal(t, true, cfg.Backup.Enabled)
	testutil.Equal(t, "http://127.0.0.1:19000", cfg.Backup.Endpoint)
	testutil.Equal(t, "ayb-testpg-run-one", cfg.Backup.Bucket)
	testutil.Equal(t, "ayb-testpg-run-one", cfg.Backup.PITR.ArchiveBucket)
	testutil.Equal(t, "roundtrip/ayb-testpg-run-one", cfg.Backup.PITR.ArchivePrefix)
	testutil.Equal(t, "access-key", cfg.Backup.AccessKey)
	testutil.Equal(t, "secret-key", cfg.Backup.SecretKey)
	testutil.Equal(t, false, cfg.Backup.UseSSL)
	testutil.Equal(t, "", cfg.Backup.Encryption)
	testutil.Equal(t, true, cfg.Backup.PITR.Enabled)
	testutil.Equal(t, false, cfg.Backup.PITR.ShadowMode)
}

func TestArchiveCommandForRuntimeQuotesPathsContainingSpaces(t *testing.T) {
	t.Setenv("TESTPG_ARCHIVE_EXECUTABLE", "")

	harness := &testutil.MinIOHarness{
		Endpoint:  "127.0.0.1:19000",
		Bucket:    "ayb-testpg-run-one",
		AccessKey: "access-key",
		SecretKey: "secret-key",
		Region:    "us-east-1",
	}
	cfg := newArchiveRuntimeConfig("postgres://example/postgres", harness, "roundtrip/run-one")
	binaryPath := filepath.Join("/tmp", "run dir", "ayb user's binary")
	configPath := filepath.Join("/tmp", "run dir", "archive user's config.toml")

	command, err := archiveCommandForRuntime(cfg, binaryPath, configPath)
	testutil.NoError(t, err)
	testutil.Equal(t,
		`'/tmp/run dir/ayb user'"'"'s binary' wal-ship --config '/tmp/run dir/archive user'"'"'s config.toml' '%p' '%f'`,
		command,
	)
}

func TestArchiveCommandForRuntimeHonorsTestOnlyExecutableOverride(t *testing.T) {
	t.Setenv("TESTPG_ARCHIVE_EXECUTABLE", "/opt/test ayb/ayb")

	harness := &testutil.MinIOHarness{
		Endpoint:  "127.0.0.1:19000",
		Bucket:    "ayb-testpg-run-one",
		AccessKey: "access-key",
		SecretKey: "secret-key",
		Region:    "us-east-1",
	}
	cfg := newArchiveRuntimeConfig("postgres://example/postgres", harness, "roundtrip/run-one")

	command, err := archiveCommandForRuntime(cfg, "/tmp/current-ayb", "/tmp/config.toml")
	testutil.NoError(t, err)
	testutil.Equal(t, `'/opt/test ayb/ayb' wal-ship --config '/tmp/config.toml' '%p' '%f'`, command)
}

func TestReplaceDatabaseInConnURL(t *testing.T) {
	t.Parallel()

	updatedURL, err := replaceDatabaseInConnURL(
		"postgresql://ayb:ayb@127.0.0.1:15432/ayb?sslmode=disable",
		"postgres",
	)
	testutil.NoError(t, err)
	testutil.Equal(t, "postgresql://ayb:ayb@127.0.0.1:15432/postgres?sslmode=disable", updatedURL)
}
