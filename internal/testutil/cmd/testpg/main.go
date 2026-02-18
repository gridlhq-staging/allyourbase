// testpg starts AYB's managed Postgres on a free port, sets TEST_DATABASE_URL,
// runs the given command (typically `go test ...`), then stops Postgres.
// This lets integration tests run without Docker or a local Postgres install.
//
// Usage: go run ./internal/testutil/cmd/testpg -- go test -tags=integration -count=1 ./...
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: testpg [--] <command> [args...]")
		os.Exit(1)
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: finding free port: %v\n", err)
		os.Exit(1)
	}

	// Reuse AYB's binary cache so we don't re-download on every run.
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: home dir: %v\n", err)
		os.Exit(1)
	}
	cacheDir := filepath.Join(home, ".ayb", "pg")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir cache: %v\n", err)
		os.Exit(1)
	}

	// Use a temp data dir — test data is throwaway.
	dataDir, err := os.MkdirTemp("", "ayb-test-pg-data-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir data: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dataDir)

	runtimeDir, err := os.MkdirTemp("", "ayb-test-pg-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testpg: mkdir runtime: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(runtimeDir)

	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(port)).
		DataPath(dataDir).
		RuntimePath(runtimeDir).
		CachePath(cacheDir).
		Version(embeddedpostgres.V16).
		Username("test").
		Password("test").
		Database("postgres"))

	fmt.Fprintf(os.Stderr, "testpg: starting managed postgres on port %d\n", port)
	if err := db.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testpg: start postgres: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Fprintln(os.Stderr, "testpg: stopping managed postgres")
		_ = db.Stop()
	}()

	url := fmt.Sprintf("postgresql://test:test@127.0.0.1:%d/postgres?sslmode=disable", port)
	fmt.Fprintf(os.Stderr, "testpg: TEST_DATABASE_URL=%s\n", url)

	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "TEST_DATABASE_URL="+url)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "testpg: %v\n", err)
		os.Exit(1)
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
