package realtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildVisibilityCheckSinglePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"id": 42, "title": "Hello"}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."posts" WHERE "id" = $1`, query)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, 42, args[0])
}

func TestBuildVisibilityCheckCompositePK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "order_items",
		PrimaryKey: []string{"order_id", "item_id"},
	}
	record := map[string]any{"order_id": 1, "item_id": 5, "qty": 3}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, `SELECT 1 FROM "public"."order_items" WHERE "order_id" = $1 AND "item_id" = $2`, query)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 1, args[0])
	testutil.Equal(t, 5, args[1])
}

func TestBuildVisibilityCheckMissingPK(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}
	record := map[string]any{"title": "Hello"} // no "id"
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Equal(t, "", query)
	testutil.Nil(t, args)
}

func TestCanSeeRecordNilPool(t *testing.T) {
	t.Parallel()

	claims := &auth.Claims{Email: "user@example.com"}
	claims.Subject = "user-1"
	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}}

	got := CanSeeRecord(context.TODO(), nil, nil, testutil.DiscardLogger(), claims, "public", event)
	testutil.Equal(t, false, got)
}

func TestCanSeeRecordNilPoolAllActions(t *testing.T) {
	t.Parallel()

	claims := &auth.Claims{Email: "user@example.com"}
	claims.Subject = "user-1"
	for _, action := range []string{"create", "update", "delete"} {
		t.Run(action, func(t *testing.T) {
			event := &Event{Action: action, Table: "posts", Record: map[string]any{"id": 1}}
			got := CanSeeRecord(context.TODO(), nil, nil, testutil.DiscardLogger(), claims, "public", event)
			testutil.Equal(t, false, got)
		})
	}
}

func TestCanSeeRecordDeleteOldRecordNilFailsClosed(t *testing.T) {
	t.Parallel()

	event := &Event{
		Action: "delete",
		Table:  "posts",
		Record: map[string]any{"id": 1},
	}

	got := canSeeRecordWithCache(event, "public", rlsEnabledPostTable())
	testutil.Equal(t, false, got)
}

func TestCanSeeRecordDeleteMissingPrimaryKeyFailsOpen(t *testing.T) {
	t.Parallel()

	event := &Event{
		Action:    "delete",
		Table:     "posts",
		Record:    map[string]any{"id": 1},
		OldRecord: map[string]any{"title": "Hello"},
	}

	testutil.True(t, canSeeRecordWithCache(event, "public", rlsEnabledPostTable()), "missing OldRecord PK should fail open")
}

func TestCanSeeRecordDeleteWithoutSelectPoliciesFailsOpen(t *testing.T) {
	t.Parallel()

	event := &Event{
		Action:    "delete",
		Table:     "posts",
		Record:    map[string]any{"id": 1},
		OldRecord: map[string]any{"id": 1, "title": "Hello"},
	}
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
		RLSEnabled: true,
	}

	testutil.True(t, canSeeRecordWithCache(event, "public", tbl), "table without SELECT policies should fail open")
}

func TestCanSeeRecordPublicMissingMetadataFailsOpen(t *testing.T) {
	t.Parallel()

	event := &Event{Action: "create", Table: "missing", Record: map[string]any{"id": 1}}
	testutil.True(t, canSeeRecordWithCache(event, "public"), "public missing metadata should fail open")
}

func TestCanSeeRecordNonPublicMissingMetadataFailsClosed(t *testing.T) {
	t.Parallel()

	event := &Event{Action: "create", Table: "missing", Record: map[string]any{"id": 1}}
	testutil.False(t, canSeeRecordWithCache(event, "tenant_a"), "non-public missing metadata should fail closed")
}

func TestCanSeeRecordNonPublicMissingPrimaryKeyFailsClosed(t *testing.T) {
	t.Parallel()

	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"title": "Hello"}}
	testutil.False(t, canSeeRecordWithCache(event, "tenant_a", &schema.Table{
		Schema:     "tenant_a",
		Name:       "posts",
		PrimaryKey: []string{"id"},
	}), "non-public missing record PK should fail closed")
}

func TestCanSeeRecordForeignTenantWithoutSelectPolicyFailsClosed(t *testing.T) {
	// The tenant-isolation floor drops a candidate tagged for another tenant when
	// the table has no enforceable RLS SELECT policy, without ever running the
	// per-record SELECT (so an empty pool is never dereferenced).
	t.Parallel()

	event := &Event{
		Action:   "create",
		Table:    "users",
		TenantID: "tenant-b",
		Record:   map[string]any{"id": 1},
	}
	tbl := &schema.Table{Schema: "public", Name: "users", PrimaryKey: []string{"id"}}

	testutil.False(t, canSeeRecordForTenant(event, "public", "tenant-a", tbl),
		"authenticated tenant-a subscriber must drop a foreign tenant-b candidate on a table without a SELECT policy")
}

func TestCanSeeRecordForeignTenantMissingMetadataFailsClosed(t *testing.T) {
	// A foreign-tenant candidate on a public table with no cached metadata must
	// fail closed rather than fall through the public missing-metadata bypass.
	t.Parallel()

	event := &Event{
		Action:   "create",
		Table:    "missing",
		TenantID: "tenant-b",
		Record:   map[string]any{"id": 1},
	}

	testutil.False(t, canSeeRecordForTenant(event, "public", "tenant-a"),
		"foreign-tenant candidate on an unmapped public table must fail closed")
}

func TestCanSeeRecordForeignTenantWithSelectPolicyButMissingPrimaryKeyFailsClosed(t *testing.T) {
	// A SELECT policy is not enough on its own; without PK metadata, the
	// per-record visibility query cannot be built and the foreign-tenant
	// candidate must fail closed instead of falling through the public-table
	// bypass.
	t.Parallel()

	event := &Event{
		Action:   "create",
		Table:    "users",
		TenantID: "tenant-b",
		Record:   map[string]any{"id": 1},
	}
	tbl := &schema.Table{
		Schema:     "public",
		Name:       "users",
		RLSEnabled: true,
		RLSPolicies: []*schema.RLSPolicy{
			{Name: "users_select", Command: "SELECT", UsingExpr: "tenant_id = current_setting('ayb.tenant_id', true)"},
		},
	}

	testutil.False(t, canSeeRecordForTenant(event, "public", "tenant-a", tbl),
		"foreign-tenant candidate without PK metadata must fail closed even when the table has a SELECT policy")
}

func TestCanSeeRecordSameTenantEmptyTenantStillReachRLS(t *testing.T) {
	// The floor only fires for a tenant mismatch; an empty event tenant is the
	// _ayb_notifications / wildcard case and must not be dropped by the floor.
	t.Parallel()

	event := &Event{Action: "create", Table: "missing", Record: map[string]any{"id": 1}}
	testutil.True(t, canSeeRecordForTenant(event, "public", "tenant-a"),
		"empty-tenant candidate must retain the public wildcard bypass regardless of subscriber tenant")
}

func TestBuildVisibilityCheckQuotesIdentifiers(t *testing.T) {
	// Verify schema, table, and column names are properly double-quoted.
	t.Parallel()

	tbl := &schema.Table{
		Schema:     "my_schema",
		Name:       "my_table",
		PrimaryKey: []string{"my_col"},
	}
	record := map[string]any{"my_col": 1}
	query, args := buildVisibilityCheck(tbl, record)

	testutil.Contains(t, query, `"my_schema"`)
	testutil.Contains(t, query, `"my_table"`)
	testutil.Contains(t, query, `"my_col"`)
	testutil.Equal(t, 1, len(args))
}

func TestBuildDeletedVisibilityCheckCastsRecordColumns(t *testing.T) {
	t.Parallel()

	tbl := &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
		Columns: []*schema.Column{
			{Name: "id", TypeName: "bigint"},
			{Name: "sentinel", TypeName: "text"},
			{Name: "tenant_id", TypeName: "uuid"},
		},
	}
	record := map[string]any{
		"id":        int64(2),
		"sentinel":  "deleted",
		"tenant_id": "8d6045f6-7716-4ab8-9f7d-4ffbf803bd82",
	}

	query, args := buildDeletedVisibilityCheck(tbl, `tenant_id = current_setting('ayb.tenant_id', true)::uuid`, record)

	testutil.Equal(t,
		`SELECT 1 FROM (VALUES ($1::bigint, $2::text, $3::uuid)) AS "posts" ("id", "sentinel", "tenant_id") WHERE tenant_id = current_setting('ayb.tenant_id', true)::uuid`,
		query)
	wantArgs := []any{int64(2), "deleted", "8d6045f6-7716-4ab8-9f7d-4ffbf803bd82"}
	if !reflect.DeepEqual(wantArgs, args) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestDeletedRecordPlaceholderRejectsUnsafeTypeName(t *testing.T) {
	t.Parallel()

	tbl := &schema.Table{
		Name:    "posts",
		Columns: []*schema.Column{{Name: "tenant_id", TypeName: "uuid); DROP TABLE posts; --"}},
	}

	testutil.Equal(t, "$1", deletedRecordPlaceholder(tbl, "tenant_id", 1))
}

func canSeeRecordWithCache(event *Event, activeSchema string, tables ...*schema.Table) bool {
	return canSeeRecordForTenantWithClaimTenant(event, activeSchema, "", tables...)
}

// canSeeRecordForTenant evaluates CanSeeRecord for a subscriber whose claims are
// scoped to claimTenant. It exercises the tenant-isolation floor without a live
// pool: the floor returns before any per-record SELECT, so the empty pool is
// never dereferenced.
func canSeeRecordForTenant(event *Event, activeSchema, claimTenant string, tables ...*schema.Table) bool {
	return canSeeRecordForTenantWithClaimTenant(event, activeSchema, claimTenant, tables...)
}

func canSeeRecordForTenantWithClaimTenant(event *Event, activeSchema, claimTenant string, tables ...*schema.Table) bool {
	cache := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	tableMap := map[string]*schema.Table{}
	for _, tbl := range tables {
		tableMap[tbl.Schema+"."+tbl.Name] = tbl
	}
	cache.SetForTesting(&schema.SchemaCache{
		Tables: tableMap,
	})
	claims := &auth.Claims{Email: "user@example.com"}
	claims.Subject = "user-1"
	claims.TenantID = claimTenant
	return CanSeeRecord(context.Background(), &pgxpool.Pool{}, cache, testutil.DiscardLogger(), claims, activeSchema, event)
}

func rlsEnabledPostTable() *schema.Table {
	return &schema.Table{
		Schema:     "public",
		Name:       "posts",
		PrimaryKey: []string{"id"},
		RLSEnabled: true,
		RLSPolicies: []*schema.RLSPolicy{
			{
				Name:      "posts_select",
				Command:   "SELECT",
				UsingExpr: "owner_id = current_setting('ayb.user_id', true)",
			},
		},
	}
}
