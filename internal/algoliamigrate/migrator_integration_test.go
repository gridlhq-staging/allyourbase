//go:build integration

package algoliamigrate

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
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

func TestImportRecordsCreatesSchemaAndInsertsFixtureValues(t *testing.T) {
	db := openIntegrationDB(t)
	resetPublicSchema(t, db)

	stats, err := NewMigrator(db, ImportOptions{TargetTable: "Products Import"}, migrate.NopReporter{}).
		ImportRecords(context.Background(), loadBrowseFixtureRecords(t))
	if err != nil {
		t.Fatalf("ImportRecords: %v", err)
	}
	if stats.Tables != 1 || stats.Records != 3 {
		t.Fatalf("stats = %#v, want one table and three records", stats)
	}

	assertColumns(t, db, "products_import", []columnInfo{
		{Name: "objectID", Type: "text", Nullable: "NO"},
		{Name: "dimensions", Type: "jsonb", Nullable: "NO"},
		{Name: "inventory_count", Type: "bigint", Nullable: "NO"},
		{Name: "price", Type: "double precision", Nullable: "NO"},
		{Name: "published", Type: "boolean", Nullable: "NO"},
		{Name: "subtitle", Type: "text", Nullable: "YES"},
		{Name: "tags", Type: "jsonb", Nullable: "NO"},
		{Name: "title", Type: "text", Nullable: "NO"},
	})
	assertPrimaryKey(t, db, "products_import", []string{"objectID"})

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM "public"."products_import"`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 3 {
		t.Fatalf("row count = %d, want 3", count)
	}

	var title string
	var price float64
	var tags string
	var width string
	err = db.QueryRow(`
		SELECT "title", "price", "tags"::text, "dimensions"->>'width_cm'
		FROM "public"."products_import"
		WHERE "objectID" = $1
	`, "product_001").Scan(&title, &price, &tags, &width)
	if err != nil {
		t.Fatalf("query representative values: %v", err)
	}
	if title != "Desk Lamp" || price != 39.95 || tags != `["lighting", "office"]` || width != "14" {
		t.Fatalf("values = title:%q price:%v tags:%q width:%q", title, price, tags, width)
	}
}

func TestImportRecordsDryRunRollsBackTargetTable(t *testing.T) {
	db := openIntegrationDB(t)
	resetPublicSchema(t, db)

	stats, err := NewMigrator(db, ImportOptions{TargetTable: "dry run products", DryRun: true}, migrate.NopReporter{}).
		ImportRecords(context.Background(), loadBrowseFixtureRecords(t))
	if err != nil {
		t.Fatalf("ImportRecords dry-run: %v", err)
	}
	if stats.Tables != 1 || stats.Records != 3 || !stats.DryRun {
		t.Fatalf("dry-run stats = %#v", stats)
	}
	if tableExists(t, db, "dry_run_products") {
		t.Fatal("dry-run target table exists after rollback")
	}
}

func TestImportRecordsWithSynonymsWritesOnlyNormalizedRegularGroups(t *testing.T) {
	db := openIntegrationDB(t)
	resetPublicSchema(t, db)
	bootstrapSystemMigrations(t)

	stats, err := NewMigrator(db, ImportOptions{
		TargetTable: "Products Import",
		Synonyms:    ptrSynonymInput(loadSynonymFixtureInput(t)),
	}, migrate.NopReporter{}).ImportRecords(context.Background(), loadBrowseFixtureRecords(t))
	if err != nil {
		t.Fatalf("ImportRecords with synonyms: %v", err)
	}
	if stats.Tables != 1 || stats.Records != 3 || stats.Synonyms.SupportedGroups != 1 {
		t.Fatalf("stats = %#v, want table, records, and one synonym group", stats)
	}
	if !tableExists(t, db, "products_import") {
		t.Fatal("target table was not created")
	}
	assertSearchSynonymTerms(t, db, "public", "products_import", []string{
		"desk supplies",
		"office supplies",
		"stationery",
	})
}

func TestImportRecordsWithSynonymsDryRunRollsBackTableAndSynonyms(t *testing.T) {
	db := openIntegrationDB(t)
	resetPublicSchema(t, db)
	bootstrapSystemMigrations(t)

	stats, err := NewMigrator(db, ImportOptions{
		TargetTable: "dry run products",
		DryRun:      true,
		Synonyms:    ptrSynonymInput(loadSynonymFixtureInput(t)),
	}, migrate.NopReporter{}).ImportRecords(context.Background(), loadBrowseFixtureRecords(t))
	if err != nil {
		t.Fatalf("ImportRecords dry-run with synonyms: %v", err)
	}
	if stats.Tables != 1 || stats.Records != 3 || stats.Synonyms.SupportedGroups != 1 || !stats.DryRun {
		t.Fatalf("dry-run stats = %#v", stats)
	}
	if tableExists(t, db, "dry_run_products") {
		t.Fatal("dry-run target table exists after rollback")
	}
	assertSearchSynonymTerms(t, db, "public", "dry_run_products", nil)
}

func TestImportRecordsCollisionPreflightLeavesExistingTableUnchanged(t *testing.T) {
	db := openIntegrationDB(t)
	resetPublicSchema(t, db)
	if _, err := db.Exec(`CREATE TABLE "public"."products" ("objectID" text PRIMARY KEY, "title" text NOT NULL)`); err != nil {
		t.Fatalf("create existing table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO "public"."products" ("objectID", "title") VALUES ($1, $2)`, "existing", "Existing Row"); err != nil {
		t.Fatalf("insert existing row: %v", err)
	}

	_, err := NewMigrator(db, ImportOptions{TargetTable: "products"}, migrate.NopReporter{}).
		ImportRecords(context.Background(), loadBrowseFixtureRecords(t))
	if err == nil {
		t.Fatal("ImportRecords unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want collision context", err)
	}

	var title string
	if err := db.QueryRow(`SELECT "title" FROM "public"."products" WHERE "objectID" = $1`, "existing").Scan(&title); err != nil {
		t.Fatalf("query existing row: %v", err)
	}
	if title != "Existing Row" {
		t.Fatalf("existing row title = %q", title)
	}
}

func bootstrapSystemMigrations(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func ptrSynonymInput(input SynonymInput) *SynonymInput {
	return &input
}

type columnInfo struct {
	Name     string
	Type     string
	Nullable string
}

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", sharedPG.ConnString)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resetPublicSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`)
	if err != nil {
		t.Fatalf("reset public schema: %v", err)
	}
}

func assertColumns(t *testing.T, db *sql.DB, table string, want []columnInfo) {
	t.Helper()
	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, table)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()

	var got []columnInfo
	for rows.Next() {
		var col columnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		got = append(got, col)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("column rows: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
}

func assertPrimaryKey(t *testing.T, db *sql.DB, table string, want []string) {
	t.Helper()
	rows, err := db.Query(`
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = 'public'
		  AND tc.table_name = $1
		  AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position
	`, table)
	if err != nil {
		t.Fatalf("query primary key: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan primary key: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("primary key rows: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("primary key = %#v, want %#v", got, want)
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("query table existence: %v", err)
	}
	return exists
}

func assertSearchSynonymTerms(t *testing.T, db *sql.DB, schemaName, tableName string, want []string) {
	t.Helper()
	rows, err := db.Query(`
		SELECT term
		FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
		ORDER BY term
	`, schemaName, tableName)
	if err != nil {
		t.Fatalf("query search synonyms: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			t.Fatalf("scan search synonym term: %v", err)
		}
		got = append(got, term)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("search synonym rows: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("synonym terms = %#v, want %#v", got, want)
	}
}
