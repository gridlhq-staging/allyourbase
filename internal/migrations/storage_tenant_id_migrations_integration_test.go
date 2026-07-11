//go:build integration

package migrations_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	legacyStorageBucketName = "legacy-bucket"
	legacyStorageObjectName = "legacy-object.txt"
	legacyStorageUploadPath = "tmp/uploads/legacy"
	legacyStorageUsageUser  = "11111111-1111-1111-1111-111111111111"
)

func TestStorageTenantIdMigrationContract(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh install", func(t *testing.T) {
		resetDB(t, ctx)
		runFullMigrations(t, ctx)

		assertStorageTenantColumns(t, ctx)
		assertStorageTenantUniqueKeys(t, ctx)
		assertStorageTenantLookupIndexes(t, ctx)
	})

	t.Run("legacy upgrade", func(t *testing.T) {
		resetDB(t, ctx)
		runLegacyStorageMigrations(t, ctx)
		seedLegacyStorageRows(t, ctx)

		runFullMigrations(t, ctx)

		assertStorageTenantColumns(t, ctx)
		assertLegacyStorageRowsPreserved(t, ctx)
		assertStorageTenantUniqueKeys(t, ctx)
		assertStorageTenantLookupIndexes(t, ctx)
	})
}

func runFullMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

func runLegacyStorageMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunnerWithFS(sharedPG.Pool, testutil.DiscardLogger(), migrationFSUpTo(t, 177))
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

func migrationFSUpTo(t *testing.T, maxNumber int) fstest.MapFS {
	t.Helper()

	entries, err := os.ReadDir("sql")
	testutil.NoError(t, err)

	filtered := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if migrationNumber(t, entry.Name()) > maxNumber {
			continue
		}

		path := "sql/" + entry.Name()
		data, err := os.ReadFile(path)
		testutil.NoError(t, err)
		filtered[path] = &fstest.MapFile{Data: data}
	}

	return filtered
}

func migrationNumber(t *testing.T, name string) int {
	t.Helper()

	prefix := strings.SplitN(name, "_", 2)[0]
	number, err := strconv.Atoi(prefix)
	testutil.NoError(t, err)
	return number
}

func seedLegacyStorageRows(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_storage_buckets (name, public)
		 VALUES ($1, true)`,
		legacyStorageBucketName,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_storage_objects (bucket, name, size, content_type)
		 VALUES ($1, $2, 42, 'text/plain')`,
		legacyStorageBucketName, legacyStorageObjectName,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_storage_uploads (
		     bucket, name, path, content_type, total_size, uploaded_size, expires_at
		 )
		 VALUES ($1, $2, $3, 'text/plain', 100, 25, NOW() + INTERVAL '1 hour')`,
		legacyStorageBucketName, legacyStorageObjectName, legacyStorageUploadPath,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_users (id, email, password_hash)
		 VALUES ($1, 'legacy-storage-usage@example.com', 'hash')`,
		legacyStorageUsageUser,
	)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_storage_usage (user_id, bytes_used, updated_at)
		 VALUES ($1, 2048, NOW())`,
		legacyStorageUsageUser,
	)
	testutil.NoError(t, err)
}

func assertStorageTenantColumns(t *testing.T, ctx context.Context) {
	t.Helper()

	for _, tableName := range []string{
		"_ayb_storage_objects",
		"_ayb_storage_buckets",
		"_ayb_storage_uploads",
		"_ayb_storage_usage",
	} {
		assertTenantColumn(t, ctx, tableName)
	}
}

func assertTenantColumn(t *testing.T, ctx context.Context, tableName string) {
	t.Helper()

	var dataType string
	var nullable bool
	var defaultExpression string
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT c.data_type,
		        c.is_nullable = 'YES',
		        pg_get_expr(d.adbin, d.adrelid)
		 FROM information_schema.columns c
		 JOIN pg_class tbl ON tbl.relname = c.table_name
		 JOIN pg_namespace n ON n.oid = tbl.relnamespace
		 JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attname = c.column_name
		 JOIN pg_attrdef d ON d.adrelid = tbl.oid AND d.adnum = a.attnum
		 WHERE c.table_schema = 'public'
		   AND c.table_name = $1
		   AND c.column_name = 'tenant_id'
		   AND n.nspname = 'public'`,
		tableName,
	).Scan(&dataType, &nullable, &defaultExpression)
	testutil.NoError(t, err)
	testutil.Equal(t, "text", dataType)
	testutil.False(t, nullable, "%s.tenant_id should be NOT NULL", tableName)
	testutil.Equal(t, "''::text", defaultExpression)
}

