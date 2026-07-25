import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { AYBError } from "./errors";
import type { AYBClient } from "./client";
import {
  AUTH_TEST_PASSWORD,
  INTEGRATION_RUN_ID,
  adminSql,
  cleanupTrackedAuthUsers,
  createTestClient,
  getAdminToken,
  makeUniqueAuthEmail,
  primeIntegrationSuite,
  sqlStringLiteral,
  trackAuthUser,
} from "./integration-helpers";

describe("SDK integration: org-admin typed helpers", () => {
  const orgSlug = `sdk-org-${INTEGRATION_RUN_ID}`;
  const updatedOrgSlug = `sdk-org-renamed-${INTEGRATION_RUN_ID}`;
  const teamSlug = `sdk-team-${INTEGRATION_RUN_ID}`;
  const updatedTeamSlug = `sdk-team-renamed-${INTEGRATION_RUN_ID}`;
  const tenantSlug = `sdk-tenant-${INTEGRATION_RUN_ID}`;

  let client: AYBClient;
  let adminToken = "";
  let userId = "";
  let originalSessionToken = "";
  let originalRefreshToken = "";
  let orgId = "";
  let teamId = "";
  let tenantId = "";

  beforeAll(async () => {
    await primeIntegrationSuite();
    adminToken = await getAdminToken();
    client = createTestClient();

    const registered = await client.auth.register(
      makeUniqueAuthEmail("org-admin"),
      AUTH_TEST_PASSWORD,
    );
    userId = registered.user.id;
    trackAuthUser(userId);
    originalSessionToken = registered.token;
    originalRefreshToken = registered.refreshToken;

    const tenant = await adminSql(
      `INSERT INTO _ayb_tenants (name, slug, isolation_mode, plan_tier, region, org_metadata)
       VALUES ('SDK Org Admin Tenant', ${sqlStringLiteral(tenantSlug)}, 'schema', 'free', 'default', '{"stage":"sdk-org-admin"}'::jsonb)
       RETURNING id`,
    );
    tenantId = String(tenant.rows[0]?.[0] ?? "");
    expect(tenantId).not.toBe("");
  }, 60_000);

  afterAll(async () => {
    await cleanupOrgAdminResidue();
    await cleanupTrackedAuthUsers();
  }, 60_000);

  it("runs the dependency-ordered org-admin lifecycle without changing user session tokens", async () => {
    const orgs = client.admin(adminToken).orgs;

    const createdOrg = await orgs.create({
      name: "SDK Org Admin Org",
      slug: orgSlug,
      planTier: "pro",
    });
    orgId = createdOrg.id;
    expect(createdOrg).toMatchObject({
      id: orgId,
      name: "SDK Org Admin Org",
      slug: orgSlug,
      planTier: "pro",
    });

    const orgList = await orgs.list();
    expect(orgList.items.some((org) => org.id === orgId && org.slug === orgSlug)).toBe(true);

    const updatedOrg = await orgs.update(orgId, {
      name: "SDK Org Admin Org Renamed",
      slug: updatedOrgSlug,
    });
    expect(updatedOrg).toMatchObject({
      id: orgId,
      name: "SDK Org Admin Org Renamed",
      slug: updatedOrgSlug,
    });

    const createdTeam = await orgs.teams.create(orgId, {
      name: "SDK Org Admin Team",
      slug: teamSlug,
    });
    teamId = createdTeam.id;
    expect(createdTeam).toMatchObject({
      id: teamId,
      orgId,
      name: "SDK Org Admin Team",
      slug: teamSlug,
    });

    const teamList = await orgs.teams.list(orgId);
    expect(teamList.items).toContainEqual(createdTeam);

    const teamDetail = await orgs.teams.get(orgId, teamId);
    expect(teamDetail).toEqual(createdTeam);

    const updatedTeam = await orgs.teams.update(orgId, teamId, {
      name: "SDK Org Admin Team Renamed",
      slug: updatedTeamSlug,
    });
    expect(updatedTeam).toMatchObject({
      id: teamId,
      orgId,
      name: "SDK Org Admin Team Renamed",
      slug: updatedTeamSlug,
    });

    const orgMember = await orgs.members.add(orgId, { userId, role: "admin" });
    expect(orgMember).toMatchObject({ orgId, userId, role: "admin" });

    const orgMembers = await orgs.members.list(orgId);
    expect(orgMembers.items).toContainEqual(orgMember);

    const updatedOrgMember = await orgs.members.updateRole(orgId, userId, {
      role: "member",
    });
    expect(updatedOrgMember).toMatchObject({ orgId, userId, role: "member" });

    const teamMember = await orgs.teamMembers.add(orgId, teamId, {
      userId,
      role: "member",
    });
    expect(teamMember).toMatchObject({ teamId, userId, role: "member" });

    const teamMembers = await orgs.teamMembers.list(orgId, teamId);
    expect(teamMembers.items).toContainEqual(teamMember);

    const updatedTeamMember = await orgs.teamMembers.updateRole(
      orgId,
      teamId,
      userId,
      { role: "lead" },
    );
    expect(updatedTeamMember).toMatchObject({ teamId, userId, role: "lead" });

    await expect(orgs.tenants.assign(orgId, { tenantId })).resolves.toEqual({
      status: "assigned",
    });

    const tenants = await orgs.tenants.list(orgId);
    expect(tenants.items).toHaveLength(1);
    expect(tenants.items[0]).toMatchObject({
      id: tenantId,
      slug: tenantSlug,
      orgId,
      orgMetadata: { stage: "sdk-org-admin" },
      state: "provisioning",
    });

    const orgDetail = await orgs.get(orgId);
    expect(orgDetail).toMatchObject({
      id: orgId,
      slug: updatedOrgSlug,
      childOrgCount: 0,
      teamCount: 1,
      tenantCount: 1,
    });

    const usage = await orgs.usage(orgId, { period: "month" });
    expect(usage).toEqual({
      orgId,
      tenantCount: 1,
      period: "month",
      data: [],
      totals: {
        apiRequests: 0,
        storageBytesUsed: 0,
        bandwidthBytes: 0,
        functionInvocations: 0,
      },
    });

    const audit = await orgs.audit(orgId, { limit: 50, offset: 0 });
    expect(audit).toEqual({ items: [], count: 0, limit: 50, offset: 0 });

    await expect(orgs.tenants.unassign(orgId, tenantId)).resolves.toBeUndefined();
    await expect(orgs.teamMembers.remove(orgId, teamId, userId)).resolves.toBeUndefined();
    await expect(orgs.members.remove(orgId, userId)).resolves.toBeUndefined();
    await expect(orgs.teams.delete(orgId, teamId)).resolves.toBeUndefined();
    await expect(orgs.delete(orgId, { confirm: true })).resolves.toBeUndefined();

    expect(client.token).toBe(originalSessionToken);
    expect(client.refreshToken).toBe(originalRefreshToken);
  }, 60_000);

  async function cleanupOrgAdminResidue(): Promise<void> {
    if (client && adminToken) {
      if (orgId && tenantId) {
        await ignoreMissing(() =>
          client.admin(adminToken).orgs.tenants.unassign(orgId, tenantId),
        );
      }
      if (orgId && teamId && userId) {
        await ignoreMissing(() =>
          client.admin(adminToken).orgs.teamMembers.remove(orgId, teamId, userId),
        );
      }
      if (orgId && userId) {
        await ignoreMissing(() => client.admin(adminToken).orgs.members.remove(orgId, userId));
      }
      if (orgId && teamId) {
        await ignoreMissing(() => client.admin(adminToken).orgs.teams.delete(orgId, teamId));
      }
      if (orgId) {
        await ignoreMissing(() => client.admin(adminToken).orgs.delete(orgId, { confirm: true }));
      }
    }

    if (tenantId) {
      await adminSql(
        `DELETE FROM _ayb_tenants WHERE id = ${sqlStringLiteral(tenantId)}::uuid`,
      );
    }

    await assertNoCapturedResidue();
  }

  async function assertNoCapturedResidue(): Promise<void> {
    const checks = [
      ["_ayb_team_memberships", "team_id", teamId],
      ["_ayb_teams", "id", teamId],
      ["_ayb_org_memberships", "org_id", orgId],
      ["_ayb_organizations", "id", orgId],
      ["_ayb_tenants", "id", tenantId],
    ];

    for (const [tableName, columnName, id] of checks) {
      if (!id) continue;
      const result = await adminSql(
        `SELECT COUNT(*) FROM ${tableName} WHERE ${columnName} = ${sqlStringLiteral(id)}::uuid`,
      );
      expect(Number(result.rows[0]?.[0] ?? -1)).toBe(0);
    }
  }
});

async function ignoreMissing(operation: () => Promise<unknown>): Promise<void> {
  try {
    await operation();
  } catch (error) {
    if (error instanceof AYBError && (error.status === 404 || error.status === 409)) {
      return;
    }
    throw error;
  }
}
