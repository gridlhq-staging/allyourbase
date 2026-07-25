import { describe, expect, it, vi } from "vitest";
import { AYBClient } from "./client";
import type { AddOrgMemberRequest, CreateOrganizationRequest } from "./index";

function mockFetch(status: number, body: unknown): typeof globalThis.fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 204 ? "No Content" : "OK",
    json: () => Promise.resolve(body),
  }) as unknown as typeof globalThis.fetch;
}

function requestCall(fetchFn: typeof globalThis.fetch) {
  return (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0] as [
    string,
    RequestInit,
  ];
}

describe("admin orgs", () => {
  it("uses an explicit admin bearer token without mutating client auth state", async () => {
    const fetchFn = mockFetch(201, {
      id: "org-1",
      name: "Fixture Org",
      slug: "fixture-org",
      planTier: "pro",
      createdAt: "2026-07-25T00:00:00Z",
      updatedAt: "2026-07-25T00:00:00Z",
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("user-token", "refresh-token");

    const request: CreateOrganizationRequest = {
      name: "Fixture Org",
      slug: "fixture-org",
      planTier: "pro",
    };
    await client.admin("admin-token").orgs.create(request);

    const [url, init] = requestCall(fetchFn);
    expect(url).toBe("http://localhost:8090/api/admin/orgs");
    expect(init.method).toBe("POST");
    expect(init.headers).toMatchObject({
      Authorization: "Bearer admin-token",
      "Content-Type": "application/json",
    });
    expect(JSON.parse(init.body as string)).toEqual(request);
    expect(client.token).toBe("user-token");
  });

  it("builds org helper paths and queries with encoded route segments", async () => {
    const fetchFn = mockFetch(200, {
      orgId: "org/one",
      tenantCount: 0,
      period: "month",
      data: [],
      totals: {
        apiRequests: 0,
        storageBytesUsed: 0,
        bandwidthBytes: 0,
        functionInvocations: 0,
      },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.admin("admin-token").orgs.usage("org/one", {
      period: "month",
      from: "2026-07-01",
      to: "2026-07-25",
    });

    const [url, init] = requestCall(fetchFn);
    expect(url).toBe(
      "http://localhost:8090/api/admin/orgs/org%2Fone/usage?period=month&from=2026-07-01&to=2026-07-25",
    );
    expect(init.headers).toMatchObject({ Authorization: "Bearer admin-token" });
  });

  it("maps org audit actorId to the server actor_id query key", async () => {
    const fetchFn = mockFetch(200, {
      items: [],
      count: 0,
      limit: 10,
      offset: 20,
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.admin("admin-token").orgs.audit("org-1", {
      from: "2026-07-01T00:00:00Z",
      to: "2026-07-25T00:00:00Z",
      action: "tenant.created",
      result: "success",
      actorId: "actor-1",
      limit: 10,
      offset: 20,
    });

    const [url] = requestCall(fetchFn);
    expect(url).toBe(
      "http://localhost:8090/api/admin/orgs/org-1/audit?from=2026-07-01T00%3A00%3A00Z&to=2026-07-25T00%3A00%3A00Z&action=tenant.created&result=success&actor_id=actor-1&limit=10&offset=20",
    );
  });

  it("implements teams, org members, team members, and tenant assignment routes", async () => {
    const fetchFn = mockFetch(200, { items: [] });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const orgMemberRequest: AddOrgMemberRequest = {
      userId: "user-1",
      role: "admin",
    };

    await client.admin("admin-token").orgs.teams.update("org-1", "team/1", {
      name: "Core",
      slug: "core",
    });
    await client.admin("admin-token").orgs.members.add("org-1", orgMemberRequest);
    await client
      .admin("admin-token")
      .orgs.teamMembers.updateRole("org-1", "team-1", "user/1", {
        role: "lead",
      });
    await client.admin("admin-token").orgs.tenants.assign("org-1", {
      tenantId: "tenant-1",
    });

    const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls as [
      string,
      RequestInit,
    ][];
    expect(calls.map(([url]) => url)).toEqual([
      "http://localhost:8090/api/admin/orgs/org-1/teams/team%2F1",
      "http://localhost:8090/api/admin/orgs/org-1/members",
      "http://localhost:8090/api/admin/orgs/org-1/teams/team-1/members/user%2F1/role",
      "http://localhost:8090/api/admin/orgs/org-1/tenants",
    ]);
    expect(calls[0][1]).toMatchObject({
      method: "PUT",
      headers: {
        Authorization: "Bearer admin-token",
        "Content-Type": "application/json",
      },
    });
    expect(JSON.parse(calls[1][1].body as string)).toEqual(orgMemberRequest);
    expect(JSON.parse(calls[2][1].body as string)).toEqual({ role: "lead" });
    expect(JSON.parse(calls[3][1].body as string)).toEqual({
      tenantId: "tenant-1",
    });
  });

  it("serializes the empty-string parent removal sentinel verbatim", async () => {
    const fetchFn = mockFetch(200, {
      id: "org-1",
      name: "Fixture Org",
      slug: "fixture-org",
      planTier: "pro",
      createdAt: "2026-07-25T00:00:00Z",
      updatedAt: "2026-07-25T00:00:00Z",
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.admin("admin-token").orgs.update("org-1", { parentOrgId: "" });

    const [url, init] = requestCall(fetchFn);
    expect(url).toBe("http://localhost:8090/api/admin/orgs/org-1");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe('{"parentOrgId":""}');
  });

  it("returns void for 204 delete and unassign routes", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await expect(
      client.admin("admin-token").orgs.delete("org-1", { confirm: true }),
    ).resolves.toBeUndefined();
    await expect(
      client.admin("admin-token").orgs.teams.delete("org-1", "team-1"),
    ).resolves.toBeUndefined();
    await expect(
      client.admin("admin-token").orgs.members.remove("org-1", "user-1"),
    ).resolves.toBeUndefined();
    await expect(
      client
        .admin("admin-token")
        .orgs.teamMembers.remove("org-1", "team-1", "user-1"),
    ).resolves.toBeUndefined();
    await expect(
      client.admin("admin-token").orgs.tenants.unassign("org-1", "tenant-1"),
    ).resolves.toBeUndefined();

    const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls as [
      string,
      RequestInit,
    ][];
    expect(calls.map(([url]) => url)).toEqual([
      "http://localhost:8090/api/admin/orgs/org-1?confirm=true",
      "http://localhost:8090/api/admin/orgs/org-1/teams/team-1",
      "http://localhost:8090/api/admin/orgs/org-1/members/user-1",
      "http://localhost:8090/api/admin/orgs/org-1/teams/team-1/members/user-1",
      "http://localhost:8090/api/admin/orgs/org-1/tenants/tenant-1",
    ]);
    expect(calls.every(([, init]) => init.method === "DELETE")).toBe(true);
  });
});
