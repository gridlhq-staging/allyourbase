//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRevokedSessionsMigrationContract(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh install", func(t *testing.T) {
		resetDB(t, ctx)
		runFullMigrations(t, ctx)

		assertRevokedSessionsSchema(t, ctx)
	})

	t.Run("legacy upgrade", func(t *testing.T) {
		resetDB(t, ctx)
		runLegacyRevokedSessionsMigrations(t, ctx)

		runFullMigrations(t, ctx)

		assertRevokedSessionsSchema(t, ctx)
	})
}

func runLegacyRevokedSessionsMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunnerWithFS(sharedPG.Pool, testutil.DiscardLogger(), migrationFSUpTo(t, 176))
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

func assertRevokedSessionsSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	assertRevokedSessionColumn(t, ctx, "session_id", "text", false, "")
	assertRevokedSessionColumn(t, ctx, "expires_at", "timestamp with time zone", false, "")
	assertRevokedSessionColumn(t, ctx, "created_at", "timestamp with time zone", false, "now()")
	assertRevokedSessionsPrimaryKey(t, ctx)
	assertIndexKeyPresent(t, ctx, "_ayb_revoked_sessions", "expires_at")
}

func assertRevokedSessionColumn(
	t *testing.T,
	ctx context.Context,
	columnName string,
	expectedType string,
	expectedNullable bool,
	expectedDefault string,
) {
	t.Helper()

	var dataType string
	var nullable bool
	var defaultExpression *string
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT c.data_type,
		        c.is_nullable = 'YES',
		        pg_get_expr(d.adbin, d.adrelid)
		   FROM information_schema.columns c
		   LEFT JOIN pg_class tbl ON tbl.relname = c.table_name
		   LEFT JOIN pg_namespace n ON n.oid = tbl.relnamespace
		   LEFT JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attname = c.column_name
		   LEFT JOIN pg_attrdef d ON d.adrelid = tbl.oid AND d.adnum = a.attnum
		  WHERE c.table_schema = 'public'
		    AND c.table_name = '_ayb_revoked_sessions'
		    AND c.column_name = $1
		    AND n.nspname = 'public'`,
		columnName,
	).Scan(&dataType, &nullable, &defaultExpression)
	testutil.NoError(t, err)
	testutil.Equal(t, expectedType, dataType)
	testutil.Equal(t, expectedNullable, nullable)
	if expectedDefault == "" {
		testutil.True(t, defaultExpression == nil || strings.TrimSpace(*defaultExpression) == "",
			"%s should not have a default expression", columnName)
		return
	}
	if defaultExpression == nil {
		t.Fatalf("%s should have default expression %q", columnName, expectedDefault)
	}
	testutil.Equal(t, expectedDefault, strings.TrimSpace(*defaultExpression))
}

func assertRevokedSessionsPrimaryKey(t *testing.T, ctx context.Context) {
	t.Helper()

	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM pg_index ix
		       JOIN pg_class tbl ON tbl.oid = ix.indrelid
		       JOIN pg_namespace n ON n.oid = tbl.relnamespace
		       JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ordinality) ON TRUE
		       JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = k.attnum
		      WHERE n.nspname = 'public'
		        AND tbl.relname = '_ayb_revoked_sessions'
		        AND ix.indisprimary
		      GROUP BY ix.indexrelid
		     HAVING string_agg(a.attname, ',' ORDER BY k.ordinality) = 'session_id'
		 )`,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_revoked_sessions should use session_id as primary key")
}
