//go:build integration

package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// setupSourceAndTarget resets the schema and creates both Supabase-like source schemas
// and AYB target tables within the same database (using separate schemas).
// Returns the connection string usable for both source and target.
func setupSourceAndTarget(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	// Reset everything.
	_, err := sharedPG.Pool.Exec(ctx, `
		DROP SCHEMA IF EXISTS public CASCADE;
		CREATE SCHEMA public;
		DROP SCHEMA IF EXISTS auth CASCADE;
		DROP SCHEMA IF EXISTS storage CASCADE;
	`)
	testutil.NoError(t, err)

	// Create Supabase-like auth schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE SCHEMA auth;

		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		CREATE TABLE auth.identities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES auth.users(id),
			provider TEXT NOT NULL,
			identity_data JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
	`)
	testutil.NoError(t, err)

	// Create Supabase-like storage schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE SCHEMA storage;

		CREATE TABLE storage.buckets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			public BOOLEAN NOT NULL DEFAULT false
		);

		CREATE TABLE storage.objects (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			bucket_id TEXT NOT NULL REFERENCES storage.buckets(id),
			name TEXT NOT NULL,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	testutil.NoError(t, err)

	// Create AYB target tables on public schema.
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_users_email ON _ayb_users (LOWER(email));

		CREATE TABLE IF NOT EXISTS _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
		CREATE INDEX IF NOT EXISTS idx_ayb_oauth_accounts_user_id ON _ayb_oauth_accounts (user_id);
	`)
	testutil.NoError(t, err)

	return sharedPG.ConnString
}

func setupTargetAYBSchema(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

// insertSourceUser inserts a user into auth.users with the given params.
func insertSourceUser(t *testing.T, pool *pgxpool.Pool, id, email, passwordHash string, confirmed bool, anonymous bool) {
	t.Helper()
	ctx := context.Background()

	var emailConfAt *time.Time
	if confirmed {
		now := time.Now()
		emailConfAt = &now
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at, is_anonymous)
		VALUES ($1, $2, $3, $4, $5)
	`, id, email, passwordHash, emailConfAt, anonymous)
	testutil.NoError(t, err)
}

// insertSourceIdentity inserts an OAuth identity into auth.identities.
func insertSourceIdentity(t *testing.T, pool *pgxpool.Pool, userID, provider, identityDataJSON string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO auth.identities (user_id, provider, identity_data)
		VALUES ($1, $2, $3::jsonb)
	`, userID, provider, identityDataJSON)
	testutil.NoError(t, err)
}

// insertSourceTable creates a public table and inserts rows (for schema+data migration tests).
func insertSourceTable(t *testing.T, pool *pgxpool.Pool, ddl string, inserts ...string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, ddl)
	testutil.NoError(t, err)
	for _, ins := range inserts {
		_, err = pool.Exec(ctx, ins)
		testutil.NoError(t, err)
	}
}

