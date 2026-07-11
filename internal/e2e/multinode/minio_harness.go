//go:build multinode

package multinode

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	minioImage     = "minio/minio:RELEASE.2025-09-07T16-13-09Z"
	minioAccessKey = "aybminio"
	minioSecretKey = "aybminiosecret"
	minioRegion    = "us-east-1"
)

type MinIOHarness struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool
	Cleanup   func()

	containerID string
}

func (h MinIOHarness) S3Config() storage.S3Config {
	return storage.S3Config{
		Endpoint:  h.Endpoint,
		Bucket:    h.Bucket,
		Region:    h.Region,
		AccessKey: h.AccessKey,
		SecretKey: h.SecretKey,
		UseSSL:    h.UseSSL,
	}
}

func StartMinIOHarness(ctx context.Context, t *testing.T) *MinIOHarness {
	t.Helper()

	port, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("minio harness: allocate host port: %v", err)
	}

	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	bucket := fmt.Sprintf("ayb-multinode-%d", time.Now().UnixNano())
	dockerBin := dockerBinary()

	// Operator equivalents:
	// DOCKER_BIN="${DOCKER_BIN:-docker}"
	// DOCKER_HOST=unix:///Users/stuart/.colima/codex/docker.sock
	// docker run -d -p 127.0.0.1:<FreePort()>:9000 ...
	// docker rm -f <container>
	containerID := runMinIOContainer(ctx, t, dockerBin, port)
	cleanup := minioCleanup(t, dockerBin, containerID)
	t.Cleanup(cleanup)

	waitForMinIOReady(ctx, t, endpoint)
	createMinIOBucket(ctx, t, endpoint, bucket)
	if _, err := storage.NewS3Backend(ctx, storage.S3Config{
		Endpoint:  endpoint,
		Bucket:    bucket,
		Region:    minioRegion,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		UseSSL:    false,
	}); err != nil {
		t.Fatalf("minio harness: verify storage.NewS3Backend against bucket %q: %v", bucket, err)
	}

	return &MinIOHarness{
		Endpoint:  endpoint,
		Bucket:    bucket,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		Region:    minioRegion,
		UseSSL:    false,
		Cleanup:   cleanup,

		containerID: containerID,
	}
}

func dockerBinary() string {
	dockerBin := os.Getenv("DOCKER_BIN")
	if dockerBin == "" {
		return "docker"
	}
	return dockerBin
}

func runMinIOContainer(ctx context.Context, t *testing.T, dockerBin string, port int) string {
	t.Helper()

	args := []string{
		"run", "-d",
		"-p", fmt.Sprintf("127.0.0.1:%d:9000", port),
		"-e", "MINIO_ROOT_USER=" + minioAccessKey,
		"-e", "MINIO_ROOT_PASSWORD=" + minioSecretKey,
		minioImage,
		"server", "/data", "--address", ":9000",
	}
	output, err := exec.CommandContext(ctx, dockerBin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("minio harness: docker run with %q failed: %v\ncommand: %s %s\noutput:\n%s",
			dockerBin, err, dockerBin, strings.Join(args, " "), strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		t.Fatalf("minio harness: docker run with %q returned empty container ID", dockerBin)
	}
	return containerID
}

func minioCleanup(t *testing.T, dockerBin, containerID string) func() {
	t.Helper()

	var once sync.Once
	return func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			args := []string{"rm", "-f", containerID}
			output, err := exec.CommandContext(ctx, dockerBin, args...).CombinedOutput()
			if err != nil {
				t.Logf("minio harness: docker rm -f with %q failed: %v\ncommand: %s %s\noutput:\n%s",
					dockerBin, err, dockerBin, strings.Join(args, " "), strings.TrimSpace(string(output)))
			}
		})
	}
}

func waitForMinIOReady(ctx context.Context, t *testing.T, endpoint string) {
	t.Helper()

	deadline := time.Now().Add(45 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + endpoint + "/minio/health/ready"
	var lastErr error

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("minio harness: build health request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("health status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			t.Fatalf("minio harness: health probe canceled for %s: %v", url, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatalf("minio harness: health probe for %s did not become ready before deadline: %v", url, lastErr)
}

func createMinIOBucket(ctx context.Context, t *testing.T, endpoint, bucket string) {
	t.Helper()

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
		Region: minioRegion,
	})
	if err != nil {
		t.Fatalf("minio harness: create MinIO client for %s: %v", endpoint, err)
	}

	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: minioRegion}); err != nil {
		t.Fatalf("minio harness: create bucket %q on %s: %v", bucket, endpoint, err)
	}
}
