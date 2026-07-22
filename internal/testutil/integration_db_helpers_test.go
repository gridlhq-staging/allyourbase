//go:build integration

package testutil

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_GetTestPoolReturnsUsablePool(t *testing.T) {
	pool := GetTestPool(t)

	var one int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query with integration test pool: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 returned %d, want 1", one)
	}
}

func TestIntegration_GetTestPoolIsolatesCallers(t *testing.T) {
	first := GetTestPool(t)
	second := GetTestPool(t)

	var firstName, secondName string
	if err := first.QueryRow(context.Background(), "SELECT current_database()").Scan(&firstName); err != nil {
		t.Fatalf("querying first database name: %v", err)
	}
	if err := second.QueryRow(context.Background(), "SELECT current_database()").Scan(&secondName); err != nil {
		t.Fatalf("querying second database name: %v", err)
	}
	if firstName == secondName {
		t.Fatalf("GetTestPool callers share database %q; want isolated databases", firstName)
	}
}

func TestIntegration_execSQLIncludesStatementOnError(t *testing.T) {
	pool := GetTestPool(t)
	badSQL := "SELECT definitely_missing_column FROM definitely_missing_table"

	err := execSQL(context.Background(), pool, badSQL)
	if err == nil {
		t.Fatal("execSQL error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), badSQL) {
		t.Fatalf("execSQL error should include statement text; got %q", err.Error())
	}
}
