package testutil

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFreePortCreatesOwnedLeaseAndRelease(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("FreePort returned non-positive port %d", port)
	}

	leasePath := leaseDir + "/" + strconv.Itoa(port)
	info, err := os.Lstat(leasePath)
	if err != nil {
		t.Fatalf("lstat port lease: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("port lease mode = %v, want symlink", info.Mode())
	}
	owner, err := os.Readlink(leasePath)
	if err != nil {
		t.Fatalf("read port lease owner: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()); owner != want {
		t.Fatalf("port lease owner = %q, want %q", owner, want)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("listen on returned port %d: %v", port, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if err := ReleasePortLease(port); err != nil {
		t.Fatalf("ReleasePortLease: %v", err)
	}
	if _, err := os.Lstat(leasePath); !os.IsNotExist(err) {
		t.Fatalf("released port lease still exists: %v", err)
	}
}

func TestFreePortDoesNotRehandCurrentProcessLease(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	candidate, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select candidate: %v", err)
	}
	if _, err := FreePortFromCandidates(candidate); err != nil {
		t.Fatalf("lease candidate: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(candidate); err != nil {
			t.Errorf("release candidate lease: %v", err)
		}
	})

	selections := 0
	_, err = freePortWithCandidateSelector(func() (int, error) {
		selections++
		return candidate, nil
	})
	if err == nil {
		t.Fatalf("FreePort rehanded current-process lease on port %d", candidate)
	}
	if selections != maxFreePortAttempts {
		t.Fatalf("candidate selections = %d, want %d before exhaustion", selections, maxFreePortAttempts)
	}
}

func TestPortLeaseDirectoryMatchesShellDefault(t *testing.T) {
	tempBase := t.TempDir() + "/"
	t.Setenv("AYB_PORT_LEASE_DIR", "")
	t.Setenv("TMPDIR", tempBase)

	cmd := exec.Command("bash", "-c", `source tests/port_helpers.sh; resolve_port_lease_dir; printf '%s\n%s\n' "$AYB_PORT_LEASE_DIR" "$(id -u)"`)
	cmd.Dir = repoRootForTest(t)
	cmd.Env = append(os.Environ(), "AYB_PORT_LEASE_DIR=", "TMPDIR="+tempBase)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve shell lease directory: %v output=%s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		t.Fatalf("shell lease directory output = %q, want directory and uid lines", output)
	}

	leaseDir, err := portLeaseDirectory()
	if err != nil {
		t.Fatalf("portLeaseDirectory: %v", err)
	}
	want := strings.TrimSuffix(tempBase, "/") + "/ayb-port-leases-" + strconv.Itoa(os.Getuid())
	if leaseDir != want {
		t.Fatalf("portLeaseDirectory = %q, want %q", leaseDir, want)
	}
	if lines[0] != leaseDir {
		t.Fatalf("shell resolve_port_lease_dir = %q, want byte-identical Go lease directory %q", lines[0], leaseDir)
	}
	if lines[1] != strconv.Itoa(os.Getuid()) {
		t.Fatalf("shell id -u = %q, want Go os.Getuid %d", lines[1], os.Getuid())
	}
	if info, err := os.Stat(leaseDir); err != nil || !info.IsDir() {
		t.Fatalf("port lease directory was not created: info=%v err=%v", info, err)
	}
	if mode := os.FileMode(0o700); mustStatMode(t, leaseDir).Perm() != mode {
		t.Fatalf("port lease directory permissions = %#o, want %#o", mustStatMode(t, leaseDir).Perm(), mode)
	}
}

func TestPortLeaseDirectoryRestrictsExistingPermissions(t *testing.T) {
	leaseDir := filepath.Join(t.TempDir(), "port-leases")
	if err := os.Mkdir(leaseDir, 0o755); err != nil {
		t.Fatalf("mkdir preexisting lease dir: %v", err)
	}
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	got, err := portLeaseDirectory()
	if err != nil {
		t.Fatalf("portLeaseDirectory: %v", err)
	}
	if got != leaseDir {
		t.Fatalf("portLeaseDirectory = %q, want %q", got, leaseDir)
	}
	if mode := os.FileMode(0o700); mustStatMode(t, leaseDir).Perm() != mode {
		t.Fatalf("restricted lease dir permissions = %#o, want %#o", mustStatMode(t, leaseDir).Perm(), mode)
	}
}

func TestPortLeaseDirectoryRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "target")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir symlink target: %v", err)
	}
	leaseDir := filepath.Join(tempDir, "port-leases")
	if err := os.Symlink(targetDir, leaseDir); err != nil {
		t.Fatalf("symlink lease directory: %v", err)
	}
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	if _, err := portLeaseDirectory(); err == nil {
		t.Fatal("portLeaseDirectory accepted a symlink")
	}
	if mode := os.FileMode(0o755); mustStatMode(t, targetDir).Perm() != mode {
		t.Fatalf("symlink target permissions = %#o, want unchanged %#o", mustStatMode(t, targetDir).Perm(), mode)
	}
}

func TestPortLeaseReapsCorruptOwner(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)
	const port = 45123
	leasePath := leaseDir + "/" + strconv.Itoa(port)
	if err := os.Symlink("not-a-pid", leasePath); err != nil {
		t.Fatalf("create corrupt lease: %v", err)
	}

	acquired, err := acquirePortLease(port)
	if err != nil {
		t.Fatalf("acquirePortLease: %v", err)
	}
	if !acquired {
		t.Fatal("acquirePortLease rejected a corrupt lease after reaping")
	}
	owner, err := os.Readlink(leasePath)
	if err != nil {
		t.Fatalf("read acquired lease: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()); owner != want {
		t.Fatalf("acquired lease owner = %q, want %q", owner, want)
	}
}

func TestPortLeaseReapsDeadNumericOwner(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)
	const port = 45125
	leasePath := leaseDir + "/" + strconv.Itoa(port)
	deadOwner := strconv.Itoa(deadNumericPID(t))
	if err := os.Symlink(deadOwner, leasePath); err != nil {
		t.Fatalf("create dead-owner lease: %v", err)
	}

	acquired, err := acquirePortLease(port)
	if err != nil {
		t.Fatalf("acquirePortLease: %v", err)
	}
	if !acquired {
		t.Fatal("acquirePortLease rejected a dead-owner lease after reaping")
	}
	owner, err := os.Readlink(leasePath)
	if err != nil {
		t.Fatalf("read acquired lease: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()); owner != want {
		t.Fatalf("acquired lease owner = %q, want %q after reaping dead owner %q", owner, want, deadOwner)
	}
}

func TestPortLeasePreservesLiveForeignOwner(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)
	const port = 45124

	owner := exec.Command("sleep", "30")
	if err := owner.Start(); err != nil {
		t.Fatalf("start live lease owner: %v", err)
	}
	t.Cleanup(func() {
		_ = owner.Process.Kill()
		_ = owner.Wait()
	})

	leasePath := leaseDir + "/" + strconv.Itoa(port)
	ownerPID := strconv.Itoa(owner.Process.Pid)
	if err := os.Symlink(ownerPID, leasePath); err != nil {
		t.Fatalf("create live foreign lease: %v", err)
	}

	acquired, err := acquirePortLease(port)
	if err != nil {
		t.Fatalf("acquirePortLease: %v", err)
	}
	if acquired {
		t.Fatal("acquirePortLease replaced a live foreign lease")
	}
	gotOwner, err := os.Readlink(leasePath)
	if err != nil {
		t.Fatalf("read preserved live lease: %v", err)
	}
	if gotOwner != ownerPID {
		t.Fatalf("preserved lease owner = %q, want %q", gotOwner, ownerPID)
	}
}

