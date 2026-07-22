package testutil

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	minIOImage     = "minio/minio:RELEASE.2025-09-07T16-13-09Z"
	minIOAccessKey = "aybminio"
	minIOSecretKey = "aybminiosecret"
	minIORegion    = "us-east-1"
)

// MinIOHarnessOptions names the caller-owned resources created for one
// isolated MinIO integration-test invocation.
type MinIOHarnessOptions struct {
	ContainerName string
	Bucket        string
}

// MinIOHarness describes a running isolated S3-compatible test service.
type MinIOHarness struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool

	dockerBinary string
	containerID  string
	closeOnce    sync.Once
	closeErr     error
}

// StartMinIOHarness starts the repository-pinned MinIO image and creates the
// caller's isolated bucket. The caller must close the returned harness.
func StartMinIOHarness(ctx context.Context, options MinIOHarnessOptions) (*MinIOHarness, error) {
	if strings.TrimSpace(options.ContainerName) == "" || strings.TrimSpace(options.Bucket) == "" {
		return nil, fmt.Errorf("minio harness requires non-empty container and bucket names")
	}

	dockerBinary := minIODockerBinary()
	containerID, port, err := startMinIOContainer(ctx, dockerBinary, options)
	if err != nil {
		return nil, err
	}
	harness := &MinIOHarness{
		Endpoint:     fmt.Sprintf("127.0.0.1:%d", port),
		Bucket:       options.Bucket,
		AccessKey:    minIOAccessKey,
		SecretKey:    minIOSecretKey,
		Region:       minIORegion,
		UseSSL:       false,
		dockerBinary: dockerBinary,
		containerID:  containerID,
	}
	if err := waitForMinIOReady(ctx, harness.Endpoint); err != nil {
		_ = harness.Close(context.Background())
		return nil, err
	}
	if err := createMinIOBucket(ctx, harness); err != nil {
		_ = harness.Close(context.Background())
		return nil, err
	}
	return harness, nil
}

// Close removes only the container started for this harness invocation.
func (h *MinIOHarness) Close(ctx context.Context) error {
	h.closeOnce.Do(func() {
		args := minIOCleanupArgs(h.containerID)
		output, err := exec.CommandContext(ctx, h.dockerBinary, args...).CombinedOutput()
		if err != nil {
			h.closeErr = fmt.Errorf("minio harness: %s %s failed: %w: %s",
				h.dockerBinary, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	})
	return h.closeErr
}

func minIODockerBinary() string {
	if dockerBinary := strings.TrimSpace(os.Getenv("DOCKER_BIN")); dockerBinary != "" {
		return dockerBinary
	}
	return "docker"
}

func startMinIOContainer(ctx context.Context, dockerBinary string, options MinIOHarnessOptions) (string, int, error) {
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		port, err := FreePort()
		if err != nil {
			return "", 0, fmt.Errorf("minio harness: allocate host port: %w", err)
		}
		args := minIOContainerArgs(options, port, attempt)
		output, runErr := exec.CommandContext(ctx, dockerBinary, args...).CombinedOutput()
		if runErr == nil {
			containerID := strings.TrimSpace(string(output))
			if containerID == "" {
				return "", 0, fmt.Errorf("minio harness: docker run returned empty container ID")
			}
			return containerID, port, nil
		}

		containerName := options.ContainerName + fmt.Sprintf("-%d", attempt)
		_, _ = exec.CommandContext(ctx, dockerBinary, minIOCleanupArgs(containerName)...).CombinedOutput()
		portCollision := strings.Contains(strings.ToLower(string(output)), "address already in use")
		if !portCollision || attempt == maxAttempts || ctx.Err() != nil {
			return "", 0, fmt.Errorf("minio harness: %s %s failed: %w: %s",
				dockerBinary, strings.Join(args, " "), runErr, strings.TrimSpace(string(output)))
		}
	}
	return "", 0, fmt.Errorf("minio harness: exhausted container start attempts")
}

func minIOContainerArgs(options MinIOHarnessOptions, port, attempt int) []string {
	return []string{
		"run", "-d", "--name", options.ContainerName + fmt.Sprintf("-%d", attempt),
		"-p", fmt.Sprintf("127.0.0.1:%d:9000", port),
		"-e", "MINIO_ROOT_USER=" + minIOAccessKey,
		"-e", "MINIO_ROOT_PASSWORD=" + minIOSecretKey,
		minIOImage,
		"server", "/data", "--address", ":9000",
	}
}

func minIOCleanupArgs(containerID string) []string {
	return []string{"rm", "-f", containerID}
}

func waitForMinIOReady(ctx context.Context, endpoint string) error {
	deadline := time.Now().Add(45 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + endpoint + "/minio/health/ready"
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("minio harness: build health request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("health status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("minio harness: health probe canceled for %s: %w", url, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("minio harness: health probe for %s did not become ready: %w", url, lastErr)
}

func createMinIOBucket(ctx context.Context, harness *MinIOHarness) error {
	client, err := minio.New(harness.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(harness.AccessKey, harness.SecretKey, ""),
		Secure: harness.UseSSL,
		Region: harness.Region,
	})
	if err != nil {
		return fmt.Errorf("minio harness: create client for %s: %w", harness.Endpoint, err)
	}
	if err := client.MakeBucket(ctx, harness.Bucket, minio.MakeBucketOptions{Region: harness.Region}); err != nil {
		return fmt.Errorf("minio harness: create bucket %q: %w", harness.Bucket, err)
	}
	return nil
}
