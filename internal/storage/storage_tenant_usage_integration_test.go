//go:build integration

package storage_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	tenantUsageUserID = "34343434-3434-3434-3434-343434343434"
	tenantUsageT1     = "tenant-usage-t1"
	tenantUsageT2     = "tenant-usage-t2"
)

type storageUsageRow struct {
	tenantID  string
	userID    string
	bytesUsed int64
}

func setupTenantUsageStorage(t *testing.T, quotaBytes int64) *storage.Service {
	t.Helper()
	ctx := context.Background()
	logger := testutil.DiscardLogger()

	runner := migrations.NewRunner(sharedPG.Pool, logger)
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx, "TRUNCATE _ayb_storage_uploads, _ayb_storage_objects, _ayb_storage_buckets, _ayb_storage_usage")
	testutil.NoError(t, err)
	ensureStorageTestUser(t, tenantUsageUserID, "tenant-usage@example.com")

	backend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	return storage.NewService(sharedPG.Pool, backend, "test-sign-key-at-least-32-chars!!", logger, quotaBytes)
}

func tenantUsageContext(tenantID string) context.Context {
	return tenant.ContextWithTenantID(context.Background(), tenantID)
}

func readStorageUsageRows(t *testing.T) []storageUsageRow {
	t.Helper()

	rows, err := sharedPG.Pool.Query(context.Background(),
		`SELECT tenant_id, user_id::text, bytes_used
		 FROM _ayb_storage_usage
		 ORDER BY tenant_id, user_id`)
	testutil.NoError(t, err)
	defer rows.Close()

	got := make([]storageUsageRow, 0)
	for rows.Next() {
		var row storageUsageRow
		testutil.NoError(t, rows.Scan(&row.tenantID, &row.userID, &row.bytesUsed))
		got = append(got, row)
	}
	testutil.NoError(t, rows.Err())
	return got
}

func assertStorageUsageRows(t *testing.T, want []storageUsageRow) {
	t.Helper()

	got := readStorageUsageRows(t)
	testutil.True(t, reflect.DeepEqual(want, got), "storage usage rows mismatch: want %#v got %#v", want, got)
}

func TestStorageUsageIsolatedPerTenant(t *testing.T) {
	storageSvc := setupTenantUsageStorage(t, 0)

	testutil.NoError(t, storageSvc.IncrementUsage(tenantUsageContext(tenantUsageT1), tenantUsageUserID, 1000))
	testutil.NoError(t, storageSvc.IncrementUsage(tenantUsageContext(tenantUsageT2), tenantUsageUserID, 512))

	assertStorageUsageRows(t, []storageUsageRow{
		{tenantID: tenantUsageT1, userID: tenantUsageUserID, bytesUsed: 1000},
		{tenantID: tenantUsageT2, userID: tenantUsageUserID, bytesUsed: 512},
	})

	t1Usage, err := storageSvc.GetTenantUsage(context.Background(), tenantUsageT1)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(1000), t1Usage)

	t2Usage, err := storageSvc.GetTenantUsage(context.Background(), tenantUsageT2)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(512), t2Usage)
}

func TestReserveQuotaTenantScoped(t *testing.T) {
	storageSvc := setupTenantUsageStorage(t, 1000)

	testutil.NoError(t, storageSvc.ReserveQuota(tenantUsageContext(tenantUsageT1), tenantUsageUserID, 1000))
	testutil.NoError(t, storageSvc.ReserveQuota(tenantUsageContext(tenantUsageT2), tenantUsageUserID, 512))

	err := storageSvc.ReserveQuota(tenantUsageContext(tenantUsageT1), tenantUsageUserID, 1)
	testutil.True(t, errors.Is(err, storage.ErrQuotaExceeded), "expected T1 quota exceeded, got %v", err)
	testutil.NoError(t, storageSvc.ReserveQuota(tenantUsageContext(tenantUsageT2), tenantUsageUserID, 488))

	assertStorageUsageRows(t, []storageUsageRow{
		{tenantID: tenantUsageT1, userID: tenantUsageUserID, bytesUsed: 1000},
		{tenantID: tenantUsageT2, userID: tenantUsageUserID, bytesUsed: 1000},
	})
}

func TestDecrementUsageTenantScoped(t *testing.T) {
	storageSvc := setupTenantUsageStorage(t, 0)

	testutil.NoError(t, storageSvc.IncrementUsage(tenantUsageContext(tenantUsageT1), tenantUsageUserID, 1000))
	testutil.NoError(t, storageSvc.IncrementUsage(tenantUsageContext(tenantUsageT2), tenantUsageUserID, 512))
	testutil.NoError(t, storageSvc.DecrementUsage(tenantUsageContext(tenantUsageT1), tenantUsageUserID, 1000))

	assertStorageUsageRows(t, []storageUsageRow{
		{tenantID: tenantUsageT1, userID: tenantUsageUserID, bytesUsed: 0},
		{tenantID: tenantUsageT2, userID: tenantUsageUserID, bytesUsed: 512},
	})

	t1Usage, err := storageSvc.GetTenantUsage(context.Background(), tenantUsageT1)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), t1Usage)

	t2Usage, err := storageSvc.GetTenantUsage(context.Background(), tenantUsageT2)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(512), t2Usage)
}
