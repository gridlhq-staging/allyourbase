package testutil

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMinIOContainerArgsUsePinnedImageAndIsolatedState(t *testing.T) {
	t.Parallel()

	options := MinIOHarnessOptions{
		ContainerName: "ayb-testpg-run-one",
		Bucket:        "ayb-testpg-run-one",
		DataDir:       "/tmp/ayb-testpg-run-one-data",
	}

	got := minIOContainerArgs(options, 19000, 2)
	want := []string{
		"run", "-d", "--name", "ayb-testpg-run-one-2",
		"-p", "127.0.0.1:19000:9000",
		"--mount", "type=bind,source=/tmp/ayb-testpg-run-one-data,target=/data",
		"-e", "MINIO_ROOT_USER=aybminio",
		"-e", "MINIO_ROOT_PASSWORD=aybminiosecret",
		"minio/minio:RELEASE.2025-09-07T16-13-09Z",
		"server", "/data", "--address", ":9000",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minIOContainerArgs() = %#v, want %#v", got, want)
	}
}

func TestMinIOCleanupArgsTargetOnlyStartedContainer(t *testing.T) {
	t.Parallel()

	first := minIOCleanupArgs("container-id-one")
	second := minIOCleanupArgs("container-id-two")
	if !reflect.DeepEqual(first, []string{"rm", "-f", "container-id-one"}) {
		t.Fatalf("first cleanup args = %#v", first)
	}
	if !reflect.DeepEqual(second, []string{"rm", "-f", "container-id-two"}) {
		t.Fatalf("second cleanup args = %#v", second)
	}
}

func TestStartMinIOContainerRetriesPortBindConflictAndCleansFailedContainer(t *testing.T) {
	tempDir := t.TempDir()
	dockerPath := filepath.Join(tempDir, "docker")
	statePath := filepath.Join(tempDir, "state")
	logPath := filepath.Join(tempDir, "commands.log")
	script := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
if [[ "$1" == "run" ]]; then
  count=0
  if [[ -f "$FAKE_DOCKER_STATE" ]]; then
    read -r count < "$FAKE_DOCKER_STATE"
  fi
  count=$((count + 1))
  printf '%s\n' "$count" > "$FAKE_DOCKER_STATE"
  if [[ "$count" -eq 1 ]]; then
    printf '%064d\n' 1
    echo 'docker: Error response from daemon: failed to bind host port: address already in use' >&2
    exit 125
  fi
  printf '%064d\n' 2
  exit 0
fi
if [[ "$1" == "rm" && "$2" == "-f" ]]; then
  exit 0
fi
exit 2
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("FAKE_DOCKER_STATE", statePath)
	t.Setenv("FAKE_DOCKER_LOG", logPath)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	containerID, port, err := startMinIOContainer(ctx, dockerPath, MinIOHarnessOptions{
		ContainerName: "ayb-minio-retry",
		Bucket:        "ayb-minio-retry",
	})
	if err != nil {
		t.Fatalf("startMinIOContainer returned error: %v", err)
	}
	if containerID != strings.Repeat("0", 63)+"2" {
		t.Fatalf("container ID = %q, want successful second-attempt ID", containerID)
	}
	if port == 0 {
		t.Fatal("selected port must be nonzero")
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(lines) != 3 {
		t.Fatalf("docker commands = %q, want first run, failed-container cleanup, second run", lines)
	}
	firstRunFields := strings.Fields(lines[0])
	var failedContainerName string
	for i, field := range firstRunFields {
		if field == "--name" && i+1 < len(firstRunFields) {
			failedContainerName = firstRunFields[i+1]
			break
		}
	}
	if failedContainerName == "" {
		t.Fatalf("first docker run did not assign an owned container name: %q", lines[0])
	}
	if lines[1] != "rm -f "+failedContainerName {
		t.Fatalf("failed-container cleanup = %q, want %q", lines[1], "rm -f "+failedContainerName)
	}
}