// insertStorageBucket creates a storage bucket and its objects.
func insertStorageBucket(t *testing.T, pool *pgxpool.Pool, id, name string, public bool, objects []struct {
	name, mime string
	size       int
}) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO storage.buckets (id, name, public) VALUES ($1, $2, $3)`, id, name, public)
	testutil.NoError(t, err)
	for _, o := range objects {
		_, err = pool.Exec(ctx, `
			INSERT INTO storage.objects (bucket_id, name, metadata)
			VALUES ($1, $2, $3::jsonb)
		`, id, o.name, `{"size": `+itoa(o.size)+`, "mimetype": "`+o.mime+`"}`)
		testutil.NoError(t, err)
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// createStorageExportDir creates a local directory mirroring the bucket/path structure.
func createStorageExportDir(t *testing.T, buckets map[string]map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for bucket, files := range buckets {
		for path, content := range files {
			fullPath := filepath.Join(dir, bucket, path)
			err := os.MkdirAll(filepath.Dir(fullPath), 0755)
			testutil.NoError(t, err)
			err = os.WriteFile(fullPath, content, 0644)
			testutil.NoError(t, err)
		}
	}
	return dir
}

func createIsolatedDatabaseURL(t *testing.T, adminConnStr, prefix string) string {
	t.Helper()

	adminDB, err := sql.Open("pgx", adminConnStr)
	testutil.NoError(t, err)
	defer adminDB.Close()

	dbName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	_, err = adminDB.Exec(`CREATE DATABASE ` + dbName)
	testutil.NoError(t, err)

	u, err := url.Parse(adminConnStr)
	testutil.NoError(t, err)
	u.Path = "/" + dbName
	return u.String()
}

func dropIsolatedDatabase(t *testing.T, adminConnStr, dbURL string) {
	t.Helper()

	u, err := url.Parse(dbURL)
	testutil.NoError(t, err)
	dbName := strings.TrimPrefix(u.Path, "/")

	adminDB, err := sql.Open("pgx", adminConnStr)
	testutil.NoError(t, err)
	defer adminDB.Close()

	_, err = adminDB.Exec(`DROP DATABASE IF EXISTS ` + dbName + ` WITH (FORCE)`)
	testutil.NoError(t, err)
}

// --- Tests ---

func TestE2E_FullMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	// Populate source data.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hashedpassword1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hashedpassword2", false, false)

	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "email",
		`{"sub": "aaaaaaaa-0000-0000-0000-000000000001", "email": "alice@example.com"}`)
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "g-12345", "email": "alice@gmail.com", "name": "Alice S"}`)

	// Create source tables with data.
	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			published BOOLEAN DEFAULT false
		)`,
		`INSERT INTO posts (title, body, published) VALUES ('First Post', 'Hello world', true)`,
		`INSERT INTO posts (title, body, published) VALUES ('Draft', 'WIP', false)`,
	)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			post_id INTEGER REFERENCES posts(id),
			text TEXT NOT NULL
		)`,
		`INSERT INTO comments (post_id, text) VALUES (1, 'Great post!')`,
	)

	// Create auth.uid() function so RLS policies referencing it can be created.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	// Create RLS policies on source.
	_, err = sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
		CREATE POLICY posts_select ON posts FOR SELECT USING (true);
		CREATE POLICY posts_insert ON posts FOR INSERT WITH CHECK (auth.uid() IS NOT NULL);
	`)
	testutil.NoError(t, err)

	// Storage.
	insertStorageBucket(t, sharedPG.Pool, "avatars", "avatars", true, []struct {
		name, mime string
		size       int
	}{
		{"photo.jpg", "image/jpeg", 14},
	})
	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"avatars": {"photo.jpg": []byte("fake-jpeg-data")},
	})
	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		Force:             true, // source and target are same DB
		Verbose:           true,
		StorageExportPath: storageExport,
		StoragePath:       tmpStorage,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Verify stats.
	// Source and target share the same DB, so existing rows are not recopied.
	testutil.Equal(t, 2, stats.Tables) // posts, comments (excludes _ayb_ tables)
	testutil.Equal(t, 0, stats.Records)
	testutil.Equal(t, 2, stats.Users)
	testutil.Equal(t, 1, stats.OAuthLinks) // google for alice (email provider skipped)
	testutil.Equal(t, 3, stats.Policies)   // two source policies plus the AYB fixture policy
	testutil.Equal(t, 1, stats.StorageFiles)
	testutil.True(t, stats.StorageBytes > 0)

	// Verify storage file.
	verifyFile(t, filepath.Join(tmpStorage, "avatars", "photo.jpg"), []byte("fake-jpeg-data"))
}

func TestE2E_SchemaAndData(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Create source with no auth users — skip OAuth/RLS for this test.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price NUMERIC(10,2),
			in_stock BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`INSERT INTO products (name, price) VALUES ('Widget', 9.99)`,
		`INSERT INTO products (name, price, in_stock) VALUES ('Gadget', 19.99, false)`,
		`INSERT INTO products (name, price) VALUES ('Doohickey', 29.99)`,
	)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipRLS:   true,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 1, stats.Tables)   // products (_ayb_ tables filtered)
	testutil.Equal(t, 0, stats.Records)  // 0: same-DB test, rows exist → ON CONFLICT DO NOTHING
	testutil.Equal(t, 1, stats.Users)    // admin
	testutil.Equal(t, 0, stats.Policies) // RLS skipped

	// Verify data exists (was already in source which shares same DB).
	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var name string
	var price float64
	err = db.QueryRow("SELECT name, price FROM products WHERE name = 'Widget'").Scan(&name, &price)
	testutil.NoError(t, err)
	testutil.Equal(t, "Widget", name)
	testutil.Equal(t, 9.99, price)
}

func TestE2E_AuthMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Insert various user types.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hash2", false, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000003", "", "", false, true) // anonymous — skipped
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000004", "oauth@example.com", "", true, false) // OAuth-only — gets $none$

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 3, stats.Users)   // alice, bob, oauth (anonymous filtered by query)
	testutil.Equal(t, 0, stats.Skipped) // anonymous filtered at SQL level, not counted as skip

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify alice is verified.
	var verified bool
	err = db.QueryRow(
		"SELECT email_verified FROM _ayb_users WHERE email = 'alice@example.com'",
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified)

	// Verify bob is not verified.
	err = db.QueryRow(
		"SELECT email_verified FROM _ayb_users WHERE email = 'bob@example.com'",
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.False(t, verified)

	// Verify OAuth-only user got $none$ password.
	var passwordHash string
	err = db.QueryRow(
		"SELECT password_hash FROM _ayb_users WHERE email = 'oauth@example.com'",
	).Scan(&passwordHash)
	testutil.NoError(t, err)
	testutil.Equal(t, "$none$", passwordHash)
}

func TestE2E_OAuthMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)

	// Email identity (should be skipped).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "email",
		`{"sub": "aaaaaaaa-0000-0000-0000-000000000001", "email": "alice@example.com"}`)
	// Google identity (should be imported).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "google-uid-123", "email": "alice@gmail.com", "name": "Alice"}`)
	// GitHub identity (should be imported).
	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "github",
		`{"sub": "github-uid-456", "email": "alice@github.com", "full_name": "Alice Dev"}`)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.OAuthLinks) // google + github (email skipped)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify google OAuth.
	var provider, providerUID, email, name string
	err = db.QueryRow(`
		SELECT provider, provider_user_id, email, name
		FROM _ayb_oauth_accounts
		WHERE provider = 'google'
	`).Scan(&provider, &providerUID, &email, &name)
	testutil.NoError(t, err)
	testutil.Equal(t, "google", provider)
	testutil.Equal(t, "google-uid-123", providerUID)
	testutil.Equal(t, "alice@gmail.com", email)
	testutil.Equal(t, "Alice", name)

	// Verify github OAuth.
	err = db.QueryRow(`
		SELECT provider, provider_user_id, email, name
		FROM _ayb_oauth_accounts
		WHERE provider = 'github'
	`).Scan(&provider, &providerUID, &email, &name)
	testutil.NoError(t, err)
	testutil.Equal(t, "github", provider)
	testutil.Equal(t, "github-uid-456", providerUID)
	testutil.Equal(t, "alice@github.com", email)
	testutil.Equal(t, "Alice Dev", name)
}

