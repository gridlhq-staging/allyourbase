//go:build integration

package pgmanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/ulikunitz/xz"
)

// testPGHarness creates a minimal PG binary tarball from the locally installed
// postgres, serves it via httptest.Server with a SHA256SUMS endpoint, and
// returns a pgmanager.Config pointing at the test server.
type testPGHarness struct {
	server  *httptest.Server
	config  Config
	tempDir string
}

const managedPGStartTimeoutEnvVar = "AYB_PGMANAGER_TEST_START_TIMEOUT"

func managedPGStartTimeout() time.Duration {
	timeoutRaw := strings.TrimSpace(os.Getenv(managedPGStartTimeoutEnvVar))
	if timeoutRaw == "" {
		return 5 * time.Minute
	}

	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil || timeout <= 0 {
		return 5 * time.Minute
	}

	return timeout
}

func newTestHarnessServer(archive []byte, sums string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			w.Write([]byte(sums))
		default:
			w.Write(archive)
		}
	}))
}

// testPGVersion is the PostgreSQL major version the harness packages and serves.
const testPGVersion = "16"

// pgFixture memoizes the shared PostgreSQL test fixture for this package.
//
// The archive is built from the local Postgres install and is byte-identical
// for every test, and the extracted binaries are immutable and read-only — so
// the two most expensive steps of harness setup (xz compression, ~12s under
// -race, and the per-test download+extract) run exactly once for the whole
// package rather than once per test. Without this, the managed-PG integration
// tests collectively overrun Go's default 10-minute per-package test timeout
// in CI.
//
// A shared read-only binary directory is safe here because the integration
// runner executes tests sequentially (`go test -p 1`) and none of these tests
// call t.Parallel. Each test still gets an isolated data dir, runtime/socket
// dir, and port, so per-test state stays fully isolated.
var pgFixture struct {
	once    sync.Once
	archive []byte // tar.xz bytes (still served so the download path is exercised)
	sums    string // SHA256SUMS body matching archive
	binDir  string // shared, pre-extracted, read-only PG binaries
	rootDir string // parent temp dir holding binDir + archive (removed in TestMain)
	skip    string // non-empty => pg_config/platform missing; every test skips
}

// ensurePGFixture builds the shared PG fixture once. It skips the calling test
// when Postgres is unavailable, and fails it when the fixture build itself
// failed (which is reported in detail by the first integration test to run).
func ensurePGFixture(t *testing.T) {
	t.Helper()
	pgFixture.once.Do(func() {
		pgConfig, err := exec.LookPath("pg_config")
		if err != nil {
			pgFixture.skip = "pg_config not found — install PostgreSQL to run integration tests"
			return
		}
		platform, err := platformKey()
		if err != nil {
			pgFixture.skip = fmt.Sprintf("unsupported test platform: %v", err)
			return
		}

		binDir := mustPGConfigDir(t, pgConfig, "--bindir")
		libDir := mustPGConfigDir(t, pgConfig, "--pkglibdir")
		shareDir := mustPGConfigDir(t, pgConfig, "--sharedir")

		archive := buildTestArchive(t, binDir, libDir, shareDir)
		hash := fmt.Sprintf("%x", sha256.Sum256(archive))
		archiveName := fmt.Sprintf("ayb-postgres-%s-%s.tar.xz", testPGVersion, platform)

		// Extract the binaries once into a shared, read-only directory. Every
		// test's Manager.Start then cache-hits via binariesReady() and skips
		// the redundant per-test download+extract. The download path itself
		// still has dedicated coverage in download_test.go / extract_test.go.
		rootDir, err := os.MkdirTemp("", "aybpgfix")
		if err != nil {
			t.Fatalf("creating PG fixture dir: %v", err)
		}
		archivePath := filepath.Join(rootDir, archiveName)
		if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
			t.Fatalf("writing PG fixture archive: %v", err)
		}
		sharedBinDir := filepath.Join(rootDir, "bin-extracted")
		if err := extractTarXZ(archivePath, sharedBinDir); err != nil {
			t.Fatalf("extracting PG fixture archive: %v", err)
		}

		pgFixture.archive = archive
		pgFixture.sums = fmt.Sprintf("%s  %s\n", hash, archiveName)
		pgFixture.binDir = sharedBinDir
		pgFixture.rootDir = rootDir
	})
	if pgFixture.skip != "" {
		t.Skip(pgFixture.skip)
	}
	if pgFixture.binDir == "" {
		t.Fatal("PG test fixture failed to build — see the first integration test failure in this package")
	}
}

// TestMain runs the integration suite and removes the shared PG fixture dir.
func TestMain(m *testing.M) {
	code := m.Run()
	if pgFixture.rootDir != "" {
		_ = os.RemoveAll(pgFixture.rootDir)
	}
	os.Exit(code)
}

