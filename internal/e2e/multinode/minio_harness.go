//go:build multinode

package multinode

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

type MinIOHarness struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool
	Cleanup   func()
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

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	shared, err := testutil.StartMinIOHarness(ctx, testutil.MinIOHarnessOptions{
		ContainerName: "ayb-multinode-" + runID,
		Bucket:        "ayb-multinode-" + runID,
	})
	if err != nil {
		t.Fatalf("start MinIO harness: %v", err)
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := shared.Close(cleanupCtx); err != nil {
			t.Logf("stop MinIO harness: %v", err)
		}
	}
	t.Cleanup(cleanup)

	harness := &MinIOHarness{
		Endpoint:  shared.Endpoint,
		Bucket:    shared.Bucket,
		AccessKey: shared.AccessKey,
		SecretKey: shared.SecretKey,
		Region:    shared.Region,
		UseSSL:    shared.UseSSL,
		Cleanup:   cleanup,
	}
	if _, err := storage.NewS3Backend(ctx, harness.S3Config()); err != nil {
		t.Fatalf("verify storage.NewS3Backend against bucket %q: %v", harness.Bucket, err)
	}
	return harness
}
