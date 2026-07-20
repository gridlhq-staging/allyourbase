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

	cmd := newChildCommand([]string{"go", "test"}, "postgres://example", "/tmp/pgbin/bin")

	env := map[string]string{}
	for _, entry := range cmd.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	testutil.Equal(t, "postgres://example", env["TEST_DATABASE_URL"])
	testutil.Equal(t, "/tmp/pgbin/bin", env["TEST_PG_BIN_DIR"])
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
