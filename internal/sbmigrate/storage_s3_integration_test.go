//go:build integration

package sbmigrate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestE2E_StorageMigrationFromS3(t *testing.T) {
	ctx := context.Background()
	fixtures := []storageMigrationFixture{
		{
			sourceBucketID:   "uploads",
			sourceBucketName: "uploads",
			normalizedBucket: "uploads",
			public:           true,
			objectName:       "images/photo.txt",
			contentType:      "text/plain",
			size:             12,
			payload:          []byte("alpha-upload"),
		},
		{
			sourceBucketID:   "project-assets",
			sourceBucketName: "project.assets.2026",
			normalizedBucket: "project-assets-2026",
			public:           false,
			objectName:       "nested/assets/logo.svg",
			contentType:      "image/svg+xml",
			size:             18,
			payload:          []byte("project-asset-2026"),
		},
		{
			sourceBucketID:   "empty-assets",
			sourceBucketName: "empty.assets.2026",
			normalizedBucket: "empty-assets-2026",
			public:           true,
		},
	}

	harness, err := testutil.StartMinIOHarness(ctx, testutil.MinIOHarnessOptions{
		ContainerName: "ayb-sbmigrate-s3",
		Bucket:        fixtures[0].sourceBucketName,
	})
	if err != nil {
		if minIOHarnessUnavailable(err) {
			t.Skipf("MinIO harness unavailable: %v", err)
		}
		t.Fatalf("StartMinIOHarness: %v", err)
	}
	defer func() {
		testutil.NoError(t, harness.Close(context.Background()))
	}()

	client, err := minio.New(harness.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(harness.AccessKey, harness.SecretKey, ""),
		Secure: harness.UseSSL,
		Region: harness.Region,
	})
	testutil.NoError(t, err)
	createMissingS3SourceBuckets(t, ctx, client, harness.Region, fixtures[1:])
	uploadStorageFixturesToS3(t, ctx, client, fixtures)

	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	for _, bucket := range storageMigrationBuckets(fixtures) {
		insertStorageBucket(t, sharedPG.Pool, bucket.id, bucket.name, bucket.public, bucket.objects)
	}

	tmpStorage := t.TempDir()
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:          connStr,
		TargetURL:          connStr,
		SkipData:           true,
		SkipRLS:            true,
		SkipOAuth:          true,
		StoragePath:        tmpStorage,
		StorageS3Endpoint:  "http://" + harness.Endpoint,
		StorageS3Region:    harness.Region,
		StorageS3AccessKey: harness.AccessKey,
		StorageS3SecretKey: harness.SecretKey,
		StorageS3UseSSL:    harness.UseSSL,
		StorageExportPath:  "",
		Verbose:            true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, storageMigrationObjectCount(fixtures), stats.StorageFiles)
	testutil.Equal(t, int64(30), stats.StorageBytes)
	assertMigratedStorageBuckets(t, fixtures)
	assertMigratedStorageObjects(t, fixtures)
	assertMigratedStorageDownloads(t, ctx, tmpStorage, fixtures)
}

func minIOHarnessUnavailable(err error) bool {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "executable file not found") ||
		strings.Contains(message, "cannot connect to the docker daemon") ||
		strings.Contains(message, "could not connect to docker") ||
		strings.Contains(message, "is the docker daemon running") ||
		strings.Contains(message, "no such host") {
		return true
	}
	var netErr net.Error
	return strings.Contains(message, "connection refused") || strings.Contains(message, "connection reset") ||
		strings.Contains(message, "operation timed out") || strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "no route to host") || strings.Contains(message, "network is unreachable") ||
		strings.Contains(message, "permission denied") || strings.Contains(message, "no space left on device") ||
		strings.Contains(message, "docker: error response from daemon") && strings.Contains(message, "failed to create") ||
		strings.Contains(message, "docker: cannot connect") || strings.Contains(message, "colima") ||
		strings.Contains(message, "docker daemon") || strings.Contains(message, "docker endpoint") ||
		strings.Contains(message, "docker host") || strings.Contains(message, "docker context") ||
		strings.Contains(message, "docker socket") || strings.Contains(message, "container start") ||
		strings.Contains(message, "failed to start") || strings.Contains(message, "health probe") ||
		strings.Contains(message, "pull access denied") || strings.Contains(message, "image pull") ||
		strings.Contains(message, "server misbehaving") || strings.Contains(message, "temporary failure") ||
		strings.Contains(message, "lookup ") || (errors.As(err, &netErr) && netErr.Timeout())
}

func createMissingS3SourceBuckets(
	t *testing.T,
	ctx context.Context,
	client *minio.Client,
	region string,
	fixtures []storageMigrationFixture,
) {
	t.Helper()
	created := map[string]bool{}
	for _, fixture := range fixtures {
		if created[fixture.sourceBucketName] {
			continue
		}
		testutil.NoError(t, client.MakeBucket(ctx, fixture.sourceBucketName, minio.MakeBucketOptions{
			Region: region,
		}))
		created[fixture.sourceBucketName] = true
	}
}

func uploadStorageFixturesToS3(
	t *testing.T,
	ctx context.Context,
	client *minio.Client,
	fixtures []storageMigrationFixture,
) {
	t.Helper()
	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		testutil.Equal(t, fixture.size, len(fixture.payload))
		_, err := client.PutObject(
			ctx,
			fixture.sourceBucketName,
			fixture.objectName,
			bytes.NewReader(fixture.payload),
			int64(len(fixture.payload)),
			minio.PutObjectOptions{ContentType: fixture.contentType},
		)
		testutil.NoError(t, err)
		obj, err := client.GetObject(ctx, fixture.sourceBucketName, fixture.objectName, minio.GetObjectOptions{})
		testutil.NoError(t, err)
		got, readErr := io.ReadAll(obj)
		closeErr := obj.Close()
		testutil.NoError(t, readErr)
		testutil.NoError(t, closeErr)
		testutil.Equal(t, string(fixture.payload), string(got))
	}
}
