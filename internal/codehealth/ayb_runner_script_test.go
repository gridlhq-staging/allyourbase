package codehealth

import (
	"errors"
	"fmt"
	"github.com/allyourbase/ayb/internal/testutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const runWithAYBScript = "scripts/run-with-ayb.sh"
const skippedPostHealthCommand = "echo should-not-run"
const runnerPortSelectionAttempts = 3

func runAYBScript(t *testing.T, postHealthCommand string, env ...string) (string, error) {
	t.Helper()

	if !hasEnvKey(env, "HOME") {
		env = append([]string{"HOME=" + t.TempDir()}, env...)
	}

	cmd := exec.Command("bash", runWithAYBScript, postHealthCommand)
	cmd.Dir = findRepoRoot(t)
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func hasEnvKey(env []string, key string) bool {
	_, found := envValue(env, key)
	return found
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix), true
		}
	}
	return "", false
}

func requireOutputContains(t *testing.T, output, want string) {
	t.Helper()

	if !strings.Contains(output, want) {
		t.Fatalf("expected output to contain %q, got: %s", want, output)
	}
}

func requireWrappedCommandSkipped(t *testing.T, output string) {
	t.Helper()

	if strings.Contains(output, "should-not-run") {
		t.Fatalf("wrapped command should not run, got: %s", output)
	}
}

func requireFileContainsTrimmed(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("expected %s to contain %q, got %q", path, want, got)
	}
}

func extractOutputValue(t *testing.T, output, prefix string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("expected output line with prefix %q, got: %s", prefix, output)
	return ""
}

func freshTokenWriterStartCommand(port int, writeDelay time.Duration) string {
	writeFreshToken := `fs.mkdirSync(path.join(home,'.ayb'), { recursive: true }); fs.writeFileSync(path.join(home,'.ayb','admin-token'), 'fresh-token\n');`
	if writeDelay > 0 {
		return fmt.Sprintf(`AYB_START_COMMAND=node -e "const fs=require('fs'); const path=require('path'); const http=require('http'); const home=process.env.HOME; http.createServer((_,res)=>res.end('ok')).listen(%d); setTimeout(() => { %s }, %d); setInterval(() => {}, 1000);"`, port, writeFreshToken, writeDelay/time.Millisecond)
	}
	return fmt.Sprintf(`AYB_START_COMMAND=node -e "const fs=require('fs'); const path=require('path'); const http=require('http'); const home=process.env.HOME; %s http.createServer((_,res)=>res.end('ok')).listen(%d); setInterval(() => {}, 1000);"`, writeFreshToken, port)
}

func runAYBScriptWithLeasedPort(
	t *testing.T,
	postHealthCommand string,
	envForPort func(int) []string,
) (string, error) {
	t.Helper()

	var output string
	var err error
	for range runnerPortSelectionAttempts {
		port := leasedRunnerPort(t)
		env := envForPort(port)
		output, err = runAYBScript(t, postHealthCommand, env...)
		if err == nil || !runFailedWithAddressInUse(output, env) {
			return output, err
		}

		// Leases prevent co-resident AYB handouts; retries cover non-AYB binders.
		if releaseErr := testutil.ReleasePortLease(port); releaseErr != nil {
			t.Fatalf("release raced runner port %d: %v", port, releaseErr)
		}
	}
	return output, err
}

func runFailedWithAddressInUse(output string, env []string) bool {
	if textReportsAddressInUse(output) {
		return true
	}
	logPath, ok := envValue(env, "AYB_START_LOG")
	if !ok {
		return false
	}
	logOutput, err := os.ReadFile(logPath)
	return err == nil && textReportsAddressInUse(string(logOutput))
}

func textReportsAddressInUse(output string) bool {
	return strings.Contains(output, "EADDRINUSE") ||
		strings.Contains(strings.ToLower(output), "address already in use")
}