func TestE2E_RLSMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	// Need at least one auth user for migration to proceed.
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

	// Create source table with RLS policies that use Supabase auth functions.
	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE documents (
			id SERIAL PRIMARY KEY,
			owner_id UUID,
			title TEXT NOT NULL
		)`,
		`INSERT INTO documents (owner_id, title) VALUES ('aaaaaaaa-0000-0000-0000-000000000001', 'My Doc')`,
	)

	// Create an auth.uid() function stub so the policies can be created.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
	`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
		CREATE POLICY documents_select ON documents FOR SELECT USING (true);
		CREATE POLICY documents_update ON documents FOR UPDATE
			USING (owner_id = auth.uid())
			WITH CHECK (owner_id = auth.uid());
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipOAuth: true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, stats.Policies) // documents_select + documents_update

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	// Verify RLS is enabled on documents table.
	var rlsEnabled bool
	err = db.QueryRow(`
		SELECT relrowsecurity FROM pg_class WHERE relname = 'documents'
	`).Scan(&rlsEnabled)
	testutil.NoError(t, err)
	testutil.True(t, rlsEnabled)

	// Verify policies exist.
	var policyCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pg_policies WHERE tablename = 'documents'
	`).Scan(&policyCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, policyCount)

	// Verify the update policy's USING expression was rewritten.
	var policyDef string
	err = db.QueryRow(`
		SELECT pg_get_expr(pol.polqual, pol.polrelid)
		FROM pg_policy pol
		JOIN pg_class c ON c.oid = pol.polrelid
		WHERE c.relname = 'documents' AND pol.polname = 'documents_update'
	`).Scan(&policyDef)
	testutil.NoError(t, err)
	testutil.Contains(t, policyDef, "ayb.user_id")
}

func TestE2E_StorageMigration(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)

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
			sourceBucketName: "Project Assets.2026",
			normalizedBucket: "project-assets-2026",
			public:           false,
			objectName:       "nested/assets/logo.svg",
			contentType:      "image/svg+xml",
			size:             18,
			payload:          []byte("project-asset-2026"),
		},
		{
			sourceBucketID:   "empty-assets",
			sourceBucketName: "Empty Assets.2026",
			normalizedBucket: "empty-assets-2026",
			public:           true,
		},
	}

	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		testutil.Equal(t, fixture.size, len(fixture.payload))
	}

	for _, bucket := range storageMigrationBuckets(fixtures) {
		insertStorageBucket(t, sharedPG.Pool, bucket.id, bucket.name, bucket.public, bucket.objects)
	}

	storageExport := createStorageExportDir(t, storageMigrationExport(fixtures))
	tmpStorage := t.TempDir()

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       tmpStorage,
		Verbose:           true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, storageMigrationObjectCount(fixtures), stats.StorageFiles)
	testutil.Equal(t, int64(30), stats.StorageBytes)

	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		verifyFile(t,
			filepath.Join(tmpStorage, fixture.normalizedBucket, fixture.objectName),
			fixture.payload,
		)
	}

	assertMigratedStorageBuckets(t, fixtures)
	assertMigratedStorageObjects(t, fixtures)
	assertMigratedStorageDownloads(t, ctx, tmpStorage, fixtures)
}

type storageMigrationFixture struct {
	sourceBucketID   string
	sourceBucketName string
	normalizedBucket string
	public           bool
	objectName       string
	contentType      string
	size             int
	payload          []byte
}

type storageBucketFixture struct {
	id      string
	name    string
	public  bool
	objects []struct {
		name, mime string
		size       int
	}
}

func storageMigrationBuckets(fixtures []storageMigrationFixture) []storageBucketFixture {
	byID := map[string]int{}
	var buckets []storageBucketFixture
	for _, fixture := range fixtures {
		idx, ok := byID[fixture.sourceBucketID]
		if !ok {
			buckets = append(buckets, storageBucketFixture{
				id:     fixture.sourceBucketID,
				name:   fixture.sourceBucketName,
				public: fixture.public,
			})
			idx = len(buckets) - 1
			byID[fixture.sourceBucketID] = idx
		}
		if fixture.objectName == "" {
			continue
		}
		buckets[idx].objects = append(buckets[idx].objects, struct {
			name, mime string
			size       int
		}{
			name: fixture.objectName,
			mime: fixture.contentType,
			size: fixture.size,
		})
	}
	return buckets
}

func storageMigrationExport(fixtures []storageMigrationFixture) map[string]map[string][]byte {
	export := map[string]map[string][]byte{}
	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		files := export[fixture.sourceBucketName]
		if files == nil {
			files = map[string][]byte{}
			export[fixture.sourceBucketName] = files
		}
		files[fixture.objectName] = fixture.payload
	}
	return export
}

func storageMigrationObjectCount(fixtures []storageMigrationFixture) int {
	count := 0
	for _, fixture := range fixtures {
		if fixture.objectName != "" {
			count++
		}
	}
	return count
}

func TestStorageRegistrationNormalizedBucketCollisionFailsBeforeWrites(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "team-docs-space", "Team Docs", true, nil)
	insertStorageBucket(t, sharedPG.Pool, "team-docs-dot", "Team.Docs", false, nil)

	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: t.TempDir(),
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.ErrorContains(t, err, "Team Docs")
	testutil.ErrorContains(t, err, "Team.Docs")
	testutil.ErrorContains(t, err, "team-docs")

	_, statErr := os.Stat(storageRoot)
	testutil.True(t, os.IsNotExist(statErr), "destination directory exists after collision: %v", statErr)
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_buckets"))
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_objects"))
}

func TestStorageRegistrationSkipsFailedObjectsAndRegistersCopiedSiblings(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "assets", "Assets", false, []struct {
		name, mime string
		size       int
	}{
		{"ok.txt", "text/plain", 7},
		{"../escape.txt", "text/plain", 6},
		{"missing.txt", "text/plain", 8},
	})

	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"Assets": {"ok.txt": []byte("copied!")},
	})
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, stats.StorageFiles)
	testutil.Equal(t, int64(7), stats.StorageBytes)
	testutil.SliceLen(t, stats.Errors, 2)
	testutil.Equal(t, 1, storageObjectRowCount(t, "assets", "ok.txt"))
	testutil.Equal(t, 0, storageObjectRowCount(t, "assets", "../escape.txt"))
	testutil.Equal(t, 0, storageObjectRowCount(t, "assets", "missing.txt"))
}

func TestStorageRegistrationRejectsUndownloadableObjectName(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "reports", "Reports", false, []struct {
		name, mime string
		size       int
	}{
		{"summary.txt", "text/plain", 7},
		{"report..final.txt", "text/plain", 7},
	})

	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"Reports": {
			"summary.txt":       []byte("summary"),
			"report..final.txt": []byte("invalid"),
		},
	})
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	stats := runStorageMigration(t, connStr, storageExport, storageRoot)

	testutil.Equal(t, 1, stats.StorageFiles)
	testutil.Equal(t, int64(7), stats.StorageBytes)
	testutil.SliceLen(t, stats.Errors, 1)
	testutil.Contains(t, strings.Join(stats.Errors, "\n"), "invalid object name")
	testutil.Equal(t, 1, storageObjectRowCount(t, "reports", "summary.txt"))
	testutil.Equal(t, 0, storageObjectRowCount(t, "reports", "report..final.txt"))

	verifyFile(t, filepath.Join(storageRoot, "reports", "summary.txt"), []byte("summary"))
	_, err := os.Stat(filepath.Join(storageRoot, "reports", "report..final.txt"))
	testutil.True(t, os.IsNotExist(err), "invalid object was copied: %v", err)
}

func TestStorageRegistrationIdempotentlyUpdatesCopiedObjectMetadata(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "uploads", "Uploads", true, []struct {
		name, mime string
		size       int
	}{
		{"file.txt", "text/plain", 5},
	})

	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"Uploads": {"file.txt": []byte("first")},
	})
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	runStorageMigration(t, connStr, storageExport, storageRoot)

	secondPayload := []byte("second-version")
	testutil.NoError(t, os.WriteFile(filepath.Join(storageExport, "Uploads", "file.txt"), secondPayload, 0644))
	_, err := sharedPG.Pool.Exec(context.Background(), `
		UPDATE storage.objects
		SET metadata = '{"size": 999, "mimetype": "text/markdown"}'::jsonb
		WHERE bucket_id = 'uploads' AND name = 'file.txt'
	`)
	testutil.NoError(t, err)

	runStorageMigration(t, connStr, storageExport, storageRoot)

	testutil.Equal(t, 1, storageMetadataRowCount(t, "_ayb_storage_buckets"))
	testutil.Equal(t, 1, storageMetadataRowCount(t, "_ayb_storage_objects"))
	row := storageObjectRow(t, "uploads", "file.txt")
	testutil.Equal(t, int64(len(secondPayload)), row.Size)
	testutil.Equal(t, "text/markdown", row.ContentType)
	verifyFile(t, filepath.Join(storageRoot, "uploads", "file.txt"), secondPayload)
}

func TestStorageRegistrationFailedRerunPreservesLiveObject(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	const objectName = "file.txt"
	initialPayload := []byte("original-download")
	fixture := storageMigrationFixture{
		sourceBucketID:   "uploads",
		sourceBucketName: "Uploads",
		normalizedBucket: "uploads",
		public:           true,
		objectName:       objectName,
		contentType:      "text/plain",
		size:             len(initialPayload),
		payload:          initialPayload,
	}

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, fixture.sourceBucketID, fixture.sourceBucketName, fixture.public, []struct {
		name, mime string
		size       int
	}{
		{fixture.objectName, fixture.contentType, fixture.size},
	})

	storageExport := createStorageExportDir(t, storageMigrationExport([]storageMigrationFixture{fixture}))
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	runStorageMigration(t, connStr, storageExport, storageRoot)

	sourcePath := filepath.Join(storageExport, fixture.sourceBucketName, fixture.objectName)
	testutil.NoError(t, os.Remove(sourcePath))
	testutil.NoError(t, os.Mkdir(sourcePath, 0755))
	_, err := sharedPG.Pool.Exec(context.Background(), `
		UPDATE storage.objects
		SET metadata = '{"size": 999, "mimetype": "application/json"}'::jsonb
		WHERE bucket_id = 'uploads' AND name = 'file.txt'
	`)
	testutil.NoError(t, err)

	stats := runStorageMigration(t, connStr, storageExport, storageRoot)

	testutil.Equal(t, 0, stats.StorageFiles)
	testutil.Equal(t, int64(0), stats.StorageBytes)
	testutil.SliceLen(t, stats.Errors, 1)
	testutil.Contains(t, strings.Join(stats.Errors, "\n"), "copying Uploads/file.txt")
	row := storageObjectRow(t, fixture.normalizedBucket, fixture.objectName)
	testutil.Equal(t, int64(fixture.size), row.Size)
	testutil.Equal(t, fixture.contentType, row.ContentType)
	assertMigratedStorageDownloads(t, context.Background(), storageRoot, []storageMigrationFixture{fixture})
}

func TestStorageRegistrationMetadataFailurePreservesLiveObject(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	initialPayload := []byte("original-download")
	fixture := storageMigrationFixture{
		sourceBucketID:   "uploads",
		sourceBucketName: "Uploads",
		normalizedBucket: "uploads",
		public:           true,
		objectName:       "file.txt",
		contentType:      "text/plain",
		size:             len(initialPayload),
		payload:          initialPayload,
	}

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, fixture.sourceBucketID, fixture.sourceBucketName, fixture.public, []struct {
		name, mime string
		size       int
	}{
		{fixture.objectName, fixture.contentType, fixture.size},
	})

	storageExport := createStorageExportDir(t, storageMigrationExport([]storageMigrationFixture{fixture}))
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	runStorageMigration(t, connStr, storageExport, storageRoot)

	replacementPayload := []byte("replacement-with-different-size")
	testutil.NoError(t, os.WriteFile(
		filepath.Join(storageExport, fixture.sourceBucketName, fixture.objectName),
		replacementPayload,
		0644,
	))
	_, err := sharedPG.Pool.Exec(context.Background(), `
		UPDATE storage.objects
		SET metadata = '{"size": 999, "mimetype": "application/json"}'::jsonb
		WHERE bucket_id = 'uploads' AND name = 'file.txt';

		CREATE FUNCTION fail_storage_object_metadata_update() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'forced storage object metadata failure';
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_storage_object_metadata_update
			BEFORE UPDATE ON _ayb_storage_objects
			FOR EACH ROW EXECUTE FUNCTION fail_storage_object_metadata_update();
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		Force:             true,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.ErrorContains(t, err, "forced storage object metadata failure")

	row := storageObjectRow(t, fixture.normalizedBucket, fixture.objectName)
	testutil.Equal(t, int64(fixture.size), row.Size)
	testutil.Equal(t, fixture.contentType, row.ContentType)
	assertMigratedStorageDownloads(t, context.Background(), storageRoot, []storageMigrationFixture{fixture})
}

