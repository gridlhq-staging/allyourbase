import { describe, it, expect, vi } from "vitest";
import type { APIRequestContext } from "@playwright/test";
import {
  cleanupPushTestData,
  isPushEnabled,
  seedPushDelivery,
  seedPushDeviceToken,
} from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown) {
  return {
    ok: () => true,
    status: () => 200,
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

function buildPushRequestMock() {
  const queries: string[] = [];
  const posts: Array<{ path: string; data?: unknown }> = [];
  const deletes: string[] = [];

  const request = {
    post: vi.fn(async (path: string, init?: { data?: { query?: string } }) => {
      posts.push({ path, data: init?.data });
      if (path === "/api/admin/sql") {
        const query = init?.data?.query || "";
        queries.push(query);
        if (query.includes("RETURNING id, title, status, device_token_id")) {
          return okResponse({
            columns: ["id", "title", "status", "device_token_id"],
            rows: [["delivery-001", "Push fixture lifecycle", "sent", "device-001"]],
            rowCount: 1,
          });
        }
        return okResponse({ columns: [], rows: [], rowCount: 0 });
      }
      if (path === "/api/auth/anonymous") {
        return okResponse({ token: "anonymous-token" });
      }
      if (path === "/api/auth/link/email") {
        return okResponse({ token: "linked-token", user: { is_anonymous: false } });
      }
      if (path === "/api/admin/apps") {
        return okResponse({ id: "app-001", name: "push-test-app" });
      }
      if (path === "/api/admin/push/devices") {
        return okResponse({
          id: "device-001",
          token: "tok-001",
          provider: "fcm",
          platform: "android",
        });
      }
      throw new Error(`Unexpected POST ${path}`);
    }),
    get: vi.fn(async (path: string) => {
      if (path.startsWith("/api/admin/apps")) {
        return okResponse({ items: [] });
      }
      if (path.startsWith("/api/admin/push/devices")) {
        return okResponse({ items: [{ id: "device-001", token: "fixture-token" }] });
      }
      if (path === "/api/auth/me") {
        return okResponse({ id: "user-001", email: "push-fixture-test@example.com" });
      }
      throw new Error(`Unexpected GET ${path}`);
    }),
    delete: vi.fn(async (path: string) => {
      deletes.push(path);
      return noContentResponse();
    }),
  };

  return { request: request as unknown as APIRequestContext, queries, posts, deletes };
}

describe("browser-unmocked push fixture helpers", () => {
  it("seedPushDeviceToken uses auth, app, and push device APIs", async () => {
    const { request, queries, posts } = buildPushRequestMock();

    await seedPushDeviceToken(request, "admin-token", { tokenValue: "fixture-token-1" });

    expect(queries).toHaveLength(0);
    expect(posts.map((call) => call.path)).toEqual([
      "/api/auth/anonymous",
      "/api/auth/link/email",
      "/api/admin/apps",
      "/api/admin/push/devices",
    ]);
  });

  it("cleanupPushTestData revokes matching push devices through the admin API", async () => {
    const { request, queries, deletes } = buildPushRequestMock();

    await cleanupPushTestData(request, "admin-token", "fixture-token");

    expect(queries).toHaveLength(1);
    expect(queries[0]).toContain("DELETE FROM _ayb_push_deliveries");
    expect(deletes).toEqual(["/api/admin/push/devices/device-001"]);
  });

  it("isPushEnabled only treats 200 as enabled and 503 as disabled", async () => {
    const request401 = {
      get: vi.fn(async () => ({
        status: () => 401,
        text: async () => "unauthorized",
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request401, "bad-token")).rejects.toThrow(
      "Push enablement check failed with status 401",
    );

    const request503 = {
      get: vi.fn(async () => ({
        status: () => 503,
        text: async () => "Push service is not enabled",
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request503, "admin-token")).resolves.toBe(false);

    const request200 = {
      get: vi.fn(async () => ({
        status: () => 200,
        text: async () => '{"items":[]}',
      })),
    } as unknown as APIRequestContext;
    await expect(isPushEnabled(request200, "admin-token")).resolves.toBe(true);
  });

  it("escapes SQL literals in push fixture helpers", async () => {
    const { request, posts } = buildPushRequestMock();

    await seedPushDeviceToken(request, "admin-token", {
      tokenValue: "abc'def",
      deviceName: "Stu's iPhone",
    });
    await cleanupPushTestData(request, "admin-token", "abc'def");

    expect(posts[3].data).toMatchObject({
      token: "abc'def",
      device_name: "Stu's iPhone",
    });
  });

  it("seedPushDelivery inserts push delivery linked to seeded device token", async () => {
    const { request, queries } = buildPushRequestMock();

    await seedPushDelivery(request, "admin-token", {
      tokenValue: "delivery-fixture-token",
      title: "Push fixture lifecycle",
      body: "Fixture seeded delivery body",
      status: "sent",
    });

    expect(queries).toHaveLength(1);
    expect(queries[0]).toContain("INSERT INTO _ayb_push_deliveries");
    expect(queries[0]).toContain("device_token_id, app_id, user_id, provider");
    expect(queries[0]).toContain("'Push fixture lifecycle'");
    expect(queries[0]).toContain("'Fixture seeded delivery body'");
  });

  it("seedPushDelivery uses NOW() for sent status and NULL for non-sent", async () => {
    const { request: reqSent, queries: qSent } = buildPushRequestMock();
    await seedPushDelivery(reqSent, "admin-token", {
      tokenValue: "sent-token",
      status: "sent",
    });
    expect(qSent[0]).toContain("NOW()");
    expect(qSent[0]).not.toContain("NULL");

    const { request: reqFailed, queries: qFailed } = buildPushRequestMock();
    await seedPushDelivery(reqFailed, "admin-token", {
      tokenValue: "failed-token",
      status: "failed",
    });
    expect(qFailed[0]).toContain("NULL");
    expect(qFailed[0]).not.toContain("NOW()");
  });
});
