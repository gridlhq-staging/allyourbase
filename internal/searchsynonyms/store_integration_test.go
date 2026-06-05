//go:build integration

package searchsynonyms

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
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

func TestStoreReplaceGroupsAndLoadGroups(t *testing.T) {
	ctx := context.Background()
	resetSearchSynonymsStoreSchema(t, ctx)
	store := NewStore(sharedPG.Pool)

	otherGroups, err := NormalizeGroups(Groups{{Terms: []string{"unchanged", "other"}}})
	if err != nil {
		t.Fatalf("NormalizeGroups other collection: %v", err)
	}
	if err := store.ReplaceGroups(ctx, "private", "posts", otherGroups); err != nil {
		t.Fatalf("ReplaceGroups other collection: %v", err)
	}

	groups, err := NormalizeGroups(Groups{
		{Terms: []string{" SciFi ", "Science Fiction"}},
		{Terms: []string{"AI", "Artificial Intelligence", "Machine Learning"}},
	})
	if err != nil {
		t.Fatalf("NormalizeGroups target collection: %v", err)
	}
	if err := store.ReplaceGroups(ctx, "public", "posts", groups); err != nil {
		t.Fatalf("ReplaceGroups target collection: %v", err)
	}

	assertLoadedSearchSynonymGroups(t, ctx, store, "public", "posts", Groups{
		{Terms: []string{"ai", "artificial intelligence", "machine learning"}},
		{Terms: []string{"science fiction", "scifi"}},
	})
	assertOneGroupIDPerSearchSynonymGroup(t, ctx, "public", "posts")

	replacement, err := NormalizeGroups(Groups{{Terms: []string{"SciFi", "Science Fiction"}}})
	if err != nil {
		t.Fatalf("NormalizeGroups replacement: %v", err)
	}
	if err := store.ReplaceGroups(ctx, "public", "posts", replacement); err != nil {
		t.Fatalf("ReplaceGroups replacement: %v", err)
	}

	assertLoadedSearchSynonymGroups(t, ctx, store, "public", "posts", Groups{
		{Terms: []string{"science fiction", "scifi"}},
	})
	assertSearchSynonymRowCount(t, ctx, "public", "posts", 2)
	assertLoadedSearchSynonymGroups(t, ctx, store, "private", "posts", Groups{
		{Terms: []string{"other", "unchanged"}},
	})
}

func TestReplaceGroupsSQLTxParticipatesInCallerRollback(t *testing.T) {
	ctx := context.Background()
	resetSearchSynonymsStoreSchema(t, ctx)

	db, err := sql.Open("pgx", sharedPG.ConnString)
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	defer db.Close()

	groups, err := NormalizeGroups(Groups{{Terms: []string{"Desk Lamp", "Task Light"}}})
	if err != nil {
		t.Fatalf("NormalizeGroups: %v", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := ReplaceGroupsSQLTx(ctx, tx, "public", "products", groups); err != nil {
		t.Fatalf("ReplaceGroupsSQLTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}

	assertSearchSynonymRowCount(t, ctx, "public", "products", 0)
}

func resetSearchSynonymsStoreSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func assertLoadedSearchSynonymGroups(t *testing.T, ctx context.Context, store Store, schemaName, tableName string, want Groups) {
	t.Helper()

	got, err := store.LoadGroups(ctx, schemaName, tableName)
	if err != nil {
		t.Fatalf("LoadGroups(%q, %q): %v", schemaName, tableName, err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("LoadGroups(%q, %q) mismatch:\nwant: %#v\n got: %#v", schemaName, tableName, want, got)
	}
}

func assertOneGroupIDPerSearchSynonymGroup(t *testing.T, ctx context.Context, schemaName, tableName string) {
	t.Helper()

	rows, err := sharedPG.Pool.Query(ctx, `
		SELECT group_id::text, array_agg(term ORDER BY term)
		FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
		GROUP BY group_id
	`, schemaName, tableName)
	if err != nil {
		t.Fatalf("query group IDs: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var groupID string
		var terms []string
		if err := rows.Scan(&groupID, &terms); err != nil {
			t.Fatalf("scan group ID row: %v", err)
		}
		if _, err := uuid.Parse(groupID); err != nil {
			t.Fatalf("group_id %q is not a UUID: %v", groupID, err)
		}
		key := stringsJoinSearchSynonymTerms(terms)
		seen[key] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate group IDs: %v", err)
	}

	want := map[string]bool{
		"ai\x00artificial intelligence\x00machine learning": true,
		"science fiction\x00scifi":                          true,
	}
	if !reflect.DeepEqual(want, seen) {
		t.Fatalf("group IDs did not preserve one ID per group:\nwant: %#v\n got: %#v", want, seen)
	}
}

func assertSearchSynonymRowCount(t *testing.T, ctx context.Context, schemaName, tableName string, want int) {
	t.Helper()

	var got int
	err := sharedPG.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
	`, schemaName, tableName).Scan(&got)
	if err != nil {
		t.Fatalf("count synonyms: %v", err)
	}
	if got != want {
		t.Fatalf("synonym row count = %d, want %d", got, want)
	}
}

func stringsJoinSearchSynonymTerms(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	out := terms[0]
	for _, term := range terms[1:] {
		out += "\x00" + term
	}
	return out
}