func TestBashAcquiredLeaseIsUnavailableToGo(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)
	stubDir := writeUnoccupiedLsofStub(t)
	candidate, fallback := distinctConsumerPortCandidates(t)
	selectedPath := filepath.Join(t.TempDir(), "selected_port")

	cmd := exec.Command("bash", "-c", `source tests/port_helpers.sh; pick_free_port "$1" "$2" >"$3"; sleep 30`, "_", strconv.Itoa(candidate), strconv.Itoa(fallback), selectedPath)
	cmd.Dir = repoRootForTest(t)
	cmd.Env = append(os.Environ(),
		"AYB_PORT_LEASE_DIR="+leaseDir,
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start bash lease owner: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	selected := waitForSelectedPort(t, selectedPath, cmd)
	if selected != strconv.Itoa(candidate) {
		t.Fatalf("bash pick_free_port selected %s, want %d", selected, candidate)
	}
	leasePath := leaseDir + "/" + strconv.Itoa(candidate)
	state, err := inspectPortLease(leasePath)
	if err != nil {
		t.Fatalf("inspect bash-acquired lease: %v", err)
	}
	if !state.exists || !state.symlink || !state.readable {
		t.Fatalf("bash-acquired lease state = %+v, want readable symlink", state)
	}
	if want := strconv.Itoa(cmd.Process.Pid); state.owner != want {
		t.Fatalf("bash-acquired lease owner = %q, want live bash PID %q", state.owner, want)
	}

	acquired, err := acquirePortLease(candidate)
	if err != nil {
		t.Fatalf("acquirePortLease against bash-acquired lease: %v", err)
	}
	if acquired {
		t.Fatalf("Go acquired candidate %d while bash PID %d owned the lease", candidate, cmd.Process.Pid)
	}
}

func TestGoAcquiredLeaseIsUnavailableToBash(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)
	stubDir := writeUnoccupiedLsofStub(t)
	candidate, fallback := distinctConsumerPortCandidates(t)

	selectedByGo, err := FreePortFromCandidates(candidate)
	if err != nil {
		t.Fatalf("FreePortFromCandidates: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(selectedByGo); err != nil {
			t.Errorf("ReleasePortLease(%d): %v", selectedByGo, err)
		}
	})
	if selectedByGo != candidate {
		t.Fatalf("Go selected %d, want candidate %d", selectedByGo, candidate)
	}

	cmd := exec.Command("bash", "-c", `source tests/port_helpers.sh; pick_free_port "$1" "$2"`, "_", strconv.Itoa(candidate), strconv.Itoa(fallback))
	cmd.Dir = repoRootForTest(t)
	cmd.Env = append(os.Environ(),
		"AYB_PORT_LEASE_DIR="+leaseDir,
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash pick_free_port against Go-acquired lease: %v output=%s", err, output)
	}
	selectedByBash := strings.TrimSpace(string(output))
	if selectedByBash != strconv.Itoa(fallback) {
		t.Fatalf("bash selected %q, want fallback %d after skipping Go-acquired lease %d", selectedByBash, fallback, candidate)
	}
	owner, err := os.Readlink(leaseDir + "/" + strconv.Itoa(candidate))
	if err != nil {
		t.Fatalf("read Go-acquired lease owner: %v", err)
	}
	if want := strconv.Itoa(os.Getpid()); owner != want {
		t.Fatalf("Go-acquired lease owner = %q, want preserved owner %q", owner, want)
	}
}

func TestFreePortFromCandidatesReentersCurrentProcessLease(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	candidate, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select candidate: %v", err)
	}
	firstPort, err := FreePortFromCandidates(candidate)
	if err != nil {
		t.Fatalf("first FreePortFromCandidates: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(firstPort); err != nil {
			t.Errorf("ReleasePortLease(%d): %v", firstPort, err)
		}
	})

	secondPort, err := FreePortFromCandidates(candidate)
	if err != nil {
		t.Fatalf("second FreePortFromCandidates: %v", err)
	}
	if firstPort != candidate || secondPort != candidate {
		t.Fatalf("same-process lease selected %d then %d, want candidate %d twice", firstPort, secondPort, candidate)
	}
}

func TestFreePortFromCandidatesAdvancesWhenReenteredLeaseBecomesOccupied(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	candidate, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select candidate: %v", err)
	}
	if _, err := FreePortFromCandidates(candidate); err != nil {
		t.Fatalf("lease candidate: %v", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
	if err != nil {
		t.Fatalf("occupy leased candidate %d: %v", candidate, err)
	}
	defer listener.Close()

	fallback, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select fallback: %v", err)
	}
	selected, err := FreePortFromCandidates(candidate, fallback)
	if err != nil {
		t.Fatalf("FreePortFromCandidates with occupied lease: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(selected); err != nil {
			t.Errorf("ReleasePortLease(%d): %v", selected, err)
		}
	})

	if selected != fallback {
		t.Fatalf("selected port = %d, want fallback %d after candidate %d became occupied", selected, fallback, candidate)
	}
	owned, err := portLeaseOwnedByCurrentProcess(candidate)
	if err != nil {
		t.Fatalf("inspect occupied candidate lease: %v", err)
	}
	if owned {
		t.Fatalf("occupied candidate %d retained its current-process lease", candidate)
	}
}