func TestStorageRegistrationIdempotentlyUpdatesBucketPublicMetadata(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "uploads", "Uploads", true, []struct {
		name, mime string
		size       int
	}{
		{"file.txt", "text/plain", 5},
	})

	storageExport := createStorageExportDir(t, map[string]map[string][]byte{
		"Uploads": {"file.txt": []byte("first")},
	})
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	runStorageMigration(t, connStr, storageExport, storageRoot)

	_, err := sharedPG.Pool.Exec(context.Background(), `
		UPDATE storage.buckets
		SET public = false
		WHERE id = 'uploads'
	`)
	testutil.NoError(t, err)

	runStorageMigration(t, connStr, storageExport, storageRoot)

	testutil.Equal(t, 1, storageMetadataRowCount(t, "_ayb_storage_buckets"))
	row := storageBucketRow(t, "uploads")
	testutil.Equal(t, "", row.TenantID)
	testutil.Equal(t, "uploads", row.Name)
	testutil.False(t, row.Public, "bucket public metadata stayed stale after rerun")
}

func TestStorageRegistrationReservedStagingBucketFailsBeforeWrites(t *testing.T) {
	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	insertStorageBucket(t, sharedPG.Pool, "staging", "ayb_resumable_staging", false, nil)

	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: t.TempDir(),
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.ErrorContains(t, err, "ayb_resumable_staging")
	testutil.ErrorContains(t, err, "reserved")

	_, statErr := os.Stat(storageRoot)
	testutil.True(t, os.IsNotExist(statErr), "destination directory exists after reserved bucket: %v", statErr)
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_buckets"))
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_objects"))
}

