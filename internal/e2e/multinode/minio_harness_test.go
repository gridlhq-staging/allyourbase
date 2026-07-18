//go:build multinode

package multinode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/storage"
)

func TestMinIOHarnessCreatesBucketForS3Backend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	harness := StartMinIOHarness(ctx, t)
	defer harness.Cleanup()

	if harness.Endpoint == "" {
		t.Fatal("harness endpoint is empty")
	}
	if harness.Bucket == "" {
		t.Fatal("harness bucket is empty")
	}
	if harness.AccessKey == "" {
		t.Fatal("harness access key is empty")
	}
	if harness.SecretKey == "" {
		t.Fatal("harness secret key is empty")
	}
	if harness.UseSSL {
		t.Fatal("harness should use plain HTTP for local MinIO")
	}

	_, err := storage.NewS3Backend(ctx, harness.S3Config())
	if err != nil {
		t.Fatalf("NewS3Backend with harness bucket: %v", err)
	}

	harness.Cleanup()
	assertDockerContainerRemoved(ctx, t, harness.containerID)
}

func assertDockerContainerRemoved(ctx context.Context, t *testing.T, containerID string) {
	t.Helper()

	output, err := exec.CommandContext(ctx, dockerBinary(), "inspect", "-f", "{{.State.Running}}", containerID).CombinedOutput()
	if err == nil {
		t.Fatalf("container %s still exists after cleanup; docker inspect output: %s", containerID, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(strings.ToLower(string(output)), "no such object") {
		t.Fatalf("checking cleanup for container %s: %v\noutput:\n%s", containerID, err, strings.TrimSpace(string(output)))
	}
}

func TestRunMinIOContainerRetriesPortBindConflictAndCleansFailedContainer(t *testing.T) {
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
	containerID, port := runMinIOContainer(ctx, t, dockerPath)
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
