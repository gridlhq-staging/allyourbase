//go:build multinode

package multinode

import (
	"context"
	"os/exec"
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
	assertDockerContainerRemoved(ctx, t, harness.dockerBinary, harness.containerID)
}

func assertDockerContainerRemoved(ctx context.Context, t *testing.T, dockerBinary, containerID string) {
	t.Helper()

	output, err := exec.CommandContext(ctx, dockerBinary, "inspect", "-f", "{{.State.Running}}", containerID).CombinedOutput()
	if err == nil {
		t.Fatalf("container %s still exists after cleanup; docker inspect output: %s", containerID, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(strings.ToLower(string(output)), "no such object") {
		t.Fatalf("checking cleanup for container %s: %v\noutput:\n%s", containerID, err, strings.TrimSpace(string(output)))
	}
}
