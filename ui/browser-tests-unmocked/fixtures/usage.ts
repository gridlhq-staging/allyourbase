/** @module Browser-test fixtures for usage metering tenant creation and daily usage row seeding. */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral, validateResponse } from "./core";

export interface SeededUsageMeteringTenant {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
}

const USAGE_SMOKE_REQUEST_COUNT_TODAY = 4_000_000_000_000_000;
const USAGE_SMOKE_REQUEST_COUNT_YESTERDAY = 2_000_000_000_000_000;

/** Creates a tenant, activates it via SQL, and inserts two usage daily rows (today + yesterday) with large sentinel values. */
export async function seedUsageMeteringTenantDailyRows(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededUsageMeteringTenant> {
  const tenantName = `Usage Smoke Tenant ${suffix}`;
  const tenantSlug = `usage-smoke-${suffix.toLowerCase()}`;

  const tenantRes = await request.post("/api/admin/tenants", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: tenantName,
      slug: tenantSlug,
      isolationMode: "shared",
      planTier: "free",
    },
  });
  await validateResponse(tenantRes, `Create usage metering tenant ${tenantSlug}`);
  const tenant = await tenantRes.json();

  const tenantId = String(tenant.id);

  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: tenant activation has no admin API for provisioning to active.
    `UPDATE _ayb_tenants
     SET state = 'active', updated_at = NOW()
     WHERE id = '${sqlLiteral(tenantId)}'`,
  );

  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: usage metering history has no deterministic seed API.
    `INSERT INTO _ayb_tenant_usage_daily (
       tenant_id,
       date,
       request_count,
       db_bytes_used,
       bandwidth_bytes,
       function_invocations,
       realtime_peak_connections,
       job_runs
     )
     VALUES
       ('${tenantId}', CURRENT_DATE, ${USAGE_SMOKE_REQUEST_COUNT_TODAY}, 8192, 16384, 240, 18, 11),
       ('${tenantId}', (CURRENT_DATE - INTERVAL '1 day')::date, ${USAGE_SMOKE_REQUEST_COUNT_YESTERDAY}, 4096, 8192, 120, 12, 6)
     ON CONFLICT (tenant_id, date) DO UPDATE SET
       request_count = EXCLUDED.request_count,
       db_bytes_used = EXCLUDED.db_bytes_used,
       bandwidth_bytes = EXCLUDED.bandwidth_bytes,
       function_invocations = EXCLUDED.function_invocations,
       realtime_peak_connections = EXCLUDED.realtime_peak_connections,
       job_runs = EXCLUDED.job_runs`,
  );

  return { tenantId, tenantName, tenantSlug };
}

export async function cleanupUsageMeteringTenant(
  request: APIRequestContext,
  token: string,
  tenantId: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/tenants/${encodeURIComponent(tenantId)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete usage metering tenant ${tenantId}`);
}
