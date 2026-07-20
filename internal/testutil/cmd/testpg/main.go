// testpg starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL,
// runs the given command (typically `go test ...`), then stops Postgres.
// This lets integration tests run without Docker or a local Postgres install.
//
// Usage: go run ./internal/testutil/cmd/testpg -- go test -tags=integration -count=1 ./...
// Package main testpg starts AYB's managed Postgres on a free port and runs a given command with TEST_DATABASE_URL set, allowing integration tests to run without Docker or a local Postgres install.
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
	"syscall"

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

	port, err := testutil.FreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: finding free port: %v\n", err)
		return 1
	}

	tempRoot, err := createTempRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: create temp postgres root: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tempRoot)

	pgLogFile, logWriter, err := openPostgresLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: create log file: %v\n", err)
		return 1
	}
	defer os.Remove(pgLogFile.Name())
	defer pgLogFile.Close()

	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))

	binRoot, err := persistentPGBinRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: resolve managed postgres binary root: %v\n", err)
		return 1
	}

	cfg := newTestPGConfig(tempRoot, binRoot, port, logger)
	mgr := pgmanager.New(cfg)

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "testpg: starting managed postgres on port %d (logs: %s)\n", port, pgLogFile.Name())
	connURL, err := startManagedPostgres(ctx, mgr, binRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start postgres: %v\n", err)
		return 1
	}
	testDBURL, err := replaceDatabaseInConnURL(connURL, "postgres")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: build TEST_DATABASE_URL: %v\n", err)
		return 1
	}

	cleanup := func() {
		fmt.Fprintln(os.Stderr, "testpg: stopping managed postgres")
		_ = mgr.Stop()
	}
	defer cleanup()

	// Trap signals so postgres is stopped on Ctrl+C / SIGTERM instead of
	// being orphaned. A second signal force-exits immediately.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "testpg: TEST_DATABASE_URL=%s\n", testDBURL)

	cmd := newChildCommand(args, testDBURL, filepath.Join(cfg.BinDir, "bin"))
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
		return 1
	}

	// Wait for either the child to finish or a signal to arrive.
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	return waitForChildOrSignal(cmd, waitCh, sigCh, cleanup)
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

func newChildCommand(args []string, connURL, pgBinDir string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "TEST_DATABASE_URL="+connURL, "TEST_PG_BIN_DIR="+pgBinDir)
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
