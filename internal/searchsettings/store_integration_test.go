//go:build integration

package searchsettings_test

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/searchsettings"
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

func TestSearchSettingsStoreSaveThenLoadRoundTripsOrderedSettings(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsDB(t, ctx)

	store := searchsettings.NewStore(sharedPG.Pool)
	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "body", Weight: searchsettings.WeightLow},
	}, CustomRanking: []searchsettings.CustomRanking{
		{Column: "published_at", Order: searchsettings.RankingOrderDesc},
		{Column: "priority", Order: searchsettings.RankingOrderAsc},
	}}

	testutil.NoError(t, store.Save(ctx, "public", "posts", settings))

	got, err := store.Load(ctx, "public", "posts")
	testutil.NoError(t, err)
	if !reflect.DeepEqual(settings, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", settings, got)
	}
}

func TestSearchSettingsStoreLoadUnsetCollectionReturnsEmptySettings(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsDB(t, ctx)

	got, err := searchsettings.NewStore(sharedPG.Pool).Load(ctx, "public", "posts")
	testutil.NoError(t, err)
	if !reflect.DeepEqual(searchsettings.Settings{}, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", searchsettings.Settings{}, got)
	}
}

func TestSearchSettingsStoreLoadRejectsInvalidPersistedSettings(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsDB(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
		VALUES ('public', 'posts', '{"attributes":[{"column":"title","weight":"heavy"}]}'::jsonb)
	`)
	testutil.NoError(t, err)

	_, err = searchsettings.NewStore(sharedPG.Pool).Load(ctx, "public", "posts")
	testutil.ErrorContains(t, err, "unknown search setting attribute weight: heavy")
}

func TestSearchSettingsStoreSaveSQLTxCommitsWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsDB(t, ctx)

	db := openSearchSettingsSQLDB(t)
	tx, err := db.BeginTx(ctx, nil)
	testutil.NoError(t, err)

	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
	}, CustomRanking: []searchsettings.CustomRanking{
		{Column: "price", Order: searchsettings.RankingOrderDesc},
	}}
	testutil.NoError(t, searchsettings.SaveSQLTx(ctx, tx, "public", "products", settings))
	testutil.NoError(t, tx.Commit())

	got, err := searchsettings.NewStore(sharedPG.Pool).Load(ctx, "public", "products")
	testutil.NoError(t, err)
	if !reflect.DeepEqual(settings, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", settings, got)
	}
}

func TestSearchSettingsStoreSaveSQLTxRollsBackWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	resetSearchSettingsDB(t, ctx)

	db := openSearchSettingsSQLDB(t)
	tx, err := db.BeginTx(ctx, nil)
	testutil.NoError(t, err)

	settings := searchsettings.Settings{Attributes: []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
	}}
	testutil.NoError(t, searchsettings.SaveSQLTx(ctx, tx, "public", "products", settings))
	testutil.NoError(t, tx.Rollback())

	got, err := searchsettings.NewStore(sharedPG.Pool).Load(ctx, "public", "products")
	testutil.NoError(t, err)
	if !reflect.DeepEqual(searchsettings.Settings{}, got) {
		t.Fatalf("settings mismatch:\nwant: %#v\n got: %#v", searchsettings.Settings{}, got)
	}
}

func resetSearchSettingsDB(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func openSearchSettingsSQLDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", sharedPG.ConnString)
	testutil.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
