package main

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/cli"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTopLevelErrorRendersPortErrorPrefixOnce(t *testing.T) {
	port, closePort := occupyPort(t)
	defer closePort()
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"ayb", "start", "--port", strconv.Itoa(port)}

	err := cli.Execute()
	testutil.NotNil(t, err)

	out := renderTopLevelError(err)
	testutil.Equal(t, 1, strings.Count(out, "Error:"))
	testutil.Contains(t, out, "Try:")
	testutil.Contains(t, out, "ayb start --port "+strconv.Itoa(port+1))
	testutil.Contains(t, out, "ayb stop")
}

func occupyPort(t *testing.T) (int, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	testutil.NoError(t, err)

	port := ln.Addr().(*net.TCPAddr).Port
	return port, func() {
		testutil.NoError(t, ln.Close())
	}
}
