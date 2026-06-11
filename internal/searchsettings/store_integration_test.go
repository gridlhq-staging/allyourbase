//go:build integration

package searchsettings_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/searchsettings"
	"github.com/allyourbase/ayb/internal/testutil"
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

func resetSearchSettingsDB(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}
