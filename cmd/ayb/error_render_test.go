package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/cli"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTopLevelErrorRendersCommandSuggestions(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		suggestion string
	}{
		{name: "init", args: []string{"init"}, suggestion: "ayb init my-app"},
		{name: "migrate", args: []string{"migrate", "create"}, suggestion: "ayb migrate create add_posts_table"},
		{name: "db", args: []string{"db", "restore"}, suggestion: "ayb db restore backup.dump"},
		{name: "demo", args: []string{"demo"}, suggestion: "ayb demo kanban"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Chdir(t.TempDir())

			originalArgs := os.Args
			t.Cleanup(func() { os.Args = originalArgs })
			os.Args = append([]string{"ayb"}, test.args...)

			err := cli.Execute()
			testutil.NotNil(t, err)

			output := renderTopLevelError(err)
			testutil.Equal(t, 1, strings.Count(output, "Error:"))
			testutil.Contains(t, output, "Try:")
			testutil.Contains(t, output, test.suggestion)
		})
	}
}

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

func TestBuiltBinaryMissingInitArgumentExitsWithGuidance(t *testing.T) {
	buildContext, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()

	binaryPath := filepath.Join(t.TempDir(), "ayb")
	build := exec.CommandContext(buildContext, "go", "build", "-o", binaryPath, "./cmd/ayb")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build ./cmd/ayb: %v\n%s", err, output)
	}

	runContext, cancelRun := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRun()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := exec.CommandContext(runContext, binaryPath, "init")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "HOME="+t.TempDir())
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("expected non-zero process exit, got %v", err)
	}
	testutil.Equal(t, 1, exitError.ExitCode())
	testutil.Equal(t, "", stdout.String())
	testutil.Equal(t, 1, strings.Count(stderr.String(), "Error:"))
	testutil.Contains(t, stderr.String(), "Try:")
	testutil.Contains(t, stderr.String(), "ayb init my-app")
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