func occupyPortWithoutServingHealth(t *testing.T, network, address string) {
	t.Helper()

	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatalf("bind deterministic competing listener: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = connection.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
}

func TestRunWithAYBScriptRequiresPostHealthCommandArgument(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("bash", runWithAYBScript)
	cmd.Dir = findRepoRoot(t)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected usage failure when no command argument is provided, got success: %s", output)
	}
	requireOutputContains(t, string(output), "Usage:")
}

func TestRunWithAYBScriptBuildsUIBeforeAYBBinary(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join(findRepoRoot(t), runWithAYBScript)
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", runWithAYBScript, err)
	}
	source := string(script)

	uiBuildIndex := strings.Index(source, "(cd ui && pnpm build)")
	goBuildIndex := strings.Index(source, "go build -o ayb ./cmd/ayb")
	if uiBuildIndex < 0 {
		t.Fatalf("%s must enter ui before invoking pnpm so Corepack honors ui/package.json", runWithAYBScript)
	}
	if strings.Contains(source, "pnpm --dir ui build") {
		t.Fatalf("%s must not invoke Corepack from the package-less repository root", runWithAYBScript)
	}
	if goBuildIndex < 0 {
		t.Fatalf("%s must build ./ayb when AYB_START_COMMAND uses it", runWithAYBScript)
	}
	if uiBuildIndex > goBuildIndex {
		t.Fatalf("%s builds ./ayb before refreshing ui/dist", runWithAYBScript)
	}
}

func TestRunWithAYBScriptRefreshesAutomaticPortsAfterBuild(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join(findRepoRoot(t), runWithAYBScript)
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", runWithAYBScript, err)
	}
	source := string(script)

	refreshDefinition := strings.Index(source, "refresh_auto_selected_runtime_ports()")
	if refreshDefinition < 0 {
		t.Fatalf("%s must own automatic port reselection in a focused helper", runWithAYBScript)
	}
	if !strings.Contains(source[refreshDefinition:], `AYB_SERVER_PORT="$(pick_free_port`) {
		t.Fatalf("%s automatic port refresh must select the AYB server port again", runWithAYBScript)
	}

	buildCall := strings.LastIndex(source, "\nensure_ayb_binary_if_needed\n")
	refreshCall := strings.LastIndex(source, "\nrefresh_auto_selected_runtime_ports\n")
	startCall := strings.LastIndex(source, `bash -lc "$AYB_START_COMMAND"`)
	if buildCall < 0 || refreshCall < 0 || startCall < 0 {
		t.Fatalf("%s must build, refresh automatic ports, and start AYB", runWithAYBScript)
	}
	if refreshCall < buildCall || refreshCall > startCall {
		t.Fatalf("%s must refresh automatic ports after the potentially slow build and before AYB starts", runWithAYBScript)
	}
}

