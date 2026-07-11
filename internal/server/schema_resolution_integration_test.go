//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestTenantSchemaScopedCRUDResolution(t *testing.T) {
	h := setupHostedModeLifecycleHarness(t)
	ctx := context.Background()

	adminToken := h.adminLogin(t)
	h.ensureUser(t, stageIntegrationOwnerUserID)
	h.ensureUser(t, stageIntegrationMemberUserID)

	slugBase := fmt.Sprintf("schema-crud-resolution-%d", time.Now().UnixNano())
	tenantA := stage7CreateTenant(t, h.srv, adminToken, slugBase+"-a", "schema")
	tenantB := stage7CreateTenant(t, h.srv, adminToken, slugBase+"-b", "schema")
	testutil.Equal(t, tenant.TenantStateActive, tenantA.State)
	testutil.Equal(t, tenant.TenantStateActive, tenantB.State)

	createDivergentItemsTables(t, ctx, h, tenantA, tenantB)
	testutil.NoError(t, h.srv.schema.Reload(ctx))

	headers := h.tenantAuthHeaders(
		t,
		stageIntegrationOwnerUserID,
		"integration-owner@example.com",
		tenantA.ID,
	)
	resp := h.collectionRequest(t, http.MethodPost, "collections/items", map[string]string{
		"tenant_note": "tenant-a-value",
	}, headers)

	if resp.Code != http.StatusCreated {
		t.Fatalf("tenant A items create status = %d, want %d; body: %s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	var body map[string]any
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	testutil.Equal(t, "tenant-a-value", body["tenant_note"])
	testutil.True(t, body["public_note"] == nil, "public items metadata must not be used")
	testutil.True(t, body["tenant_b_note"] == nil, "tenant B items metadata must not be used")
}

func createDivergentItemsTables(
	t *testing.T,
	ctx context.Context,
	h *hostedModeLifecycleHarness,
	tenantA tenant.Tenant,
	tenantB tenant.Tenant,
) {
	t.Helper()

	schemaA := pgx.Identifier{tenantA.Slug}.Sanitize()
	schemaB := pgx.Identifier{tenantB.Slug}.Sanitize()

	_, err := h.pool.Exec(ctx, `CREATE TABLE public.items (id SERIAL PRIMARY KEY, public_note TEXT NOT NULL)`)
	testutil.NoError(t, err)
	grantItemsTableAccess(t, ctx, h, "public")
	_, err = h.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s.items (id SERIAL PRIMARY KEY, tenant_note TEXT NOT NULL)`,
		schemaA,
	))
	testutil.NoError(t, err)
	grantItemsTableAccess(t, ctx, h, schemaA)
	_, err = h.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s.items (id SERIAL PRIMARY KEY, tenant_b_note TEXT NOT NULL)`,
		schemaB,
	))
	testutil.NoError(t, err)
	grantItemsTableAccess(t, ctx, h, schemaB)
}

func grantItemsTableAccess(t *testing.T, ctx context.Context, h *hostedModeLifecycleHarness, schemaIdent string) {
	t.Helper()

	_, err := h.pool.Exec(ctx, fmt.Sprintf(
		`GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE %s.items TO ayb_authenticated`,
		schemaIdent,
	))
	testutil.NoError(t, err)
	_, err = h.pool.Exec(ctx, fmt.Sprintf(
		`GRANT USAGE, SELECT ON SEQUENCE %s.items_id_seq TO ayb_authenticated`,
		schemaIdent,
	))
	testutil.NoError(t, err)
}
