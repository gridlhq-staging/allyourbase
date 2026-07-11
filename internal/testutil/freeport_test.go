package testutil

import (
	"fmt"
	"net"
	"testing"
)

func TestFreePortReturnsReusableLoopbackPort(t *testing.T) {
	port, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("FreePort returned non-positive port %d", port)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen on returned port %d: %v", port, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
}
