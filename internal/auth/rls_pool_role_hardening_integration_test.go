//go:build integration

package auth_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

func TestRLSPoolRoleHardening(t *testing.T) {
	ctx := context.Background()
	pool := sharedPG.Pool
	ownerRole, tableName := newRLSPoolRoleFixtureNames()
	quotedOwner := sqlutil.QuoteIdent(ownerRole)
	quotedTable := sqlutil.QuoteIdent(tableName)
	var ownerPool *pgxpool.Pool

	t.Cleanup(func() {
		if ownerPool != nil {
			ownerPool.Close()
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanupRLSPoolRoleFixture(t, cleanupCtx, quotedOwner, quotedTable, ownerRole)
	})

	cleanupRLSPoolRoleFixture(t, ctx, quotedOwner, quotedTable, ownerRole)
	setupRLSPoolRoleFixture(t, ctx, quotedOwner, quotedTable)

	var err error
	ownerPool, err = newRLSPoolRoleOwnerPool(ctx, ownerRole)
	if err != nil {
		t.Fatalf("opening hosted owner pool: %v", err)
	}

	assertHostedOwnerRole(t, ctx, ownerPool, ownerRole)

	countA := countRows(t, ctx, ownerPool, quotedTable)
	recordRLSPoolRoleCountEvidence(t, "A", countA)
	if countA != 2 {
		t.Fatalf("leg A with ENABLE only returned %d rows, want 2", countA)
	}

	countCBeforeForce := countRowsAsAuthenticatedRole(t, ctx, ownerPool, quotedTable)
	recordRLSPoolRoleCountEvidence(t, "C_before_force", countCBeforeForce)
	if countCBeforeForce != 1 {
		t.Fatalf("leg C before FORCE returned %d rows, want 1", countCBeforeForce)
	}

	if _, err := pool.Exec(ctx, "ALTER TABLE "+quotedTable+" FORCE ROW LEVEL SECURITY"); err != nil {
		t.Fatalf("forcing RLS on probe table: %v", err)
	}

	countB := countRows(t, ctx, ownerPool, quotedTable)
	recordRLSPoolRoleCountEvidence(t, "B", countB)
	// Red specimen: if hosted_owner_<suffix> becomes superuser or BYPASSRLS,
	// or FORCE is not applied, leg B returns 2 instead of 0.
	if countB != 0 {
		t.Fatalf("leg B after FORCE returned %d rows, want 0", countB)
	}

	countCAfterForce := countRowsAsAuthenticatedRole(t, ctx, ownerPool, quotedTable)
	recordRLSPoolRoleCountEvidence(t, "C_after_force", countCAfterForce)
	if countCAfterForce != 1 {
		t.Fatalf("leg C after FORCE returned %d rows, want 1", countCAfterForce)
	}
}

func recordRLSPoolRoleCountEvidence(t *testing.T, label string, count int) {
	t.Helper()

	evidence := fmt.Sprintf("%s=%d", label, count)
	t.Logf("%s", evidence)
	t.Run(evidence, func(t *testing.T) {})
}

func newRLSPoolRoleFixtureNames() (string, string) {
	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), os.Getpid())
	return "hosted_owner_" + suffix, "rls_probe_" + suffix
}

