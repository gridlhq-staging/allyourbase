// testpg starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL,
// runs the given command (typically `go test ...`), then stops Postgres.
// This lets integration tests run without Docker or a local Postgres install.
//
// Usage: go run ./internal/testutil/cmd/testpg -- go test -tags=integration -count=1 ./...
// Package main testpg starts AYB's managed Postgres on a free port and runs a given command with TEST_DATABASE_URL set, allowing integration tests to run without Docker or a local Postgres install.
// Package main Stub summary for internal/testutil/cmd/testpg/main.go.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/pgmanager"
	"github.com/allyourbase/ayb/internal/testutil"
)

func main() {
	os.Exit(run())
}

// run starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL, executes the given command with that environment, and gracefully stops Postgres on command completion or signal. It returns the command's exit code, or a non-zero code if startup fails.
func run() int {
	args, ok := parseChildArgs(os.Args[1:])
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: testpg [--] <command> [args...]")
		return 1
	}

	ctx := context.Background()
	runtime, err := startTestPGRuntime(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
		return 1
	}
	defer runtime.cleanup()

	// Trap signals so postgres is stopped on Ctrl+C / SIGTERM instead of
	// being orphaned. A second signal force-exits immediately.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "testpg: TEST_DATABASE_URL=%s\n", runtime.databaseURL)

	cmd := newChildCommand(args, childCommandEnvironment{
		databaseURL:      runtime.databaseURL,
		pgBinDir:         filepath.Join(runtime.cfg.BinDir, "bin"),
		backupConfigPath: runtime.archive.configPath,
		projectID:        testPGArchiveProjectID,
	})
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
		return 1
	}

	// Wait for either the child to finish or a signal to arrive.
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	return waitForChildOrSignal(cmd, waitCh, sigCh, runtime.cleanup)
}

type testPGRuntime struct {
	cfg         pgmanager.Config
	archive     *archiveRuntime
	databaseURL string
	cleanup     func()
}

func startTestPGRuntime(ctx context.Context) (*testPGRuntime, error) {
	port, err := testutil.FreePort()
	if err != nil {
		return nil, fmt.Errorf("finding free port: %w", err)
	}

	tempRoot, err := createTempRoot()
	if err != nil {
		return nil, fmt.Errorf("create temp postgres root: %w", err)
	}
	pgLogFile, logWriter, err := openPostgresLog()
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, fmt.Errorf("create log file: %w", err)
	}

	runtime, err := openTestPGRuntime(ctx, tempRoot, port, pgLogFile, logWriter)
	if err != nil {
		_ = pgLogFile.Close()
		_ = os.Remove(pgLogFile.Name())
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}
	return runtime, nil
}

func openTestPGRuntime(
	ctx context.Context,
	tempRoot string,
	port int,
	pgLogFile *os.File,
	logWriter io.Writer,
) (*testPGRuntime, error) {
	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))
	binRoot, err := persistentPGBinRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve managed postgres binary root: %w", err)
	}
	archive, err := prepareArchiveRuntime(ctx, tempRoot, port)
	if err != nil {
		return nil, fmt.Errorf("prepare WAL archive runtime: %w", err)
	}
	restoreProjectEnvironment, err := setArchiveProjectEnvironment()
	if err != nil {
		closeArchive(archive)
		return nil, fmt.Errorf("set archive project environment: %w", err)
	}

	cfg := newTestPGConfig(tempRoot, binRoot, port, logger)
	cfg.ArchiveCommand = archive.command
	mgr := pgmanager.New(cfg)
	fmt.Fprintf(os.Stderr, "testpg: starting managed postgres on port %d (logs: %s)\n", port, pgLogFile.Name())
	connURL, err := startManagedPostgres(ctx, mgr, binRoot)
	if err != nil {
		restoreProjectEnvironment()
		closeArchive(archive)
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	databaseURL, err := replaceDatabaseInConnURL(connURL, "postgres")
	if err != nil {
		restoreProjectEnvironment()
		_ = mgr.Stop()
		closeArchive(archive)
		return nil, fmt.Errorf("build TEST_DATABASE_URL: %w", err)
	}

	return &testPGRuntime{
		cfg:         cfg,
		archive:     archive,
		databaseURL: databaseURL,
		cleanup:     testPGCleanup(tempRoot, pgLogFile, mgr, archive, restoreProjectEnvironment),
	}, nil
}

func testPGCleanup(
	tempRoot string,
	pgLogFile *os.File,
	mgr *pgmanager.Manager,
	archive *archiveRuntime,
	restoreProjectEnvironment func(),
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			fmt.Fprintln(os.Stderr, "testpg: stopping managed postgres")
			_ = mgr.Stop()
			closeArchive(archive)
			restoreProjectEnvironment()
			_ = os.Remove(pgLogFile.Name())
			_ = pgLogFile.Close()
			_ = os.RemoveAll(tempRoot)
		})
	}
}