func TestStorageRegistrationRejectsUnsafeBucketExportPathsBeforeWrites(t *testing.T) {
	t.Run("empty bucket escapes export root", func(t *testing.T) {
		assertStorageRegistrationRejectsUnsafeBucketExportPath(t, []storageMigrationFixture{
			{
				sourceBucketID:   "empty-escape",
				sourceBucketName: "../empty-escape",
				public:           true,
			},
		}, "../empty-escape")
	})

	t.Run("non-empty bucket aliases another export directory", func(t *testing.T) {
		assertStorageRegistrationRejectsUnsafeBucketExportPath(t, []storageMigrationFixture{
			{
				sourceBucketID:   "uploads",
				sourceBucketName: "Uploads",
				public:           false,
			},
			{
				sourceBucketID:   "uploads-alias",
				sourceBucketName: "nested/../Uploads",
				public:           true,
				objectName:       "file.txt",
				contentType:      "text/plain",
				size:             7,
				payload:          []byte("payload"),
			},
		}, "nested/../Uploads")
	})
}

func assertStorageRegistrationRejectsUnsafeBucketExportPath(
	t *testing.T,
	fixtures []storageMigrationFixture,
	unsafeBucketName string,
) {
	t.Helper()

	connStr := setupSourceAndTarget(t)
	setupTargetAYBSchema(t)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "admin@example.com", "$2a$10$hash", true, false)
	for _, bucket := range storageMigrationBuckets(fixtures) {
		insertStorageBucket(t, sharedPG.Pool, bucket.id, bucket.name, bucket.public, bucket.objects)
	}

	storageExport := createStorageExportDir(t, storageMigrationExport(fixtures))
	storageRoot := filepath.Join(t.TempDir(), "ayb-storage")
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	_, err = migrator.Migrate(context.Background())
	testutil.ErrorContains(t, err, unsafeBucketName)
	testutil.ErrorContains(t, err, "unsafe export directory")

	_, statErr := os.Stat(storageRoot)
	testutil.True(t, os.IsNotExist(statErr), "destination directory exists after unsafe bucket: %v", statErr)
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_buckets"))
	testutil.Equal(t, 0, storageMetadataRowCount(t, "_ayb_storage_objects"))
}

type migratedStorageBucketRow struct {
	TenantID string
	Name     string
	Public   bool
}

