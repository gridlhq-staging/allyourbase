/**
 * @module Organization admin SDK subclient.
 */
import { encodePathSegment } from "./helpers";
import type {
  AddOrgMemberRequest,
  AddTeamMemberRequest,
  AssignOrgTenantRequest,
  AssignOrgTenantResponse,
  CreateOrganizationRequest,
  CreateTeamRequest,
  DeleteOrganizationOptions,
  Organization,
  OrganizationDetail,
  OrganizationListResponse,
  OrgAuditListResponse,
  OrgAuditOptions,
  OrgMembership,
  OrgMembershipListResponse,
  OrgTenantListResponse,
  OrgUsageOptions,
  OrgUsageSummary,
  Team,
  TeamListResponse,
  TeamMembership,
  TeamMembershipListResponse,
  UpdateOrganizationRequest,
  UpdateOrgMemberRoleRequest,
  UpdateTeamMemberRoleRequest,
  UpdateTeamRequest,
} from "./types";

interface AdminRequestRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
}

type QueryValue = string | number | undefined;

/** Explicit-token admin surface. It does not read or mutate user auth state. */
export class AdminClient {
  readonly orgs: AdminOrgsClient;

  constructor(client: AdminRequestRuntime, adminToken: string) {
    const runtime: AdminRequestRuntime = {
      request: <T>(path: string, init?: RequestInit & { skipAuth?: boolean }) =>
        client.request<T>(path, withAdminBearer(adminToken, init)),
    };
    this.orgs = new AdminOrgsClient(runtime);
  }
}

/** Organization, team, membership, usage, audit, and tenant assignment helpers. */
export class AdminOrgsClient {
  readonly teams: AdminOrgTeamsClient;
  readonly members: AdminOrgMembersClient;
  readonly teamMembers: AdminOrgTeamMembersClient;
  readonly tenants: AdminOrgTenantsClient;

  constructor(private client: AdminRequestRuntime) {
    this.teams = new AdminOrgTeamsClient(client);
    this.members = new AdminOrgMembersClient(client);
    this.teamMembers = new AdminOrgTeamMembersClient(client);
    this.tenants = new AdminOrgTenantsClient(client);
  }

  async create(request: CreateOrganizationRequest): Promise<Organization> {
    return this.client.request<Organization>(orgsPath(), jsonRequest("POST", request));
  }

  async list(): Promise<OrganizationListResponse> {
    return this.client.request<OrganizationListResponse>(orgsPath());
  }

  async get(orgId: string): Promise<OrganizationDetail> {
    return this.client.request<OrganizationDetail>(orgPath(orgId));
  }

  async update(
    orgId: string,
    request: UpdateOrganizationRequest,
  ): Promise<Organization> {
    return this.client.request<Organization>(orgPath(orgId), jsonRequest("PUT", request));
  }

  async delete(
    orgId: string,
    options: DeleteOrganizationOptions,
  ): Promise<void> {
    const query = queryString([["confirm", options.confirm ? "true" : undefined]]);
    return this.client.request<void>(`${orgPath(orgId)}${query}`, {
      method: "DELETE",
    });
  }

  async usage(orgId: string, options?: OrgUsageOptions): Promise<OrgUsageSummary> {
    const query = queryString([
      ["period", options?.period],
      ["from", options?.from],
      ["to", options?.to],
    ]);
    return this.client.request<OrgUsageSummary>(`${orgPath(orgId)}/usage${query}`);
  }

  async audit(
    orgId: string,
    options?: OrgAuditOptions,
  ): Promise<OrgAuditListResponse> {
    const query = queryString([
      ["from", options?.from],
      ["to", options?.to],
      ["action", options?.action],
      ["result", options?.result],
      ["actor_id", options?.actorId],
      ["limit", options?.limit],
      ["offset", options?.offset],
    ]);
    return this.client.request<OrgAuditListResponse>(`${orgPath(orgId)}/audit${query}`);
  }
}

/** Team helpers scoped to an organization. */
export class AdminOrgTeamsClient {
  constructor(private client: AdminRequestRuntime) {}

  async create(orgId: string, request: CreateTeamRequest): Promise<Team> {
    return this.client.request<Team>(teamsPath(orgId), jsonRequest("POST", request));
  }

  async list(orgId: string): Promise<TeamListResponse> {
    return this.client.request<TeamListResponse>(teamsPath(orgId));
  }

  async get(orgId: string, teamId: string): Promise<Team> {
    return this.client.request<Team>(teamPath(orgId, teamId));
  }

  async update(
    orgId: string,
    teamId: string,
    request: UpdateTeamRequest,
  ): Promise<Team> {
    return this.client.request<Team>(
      teamPath(orgId, teamId),
      jsonRequest("PUT", request),
    );
  }