func TestRunWithAYBScriptFailsFastOnHealthTimeout(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-timeout.log")

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_START_COMMAND=sleep 30",
		"AYB_HEALTH_URL=http://127.0.0.1:9/health",
		"AYB_HEALTH_TIMEOUT_SECONDS=1",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err == nil {
		t.Fatalf("expected timeout failure, got success: %s", output)
	}
	requireOutputContains(t, output, "Timed out waiting for AYB health check")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRejectsInvalidHealthTimeoutConfig(t *testing.T) {
	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_HEALTH_TIMEOUT_SECONDS=0",
	)
	if err == nil {
		t.Fatalf("expected invalid-timeout failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB_HEALTH_TIMEOUT_SECONDS must be a positive integer")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRejectsInvalidHealthPollIntervalConfig(t *testing.T) {
	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=not-a-number",
	)
	if err == nil {
		t.Fatalf("expected invalid-poll-interval failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB_HEALTH_POLL_INTERVAL_SECONDS must be a positive number")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptFailsIfProcessExitsBeforeHealthy(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-exit.log")

	output, err := runAYBScript(t, skippedPostHealthCommand,
		"AYB_START_COMMAND=sh -c 'exit 0'",
		"AYB_HEALTH_URL=http://127.0.0.1:9/health",
		"AYB_HEALTH_TIMEOUT_SECONDS=3",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err == nil {
		t.Fatalf("expected process-exit failure, got success: %s", output)
	}
	requireOutputContains(t, output, "AYB process exited before health check passed")
	requireWrappedCommandSkipped(t, output)
}

func TestRunWithAYBScriptRunsCommandAfterHealthReady(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-success.log")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)

	output, err := runAYBScript(t, "echo command-finished",
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d);"`, healthPort),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestRunWithAYBScriptSurvivesHelperToConsumerPortCollision(t *testing.T) {
	attempts := 0
	logDir := t.TempDir()
	output, err := runAYBScriptWithLeasedPort(t, "echo command-finished", func(healthPort int) []string {
		attempts++
		if attempts == 1 {
			occupyPortWithoutServingHealth(t, "tcp", fmt.Sprintf("127.0.0.1:%d", healthPort))
		}
		return []string{
			fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); http.createServer((_,res)=>res.end('ok')).listen(%d, '127.0.0.1');"`, healthPort),
			fmt.Sprintf("AYB_HEALTH_URL=http://127.0.0.1:%d/health", healthPort),
			"AYB_HEALTH_TIMEOUT_SECONDS=3",
			"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
			"AYB_ADMIN_PASSWORD=test-admin-password",
			fmt.Sprintf("AYB_START_LOG=%s/attempt-%d.log", logDir, attempts),
		}
	})
	if err != nil {
		t.Fatalf("expected foreign address-in-use retry to recover: %v output=%s", err, output)
	}
	if attempts < 2 {
		t.Fatalf("select-to-spawn attempts = %d, want at least 2 after one deterministic collision", attempts)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestReserveLocalhostPortCoversAllInterfaceConsumerBind(t *testing.T) {
	consumerListener, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skipf("IPv6 all-interface listeners are unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = consumerListener.Close()
	})

	address, ok := consumerListener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("IPv6 all-interface listener returned invalid address: %#v", consumerListener.Addr())
	}

	leasedPort := leasedRunnerPortFromCandidatesOrFree(t, address.Port)
	if leasedPort == address.Port {
		t.Fatalf("leased port = occupied all-interface port %d", address.Port)
	}
	candidateListener, err := net.Listen("tcp", fmt.Sprintf(":%d", leasedPort))
	if err != nil {
		t.Fatalf("leased runner port %d does not support the consumer all-interface bind: %v", leasedPort, err)
	}
	if err := candidateListener.Close(); err != nil {
		t.Fatalf("close all-interface candidate: %v", err)
	}
}

func TestPortHelperSubprocessLeavesNoLivePIDLeases(t *testing.T) {
	leaseDir := t.TempDir()
	cmd := exec.Command("bash", "tests/test_port_helpers.sh")
	cmd.Dir = findRepoRoot(t)
	cmd.Env = append(os.Environ(), "AYB_PORT_LEASE_DIR="+leaseDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("port helper subprocess failed: %v output=%s", err, output)
	}
	requireOutputContains(t, string(output), "leases_remaining_after_run=0")

	liveLeaseOwners := 0
	err = filepath.WalkDir(leaseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		owner, readErr := os.Readlink(path)
		if readErr != nil {
			return readErr
		}
		pid, atoiErr := strconv.Atoi(owner)
		if atoiErr == nil && pid > 0 && syscall.Kill(pid, 0) == nil {
			liveLeaseOwners++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect subprocess lease directory: %v", err)
	}
	if liveLeaseOwners != 0 {
		t.Fatalf("port helper subprocess left %d symlink lease(s) pointing at live PIDs in %s", liveLeaseOwners, leaseDir)
	}
}

func TestRunWithAYBScriptIsolatesEmbeddedDataDirForOwnedServer(t *testing.T) {
	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-isolated-data-dir.log")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	defaultDataDir := filepath.Join(homeDir, ".ayb", "data")

	output, err := runAYBScript(t, `test -d "$AYB_DATABASE_EMBEDDED_DATA_DIR" && echo "data-dir=$AYB_DATABASE_EMBEDDED_DATA_DIR"`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 0),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}

	dataDir := extractOutputValue(t, output, "data-dir=")
	if dataDir == defaultDataDir {
		t.Fatalf("wrapper-owned startup should not use default embedded data dir %s", defaultDataDir)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected wrapper-owned embedded data dir to be removed after exit, stat err=%v", err)
	}
}

func TestRunWithAYBScriptReusesExistingHealthyServer(t *testing.T) {
	server, healthURL := startHealthServerOnRandomPort(t)
	defer server.Close()

	output, err := runAYBScript(t, "echo command-finished",
		"AYB_START_COMMAND=sh -c 'echo should-not-start; exit 42'",
		"AYB_HEALTH_URL="+healthURL,
		"AYB_ADMIN_TOKEN=test-admin-token",
	)
	if err != nil {
		t.Fatalf("expected existing healthy server reuse, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	if strings.Contains(output, "should-not-start") {
		t.Fatalf("wrapper should not start a second server when health is already ready, got: %s", output)
	}
}

func TestRunWithAYBScriptReuseMaterializesCanonicalAdminToken(t *testing.T) {
	homeDir := t.TempDir()
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	server, healthURL := startHealthServerOnRandomPort(t)
	defer server.Close()

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = test-admin-token && echo command-finished`,
		"HOME="+homeDir,
		"AYB_START_COMMAND=sh -c 'echo should-not-start; exit 42'",
		"AYB_HEALTH_URL="+healthURL,
		"AYB_ADMIN_TOKEN=test-admin-token",
	)
	if err != nil {
		t.Fatalf("expected canonical token materialization during runtime reuse, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected wrapper to restore canonical token state after reuse, stat err=%v", err)
	}
}

func TestRunWithAYBScriptDerivesHealthAndBaseURLFromServerHostPort(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-derived-host-port.log")
	healthPort := leasedRunnerPort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", healthPort)

	output, err := runAYBScript(t, fmt.Sprintf(`test "$AYB_BASE_URL" = %q && echo command-finished`, baseURL),
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d, '127.0.0.1');"`, healthPort),
		"AYB_SERVER_HOST=127.0.0.1",
		fmt.Sprintf("AYB_SERVER_PORT=%d", healthPort),
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestRunWithAYBScriptUsesExplicitBaseURLForHealthAndChildren(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-explicit-base-url.log")
	healthPort := leasedRunnerPort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", healthPort)

	output, err := runAYBScript(t, fmt.Sprintf(`test "$AYB_BASE_URL" = %q && echo command-finished`, baseURL),
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d, '127.0.0.1');"`, healthPort),
		"AYB_BASE_URL="+baseURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestRunWithAYBScriptTreatsAdminTokenEnvAsReadyCredentials(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-admin-token-env.log")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)

	output, err := runAYBScript(t, "echo command-finished",
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d);"`, healthPort),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_TOKEN=test-admin-token",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
}

func TestRunWithAYBScriptUsesFreshAdminTokenAndRestoresOriginalToken(t *testing.T) {
	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-token.log")
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	writeTextFile(t, tokenPath, "original-token\n")

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = fresh-token && echo command-finished`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 500*time.Millisecond),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	requireFileContainsTrimmed(t, tokenPath, "original-token")
}

func TestRunWithAYBScriptPreservesCallerOwnedCustomAdminTokenPath(t *testing.T) {
	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-custom-token-path.log")
	customTokenPath := filepath.Join(t.TempDir(), "caller-owned-token")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	writeTextFile(t, customTokenPath, "caller-token\n")

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = fresh-token && echo command-finished`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 0),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_TOKEN_PATH="+customTokenPath,
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	requireFileContainsTrimmed(t, customTokenPath, "caller-token")
}

func TestRunWithAYBScriptRestoresOriginalTokenWhenAdminPasswordProvided(t *testing.T) {
	homeDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ayb-token-with-password.log")
	tokenPath := filepath.Join(homeDir, ".ayb", "admin-token")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)
	writeTextFile(t, tokenPath, "original-token\n")

	output, err := runAYBScript(t, `test "$(cat "$HOME/.ayb/admin-token")" = fresh-token && echo command-finished`,
		"HOME="+homeDir,
		freshTokenWriterStartCommand(healthPort, 0),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")
	requireFileContainsTrimmed(t, tokenPath, "original-token")
}

func TestRunWithAYBScriptStopsForegroundServerOnExit(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ayb-cleanup.log")
	healthPort := leasedRunnerPort(t)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", healthPort)

	output, err := runAYBScript(t, "echo command-finished",
		fmt.Sprintf(`AYB_START_COMMAND=node -e "const http=require('http'); const server=http.createServer((_,res)=>res.end('ok')); server.listen(%d);"`, healthPort),
		"AYB_HEALTH_URL="+healthURL,
		"AYB_HEALTH_TIMEOUT_SECONDS=10",
		"AYB_HEALTH_POLL_INTERVAL_SECONDS=0.1",
		"AYB_ADMIN_PASSWORD=test-admin-password",
		"AYB_START_LOG="+logPath,
	)
	if err != nil {
		t.Fatalf("expected success, got error: %v output=%s", err, output)
	}
	requireOutputContains(t, output, "command-finished")

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, getErr := client.Get(healthURL)
		if getErr != nil {
			return
		}
		resp.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("expected foreground AYB process to be stopped after wrapper exit")
}

func leasedRunnerPort(t *testing.T) int {
	t.Helper()

	port, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("lease runner port: %v", err)
	}
	t.Cleanup(func() {
		if err := testutil.ReleasePortLease(port); err != nil {
			t.Errorf("release runner port %d: %v", port, err)
		}
	})
	return port
}

func leasedRunnerPortFromCandidates(t *testing.T, candidates ...int) int {
	t.Helper()

	port, err := testutil.FreePortFromCandidates(candidates...)
	if err != nil {
		t.Fatalf("lease runner port from candidates %v: %v", candidates, err)
	}
	t.Cleanup(func() {
		if err := testutil.ReleasePortLease(port); err != nil {
			t.Errorf("release runner port %d: %v", port, err)
		}
	})
	return port
}

func leasedRunnerPortFromCandidatesOrFree(t *testing.T, candidates ...int) int {
	t.Helper()

	port, err := testutil.FreePortFromCandidatesOrFree(candidates...)
	if err != nil {
		t.Fatalf("lease runner port from candidates or free fallback %v: %v", candidates, err)
	}
	t.Cleanup(func() {
		if err := testutil.ReleasePortLease(port); err != nil {
			t.Errorf("release runner port %d: %v", port, err)
		}
	})
	return port
}

func startHealthServerOnRandomPort(t *testing.T) (*http.Server, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startHealthServerOnRandomPort listen: %v", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		listener.Close()
		t.Fatalf("startHealthServerOnRandomPort invalid address: %#v", listener.Addr())
	}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", addr.Port)

	server := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }),
		ReadHeaderTimeout: time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	deadline := time.Now().Add(2 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			return server, healthURL
		}
		select {
		case listenErr := <-errCh:
			if errors.Is(listenErr, http.ErrServerClosed) {
				t.Fatalf("health server closed before readiness")
			}
			t.Fatalf("health server exited before readiness: %v", listenErr)
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("health server did not become ready at %s", healthURL)
	return server, healthURL
}
