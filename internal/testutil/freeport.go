package testutil

import (
	"fmt"
	"net"
)

// FreePort returns an available TCP port on the loopback interface.
func FreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen on 127.0.0.1:0: %w", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listener address %q has type %T, want *net.TCPAddr", listener.Addr(), listener.Addr())
	}
	return addr.Port, nil
}
