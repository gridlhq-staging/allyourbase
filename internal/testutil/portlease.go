package testutil

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const portLeaseDirEnv = "AYB_PORT_LEASE_DIR"

type portLeaseState struct {
	exists   bool
	symlink  bool
	readable bool
	owner    string
}

func portLeaseDirectory() (string, error) {
	leaseDir := os.Getenv(portLeaseDirEnv)
	if leaseDir == "" {
		tempBase := strings.TrimSuffix(os.TempDir(), "/")
		leaseDir = tempBase + "/ayb-port-leases-" + strconv.Itoa(os.Getuid())
	}
	if err := os.MkdirAll(leaseDir, 0o700); err != nil {
		return "", fmt.Errorf("create port lease directory %s: %w", leaseDir, err)
	}
	dir, err := os.OpenFile(leaseDir, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("open port lease directory %s without following symlinks: %w", leaseDir, err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return "", fmt.Errorf("stat port lease directory %s: %w", leaseDir, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return "", fmt.Errorf("port lease directory %s is not owned by the current user", leaseDir)
	}
	if err := dir.Chmod(0o700); err != nil {
		return "", fmt.Errorf("restrict port lease directory %s: %w", leaseDir, err)
	}
	return leaseDir, nil
}

func portLeasePath(port int) (string, error) {
	if port <= 0 {
		return "", fmt.Errorf("port must be positive, got %d", port)
	}
	leaseDir, err := portLeaseDirectory()
	if err != nil {
		return "", err
	}
	return leaseDir + "/" + strconv.Itoa(port), nil
}

func inspectPortLease(path string) (portLeaseState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return portLeaseState{}, nil
	}
	if err != nil {
		return portLeaseState{}, fmt.Errorf("lstat port lease %s: %w", path, err)
	}

	state := portLeaseState{exists: true, symlink: info.Mode()&os.ModeSymlink != 0}
	if !state.symlink {
		return state, nil
	}
	owner, err := os.Readlink(path)
	if err != nil {
		return state, nil
	}
	state.readable = true
	state.owner = owner
	return state, nil
}

func portLeaseOwnerIsLive(owner string) bool {
	pid, err := strconv.Atoi(owner)
	if err != nil || pid <= 0 {
		return false
	}
	// The per-uid lease namespace makes EPERM unreachable in normal use.
	return syscall.Kill(pid, 0) == nil
}

func acquirePortLease(port int) (bool, error) {
	return acquirePortLeaseWithReentry(port, true)
}

func acquireFreshPortLease(port int) (bool, error) {
	return acquirePortLeaseWithReentry(port, false)
}

func acquirePortLeaseWithReentry(port int, allowCurrentProcessLease bool) (bool, error) {
	path, err := portLeasePath(port)
	if err != nil {
		return false, err
	}

	observed, err := inspectPortLease(path)
	if err != nil {
		return false, err
	}
	if observed.exists {
		if !observed.symlink {
			return false, nil
		}
		if observed.readable && observed.owner == strconv.Itoa(os.Getpid()) {
			return allowCurrentProcessLease, nil
		}
		if observed.readable && portLeaseOwnerIsLive(observed.owner) {
			return false, nil
		}

		current, err := inspectPortLease(path)
		if err != nil {
			return false, err
		}
		if current != observed {
			return false, nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("reap stale port lease %s: %w", path, err)
		}
	}

	err = os.Symlink(strconv.Itoa(os.Getpid()), path)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("acquire port lease %s: %w", path, err)
	}
	return true, nil
}

func portLeaseOwnedByCurrentProcess(port int) (bool, error) {
	path, err := portLeasePath(port)
	if err != nil {
		return false, err
	}
	state, err := inspectPortLease(path)
	if err != nil {
		return false, err
	}
	return state.exists && state.symlink && state.readable &&
		state.owner == strconv.Itoa(os.Getpid()), nil
}

// ReleasePortLease releases port when it is leased by the current process.
// Missing leases and leases owned by other processes are left unchanged.
func ReleasePortLease(port int) error {
	path, err := portLeasePath(port)
	if err != nil {
		return err
	}
	owned, err := portLeaseOwnedByCurrentProcess(port)
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("release port lease %s: %w", path, err)
	}
	return nil
}
