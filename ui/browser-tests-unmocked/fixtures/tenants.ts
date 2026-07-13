/** @module Browser-test fixtures for tenant seeding (via SQL for active state) and slug-based cleanup. */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral, validateResponse } from "./core";

export interface SeededTenantDashboardTenant {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
}

/** Inserts a tenant in active state via SQL (no admin API for provisioning to active) and returns its id, name, and slug. */
export async function seedTenantDashboardSmokeTenant(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededTenantDashboardTenant> {
  const tenantName = `Tenants Smoke Tenant ${suffix}`;
  const tenantSlug = `tenants-smoke-${suffix.toLowerCase()}`;

  // Stage 1 product gap: tenant activation has no admin API for provisioning -> active.
  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: tenant activation has no admin API for provisioning to active.
    `INSERT INTO _ayb_tenants (name, slug, state)
     VALUES ('${sqlLiteral(tenantName)}', '${sqlLiteral(tenantSlug)}', 'active')
     RETURNING id, name, slug`,
  );

  return {
    tenantId: String(result.rows[0][0]),
    tenantName: String(result.rows[0][1]),
    tenantSlug: String(result.rows[0][2]),
  };
}

export async function cleanupTenantDashboardSmokeTenant(
  request: APIRequestContext,
  token: string,
  tenantId: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/tenants/${encodeURIComponent(tenantId)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete tenant ${tenantId}`);
}

/** Lists all tenants and deletes those whose slug is in the given set. */
export async function cleanupTenantsBySlug(
  request: APIRequestContext,
  token: string,
  slugs: string[],
): Promise<void> {
  if (slugs.length === 0) {
    return;
  }
  const res = await request.get("/api/admin/tenants?perPage=100", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List tenants for slug cleanup");
  const body = await res.json();
  const items = Array.isArray(body?.items) ? body.items : [];
  const slugSet = new Set(slugs);
  for (const item of items) {
    if (typeof item?.id !== "string" || typeof item?.slug !== "string" || !slugSet.has(item.slug)) {
      continue;
    }
    await cleanupTenantDashboardSmokeTenant(request, token, item.id).catch(() => {});
  }
}
