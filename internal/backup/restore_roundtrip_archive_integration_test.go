//go:build integration

package backup

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

type restoreRoundTripArchiveRuntime struct {
	store      *S3Store
	config     PITRConfig
	projectID  string
	databaseID string
}

func requireRoundTripArchiveRuntime(t *testing.T, ctx context.Context, databaseURL string) restoreRoundTripArchiveRuntime {
	t.Helper()

	configPath := strings.TrimSpace(os.Getenv(testutil.BackupConfigPathEnv))
	if configPath == "" {
		t.Fatalf("%s is required for automatic WAL archival", testutil.BackupConfigPathEnv)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read test-local backup config: %v", err)
	}
	runtimeConfig, err := config.ParseTOML(data)
	if err != nil {
		t.Fatalf("parse test-local backup config: %v", err)
	}
	if runtimeConfig.Database.URL != databaseURL {
		t.Fatalf("test-local backup config database URL = %q, TEST_DATABASE_URL = %q", runtimeConfig.Database.URL, databaseURL)
	}
	projectID := strings.TrimSpace(os.Getenv(testutil.ArchiveProjectIDEnv))
	if projectID == "" {
		t.Fatalf("%s is required for automatic WAL archival metadata", testutil.ArchiveProjectIDEnv)
	}
	databaseID := databaseNameFromRoundTripURL(t, databaseURL)
	store, err := NewS3Store(ctx, S3Config{
		Endpoint:   runtimeConfig.Backup.Endpoint,
		Bucket:     runtimeConfig.Backup.PITR.ArchiveBucket,
		Region:     runtimeConfig.Backup.Region,
		AccessKey:  runtimeConfig.Backup.AccessKey,
		SecretKey:  runtimeConfig.Backup.SecretKey,
		Encryption: runtimeConfig.Backup.Encryption,
		KMSKeyID:   runtimeConfig.Backup.PITR.KMSKeyID,
		UseSSL:     runtimeConfig.Backup.UseSSL,
	})
	if err != nil {
		t.Fatalf("create production S3 store from test-local config: %v", err)
	}
	return restoreRoundTripArchiveRuntime{
		store: store,
		config: PITRConfig{
			Enabled:          runtimeConfig.Backup.PITR.Enabled,
			ArchiveBucket:    runtimeConfig.Backup.PITR.ArchiveBucket,
			ArchivePrefix:    runtimeConfig.Backup.PITR.ArchivePrefix,
			EnvironmentClass: runtimeConfig.Backup.PITR.EnvironmentClass,
			KMSKeyID:         runtimeConfig.Backup.PITR.KMSKeyID,
			ShadowMode:       runtimeConfig.Backup.PITR.ShadowMode,
		},
		projectID:  projectID,
		databaseID: databaseID,
	}
}

func databaseNameFromRoundTripURL(t *testing.T, databaseURL string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	databaseName := strings.Trim(parsed.Path, "/")
	if databaseName == "" {
		t.Fatal("TEST_DATABASE_URL does not name a database")
	}
	return databaseName
}