// mustPGConfigDir runs `pg_config <flag>` and returns the trimmed directory path.
func mustPGConfigDir(t *testing.T, pgConfig, flag string) string {
	t.Helper()
	out, err := exec.Command(pgConfig, flag).Output()
	if err != nil {
		t.Fatalf("pg_config %s failed: %v", flag, err)
	}
	return trimNL(string(out))
}

// newTestPGHarness serves the (memoized) system PG binary fixture over HTTP and
// returns a per-test configured harness with isolated data/runtime/port that
// reuses the package-shared, pre-extracted binary directory.
func newTestPGHarness(t *testing.T) *testPGHarness {
	t.Helper()

	ensurePGFixture(t)

	srv := newTestHarnessServer(pgFixture.archive, pgFixture.sums)

	tempDir := t.TempDir()

	cfg := Config{
		Port:                   findFreePort(t),
		DataDir:                filepath.Join(tempDir, "data"),
		RuntimeDir:             shortSocketDir(t),
		BinCacheDir:            filepath.Join(tempDir, "cache"),
		BinDir:                 pgFixture.binDir, // shared, pre-extracted, read-only
		BinaryURL:              srv.URL + "/{version}/{platform}.tar.xz",
		PGVersion:              testPGVersion,
		Extensions:             nil, // extensions tested separately
		SharedPreloadLibraries: nil,
		Logger:                 testutil.DiscardLogger(),
	}

	t.Cleanup(func() {
		srv.Close()
	})

	return &testPGHarness{
		server:  srv,
		config:  cfg,
		tempDir: tempDir,
	}
}

// buildTestArchive creates a .tar.xz archive from system PG binaries.
func buildTestArchive(t *testing.T, binDir, libDir, shareDir string) []byte {
	t.Helper()

	var buf bytes.Buffer
	xzw, err := xz.NewWriter(&buf)
	testutil.NoError(t, err)
	tw := tar.NewWriter(xzw)

	prefix := "ayb-postgres-16/"

	// Add essential binaries.
	for _, name := range []string{"postgres", "initdb", "pg_ctl", "pg_isready"} {
		addFileToTar(t, tw, filepath.Join(binDir, name), prefix+"bin/"+name)
	}

	// Add lib directory (shared libraries).
	addDirToTar(t, tw, libDir, prefix+"lib/")

	// Add share directory (timezone data, SQL files, etc.).
	addDirToTar(t, tw, shareDir, prefix+"share/")

	// Add PG_VERSION.
	testutil.NoError(t, tw.WriteHeader(&tar.Header{
		Name: prefix + "PG_VERSION",
		Mode: 0o644,
		Size: 3,
	}))
	tw.Write([]byte("16\n"))

	testutil.NoError(t, tw.Close())
	testutil.NoError(t, xzw.Close())
	return buf.Bytes()
}

// addFileToTar adds a single file to the tar archive.
func addFileToTar(t *testing.T, tw *tar.Writer, src, name string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		t.Logf("skipping %s: %v", src, err)
		return
	}
	hdr, err := tar.FileInfoHeader(info, "")
	testutil.NoError(t, err)
	hdr.Name = name

	testutil.NoError(t, tw.WriteHeader(hdr))
	f, err := os.Open(src)
	testutil.NoError(t, err)
	defer f.Close()
	io.Copy(tw, f)
}

// addDirToTar recursively adds a directory to the tar archive.
func addDirToTar(t *testing.T, tw *tar.Writer, dir, prefix string) {
	t.Helper()
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		rel, _ := filepath.Rel(dir, path)
		name := prefix + rel

		if info.IsDir() {
			tw.WriteHeader(&tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
			})
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil // skip symlinks etc for simplicity
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		hdr.Name = name
		tw.WriteHeader(hdr)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		io.Copy(tw, f)
		return nil
	})
}

// findFreePort finds an available TCP port by binding to :0 and reading the
// assigned port. The port is released before returning, so there is a small
// TOCTOU window, but this is acceptable for test use.
func findFreePort(t *testing.T) uint32 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return uint32(port)
}