func assertMigratedStorageBuckets(t *testing.T, fixtures []storageMigrationFixture) {
	t.Helper()

	rows, err := sharedPG.Pool.Query(context.Background(), `
		SELECT tenant_id, name, public
		FROM _ayb_storage_buckets
		ORDER BY tenant_id, name
	`)
	testutil.NoError(t, err)
	defer rows.Close()

	var got []migratedStorageBucketRow
	for rows.Next() {
		var row migratedStorageBucketRow
		testutil.NoError(t, rows.Scan(&row.TenantID, &row.Name, &row.Public))
		got = append(got, row)
	}
	testutil.NoError(t, rows.Err())

	wantByName := map[string]migratedStorageBucketRow{}
	for _, fixture := range fixtures {
		wantByName[fixture.normalizedBucket] = migratedStorageBucketRow{
			TenantID: "",
			Name:     fixture.normalizedBucket,
			Public:   fixture.public,
		}
	}
	want := make([]migratedStorageBucketRow, 0, len(wantByName))
	for _, row := range wantByName {
		want = append(want, row)
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].TenantID != want[j].TenantID {
			return want[i].TenantID < want[j].TenantID
		}
		return want[i].Name < want[j].Name
	})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("_ayb_storage_buckets rows:\ngot  %#v\nwant %#v", got, want)
	}
}

type migratedStorageObjectRow struct {
	TenantID     string
	Bucket       string
	Name         string
	Size         int64
	ContentType  string
	UserIDIsNull bool
}

func assertMigratedStorageObjects(t *testing.T, fixtures []storageMigrationFixture) {
	t.Helper()

	rows, err := sharedPG.Pool.Query(context.Background(), `
		SELECT tenant_id, bucket, name, size, content_type, user_id IS NULL
		FROM _ayb_storage_objects
		ORDER BY tenant_id, bucket, name
	`)
	testutil.NoError(t, err)
	defer rows.Close()

	var got []migratedStorageObjectRow
	for rows.Next() {
		var row migratedStorageObjectRow
		testutil.NoError(t, rows.Scan(
			&row.TenantID,
			&row.Bucket,
			&row.Name,
			&row.Size,
			&row.ContentType,
			&row.UserIDIsNull,
		))
		got = append(got, row)
	}
	testutil.NoError(t, rows.Err())

	want := make([]migratedStorageObjectRow, 0, len(fixtures))
	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		want = append(want, storageObjectRowForFixture(fixture))
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].TenantID != want[j].TenantID {
			return want[i].TenantID < want[j].TenantID
		}
		if want[i].Bucket != want[j].Bucket {
			return want[i].Bucket < want[j].Bucket
		}
		return want[i].Name < want[j].Name
	})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("_ayb_storage_objects rows:\ngot  %#v\nwant %#v", got, want)
	}
}

func storageObjectRowForFixture(fixture storageMigrationFixture) migratedStorageObjectRow {
	return migratedStorageObjectRow{
		TenantID:     "",
		Bucket:       fixture.normalizedBucket,
		Name:         fixture.objectName,
		Size:         int64(fixture.size),
		ContentType:  fixture.contentType,
		UserIDIsNull: true,
	}
}

func assertMigratedStorageDownloads(
	t *testing.T,
	ctx context.Context,
	tmpStorage string,
	fixtures []storageMigrationFixture,
) {
	t.Helper()

	backend, err := storage.NewLocalBackend(tmpStorage)
	testutil.NoError(t, err)
	service := storage.NewService(
		sharedPG.Pool,
		backend,
		"test-sign-key-at-least-32-chars!!",
		testutil.DiscardLogger(),
		0,
	)

	for _, fixture := range fixtures {
		if fixture.objectName == "" {
			continue
		}
		reader, object, err := service.Download(ctx, fixture.normalizedBucket, fixture.objectName)
		if err != nil {
			t.Errorf("Download(%q, %q): %v", fixture.normalizedBucket, fixture.objectName, err)
			continue
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		testutil.NoError(t, readErr)
		testutil.NoError(t, closeErr)
		if !reflect.DeepEqual(data, fixture.payload) {
			t.Errorf("Download(%q, %q) bytes: got %q, want %q",
				fixture.normalizedBucket, fixture.objectName, data, fixture.payload)
		}
		testutil.Equal(t, fixture.normalizedBucket, object.Bucket)
		testutil.Equal(t, fixture.objectName, object.Name)
		testutil.Equal(t, int64(fixture.size), object.Size)
		testutil.Equal(t, fixture.contentType, object.ContentType)
		testutil.Nil(t, object.UserID)
	}
}

func TestE2E_DryRun(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash", true, false)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE notes (id SERIAL PRIMARY KEY, text TEXT NOT NULL)`,
		`INSERT INTO notes (text) VALUES ('hello')`,
	)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipRLS:   true,
		SkipOAuth: true,
		DryRun:    true,
		Verbose:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	stats, err := migrator.Migrate(ctx)
	testutil.NoError(t, err)

	// Stats should be populated even in dry-run.
	testutil.Equal(t, 1, stats.Tables)
	testutil.Equal(t, 0, stats.Records) // same-DB: rows already exist → ON CONFLICT DO NOTHING
	testutil.Equal(t, 1, stats.Users)

	// Verify the user was rolled back (not persisted).
	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM _ayb_users").Scan(&userCount)
	testutil.NoError(t, err)
	// The user INSERT happened inside the transaction, and DryRun triggers rollback.
	testutil.Equal(t, 0, userCount)
}

