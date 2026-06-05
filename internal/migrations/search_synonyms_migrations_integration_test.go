//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

type searchSynonymCollection struct {
	schemaName string
	tableName  string
}

type searchSynonymMembership struct {
	collection searchSynonymCollection
	groupID    string
	term       string
}

func TestSearchSynonymsMigrationContract(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)

	assertSearchSynonymsColumns(t, ctx)
	assertSearchSynonymsLookupIndex(t, ctx)
	assertSearchSynonymsMembershipRules(t, ctx)
}

func assertSearchSynonymsColumns(t *testing.T, ctx context.Context) {
	t.Helper()

	var tableExists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT to_regclass('public._ayb_search_synonyms') IS NOT NULL`,
	).Scan(&tableExists)
	testutil.NoError(t, err)
	testutil.True(t, tableExists, "_ayb_search_synonyms table should exist")

	type columnInfo struct {
		dataType string
		nullable bool
	}
	rows, err := sharedPG.Pool.Query(ctx,
		`SELECT column_name, data_type, is_nullable = 'YES'
		 FROM information_schema.columns
		 WHERE table_schema = 'public'
		   AND table_name = '_ayb_search_synonyms'`,
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
		"group_id":    "uuid",
		"term":        "text",
		"created_at":  "timestamp with time zone",
	}
	for name, dataType := range expectedColumns {
		info, ok := columns[name]
		testutil.True(t, ok, "%s column should exist", name)
		testutil.Equal(t, dataType, info.dataType)
		testutil.False(t, info.nullable, "%s column should be NOT NULL", name)
	}
}

func assertSearchSynonymsLookupIndex(t *testing.T, ctx context.Context) {
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
		   AND tbl.relname = '_ayb_search_synonyms'
		   AND i.relname = 'uq_ayb_search_synonyms_collection_term'
		 GROUP BY ix.indisunique`,
	).Scan(&indexUnique, &indexKeys)
	testutil.NoError(t, err)
	testutil.True(t, indexUnique, "collection+term lookup index should enforce uniqueness")
	testutil.Equal(t, "schema_name,table_name,term", indexKeys)
}

func assertSearchSynonymsMembershipRules(t *testing.T, ctx context.Context) {
	t.Helper()

	movies := searchSynonymCollection{schemaName: "public", tableName: "movies"}
	privateMovies := searchSynonymCollection{schemaName: "private", tableName: "movies"}
	shows := searchSynonymCollection{schemaName: "public", tableName: "shows"}
	const carsGroupID = "11111111-1111-1111-1111-111111111111"
	insertSearchSynonym(t, ctx, searchSynonymMembership{collection: movies, groupID: carsGroupID, term: "car"})
	insertSearchSynonym(t, ctx, searchSynonymMembership{collection: movies, groupID: carsGroupID, term: "auto"})

	testutil.Equal(t, carsGroupID, lookupSynonymGroup(t, ctx, movies, "car"))
	testutil.Equal(t, carsGroupID, lookupSynonymGroup(t, ctx, movies, "auto"))

	insertSearchSynonym(t, ctx, searchSynonymMembership{
		collection: privateMovies,
		groupID:    "22222222-2222-2222-2222-222222222222",
		term:       "car",
	})
	insertSearchSynonym(t, ctx, searchSynonymMembership{
		collection: shows,
		groupID:    "33333333-3333-3333-3333-333333333333",
		term:       "car",
	})

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
		 VALUES ($1, $2, $3, $4)`,
		movies.schemaName, movies.tableName, "44444444-4444-4444-4444-444444444444", "car",
	)
	testutil.ErrorContains(t, err, "uq_ayb_search_synonyms_collection_term")

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
		 VALUES ($1, $2, $3, $4)`,
		movies.schemaName, movies.tableName, "55555555-5555-5555-5555-555555555555", "Car",
	)
	testutil.ErrorContains(t, err, "chk_ayb_search_synonyms_term_lowercase")
}

func insertSearchSynonym(t *testing.T, ctx context.Context, membership searchSynonymMembership) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
		 VALUES ($1, $2, $3, $4)`,
		membership.collection.schemaName,
		membership.collection.tableName,
		membership.groupID,
		membership.term,
	)
	testutil.NoError(t, err)
}

func lookupSynonymGroup(t *testing.T, ctx context.Context, collection searchSynonymCollection, term string) string {
	t.Helper()

	var groupID string
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT group_id::text
		 FROM _ayb_search_synonyms
		 WHERE schema_name = $1
		   AND table_name = $2
		   AND term = $3`,
		collection.schemaName, collection.tableName, term,
	).Scan(&groupID)
	testutil.NoError(t, err)
	return groupID
}
