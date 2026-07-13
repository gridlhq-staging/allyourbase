import { describe, expect, it, vi } from "vitest";
import type { APIRequestContext } from "@playwright/test";
import {
  cleanupUsageMeteringTenant,
  seedUsageMeteringTenantDailyRows,
} from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown, statusCode = 200) {
  return {
    ok: () => statusCode >= 200 && statusCode < 300,
    status: () => statusCode,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function noContentResponse() {
  return {
    ok: () => true,
    status: () => 204,
    json: async () => ({}),
    text: async () => "",
  };
}

function buildUsageRequestMock() {
  const posts: Array<{ path: string; data?: unknown; headers?: unknown }> = [];
  const deletes: Array<{ path: string; headers?: unknown }> = [];

  const request = {
    post: vi.fn(async (path: string, init?: { data?: unknown; headers?: unknown }) => {
      posts.push({ path, data: init?.data, headers: init?.headers });
      if (path === "/api/admin/tenants") {
        return okResponse({
          id: "tenant-001",
          name: "Usage Smoke Tenant RunOne",
          slug: "usage-smoke-runone",
        });
      }
      if (path === "/api/admin/sql") {
        return okResponse({ columns: [], rows: [], rowCount: 2 });
      }
      throw new Error(`Unexpected POST ${path}`);
    }),
    delete: vi.fn(async (path: string, init?: { headers?: unknown }) => {
      deletes.push({ path, headers: init?.headers });
      return noContentResponse();
    }),
  };

  return { request: request as unknown as APIRequestContext, posts, deletes };
}

describe("browser-unmocked usage fixture helpers", () => {
  it("creates the smoke tenant with the canonical shared isolation mode", async () => {
    const { request, posts } = buildUsageRequestMock();

    const seeded = await seedUsageMeteringTenantDailyRows(request, "admin-token", "RunOne");

    expect(seeded).toEqual({
      tenantId: "tenant-001",
      tenantName: "Usage Smoke Tenant RunOne",
      tenantSlug: "usage-smoke-runone",
    });
    expect(posts[0]).toMatchObject({
      path: "/api/admin/tenants",
      data: {
        name: "Usage Smoke Tenant RunOne",
        slug: "usage-smoke-runone",
        isolationMode: "shared",
        planTier: "free",
      },
      headers: { Authorization: "Bearer admin-token", "Content-Type": "application/json" },
    });
  });

  it("seeds daily usage rows for today and yesterday", async () => {
    const { request, posts } = buildUsageRequestMock();

    await seedUsageMeteringTenantDailyRows(request, "admin-token", "RunOne");

    const activationQuery = (posts[1]?.data as { query?: string } | undefined)?.query ?? "";
    expect(posts[1]?.path).toBe("/api/admin/sql");
    expect(activationQuery).toContain("UPDATE _ayb_tenants");
    expect(activationQuery).toContain("SET state = 'active'");
    expect(activationQuery).toContain("WHERE id = 'tenant-001'");

    const usageSeedQuery = (posts[2]?.data as { query?: string } | undefined)?.query ?? "";
    expect(posts[2]?.path).toBe("/api/admin/sql");
    expect(usageSeedQuery).toContain("INSERT INTO _ayb_tenant_usage_daily");
    expect(usageSeedQuery).toContain("'tenant-001'");
    expect(usageSeedQuery).toContain("4000000000000000");
    expect(usageSeedQuery).toContain("2000000000000000");
    expect(usageSeedQuery).toContain("ON CONFLICT (tenant_id, date) DO UPDATE");
  });

  it("deletes the seeded tenant through the admin API", async () => {
    const { request, deletes } = buildUsageRequestMock();

    await cleanupUsageMeteringTenant(request, "admin-token", "tenant-001");

    expect(deletes).toEqual([
      {
        path: "/api/admin/tenants/tenant-001",
        headers: { Authorization: "Bearer admin-token" },
      },
    ]);
  });
});