func closeArchive(archive *archiveRuntime) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := archive.close(cleanupCtx); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: stop WAL archive runtime: %v\n", err)
	}
}

func parseChildArgs(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	return args, len(args) >= 1
}

func waitForChildOrSignal(cmd *exec.Cmd, waitCh <-chan error, sigCh chan os.Signal, cleanup func()) int {
	select {
	case err := <-waitCh:
		return childExitCode(err)
	case sig := <-sigCh:
		return handleChildSignal(cmd, waitCh, sigCh, cleanup, sig)
	}
}

func childExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
	return 1
}

func handleChildSignal(cmd *exec.Cmd, waitCh <-chan error, sigCh chan os.Signal, cleanup func(), sig os.Signal) int {
	fmt.Fprintf(os.Stderr, "\ntestpg: received %s, shutting down\n", sig)
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
	}
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "testpg: forced exit")
		cleanup()
		os.Exit(1)
	}()
	<-waitCh
	return 128 + int(sig.(syscall.Signal))
}

func createTempRoot() (string, error) {
	return os.MkdirTemp("", "ayb-testpg-*")
}

func openPostgresLog() (*os.File, io.Writer, error) {
	pgLogFile, err := os.CreateTemp("", "ayb-test-pg-log-*.log")
	if err != nil {
		return nil, nil, err
	}

	logWriter := io.Writer(pgLogFile)
	if os.Getenv("TESTPG_VERBOSE") != "" {
		logWriter = io.MultiWriter(pgLogFile, os.Stderr)
	}
	return pgLogFile, logWriter, nil
}

func startManagedPostgres(ctx context.Context, mgr *pgmanager.Manager, binRoot string) (string, error) {
	var connURL string
	err := withSharedPGBinaryLock(binRoot, func() error {
		var startErr error
		connURL, startErr = mgr.Start(ctx)
		return startErr
	})
	if err != nil {
		return "", err
	}
	return connURL, nil
}

type childCommandEnvironment struct {
	databaseURL      string
	pgBinDir         string
	backupConfigPath string
	projectID        string
}

func newChildCommand(args []string, environment childCommandEnvironment) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"TEST_DATABASE_URL="+environment.databaseURL,
		"TEST_PG_BIN_DIR="+environment.pgBinDir,
		testutil.BackupConfigPathEnv+"="+environment.backupConfigPath,
		testutil.ArchiveProjectIDEnv+"="+environment.projectID,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// persistentPGBinRoot returns the directory that holds the downloaded archive
// cache and the extracted managed-Postgres binaries. It deliberately lives
// outside the per-invocation temp root so repeated integration runs reuse one
// download instead of re-fetching and re-extracting a Postgres distribution
// every time. TESTPG_PG_HOME overrides it for sandboxes without a writable
// home directory; the default mirrors pgmanager's own ~/.ayb layout.
func persistentPGBinRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TESTPG_PG_HOME")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ayb"), nil
}

// withSharedPGBinaryLock serializes setup of the shared managed-Postgres
// binary tree so concurrent testpg processes cannot delete and replace one
// another's reused install during first download or PG-version changes.
func withSharedPGBinaryLock(binRoot string, fn func() error) error {
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		return fmt.Errorf("create shared managed postgres root: %w", err)
	}

	lockFile, err := os.OpenFile(filepath.Join(binRoot, ".testpg-pgbin.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open shared managed postgres lock: %w", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock shared managed postgres binaries: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// newTestPGConfig keeps mutable per-run state (data dir, runtime dir, PID file)
// under the disposable root while pinning the binary cache and extracted
// binaries to the shared, persistent binRoot.
func newTestPGConfig(root, binRoot string, port int, logger *slog.Logger) pgmanager.Config {
	return pgmanager.Config{
		BaseDir:     root,
		Port:        uint32(port),
		DataDir:     filepath.Join(root, "data"),
		RuntimeDir:  filepath.Join(root, "run"),
		PIDFile:     filepath.Join(root, "pg.pid"),
		BinCacheDir: filepath.Join(binRoot, "pg"),
		BinDir:      filepath.Join(binRoot, "pgbin"),
		Logger:      logger,
	}
}

func replaceDatabaseInConnURL(connURL, databaseName string) (string, error) {
	parsedURL, err := url.Parse(connURL)
	if err != nil {
		return "", err
	}
	parsedURL.Path = "/" + databaseName
	return parsedURL.String(), nil
}