  async delete(orgId: string, teamId: string): Promise<void> {
    return this.client.request<void>(teamPath(orgId, teamId), { method: "DELETE" });
  }
}

/** Organization membership helpers. */
export class AdminOrgMembersClient {
  constructor(private client: AdminRequestRuntime) {}

  async add(orgId: string, request: AddOrgMemberRequest): Promise<OrgMembership> {
    return this.client.request<OrgMembership>(
      orgMembersPath(orgId),
      jsonRequest("POST", request),
    );
  }

  async list(orgId: string): Promise<OrgMembershipListResponse> {
    return this.client.request<OrgMembershipListResponse>(orgMembersPath(orgId));
  }

  async remove(orgId: string, userId: string): Promise<void> {
    return this.client.request<void>(orgMemberPath(orgId, userId), {
      method: "DELETE",
    });
  }

  async updateRole(
    orgId: string,
    userId: string,
    request: UpdateOrgMemberRoleRequest,
  ): Promise<OrgMembership> {
    return this.client.request<OrgMembership>(
      `${orgMemberPath(orgId, userId)}/role`,
      jsonRequest("PUT", request),
    );
  }
}

/** Team membership helpers scoped to an organization and team. */
export class AdminOrgTeamMembersClient {
  constructor(private client: AdminRequestRuntime) {}

  async add(
    orgId: string,
    teamId: string,
    request: AddTeamMemberRequest,
  ): Promise<TeamMembership> {
    return this.client.request<TeamMembership>(
      teamMembersPath(orgId, teamId),
      jsonRequest("POST", request),
    );
  }

  async list(
    orgId: string,
    teamId: string,
  ): Promise<TeamMembershipListResponse> {
    return this.client.request<TeamMembershipListResponse>(
      teamMembersPath(orgId, teamId),
    );
  }

  async remove(orgId: string, teamId: string, userId: string): Promise<void> {
    return this.client.request<void>(teamMemberPath(orgId, teamId, userId), {
      method: "DELETE",
    });
  }

  async updateRole(
    orgId: string,
    teamId: string,
    userId: string,
    request: UpdateTeamMemberRoleRequest,
  ): Promise<TeamMembership> {
    return this.client.request<TeamMembership>(
      `${teamMemberPath(orgId, teamId, userId)}/role`,
      jsonRequest("PUT", request),
    );
  }
}

/** Organization tenant assignment helpers. */
export class AdminOrgTenantsClient {
  constructor(private client: AdminRequestRuntime) {}

  async assign(
    orgId: string,
    request: AssignOrgTenantRequest,
  ): Promise<AssignOrgTenantResponse> {
    return this.client.request<AssignOrgTenantResponse>(
      orgTenantsPath(orgId),
      jsonRequest("POST", request),
    );
  }

  async list(orgId: string): Promise<OrgTenantListResponse> {
    return this.client.request<OrgTenantListResponse>(orgTenantsPath(orgId));
  }

  async unassign(orgId: string, tenantId: string): Promise<void> {
    return this.client.request<void>(
      `${orgTenantsPath(orgId)}/${encodePathSegment(tenantId)}`,
      { method: "DELETE" },
    );
  }
}

function withAdminBearer(
  adminToken: string,
  init?: RequestInit & { skipAuth?: boolean },
): RequestInit & { skipAuth: true } {
  return {
    ...init,
    skipAuth: true,
    headers: {
      Authorization: `Bearer ${adminToken}`,
      ...(init?.headers as Record<string, string> | undefined),
    },
  };
}

function jsonRequest(method: "POST" | "PUT", body: unknown): RequestInit {
  return {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
}

function queryString(entries: [string, QueryValue][]): string {
  const params = new URLSearchParams();
  for (const [key, value] of entries) {
    if (value != null) {
      params.set(key, String(value));
    }
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
}

function orgsPath(): string {
  return "/api/admin/orgs";
}

function orgPath(orgId: string): string {
  return `${orgsPath()}/${encodePathSegment(orgId)}`;
}

function teamsPath(orgId: string): string {
  return `${orgPath(orgId)}/teams`;
}

function teamPath(orgId: string, teamId: string): string {
  return `${teamsPath(orgId)}/${encodePathSegment(teamId)}`;
}

function orgMembersPath(orgId: string): string {
  return `${orgPath(orgId)}/members`;
}

function orgMemberPath(orgId: string, userId: string): string {
  return `${orgMembersPath(orgId)}/${encodePathSegment(userId)}`;
}

function teamMembersPath(orgId: string, teamId: string): string {
  return `${teamPath(orgId, teamId)}/members`;
}

function teamMemberPath(orgId: string, teamId: string, userId: string): string {
  return `${teamMembersPath(orgId, teamId)}/${encodePathSegment(userId)}`;
}

function orgTenantsPath(orgId: string): string {
  return `${orgPath(orgId)}/tenants`;
}
