package codehealth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const demoRunnerAYBStub = `#!/bin/bash
if [ "${1:-}" = "demo" ]; then
    echo "owner|ayb" >> "$OBSERVATIONS_PATH"
    echo "ayb|${AYB_DEMO_APP_PORT:-unset}|${AYB_DEMO_EXTERNAL_SERVER:-unset}" >> "$OBSERVATIONS_PATH"
fi
`

const demoRunnerHarnessTemplate = `set -uo pipefail
source %q
OBSERVATIONS_PATH=%q
export OBSERVATIONS_PATH
AYB_BIN=%q
REPO_ROOT=%q
SERVER_PORT=48123
DATABASE_PORT=45432
EXPECT_AYB_OWNER=%t
CYAN=""
NC=""
unset AYB_DEMO_EXTERNAL_SERVER

lsof() {
    case "$*" in
        *":$busy_port"*) return 0 ;;
        *) return 1 ;;
    esac
}
wait_for_url() {
    if [ "$EXPECT_AYB_OWNER" = "true" ] && [ "$1" = "http://127.0.0.1:${SERVER_PORT}/health" ]; then
        attempts=0
        while ! grep -q '^owner|ayb$' "$OBSERVATIONS_PATH" 2>/dev/null; do
            attempts=$((attempts + 1))
            if [ "$attempts" -ge 500 ]; then
                return 1
            fi
            sleep 0.02
        done
    fi
    echo "health|$1" >> "$OBSERVATIONS_PATH"
    return 0
}
npm() {
    echo "npm|${1:-missing}|${AYB_DEMO_EXTERNAL_SERVER:-unset}" >> "$OBSERVATIONS_PATH"
    return 0
}
npx() {
    case "${1:-} ${2:-}" in
        "playwright install")
            echo "npx|install|${AYB_DEMO_EXTERNAL_SERVER:-unset}" >> "$OBSERVATIONS_PATH"
            ;;
        "playwright test")
            echo "npx|test|${AYB_DEMO_APP_PORT:-unset}|${AYB_DEMO_EXTERNAL_SERVER:-unset}" >> "$OBSERVATIONS_PATH"
            if [ "${AYB_DEMO_EXTERNAL_SERVER:-unset}" != "1" ]; then
                echo "owner|playwright" >> "$OBSERVATIONS_PATH"
            fi
            ;;
    esac
    return 0
}
bash() {
    echo "guard|$*" >> "$OBSERVATIONS_PATH"
    return 0
}
prepare_isolated_home() {
    mkdir -p "$1/.ayb"
}
cleanup_demo_e2e_resources() {
    rm -f "${3:-}" "${4:-}"
    rm -rf "${5:-}" "${6:-}"
}
ensure_stopped() { return 0; }
pass() { return 0; }
fail() { return 1; }

run_demo_e2e() {%s
}

busy_port=$((42000 + $$ %% 1000))
selected_candidate=$((busy_port + 1000))
selected_port=$(pick_free_port "$busy_port" "$selected_candidate")
echo "picked|$selected_port" >> "$OBSERVATIONS_PATH"
run_demo_e2e kanban "$selected_port" %q
`

func runDemoRunnerHarness(t *testing.T, repoRoot, runDemoE2E string, expectAYBOwner bool) string {
	t.Helper()

	tempDir := t.TempDir()
	observationsPath := filepath.Join(tempDir, "observations.log")
	aybStubPath := filepath.Join(tempDir, "ayb_stub")
	writeTextFile(t, aybStubPath, demoRunnerAYBStub)
	if err := os.Chmod(aybStubPath, 0o755); err != nil {
		t.Fatalf("make AYB stub executable: %v", err)
	}

	harness := fmt.Sprintf(demoRunnerHarnessTemplate,
		filepath.Join(repoRoot, "tests", "port_helpers.sh"),
		observationsPath,
		aybStubPath,
		repoRoot,
		expectAYBOwner,
		runDemoE2E,
		tempDir,
	)

	cmd := exec.Command("bash", "-c", harness)
	cmd.Dir = repoRoot
	// The harness calls pick_free_port for real, which writes lease markers.
	// Keep them inside the test's temp dir instead of the host-shared default
	// namespace, so a lease taken here cannot block a concurrently running
	// process as the same uid. The rest of the environment is inherited.
	cmd.Env = append(os.Environ(), "AYB_PORT_LEASE_DIR="+filepath.Join(tempDir, "port_leases"))
	output, err := cmd.CombinedOutput()
	observations, readErr := os.ReadFile(observationsPath)
	if readErr != nil {
		t.Fatalf("read demo runner observations: %v; harness output: %s", readErr, output)
	}
	if err != nil {
		t.Fatalf("run isolated demo runner harness: %v\noutput:\n%s\nobservations:\n%s", err, output, observations)
	}
	return string(observations)
}

func requireSingleDemoAppServerOwner(t *testing.T, observations string) {
	t.Helper()
	if ownerCount := strings.Count(observations, "owner|"); ownerCount != 1 {
		t.Fatalf("demo invocation must have exactly one app-server owner, got %d in:\n%s", ownerCount, observations)
	}
}

func requireDemoAppServerOwnerCountRejected(t *testing.T, observations string, wantCount int) {
	t.Helper()
	ownerCount := strings.Count(observations, "owner|")
	if ownerCount != wantCount {
		t.Fatalf("negative wiring fixture must observe %d owners, got %d in:\n%s", wantCount, ownerCount, observations)
	}
	if ownerCount == 1 {
		t.Fatalf("negative wiring fixture unexpectedly satisfied exactly-one ownership:\n%s", observations)
	}
}