func setupRLSPoolRoleFixture(t *testing.T, ctx context.Context, quotedOwner, quotedTable string) {
	t.Helper()

	statements := []string{
		`DO $$ BEGIN
			CREATE ROLE ` + sqlutil.QuoteIdent(auth.AuthenticatedRole) + ` NOLOGIN;
			EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,
		`ALTER ROLE ` + sqlutil.QuoteIdent(auth.AuthenticatedRole) + ` NOLOGIN NOSUPERUSER NOBYPASSRLS`,
		`CREATE ROLE ` + quotedOwner + ` LOGIN NOSUPERUSER NOBYPASSRLS`,
		`GRANT USAGE ON SCHEMA public TO ` + sqlutil.QuoteIdent(auth.AuthenticatedRole),
		`GRANT USAGE ON SCHEMA public TO ` + quotedOwner,
		`CREATE TABLE ` + quotedTable + ` (
			tenant_id TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			content TEXT NOT NULL
		)`,
		`INSERT INTO ` + quotedTable + ` (tenant_id, owner_id, content) VALUES
			('t-a', 'alice', 'A'),
			('t-b', 'bob', 'B')`,
		`ALTER TABLE ` + quotedTable + ` OWNER TO ` + quotedOwner,
		`ALTER TABLE ` + quotedTable + ` ENABLE ROW LEVEL SECURITY`,
		`CREATE POLICY tenant_owner_isolation ON ` + quotedTable + `
			FOR ALL TO ` + sqlutil.QuoteIdent(auth.AuthenticatedRole) + `
			USING (
				tenant_id = current_setting('ayb.tenant_id', true)
				AND owner_id = current_setting('ayb.user_id', true)
			)
			WITH CHECK (
				tenant_id = current_setting('ayb.tenant_id', true)
				AND owner_id = current_setting('ayb.user_id', true)
			)`,
		`GRANT SELECT ON ` + quotedTable + ` TO ` + sqlutil.QuoteIdent(auth.AuthenticatedRole),
		`GRANT ` + sqlutil.QuoteIdent(auth.AuthenticatedRole) + ` TO ` + quotedOwner,
	}

	for _, stmt := range statements {
		if _, err := sharedPG.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("setting up RLS pool-role fixture with %q: %v", stmt, err)
		}
	}
}

func newRLSPoolRoleOwnerPool(ctx context.Context, ownerRole string) (*pgxpool.Pool, error) {
	ownerURL, err := url.Parse(sharedPG.ConnString)
	if err != nil {
		return nil, fmt.Errorf("parsing shared Postgres DSN: %w", err)
	}
	ownerURL.User = url.User(ownerRole)

	cfg, err := pgxpool.ParseConfig(ownerURL.String())
	if err != nil {
		return nil, fmt.Errorf("parsing hosted owner pool config: %w", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	cfg.ConnConfig.StatementCacheCapacity = 0
	cfg.ConnConfig.DescriptionCacheCapacity = 0

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting hosted owner pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging hosted owner pool: %w", err)
	}
	return pool, nil
}

func assertHostedOwnerRole(t *testing.T, ctx context.Context, ownerPool *pgxpool.Pool, wantRole string) {
	t.Helper()

	var currentUser string
	var roleSuper, roleBypassRLS bool
	err := ownerPool.QueryRow(ctx, `
		SELECT current_user, rolsuper, rolbypassrls
		FROM pg_roles
		WHERE rolname = current_user
	`).Scan(&currentUser, &roleSuper, &roleBypassRLS)
	if err != nil {
		t.Fatalf("querying hosted owner role properties: %v", err)
	}
	if currentUser != wantRole {
		t.Fatalf("hosted owner pool current_user=%q, want %q", currentUser, wantRole)
	}
	if roleSuper || roleBypassRLS {
		t.Fatalf("hosted owner role must have rolsuper=false and rolbypassrls=false, got rolsuper=%t rolbypassrls=%t", roleSuper, roleBypassRLS)
	}
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, quotedTable string) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+quotedTable).Scan(&count); err != nil {
		t.Fatalf("counting rows in %s: %v", quotedTable, err)
	}
	return count
}

func countRowsAsAuthenticatedRole(t *testing.T, ctx context.Context, ownerPool *pgxpool.Pool, quotedTable string) int {
	t.Helper()

	tx, err := ownerPool.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning authenticated RLS transaction: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "alice"},
		TenantID:         "t-a",
		Email:            "alice@example.test",
	}
	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		t.Fatalf("setting authenticated RLS context: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM "+quotedTable).Scan(&count); err != nil {
		t.Fatalf("counting authenticated rows in %s: %v", quotedTable, err)
	}
	return count
}

func cleanupRLSPoolRoleFixture(t *testing.T, ctx context.Context, quotedOwner, quotedTable, ownerRole string) {
	t.Helper()

	if _, err := sharedPG.Pool.Exec(ctx, "DROP TABLE IF EXISTS "+quotedTable+" CASCADE"); err != nil {
		t.Fatalf("dropping RLS probe table: %v", err)
	}
	if roleExists(t, ctx, ownerRole) {
		if _, err := sharedPG.Pool.Exec(ctx, "DROP OWNED BY "+quotedOwner); err != nil {
			t.Fatalf("dropping objects owned by hosted owner: %v", err)
		}
	}
	if _, err := sharedPG.Pool.Exec(ctx, "DROP ROLE IF EXISTS "+quotedOwner); err != nil {
		t.Fatalf("dropping hosted owner role: %v", err)
	}
}

func roleExists(t *testing.T, ctx context.Context, roleName string) bool {
	t.Helper()

	var exists bool
	if err := sharedPG.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", roleName).Scan(&exists); err != nil {
		t.Fatalf("checking role existence for %q: %v", roleName, err)
	}
	return exists
}
