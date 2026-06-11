//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSearchSettingsMigrationContract(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)

	assertSearchSettingsColumns(t, ctx)
	assertSearchSettingsCollectionKey(t, ctx)
	assertSearchSettingsJSONBContract(t, ctx)
}

func assertSearchSettingsColumns(t *testing.T, ctx context.Context) {
	t.Helper()

	var tableExists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT to_regclass('public._ayb_search_settings') IS NOT NULL`,
	).Scan(&tableExists)
	testutil.NoError(t, err)
	testutil.True(t, tableExists, "_ayb_search_settings table should exist")

	type columnInfo struct {
		dataType string
		nullable bool
	}
	rows, err := sharedPG.Pool.Query(ctx,
		`SELECT column_name, data_type, is_nullable = 'YES'
		 FROM information_schema.columns
		 WHERE table_schema = 'public'
		   AND table_name = '_ayb_search_settings'`,
	)
	testutil.NoError(t, err)
	defer rows.Close()

	columns := map[string]columnInfo{}
	for rows.Next() {
		var name string
		var info columnInfo
		testutil.NoError(t, rows.Scan(&name, &info.dataType, &info.nullable))
		columns[name] = info
	}
	testutil.NoError(t, rows.Err())

	expectedColumns := map[string]string{
		"schema_name": "text",
		"table_name":  "text",
		"settings":    "jsonb",
		"created_at":  "timestamp with time zone",
	}
	for name, dataType := range expectedColumns {
		info, ok := columns[name]
		testutil.True(t, ok, "%s column should exist", name)
		testutil.Equal(t, dataType, info.dataType)
		testutil.False(t, info.nullable, "%s column should be NOT NULL", name)
	}
}

func assertSearchSettingsCollectionKey(t *testing.T, ctx context.Context) {
	t.Helper()

	var indexKeys string
	var indexUnique bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT ix.indisunique, string_agg(a.attname, ',' ORDER BY k.ordinality)
		 FROM pg_class i
		 JOIN pg_index ix ON ix.indexrelid = i.oid
		 JOIN pg_class tbl ON tbl.oid = ix.indrelid
		 JOIN pg_namespace n ON n.oid = tbl.relnamespace
		 JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		 JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = k.attnum
		 WHERE n.nspname = 'public'
		   AND tbl.relname = '_ayb_search_settings'
		   AND i.relname = 'uq_ayb_search_settings_collection'
		 GROUP BY ix.indisunique`,
	).Scan(&indexUnique, &indexKeys)
	testutil.NoError(t, err)
	testutil.True(t, indexUnique, "collection key should enforce uniqueness")
	testutil.Equal(t, "schema_name,table_name", indexKeys)
}

func assertSearchSettingsJSONBContract(t *testing.T, ctx context.Context) {
	t.Helper()

	const settingsJSON = `{"attributes":[{"column":"title","weight":"high"},{"column":"body","weight":"low"}]}`
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
		 VALUES ('public', 'posts', $1::jsonb)`,
		settingsJSON,
	)
	testutil.NoError(t, err)

	var matches bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT settings = $1::jsonb
		 FROM _ayb_search_settings
		 WHERE schema_name = 'public' AND table_name = 'posts'`,
		settingsJSON,
	).Scan(&matches)
	testutil.NoError(t, err)
	testutil.True(t, matches, "settings should persist as the expected JSONB object")

	var firstColumn string
	var secondColumn string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT settings->'attributes'->0->>'column',
		        settings->'attributes'->1->>'column'
		 FROM _ayb_search_settings
		 WHERE schema_name = 'public' AND table_name = 'posts'`,
	).Scan(&firstColumn, &secondColumn)
	testutil.NoError(t, err)
	testutil.Equal(t, "title", firstColumn)
	testutil.Equal(t, "body", secondColumn)

	assertSearchSettingsRejectsMalformedJSONB(t, ctx, `{}`)
	assertSearchSettingsRejectsMalformedJSONB(t, ctx, `{"attributes":null}`)

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
		 VALUES ('public', 'posts', $1::jsonb)`,
		settingsJSON,
	)
	testutil.ErrorContains(t, err, "uq_ayb_search_settings_collection")
}

func assertSearchSettingsRejectsMalformedJSONB(t *testing.T, ctx context.Context, payload string) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
		 VALUES ('public', 'malformed_' || md5($1), $1::jsonb)`,
		payload,
	)
	testutil.ErrorContains(t, err, "chk_ayb_search_settings_attributes")
}