func TestE2E_Analyze(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "alice@example.com", "$2a$10$hash1", true, false)
	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000002", "bob@example.com", "$2a$10$hash2", false, false)

	insertSourceIdentity(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000001", "google",
		`{"sub": "g-123", "email": "alice@gmail.com"}`)

	insertSourceTable(t, sharedPG.Pool,
		`CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT)`,
		`INSERT INTO items (name) VALUES ('a')`,
		`INSERT INTO items (name) VALUES ('b')`,
	)

	// Add RLS policy.
	_, err := sharedPG.Pool.Exec(context.Background(), `
		CREATE OR REPLACE FUNCTION auth.uid() RETURNS UUID AS $$
			SELECT gen_random_uuid();
		$$ LANGUAGE SQL;
		ALTER TABLE items ENABLE ROW LEVEL SECURITY;
		CREATE POLICY items_select ON items FOR SELECT USING (true);
	`)
	testutil.NoError(t, err)

	// Storage buckets.
	insertStorageBucket(t, sharedPG.Pool, "media", "media", true, []struct {
		name, mime string
		size       int
	}{
		{"a.jpg", "image/jpeg", 100},
		{"b.jpg", "image/jpeg", 200},
	})

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	testutil.NoError(t, err)

	testutil.Equal(t, "Supabase", report.SourceType)
	testutil.Equal(t, 2, report.AuthUsers)
	testutil.Equal(t, 1, report.OAuthLinks)
	testutil.Equal(t, 1, report.Tables)      // items (_ayb_ tables filtered)
	testutil.Equal(t, 2, report.Records)     // 2 items
	testutil.Equal(t, 1, report.RLSPolicies) // items_select
	testutil.Equal(t, 2, report.Files)       // 2 storage objects
	testutil.True(t, report.FileSizeBytes > 0)
}

func TestE2E_AnalyzeWithoutIsAnonymousColumn(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	_, err := sharedPG.Pool.Exec(context.Background(), `ALTER TABLE auth.users DROP COLUMN is_anonymous`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000011', 'legacy@example.com', '$2a$10$hash', NOW())
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	report, err := migrator.Analyze(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, report.AuthUsers)
}

func TestE2E_AuthMigrationWithoutIsAnonymousColumn(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	_, err := sharedPG.Pool.Exec(context.Background(), `ALTER TABLE auth.users DROP COLUMN is_anonymous`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES
		  ('aaaaaaaa-0000-0000-0000-000000000021', 'alice@example.com', '$2a$10$hash1', NOW()),
		  ('aaaaaaaa-0000-0000-0000-000000000022', 'bob@example.com', '$2a$10$hash2', NULL)
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipOAuth: true,
		SkipRLS:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Users)
}

func TestE2E_OAuthMigrationWithoutIdentityDataAndCreatedAt(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	insertSourceUser(t, sharedPG.Pool,
		"aaaaaaaa-0000-0000-0000-000000000031", "alice@example.com", "$2a$10$hash1", true, false)

	_, err := sharedPG.Pool.Exec(context.Background(), `
		ALTER TABLE auth.identities DROP COLUMN identity_data;
		ALTER TABLE auth.identities DROP COLUMN created_at;
		ALTER TABLE auth.identities ADD COLUMN provider_id TEXT;
	`)
	testutil.NoError(t, err)

	_, err = sharedPG.Pool.Exec(context.Background(), `
		INSERT INTO auth.identities (user_id, provider, provider_id)
		VALUES
		  ('aaaaaaaa-0000-0000-0000-000000000031', 'google', 'google-legacy-123'),
		  ('aaaaaaaa-0000-0000-0000-000000000031', 'email', 'email-legacy-ignored')
	`)
	testutil.NoError(t, err)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL: connStr,
		TargetURL: connStr,
		SkipData:  true,
		SkipRLS:   true,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, stats.OAuthLinks)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	var providerUserID string
	err = db.QueryRow(`
		SELECT provider_user_id
		FROM _ayb_oauth_accounts
		WHERE provider = 'google'
	`).Scan(&providerUserID)
	testutil.NoError(t, err)
	testutil.Equal(t, "google-legacy-123", providerUserID)
}