func assertLegacyStorageRowsPreserved(t *testing.T, ctx context.Context) {
	t.Helper()

	assertLegacyTenantValue(t, ctx, "_ayb_storage_buckets", "name = $1", legacyStorageBucketName)
	assertLegacyTenantValue(t, ctx, "_ayb_storage_objects", "bucket = $1 AND name = $2",
		legacyStorageBucketName, legacyStorageObjectName)
	assertLegacyTenantValue(t, ctx, "_ayb_storage_uploads", "bucket = $1 AND name = $2 AND path = $3",
		legacyStorageBucketName, legacyStorageObjectName, legacyStorageUploadPath)
	assertLegacyTenantValue(t, ctx, "_ayb_storage_usage", "user_id = $1", legacyStorageUsageUser)

	var bytesUsed int64
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = '' AND user_id = $1`,
		legacyStorageUsageUser,
	).Scan(&bytesUsed)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(2048), bytesUsed)
}

func assertLegacyTenantValue(t *testing.T, ctx context.Context, tableName string, predicate string, args ...any) {
	t.Helper()

	var tenantID string
	query := "SELECT tenant_id FROM " + tableName + " WHERE " + predicate
	err := sharedPG.Pool.QueryRow(ctx, query, args...).Scan(&tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, "", tenantID)
}

func assertStorageTenantUniqueKeys(t *testing.T, ctx context.Context) {
	t.Helper()

	assertUniqueKeyPresent(t, ctx, "_ayb_storage_objects", "tenant_id,bucket,name")
	assertUniqueKeyPresent(t, ctx, "_ayb_storage_buckets", "tenant_id,name")
	assertUniqueKeyPresent(t, ctx, "_ayb_storage_uploads", "tenant_id,bucket,name,path")
	assertUniqueKeyPresent(t, ctx, "_ayb_storage_usage", "tenant_id,user_id")

	assertUniqueKeyAbsent(t, ctx, "_ayb_storage_objects", "bucket,name")
	assertUniqueKeyAbsent(t, ctx, "_ayb_storage_buckets", "name")
	assertUniqueKeyAbsent(t, ctx, "_ayb_storage_uploads", "bucket,name,path")
	assertUniqueKeyAbsent(t, ctx, "_ayb_storage_usage", "user_id")
}

func assertUniqueKeyPresent(t *testing.T, ctx context.Context, tableName string, expectedKeys string) {
	t.Helper()

	testutil.True(t, uniqueKeyExists(t, ctx, tableName, expectedKeys),
		"%s should have unique key on %s", tableName, expectedKeys)
}

func assertUniqueKeyAbsent(t *testing.T, ctx context.Context, tableName string, expectedKeys string) {
	t.Helper()

	testutil.False(t, uniqueKeyExists(t, ctx, tableName, expectedKeys),
		"%s should not keep unique key on %s", tableName, expectedKeys)
}

func uniqueKeyExists(t *testing.T, ctx context.Context, tableName string, expectedKeys string) bool {
	t.Helper()

	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		     FROM (
		         SELECT string_agg(a.attname, ',' ORDER BY k.ordinality) AS keys
		         FROM pg_index ix
		         JOIN pg_class tbl ON tbl.oid = ix.indrelid
		         JOIN pg_namespace n ON n.oid = tbl.relnamespace
		         JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		         JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = k.attnum
		         WHERE n.nspname = 'public'
		           AND tbl.relname = $1
		           AND ix.indisunique
		         GROUP BY ix.indexrelid
		     ) unique_keys
		     WHERE keys = $2
		 )`,
		tableName, expectedKeys,
	).Scan(&exists)
	testutil.NoError(t, err)
	return exists
}

func assertStorageTenantLookupIndexes(t *testing.T, ctx context.Context) {
	t.Helper()

	assertIndexKeyPresent(t, ctx, "_ayb_storage_objects", "tenant_id,bucket")
	assertIndexKeyPresent(t, ctx, "_ayb_storage_uploads", "tenant_id,bucket,name")
	assertIndexKeyPresent(t, ctx, "_ayb_storage_usage", "tenant_id")
}

func assertIndexKeyPresent(t *testing.T, ctx context.Context, tableName string, expectedKeys string) {
	t.Helper()

	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		     FROM (
		         SELECT string_agg(a.attname, ',' ORDER BY k.ordinality) AS keys
		         FROM pg_index ix
		         JOIN pg_class tbl ON tbl.oid = ix.indrelid
		         JOIN pg_namespace n ON n.oid = tbl.relnamespace
		         JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		         JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = k.attnum
		         WHERE n.nspname = 'public'
		           AND tbl.relname = $1
		         GROUP BY ix.indexrelid
		     ) indexes
		     WHERE keys = $2
		 )`,
		tableName, expectedKeys,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "%s should have lookup index on %s", tableName, expectedKeys)
}
