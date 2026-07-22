//go:build integration

package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetTestPool returns a connection pool to an isolated temporary database
// created from TEST_DATABASE_URL. It preserves the existing skip contract when
// the variable is unset so integration callers can remain environment-aware.
func GetTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	pg, cleanup := StartPostgresForTestMain(context.Background())
	t.Cleanup(cleanup)
	return pg.Pool
}

// ExecSQL executes statement text against the provided pool and fails the test
// with SQL context on any execution error.
func ExecSQL(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()

	if err := execSQL(context.Background(), pool, sql); err != nil {
		t.Fatalf("%v", err)
	}
}

func execSQL(ctx context.Context, pool *pgxpool.Pool, sql string) error {
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec SQL: %v\nSQL: %s", err, sql)
	}
	return nil
}
