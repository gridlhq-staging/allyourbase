package testutil

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

const maxFreePortAttempts = 100

// FreePort returns a leased TCP port available to an all-interface consumer.
func FreePort() (int, error) {
	return freePortWithCandidateSelector(selectConsumerPortCandidate)
}

func freePortWithCandidateSelector(selectCandidate func() (int, error)) (int, error) {
	for range maxFreePortAttempts {
		port, err := selectCandidate()
		if err != nil {
			return 0, err
		}

		leased, err := leaseFreshConsumerPortCandidate(port)
		if err != nil {
			return 0, err
		}
		if leased {
			return port, nil
		}
	}

	return 0, fmt.Errorf("failed to lease a free TCP port after %d attempts", maxFreePortAttempts)
}

// FreePortFromCandidates returns the first leased candidate available to an
// all-interface consumer.
func FreePortFromCandidates(candidates ...int) (int, error) {
	if len(candidates) == 0 {
		return 0, errors.New("at least one port candidate is required")
	}
	for _, port := range candidates {
		leased, err := leaseConsumerPortCandidate(port)
		if err != nil {
			return 0, err
		}
		if leased {
			return port, nil
		}
	}
	return 0, fmt.Errorf("failed to lease a free TCP port from %d candidates", len(candidates))
}

// FreePortFromCandidatesOrFree returns the first leased candidate available to
// an all-interface consumer, falling back to an internally selected leased port.
func FreePortFromCandidatesOrFree(candidates ...int) (int, error) {
	for _, port := range candidates {
		leased, err := leaseConsumerPortCandidate(port)
		if err != nil {
			return 0, err
		}
		if leased {
			return port, nil
		}
	}
	return FreePort()
}

func leaseConsumerPortCandidate(port int) (bool, error) {
	return leaseConsumerPortCandidateWith(port, acquirePortLease)
}

func leaseFreshConsumerPortCandidate(port int) (bool, error) {
	return leaseConsumerPortCandidateWith(port, acquireFreshPortLease)
}

func leaseConsumerPortCandidateWith(
	port int,
	acquireLease func(int) (bool, error),
) (bool, error) {
	acquired, err := acquireLease(port)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}

	if err := probeConsumerPort(port); err != nil {
		if releaseErr := ReleasePortLease(port); releaseErr != nil {
			return false, releaseErr
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			return false, nil
		}
		return false, fmt.Errorf("probe consumer port %d: %w", port, err)
	}

	owned, err := portLeaseOwnedByCurrentProcess(port)
	if err != nil {
		return false, err
	}
	return owned, nil
}

func selectConsumerPortCandidate() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("listen on all interfaces: %w", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return 0, fmt.Errorf("listener address %q has type %T, want *net.TCPAddr", listener.Addr(), listener.Addr())
	}
	port := addr.Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("close candidate listener: %w", err)
	}
	return port, nil
}

func probeConsumerPort(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	if err := listener.Close(); err != nil {
		return fmt.Errorf("close consumer probe: %w", err)
	}
	return nil
}