// shortSocketDir returns a short-path directory for Postgres's Unix socket.
// Postgres places its socket at <RuntimeDir>/.s.PGSQL.<port>, and the OS caps
// the socket path at sizeof(sockaddr_un.sun_path) — 104 bytes on macOS, 108 on
// Linux. A t.TempDir() path embeds the test name plus a "/NNN" subdir, and on
// macOS (where the temp root is a long /var/folders/... path) that overflows
// the limit for longer-named tests — Postgres then fails to create the socket
// and pg_ctl exits non-zero. Allocating the socket dir via os.MkdirTemp with a
// short prefix keeps the full socket path comfortably under the limit while
// still being unique per test and cleaned up afterward.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "aybpg")
	testutil.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestManagedPGStartTimeoutDefault(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutFromEnv(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "7m30s")
	testutil.Equal(t, 7*time.Minute+30*time.Second, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutInvalidEnvFallsBack(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "invalid")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestManagedPGStartTimeoutNonPositiveFallsBack(t *testing.T) {
	t.Setenv(managedPGStartTimeoutEnvVar, "0s")
	testutil.Equal(t, 5*time.Minute, managedPGStartTimeout())
}

func TestHarnessServesVersionedSHA256SumsPath(t *testing.T) {
	t.Parallel()

	archive := []byte("fake-archive-bytes")
	sums := "abc123  ayb-postgres-16-darwin-arm64.tar.xz\n"
	srv := newTestHarnessServer(archive, sums)
	t.Cleanup(srv.Close)

	url := sha256SumsURL(srv.URL+"/{version}/{platform}.tar.xz", "16")
	resp, err := http.Get(url) //nolint:noctx
	testutil.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	testutil.NoError(t, err)
	testutil.Equal(t, sums, string(body))
}

// --- Integration Tests ---

func TestFullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)
	mgr := New(harness.config)

	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	// Start.
	connURL, err := mgr.Start(ctx)
	testutil.NoError(t, err)
	testutil.True(t, connURL != "", "expected non-empty connection URL")
	testutil.True(t, mgr.IsRunning(), "manager should be running")

	// SELECT 1.
	db, err := sql.Open("pgx", connURL)
	testutil.NoError(t, err)
	defer db.Close()

	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)

	// Create a test table and insert data.
	_, err = db.ExecContext(ctx, "CREATE TABLE test_persist (id serial PRIMARY KEY, val text)")
	testutil.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO test_persist (val) VALUES ('hello')")
	testutil.NoError(t, err)
	db.Close()

	// Stop.
	err = mgr.Stop()
	testutil.NoError(t, err)
	testutil.False(t, mgr.IsRunning(), "manager should not be running after stop")

	// Restart with existing data.
	mgr2 := New(harness.config)
	connURL2, err := mgr2.Start(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, connURL, connURL2)

	db2, err := sql.Open("pgx", connURL2)
	testutil.NoError(t, err)
	defer db2.Close()

	// Verify data persisted.
	var val string
	err = db2.QueryRowContext(ctx, "SELECT val FROM test_persist WHERE id = 1").Scan(&val)
	testutil.NoError(t, err)
	testutil.Equal(t, "hello", val)

	// Final stop.
	err = mgr2.Stop()
	testutil.NoError(t, err)
}

func TestExtensionInit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)
	harness.config.Extensions = []string{"pgvector"}
	harness.config.Port = findFreePort(t)
	harness.config.DataDir = filepath.Join(harness.tempDir, "data-ext")

	mgr := New(harness.config)
	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	connURL, err := mgr.Start(ctx)
	testutil.NoError(t, err)
	defer mgr.Stop()

	db, err := sql.Open("pgx", connURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify pgvector type exists.
	var typname string
	err = db.QueryRowContext(ctx, "SELECT typname FROM pg_type WHERE typname = 'vector'").Scan(&typname)
	if err != nil {
		t.Logf("pgvector not available on this system — this is expected if pgvector is not installed: %v", err)
		t.Skip("pgvector extension not available")
	}
	testutil.Equal(t, "vector", typname)
}

func TestPortAlreadyInUse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)

	// Occupy the port with a raw TCP listener instead of a second Postgres.
	// Using a real Postgres to hold the port is unreliable: pg_ctl -w checks
	// readiness by connecting to the port, and on Linux it can reach the
	// first Postgres and falsely report the second instance as started.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", harness.config.Port))
	testutil.NoError(t, err)
	defer ln.Close()

	mgr := New(harness.config)
	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	_, err = mgr.Start(ctx)
	if err == nil {
		mgr.Stop()
		t.Fatal("expected error when starting on an already-used port, got nil")
	}
	// The error wraps pg_ctl or postgres bind failure.
	testutil.Contains(t, err.Error(), "managed postgres")
}

func TestManagedPGOutageRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	harness := newTestPGHarness(t)
	mgr := New(harness.config)

	ctx, cancel := context.WithTimeout(context.Background(), managedPGStartTimeout())
	defer cancel()

	connURL, err := mgr.Start(ctx)
	testutil.NoError(t, err)
	testutil.True(t, connURL != "", "expected non-empty connection URL")

	db, err := sql.Open("pgx", connURL)
	testutil.NoError(t, err)
	defer db.Close()

	// Phase 1: healthy instance should answer queries.
	var healthyResult int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&healthyResult)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, healthyResult)

	// Phase 2: same DB handle should fail while managed Postgres is stopped.
	err = mgr.Stop()
	testutil.NoError(t, err)

	var outageResult int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&outageResult)
	testutil.True(t, err != nil, "expected query error while managed Postgres is stopped")

	// Phase 3: restart managed Postgres and verify the same DB handle recovers.
	mgr2 := New(harness.config)
	defer mgr2.Stop()

	connURL2, err := mgr2.Start(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, connURL, connURL2)

	var recoveredResult int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&recoveredResult)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, recoveredResult)
}