func TestE2E_IntrospectTablesFiltersHostedManagedTables(t *testing.T) {
	connStr := setupSourceAndTarget(t)

	db, err := sql.Open("pgx", connStr)
	testutil.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS products (id SERIAL PRIMARY KEY, name TEXT);
		CREATE TABLE IF NOT EXISTS supabase_migrations (version TEXT);
		CREATE TABLE IF NOT EXISTS buckets_vectors (id TEXT);
		CREATE TABLE IF NOT EXISTS vector_indexes (id TEXT);
	`)
	testutil.NoError(t, err)

	tables, err := introspectTables(context.Background(), db)
	testutil.NoError(t, err)

	names := make(map[string]bool, len(tables))
	for _, tbl := range tables {
		names[tbl.Name] = true
	}
	testutil.True(t, names["products"], "expected products table")
	testutil.False(t, names["supabase_migrations"], "internal table should be filtered")
	testutil.False(t, names["buckets_vectors"], "internal table should be filtered")
	testutil.False(t, names["vector_indexes"], "internal table should be filtered")
}

func TestE2E_SchemaMigrationSkipsIncompatibleFKChains(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB, err := sql.Open("pgx", sourceURL)
	testutil.NoError(t, err)
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", targetURL)
	testutil.NoError(t, err)
	defer targetDB.Close()

	_, err = sourceDB.Exec(`
		CREATE SCHEMA auth;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);
		CREATE FUNCTION legacy_parent_source_only_uuid()
		RETURNS UUID
		LANGUAGE SQL
		AS $$
			SELECT gen_random_uuid();
		$$;

		-- Intentionally uses gen_random_uuid() so this FK-skip fixture avoids optional uuid-ossp setup.
		CREATE TABLE legacy_parent (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_only_marker UUID NOT NULL DEFAULT legacy_parent_source_only_uuid(),
			name TEXT NOT NULL
		);
		CREATE TABLE legacy_child (
			id SERIAL PRIMARY KEY,
			parent_id UUID NOT NULL REFERENCES legacy_parent(id),
			note TEXT
		);
		INSERT INTO legacy_parent (name) VALUES ('parent-a');
		INSERT INTO legacy_child (parent_id, note)
		SELECT id, 'child-a' FROM legacy_parent LIMIT 1;
	`)
	testutil.NoError(t, err)

	_, err = targetDB.Exec(`
		CREATE TABLE _ayb_users (
			id UUID PRIMARY KEY,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
	`)
	testutil.NoError(t, err)

	var out strings.Builder
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         sourceURL,
		TargetURL:         targetURL,
		Force:             true,
		SkipOAuth:         true,
		SkipRLS:           true,
		SkipStorage:       true,
		Verbose:           true,
		StoragePath:       t.TempDir(),
		StorageExportPath: "",
		Progress:          migrate.NewCLIReporter(&out),
	})
	testutil.NoError(t, err)
	defer migrator.Close()
	migrator.output = &out

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.True(t, stats.Skipped >= 2, "expected incompatible FK chain tables to be skipped")
	testutil.Contains(t, out.String(), "skipping table legacy_parent")
	testutil.Contains(t, out.String(), "skipping table legacy_child")
	testutil.Contains(t, out.String(), "source/target schema incompatibility")

	var parentExists bool
	err = targetDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'legacy_parent'
		)
	`).Scan(&parentExists)
	testutil.NoError(t, err)
	testutil.False(t, parentExists, "legacy_parent should be skipped in target")

	var childExists bool
	err = targetDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'legacy_child'
		)
	`).Scan(&childExists)
	testutil.NoError(t, err)
	testutil.False(t, childExists, "legacy_child should be skipped in target")
}

func TestE2E_SchemaMigrationRetriesDeferredFKDependencies(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_retry")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_retry")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB, err := sql.Open("pgx", sourceURL)
	testutil.NoError(t, err)
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", targetURL)
	testutil.NoError(t, err)
	defer targetDB.Close()

	_, err = sourceDB.Exec(`
		CREATE SCHEMA auth;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000041', 'seed@example.com', '$2a$10$hash', NOW());

		CREATE TABLE products (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL
		);

		CREATE TABLE orders (
			id UUID PRIMARY KEY,
			product_id UUID NOT NULL REFERENCES products(id),
			qty INTEGER NOT NULL
		);

		INSERT INTO products (id, name) VALUES ('11111111-1111-1111-1111-111111111111', 'Widget');
		INSERT INTO orders (id, product_id, qty) VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 2);
	`)
	testutil.NoError(t, err)

	_, err = targetDB.Exec(`
		CREATE TABLE _ayb_users (
			id UUID PRIMARY KEY,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE _ayb_oauth_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			email TEXT,
			name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(provider, provider_user_id)
		);
	`)
	testutil.NoError(t, err)

	var out strings.Builder
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         sourceURL,
		TargetURL:         targetURL,
		Force:             true,
		SkipOAuth:         true,
		SkipRLS:           true,
		SkipStorage:       true,
		Verbose:           true,
		StoragePath:       t.TempDir(),
		StorageExportPath: "",
		Progress:          migrate.NewCLIReporter(&out),
	})
	testutil.NoError(t, err)
	defer migrator.Close()
	migrator.output = &out

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Tables)
	testutil.Equal(t, 2, stats.Records)
	testutil.Equal(t, 0, stats.Skipped)
	testutil.Contains(t, out.String(), "CREATE TABLE products")
	testutil.Contains(t, out.String(), "CREATE TABLE orders")
	testutil.False(t, strings.Contains(out.String(), "skipping table orders"),
		"deferred table should be retried and created, not skipped")

	var orderCount int
	err = targetDB.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orderCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, orderCount)
}

// --- Helpers ---

func runStorageMigration(t *testing.T, connStr, storageExport, storageRoot string) *MigrationStats {
	t.Helper()

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:         connStr,
		TargetURL:         connStr,
		Force:             true,
		SkipData:          true,
		SkipRLS:           true,
		SkipOAuth:         true,
		StorageExportPath: storageExport,
		StoragePath:       storageRoot,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	return stats
}

func storageMetadataRowCount(t *testing.T, table string) int {
	t.Helper()

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	var count int
	err := sharedPG.Pool.QueryRow(context.Background(), query).Scan(&count)
	testutil.NoError(t, err)
	return count
}

func storageObjectRowCount(t *testing.T, bucket, name string) int {
	t.Helper()

	var count int
	err := sharedPG.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM _ayb_storage_objects
		WHERE tenant_id = '' AND bucket = $1 AND name = $2
	`, bucket, name).Scan(&count)
	testutil.NoError(t, err)
	return count
}

func storageBucketRow(t *testing.T, name string) migratedStorageBucketRow {
	t.Helper()

	var row migratedStorageBucketRow
	err := sharedPG.Pool.QueryRow(context.Background(), `
		SELECT tenant_id, name, public
		FROM _ayb_storage_buckets
		WHERE tenant_id = '' AND name = $1
	`, name).Scan(&row.TenantID, &row.Name, &row.Public)
	testutil.NoError(t, err)
	return row
}

func storageObjectRow(t *testing.T, bucket, name string) migratedStorageObjectRow {
	t.Helper()

	var row migratedStorageObjectRow
	err := sharedPG.Pool.QueryRow(context.Background(), `
		SELECT tenant_id, bucket, name, size, content_type, user_id IS NULL
		FROM _ayb_storage_objects
		WHERE tenant_id = '' AND bucket = $1 AND name = $2
	`, bucket, name).Scan(
		&row.TenantID,
		&row.Bucket,
		&row.Name,
		&row.Size,
		&row.ContentType,
		&row.UserIDIsNull,
	)
	testutil.NoError(t, err)
	return row
}

func verifyFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	content, err := os.ReadFile(path)
	testutil.NoError(t, err)
	testutil.Equal(t, string(expected), string(content))
}
