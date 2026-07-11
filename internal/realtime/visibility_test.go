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
	// When pool is nil, RLS filtering is disabled — all events pass through.
	t.Parallel()

	h := &Handler{pool: nil}
	event := &Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}}
	testutil.True(t, h.canSeeRecord(context.TODO(), nil, "public", event), "nil pool should allow all events")
}

func TestCanSeeRecordNilPoolAllActions(t *testing.T) {
	// Verify nil pool allows all event types.
	t.Parallel()

	h := &Handler{pool: nil}
	for _, action := range []string{"create", "update", "delete"} {
		event := &Event{Action: action, Table: "posts", Record: map[string]any{"id": 1}}
		testutil.True(t, h.canSeeRecord(context.TODO(), nil, "public", event),
			"nil pool should allow %s events", action)
	}
}

func TestCanSeeRecordDeleteOldRecordNilFailsOpen(t *testing.T) {
	t.Parallel()

	event := &Event{
		Action: "delete",
		Table:  "posts",
		Record: map[string]any{"id": 1},
	}

	testutil.True(t, canSeeRecordWithCache(event, "public", rlsEnabledPostTable()), "nil OldRecord should fail open")
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
