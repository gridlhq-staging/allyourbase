//go:build cell

// Package celltopology drives the Stage 1 compose cell (postgres + MinIO +
// ayb1/ayb2 + nginx round-robin LB) end to end. All client traffic enters
// through the LB; the driver proves cross-node realtime, session revocation,
// and storage isolation, and that requests actually spanned both AYB upstreams
// via the LB's X-AYB-Upstream attribution header.
package celltopology

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/e2e/crossnode"
	"github.com/allyourbase/ayb/internal/testutil"
)

// upstreamHeader is the attribution header nginx adds with $upstream_addr
// (IP:port of the AYB container that served the request).
const upstreamHeader = "X-AYB-Upstream"

// realtimeProofTable is the id/sentinel table the realtime fanout proof uses.
const realtimeProofTable = "cell_realtime_events"

// cell holds the discovered endpoints for a booted compose cell.
type cell struct {
	lbURL       string // http://127.0.0.1:<lbPort> — the nginx LB
	pgURL       string // host-side postgres URL for the realtime seed step
	projectName string
}

// bootCell builds and starts the compose cell, waits for the LB /health route,
// registers teardown that always runs, and returns the discovered endpoints.
func bootCell(t *testing.T) *cell {
	t.Helper()

	lbPort, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("allocate LB host port: %v", err)
	}
	projectName := fmt.Sprintf("aybcell-%d-%s", os.Getpid(), crossnode.RandomHex(t, 4))
	composeEnv := append(os.Environ(), fmt.Sprintf("AYB_CELL_LB_PORT=%d", lbPort))
	baseArgs := composeBaseArgs(t, projectName)

	// Always tear down, even if boot or a later proof fails.
	t.Cleanup(func() {
		downArgs := composeDownArgs(baseArgs)
		out, err := runCompose(context.Background(), composeEnv, 120*time.Second, downArgs...)
		if err != nil {
			t.Logf("compose down failed for %s: %v\n%s", projectName, err, out)
		}
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logArgs := append(append([]string{}, baseArgs...), "logs", "--no-color", "--tail", "200",
			"ayb1", "ayb2", "minio", "postgres")
		out, err := runCompose(context.Background(), composeEnv, 30*time.Second, logArgs...)
		t.Logf("compose service logs for failed project %s (error=%v):\n%s", projectName, err, out)
	})

	upCtx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	upArgs := append(append([]string{}, baseArgs...), "up", "-d", "--build", "--wait")
	if out, err := runCompose(upCtx, composeEnv, 8*time.Minute, upArgs...); err != nil {
		t.Fatalf("compose up failed for %s: %v\n%s", projectName, err, out)
	}

	lbURL := fmt.Sprintf("http://127.0.0.1:%d", lbPort)
	waitForLBHealth(t, lbURL, 90*time.Second)
	pgURL := discoverPostgresURL(t, composeEnv, baseArgs)

	return &cell{lbURL: lbURL, pgURL: pgURL, projectName: projectName}
}

func composeDownArgs(baseArgs []string) []string {
	downArgs := append([]string{}, baseArgs...)
	return append(downArgs, "down", "-v", "--rmi", "local", "--remove-orphans")
}

// composeBaseArgs returns the shared "-f <compose> -f <override> -p <project>"
// argument prefix. The shipped compose file is passed first so compose resolves
// build contexts and the nginx config relative to deploy/compose.
func composeBaseArgs(t *testing.T, projectName string) []string {
	t.Helper()
	root := repoRoot(t)
	composeFile := filepath.Join(root, "deploy", "compose", "docker-compose.yml")
	overrideFile := filepath.Join(packageDir(t), "testdata", "compose.override.yml")
	for _, path := range []string{composeFile, overrideFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("compose file missing: %s: %v", path, err)
		}
	}
	return []string{"-f", composeFile, "-f", overrideFile, "-p", projectName}
}

// runCompose runs `docker compose <args...>` with a bounded timeout and returns
// combined output.
func runCompose(ctx context.Context, env []string, timeout time.Duration, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	fullArgs := append([]string{"compose"}, args...)
	cmd := exec.CommandContext(runCtx, "docker", fullArgs...) //nolint:gosec
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// waitForLBHealth polls the LB /health route until it returns HTTP 200.
func waitForLBHealth(t *testing.T, lbURL string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)
	healthURL := lbURL + "/health"
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("LB /health did not return 200 at %s before deadline: %v", healthURL, lastErr)
}

// discoverPostgresURL resolves the ephemeral host port the override published
// for postgres and builds a host-side connection URL.
func discoverPostgresURL(t *testing.T, env []string, baseArgs []string) string {
	t.Helper()
	portArgs := append(append([]string{}, baseArgs...), "port", "postgres", "5432")
	out, err := runCompose(context.Background(), env, 30*time.Second, portArgs...)
	if err != nil {
		t.Fatalf("discover postgres host port: %v\n%s", err, out)
	}
	// Output is host:port (e.g. 127.0.0.1:54321); take the last colon-field.
	line := strings.TrimSpace(lastLine(out))
	idx := strings.LastIndex(line, ":")
	if idx < 0 {
		t.Fatalf("unexpected `docker compose port postgres 5432` output: %q", out)
	}
	hostPort := line[idx+1:]
	return fmt.Sprintf("postgresql://ayb:ayb@127.0.0.1:%s/ayb?sslmode=disable", hostPort)
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}

// packageDir returns the directory of this test package (its working dir).
func packageDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return dir
}

// repoRoot walks up from the package dir to the module root (go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir := packageDir(t)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
