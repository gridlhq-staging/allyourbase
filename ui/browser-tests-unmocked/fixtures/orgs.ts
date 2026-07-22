/** @module Browser-test fixtures for organization seeding and slug-based cleanup. */
import type { APIRequestContext } from "@playwright/test";
import { validateResponse } from "./core";

export interface SeededOrganizationDashboardOrg {
  orgId: string;
  orgName: string;
  orgSlug: string;
}

/** Creates an organization with a generated name and slug from the given suffix. */
export async function seedOrganizationDashboardSmokeOrg(
  request: APIRequestContext,
  token: string,
  suffix: string,
): Promise<SeededOrganizationDashboardOrg> {
  const orgName = `Organizations Smoke Org ${suffix}`;
  const orgSlug = `organizations-smoke-${suffix.toLowerCase()}`;

  const res = await request.post("/api/admin/orgs", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name: orgName, slug: orgSlug, planTier: "free" },
  });
  await validateResponse(res, `Create organization ${orgSlug}`);
  const org = await res.json();

  return {
    orgId: String(org.id),
    orgName: String(org.name),
    orgSlug: String(org.slug),
  };
}

export async function cleanupOrganizationDashboardSmokeOrg(
  request: APIRequestContext,
  token: string,
  orgId: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/orgs/${encodeURIComponent(orgId)}?confirm=true`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete organization ${orgId}`);
}

/** Lists all organizations and deletes those whose slug is in the given set. */
export async function cleanupOrganizationsBySlug(
  request: APIRequestContext,
  token: string,
  slugs: string[],
): Promise<void> {
  if (slugs.length === 0) {
    return;
  }
  const res = await request.get("/api/admin/orgs", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List organizations for slug cleanup");
  const body = await res.json();
  const items = Array.isArray(body?.items) ? body.items : [];
  const slugSet = new Set(slugs);
  for (const item of items) {
    if (typeof item?.id !== "string" || typeof item?.slug !== "string" || !slugSet.has(item.slug)) {
      continue;
    }
    await cleanupOrganizationDashboardSmokeOrg(request, token, item.id).catch(() => {});
  }
}

// Fixture helper: fetch an organization by ID via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule.
export async function getOrganizationById(
  request: APIRequestContext,
  adminToken: string,
  orgId: string,
): Promise<{ status: number; body: unknown }> {
  const res = await request.get(`/api/admin/orgs/${orgId}`, {
    headers: { Authorization: `Bearer ${adminToken}` },
  });
  const body = res.ok() ? await res.json().catch(() => null) : null;
  return { status: res.status(), body };
}
