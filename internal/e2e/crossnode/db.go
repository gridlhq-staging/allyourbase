package crossnode

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedRealtimeProofTable (re)creates a minimal id/sentinel table that the
// realtime fanout proof writes into and observes events from. tableName is a
// caller-controlled identifier and is sanitized before interpolation.
func SeedRealtimeProofTable(ctx context.Context, t *testing.T, databaseURL, tableName string) {
	t.Helper()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database for realtime seed: %v", err)
	}
	defer pool.Close()

	table := pgx.Identifier{tableName}.Sanitize()
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		DROP TABLE IF EXISTS %s;
		CREATE TABLE %s (
			id BIGSERIAL PRIMARY KEY,
			sentinel TEXT NOT NULL
		);
	`, table, table))
	if err != nil {
		t.Fatalf("seed realtime proof table: %v", err)
	}
}

// ConfigureUsersAsSharedTenants flips each user's resolved tenant into the
// "shared" isolation mode and marks it active, so cross-node proofs observe a
// single shared tenant. It fails if any user does not resolve to a tenant.
func ConfigureUsersAsSharedTenants(ctx context.Context, t *testing.T, databaseURL string, userIDs ...string) {
	t.Helper()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database for tenant mode setup: %v", err)
	}
	defer pool.Close()

	for _, userID := range userIDs {
		var tenantID string
		err := pool.QueryRow(ctx, `
			UPDATE _ayb_tenants AS tenants
			   SET isolation_mode = 'shared', state = 'active', updated_at = NOW()
			  FROM _ayb_tenant_memberships AS memberships
			 WHERE memberships.tenant_id = tenants.id
			   AND memberships.user_id = $1
			 RETURNING tenants.id::text
		`, userID).Scan(&tenantID)
		if err != nil {
			if err == pgx.ErrNoRows {
				t.Fatalf("user %s did not resolve to a tenant", userID)
			}
			t.Fatalf("configure user %s tenant as shared: %v", userID, err)
		}
		if tenantID == "" {
			t.Fatalf("user %s did not resolve to a tenant", userID)
		}
	}
}