func TestFreePortProbeRejectsAllInterfaceConflict(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skipf("IPv6 all-interface listeners are unavailable: %v", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("all-interface listener returned invalid address: %#v", listener.Addr())
	}
	err = probeConsumerPort(address.Port)
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("probeConsumerPort(%d) error = %v, want EADDRINUSE", address.Port, err)
	}
}

func TestFreePortFromCandidatesRejectsAllInterfaceConflict(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	listener, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skipf("IPv6 all-interface listeners are unavailable: %v", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("all-interface listener returned invalid address: %#v", listener.Addr())
	}
	fallbackPort, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select fallback candidate: %v", err)
	}

	port, err := FreePortFromCandidates(address.Port, fallbackPort)
	if err != nil {
		t.Fatalf("FreePortFromCandidates: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(port); err != nil {
			t.Errorf("ReleasePortLease(%d): %v", port, err)
		}
	})
	if port != fallbackPort {
		t.Fatalf("FreePortFromCandidates returned %d, want fallback %d after rejecting occupied port %d", port, fallbackPort, address.Port)
	}
	occupiedOwned, err := portLeaseOwnedByCurrentProcess(address.Port)
	if err != nil {
		t.Fatalf("inspect occupied port lease: %v", err)
	}
	if occupiedOwned {
		t.Fatalf("occupied port %d still has a current-process lease after rejection", address.Port)
	}
}

func TestFreePortFromCandidatesOrFreeOwnsFallbackSelection(t *testing.T) {
	leaseDir := t.TempDir()
	t.Setenv("AYB_PORT_LEASE_DIR", leaseDir)

	listener, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skipf("IPv6 all-interface listeners are unavailable: %v", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("all-interface listener returned invalid address: %#v", listener.Addr())
	}

	port, err := FreePortFromCandidatesOrFree(address.Port)
	if err != nil {
		t.Fatalf("FreePortFromCandidatesOrFree: %v", err)
	}
	t.Cleanup(func() {
		if err := ReleasePortLease(port); err != nil {
			t.Errorf("ReleasePortLease(%d): %v", port, err)
		}
	})
	if port == address.Port {
		t.Fatalf("FreePortFromCandidatesOrFree returned occupied candidate %d", port)
	}
	owned, err := portLeaseOwnedByCurrentProcess(port)
	if err != nil {
		t.Fatalf("inspect fallback lease: %v", err)
	}
	if !owned {
		t.Fatalf("fallback port %d was not leased by the current process", port)
	}
}

func deadNumericPID(t *testing.T) int {
	t.Helper()

	for pid := 999999; pid > 1; pid-- {
		// Only ESRCH ("no such process") proves the PID is dead. A generic
		// kill(pid, 0) failure is not enough: on multi-user hosts EPERM means
		// a live foreign process owns the PID but we may not signal it, so
		// selecting it would hand the stale-owner test a live specimen.
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return pid
		}
	}
	t.Fatal("failed to find a dead numeric PID specimen")
	return 0
}

func distinctConsumerPortCandidates(t *testing.T) (int, int) {
	t.Helper()

	first, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select first candidate: %v", err)
	}
	second, err := selectConsumerPortCandidate()
	if err != nil {
		t.Fatalf("select second candidate: %v", err)
	}
	for second == first {
		second, err = selectConsumerPortCandidate()
		if err != nil {
			t.Fatalf("select distinct second candidate: %v", err)
		}
	}
	return first, second
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join("..", ".."))
}

func mustStatMode(t *testing.T, path string) os.FileMode {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode()
}

func waitForSelectedPort(t *testing.T, path string, cmd *exec.Cmd) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
		if signalErr := cmd.Process.Signal(syscall.Signal(0)); signalErr != nil {
			t.Fatalf("bash lease owner exited before writing selected port: %v", signalErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("bash lease owner did not write selected port to %s", path)
	return ""
}

func writeUnoccupiedLsofStub(t *testing.T) string {
	t.Helper()

	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "lsof"), []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write lsof stub: %v", err)
	}
	return stubDir
}
