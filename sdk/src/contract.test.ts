import { describe, it, expect, vi } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { basename, resolve } from "node:path";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import {
  buildOAuthStartURL,
  serializeWebAuthnLoginFinishRequest,
  serializeWebAuthnMFAVerifyRequest,
} from "./auth";
import {
  normalizeAuthResponse,
  normalizeWebAuthnLoginBeginResponse,
  normalizeWebAuthnMFAChallengeResponse,
} from "./helpers";
import { AYBClient as PublicAYBClient } from "./index";
import { createInstantSearchClient } from "./instantsearch";
import type {
  AuthResponse as PublicAuthResponse,
  FacetValueSearchHit as PublicFacetValueSearchHit,
  FacetValueSearchParams as PublicFacetValueSearchParams,
  FacetValueSearchResponse as PublicFacetValueSearchResponse,
  ListResponse as PublicListResponse,
  SearchSynonymsRequest as PublicSearchSynonymsRequest,
  SearchSynonymsResponse as PublicSearchSynonymsResponse,
  SearchHit as PublicSearchHit,
  StorageObject as PublicStorageObject,
  User as PublicUser,
  WebAuthnEnrollBeginResponse as PublicWebAuthnEnrollBeginResponse,
  WebAuthnEnrollConfirmRequest as PublicWebAuthnEnrollConfirmRequest,
  WebAuthnLoginBeginResponse as PublicWebAuthnLoginBeginResponse,
  WebAuthnMFAChallengeResponse as PublicWebAuthnMFAChallengeResponse,
  WebAuthnMFAVerifyRequest as PublicWebAuthnMFAVerifyRequest,
} from "./index";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";
import type {
  AddOrgMemberRequest,
  AddTeamMemberRequest,
  AssignOrgTenantRequest,
  CreateOrganizationRequest,
  CreateTeamRequest,
  AuthResponse,
  FacetValueSearchHit,
  FacetValueSearchParams,
  FacetValueSearchResponse,
  ListResponse,
  UpdateOrganizationRequest,
  UpdateOrgMemberRoleRequest,
  UpdateTeamMemberRoleRequest,
  UpdateTeamRequest,
  SearchSynonymsRequest,
  SearchSynonymsResponse,
  SearchHit,
  StorageObject,
  User,
  WebAuthnEnrollBeginResponse,
  WebAuthnEnrollConfirmRequest,
  WebAuthnLoginBeginResponse,
  WebAuthnLoginFinishRequest,
  WebAuthnMFAChallengeResponse,
  WebAuthnMFAVerifyRequest,
} from "./types";
import type {
  InstantSearchClient,
  InstantSearchFacetValueRequest,
  InstantSearchFacetValueResponse,
  InstantSearchFacetValueResult,
  InstantSearchResponse,
  InstantSearchSearchRequest,
} from "./instantsearch";

function loadContractFixture(name: string): unknown {
  if (name === "" || name === "." || name === ".." || basename(name) !== name || /[\\/]/.test(name)) {
    throw new Error(`sdk contract fixture name must be a single filename: ${name}`);
  }
  const fixturePath = resolve(__dirname, "../../tests/contract/fixtures/sdk_contract", name);
  return JSON.parse(readFileSync(fixturePath, "utf8")) as unknown;
}

interface OAuthStartURLFixtureCase {
  base_url: string;
  provider: "google" | "github";
  state: string;
  scopes?: string[];
  redirect_to?: string;
  expected_path_query: string;
}

type OrgAdminFixtureCase = {
  name: string;
  requestFixture?: string;
  responseFixture: string;
  expectedPath: string;
  expectedMethod: string;
  exercise: (client: AYBClient, requestFixture: unknown) => Promise<unknown>;
};

const orgAdminFixtureCases: OrgAdminFixtureCase[] = [
  {
    name: "create organization",
    requestFixture: "org_admin_org_create_request.json",
    responseFixture: "org_admin_org_create_response.json",
    expectedPath: "/api/admin/orgs",
    expectedMethod: "POST",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.create(requestFixture as CreateOrganizationRequest),
  },
  {
    name: "list organizations",
    responseFixture: "org_admin_org_list_response.json",
    expectedPath: "/api/admin/orgs",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.list(),
  },
  {
    name: "get organization",
    responseFixture: "org_admin_org_get_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.get("org-fixture-id"),
  },
  {
    name: "update organization",
    requestFixture: "org_admin_org_update_request.json",
    responseFixture: "org_admin_org_update_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id",
    expectedMethod: "PUT",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.update("org-fixture-id", requestFixture as UpdateOrganizationRequest),
  },
  {
    name: "organization usage",
    responseFixture: "org_admin_org_usage_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/usage",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.usage("org-fixture-id"),
  },
  {
    name: "organization audit",
    responseFixture: "org_admin_org_audit_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/audit",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.audit("org-fixture-id"),
  },
  {
    name: "create team",
    requestFixture: "org_admin_team_create_request.json",
    responseFixture: "org_admin_team_create_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams",
    expectedMethod: "POST",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.teams.create("org-fixture-id", requestFixture as CreateTeamRequest),
  },
  {
    name: "list teams",
    responseFixture: "org_admin_team_list_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.teams.list("org-fixture-id"),
  },
  {
    name: "get team",
    responseFixture: "org_admin_team_get_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams/team-fixture-id",
    expectedMethod: "GET",
    exercise: (client) =>
      client.admin("admin-token").orgs.teams.get("org-fixture-id", "team-fixture-id"),
  },
  {
    name: "update team",
    requestFixture: "org_admin_team_update_request.json",
    responseFixture: "org_admin_team_update_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams/team-fixture-id",
    expectedMethod: "PUT",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.teams.update(
          "org-fixture-id",
          "team-fixture-id",
          requestFixture as UpdateTeamRequest,
        ),
  },
  {
    name: "add organization member",
    requestFixture: "org_admin_org_member_add_request.json",
    responseFixture: "org_admin_org_member_add_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/members",
    expectedMethod: "POST",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.members.add("org-fixture-id", requestFixture as AddOrgMemberRequest),
  },
  {
    name: "list organization members",
    responseFixture: "org_admin_org_member_list_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/members",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.members.list("org-fixture-id"),
  },
  {
    name: "update organization member role",
    requestFixture: "org_admin_org_member_role_update_request.json",
    responseFixture: "org_admin_org_member_role_update_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/members/user-fixture-id/role",
    expectedMethod: "PUT",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.members.updateRole(
          "org-fixture-id",
          "user-fixture-id",
          requestFixture as UpdateOrgMemberRoleRequest,
        ),
  },
  {
    name: "add team member",
    requestFixture: "org_admin_team_member_add_request.json",
    responseFixture: "org_admin_team_member_add_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams/team-fixture-id/members",
    expectedMethod: "POST",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.teamMembers.add(
          "org-fixture-id",
          "team-fixture-id",
          requestFixture as AddTeamMemberRequest,
        ),
  },
  {
    name: "list team members",
    responseFixture: "org_admin_team_member_list_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams/team-fixture-id/members",
    expectedMethod: "GET",
    exercise: (client) =>
      client.admin("admin-token").orgs.teamMembers.list("org-fixture-id", "team-fixture-id"),
  },
  {
    name: "update team member role",
    requestFixture: "org_admin_team_member_role_update_request.json",
    responseFixture: "org_admin_team_member_role_update_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/teams/team-fixture-id/members/user-fixture-id/role",
    expectedMethod: "PUT",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.teamMembers.updateRole(
          "org-fixture-id",
          "team-fixture-id",
          "user-fixture-id",
          requestFixture as UpdateTeamMemberRoleRequest,
        ),
  },
  {
    name: "assign tenant",
    requestFixture: "org_admin_tenant_assign_request.json",
    responseFixture: "org_admin_tenant_assign_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/tenants",
    expectedMethod: "POST",
    exercise: (client, requestFixture) =>
      client
        .admin("admin-token")
        .orgs.tenants.assign("org-fixture-id", requestFixture as AssignOrgTenantRequest),
  },
  {
    name: "list tenants",
    responseFixture: "org_admin_tenant_list_response.json",
    expectedPath: "/api/admin/orgs/org-fixture-id/tenants",
    expectedMethod: "GET",
    exercise: (client) => client.admin("admin-token").orgs.tenants.list("org-fixture-id"),
  },
];

function orgAdminFixtureInventory(): string[] {
  const fixtureDir = resolve(__dirname, "../../tests/contract/fixtures/sdk_contract");
  return readdirSync(fixtureDir)
    .filter((fileName) => /^org_admin_.*\.json$/.test(fileName))
    .sort();
}

function orgAdminFixtureTableNames(): string[] {
  return orgAdminFixtureCases
    .flatMap((fixtureCase) =>
      fixtureCase.requestFixture
        ? [fixtureCase.requestFixture, fixtureCase.responseFixture]
        : [fixtureCase.responseFixture],
    )
    .sort();
}

describe("SDK contract fixtures", () => {
  it("rejects fixture paths outside the canonical sdk_contract directory", () => {
    expect(() => loadContractFixture("../auth_response.json")).toThrow(
      "sdk contract fixture name must be a single filename",
    );
    expect(() => loadContractFixture("..\\auth_response.json")).toThrow(
      "sdk contract fixture name must be a single filename",
    );
  });

  it("keeps magic-link fixtures canonical to sdk_contract tree", () => {
    const sdkParityFixtureDir = resolve(__dirname, "../../tests/contract/fixtures/sdk_parity");
    const duplicateMagicLinkFixtures = readdirSync(sdkParityFixtureDir).filter((fileName) =>
      fileName.startsWith("magic_link_"),
    );

    expect(duplicateMagicLinkFixtures).toEqual([]);
  });

  it("org-admin fixture table covers every committed org-admin fixture", () => {
    expect(orgAdminFixtureTableNames()).toEqual(orgAdminFixtureInventory());
  });

  it.each(orgAdminFixtureCases)(
    "org-admin fixture $name round trips through public typed helper",
    async (fixtureCase) => {
      const requestFixture = fixtureCase.requestFixture
        ? loadContractFixture(fixtureCase.requestFixture)
        : undefined;
      const responseFixture = loadContractFixture(fixtureCase.responseFixture);
      const fetchFn = mockFetchSequence([{ status: 200, body: responseFixture }]);
      const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
      client.setTokens("user-session-token", "user-refresh-token");

      const result = await fixtureCase.exercise(client, requestFixture);

      expect(result).toEqual(responseFixture);
      expect(client.token).toBe("user-session-token");
      expect(client.refreshToken).toBe("user-refresh-token");

      const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls as [
        string,
        RequestInit,
      ][];
      expect(calls).toHaveLength(1);
      const [url, init] = calls[0];
      expect(url).toBe(`https://api.example.com${fixtureCase.expectedPath}`);
      expect(init.method ?? "GET").toBe(fixtureCase.expectedMethod);
      expect(init.headers).toMatchObject({
        Authorization: "Bearer admin-token",
      });

      if (requestFixture !== undefined) {
        expect(init.headers).toMatchObject({
          "Content-Type": "application/json",
        });
        expect(JSON.parse(init.body as string)).toEqual(requestFixture);
      } else {
        expect(init.body).toBeUndefined();
      }
    },
  );

  it("public barrel re-exports core client and canonical types", () => {
    const publicClient = new PublicAYBClient("https://api.example.com");
    expect(publicClient).toBeInstanceOf(AYBClient);

    const assertAuthType = (_value: AuthResponse): void => {};
    const assertListType = (_value: ListResponse<Record<string, unknown>>): void => {};
    const assertSearchHitType = (_value: ListResponse<SearchHit<{ id: string }>>): void => {};
    const assertStorageType = (_value: StorageObject): void => {};
    const assertUserType = (_value: User): void => {};
    const assertWebAuthnBeginType = (_value: WebAuthnLoginBeginResponse): void => {};
    const assertWebAuthnEnrollBeginType = (_value: WebAuthnEnrollBeginResponse): void => {};
    const assertWebAuthnEnrollConfirmType = (_value: WebAuthnEnrollConfirmRequest): void => {};
    const assertWebAuthnMFAChallengeType = (_value: WebAuthnMFAChallengeResponse): void => {};
    const assertWebAuthnMFAVerifyType = (_value: WebAuthnMFAVerifyRequest): void => {};
    const assertSearchSynonymsRequestType = (_value: SearchSynonymsRequest): void => {};
    const assertSearchSynonymsResponseType = (_value: SearchSynonymsResponse): void => {};
    const assertFacetHitType = (_value: FacetValueSearchHit): void => {};
    const assertFacetParamsType = (_value: FacetValueSearchParams): void => {};
    const assertFacetResponseType = (_value: FacetValueSearchResponse): void => {};

    assertAuthType({} as PublicAuthResponse);
    assertListType({} as PublicListResponse<Record<string, unknown>>);
    assertSearchHitType({} as PublicListResponse<PublicSearchHit<{ id: string }>>);
    assertStorageType({} as PublicStorageObject);
    assertUserType({} as PublicUser);
    assertWebAuthnBeginType({} as PublicWebAuthnLoginBeginResponse);
    assertWebAuthnEnrollBeginType({} as PublicWebAuthnEnrollBeginResponse);
    assertWebAuthnEnrollConfirmType({} as PublicWebAuthnEnrollConfirmRequest);
    assertWebAuthnMFAChallengeType({} as PublicWebAuthnMFAChallengeResponse);
    assertWebAuthnMFAVerifyType({} as PublicWebAuthnMFAVerifyRequest);
    assertSearchSynonymsRequestType({} as PublicSearchSynonymsRequest);
    assertSearchSynonymsResponseType({} as PublicSearchSynonymsResponse);
    assertFacetHitType({} as PublicFacetValueSearchHit);
    assertFacetParamsType({} as PublicFacetValueSearchParams);
    assertFacetResponseType({} as PublicFacetValueSearchResponse);
  });

  it("OAuth start URL fixture cases preserve exact path and query bytes", () => {
    const cases = loadContractFixture(
      "oauth_start_url_cases.json",
    ) as OAuthStartURLFixtureCase[];

    expect(cases).toHaveLength(6);

    for (const fixtureCase of cases) {
      const actualURL = new URL(
        buildOAuthStartURL(
          fixtureCase.base_url,
          fixtureCase.provider,
          fixtureCase.state,
          {
            scopes: fixtureCase.scopes,
            redirectTo: fixtureCase.redirect_to,
          },
        ),
      );
      const expectedURL = new URL(
        `${fixtureCase.base_url}${fixtureCase.expected_path_query}`,
      );

      expect(`${actualURL.pathname}${actualURL.search}`).toBe(
        fixtureCase.expected_path_query,
      );
      expect(actualURL.pathname).toBe(expectedURL.pathname);
      expect([...actualURL.searchParams.keys()]).toEqual([
        ...expectedURL.searchParams.keys(),
      ]);
      expect(actualURL.searchParams.get("state")).toBe(fixtureCase.state);

      if (fixtureCase.scopes) {
        expect(actualURL.search).toContain("scopes=");
        expect(actualURL.searchParams.get("scopes")).toBe(
          fixtureCase.scopes.join(","),
        );
      } else {
        expect(actualURL.searchParams.has("scopes")).toBe(false);
      }

      if (fixtureCase.redirect_to) {
        expect(actualURL.search).toContain("redirect_to=");
        expect(actualURL.searchParams.get("redirect_to")).toBe(
          fixtureCase.redirect_to,
        );
      } else {
        expect(actualURL.searchParams.has("redirect_to")).toBe(false);
      }
    }
  });

  it("InstantSearch subpath owner exposes the adapter factory and local types", () => {
    const client = {
      records: {
        list: async () => ({
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        }),
      },
    };

    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "posts",
    });

    const assertSearchClient = (_value: InstantSearchClient): void => {};
    const assertRequest = (_value: InstantSearchSearchRequest): void => {};
    const assertResponse = (_value: InstantSearchResponse): void => {};
    const assertFacetRequest = (_value: InstantSearchFacetValueRequest): void => {};
    const assertFacetResponse = (_value: InstantSearchFacetValueResponse): void => {};
    const assertFacetResult = (_value: InstantSearchFacetValueResult): void => {};

    assertSearchClient(searchClient);
    assertRequest({ indexName: "posts", params: { query: "postgres" } });
    assertResponse({ results: [] });
    assertFacetRequest({
      indexName: "products",
      params: { facetName: "brand", facetQuery: "ac" },
    });
    assertFacetResponse([]);
    assertFacetResult({ facetHits: [], exhaustiveFacetsCount: true, processingTimeMS: 0 });
    expect(typeof searchClient.search).toBe("function");
    expect(typeof searchClient.searchForFacetValues).toBe("function");
  });

  it("InstantSearch adapter accepts a records owner that ALSO exposes searchFacetValues", () => {
    const client = {
      records: {
        list: async () => ({
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        }),
        searchFacetValues: async () => ({
          facetHits: [],
          exhaustiveFacetsCount: true,
        }),
      },
    };

    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    expect(typeof searchClient.searchForFacetValues).toBe("function");
  });

  it("auth response fixture normalizes user aliases", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          token: "jwt_stage3",
          refreshToken: "refresh_stage3",
          user: {
            id: "usr_1",
            email: "dev@allyourbase.io",
            email_verified: true,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: null,
          },
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const auth = await client.auth.login("dev@allyourbase.io", "secret");

    expect(auth.token).toBe("jwt_stage3");
    expect(auth.refreshToken).toBe("refresh_stage3");
    expect(auth.user.emailVerified).toBe(true);
    expect(auth.user.createdAt).toBe("2026-01-01T00:00:00Z");
    expect(auth.user.updatedAt).toBeUndefined();
  });

  it("list response fixture preserves metadata and order", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [
            { id: "rec_1", title: "First" },
            { id: "rec_2", title: "Second" },
          ],
          page: 1,
          perPage: 2,
          totalItems: 2,
          totalPages: 1,
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const list = await client.records.list("posts");

    expect(list.totalItems).toBe(2);
    expect(list.items[0].title).toBe("First");
    expect(list.items[1].title).toBe("Second");
  });

  it("error fixtures normalize numeric and string code variants", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 403,
        body: {
          code: 403,
          message: "forbidden",
          data: { resource: "posts" },
          doc_url: "https://allyourbase.io/docs/errors#forbidden",
        },
      },
      {
        status: 400,
        body: {
          code: "auth/missing-refresh-token",
          message: "Missing refresh token",
          data: { detail: "refresh token not available" },
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });

    await expect(client.records.list("posts")).rejects.toMatchObject<Partial<AYBError>>({
      status: 403,
      message: "forbidden",
      code: "403",
      data: { resource: "posts" },
      docUrl: "https://allyourbase.io/docs/errors#forbidden",
    });

    await expect(client.auth.refresh()).rejects.toMatchObject<Partial<AYBError>>({
      status: 400,
      message: "Missing refresh token",
      code: "auth/missing-refresh-token",
      data: { detail: "refresh token not available" },
    });
  });

  it("storage object fixture and list fixture decode nullable fields", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          id: "file_abc123",
          bucket: "uploads",
          name: "document.pdf",
          size: 1024,
          contentType: "application/pdf",
          userId: "usr_1",
          createdAt: "2026-01-01T00:00:00Z",
          updatedAt: "2026-01-02T12:30:00Z",
        },
      },
      {
        status: 200,
        body: {
          items: [
            {
              id: "file_1",
              bucket: "uploads",
              name: "doc1.pdf",
              size: 1024,
              contentType: "application/pdf",
              userId: "usr_1",
              createdAt: "2026-01-01T00:00:00Z",
              updatedAt: null,
            },
            {
              id: "file_2",
              bucket: "uploads",
              name: "image.png",
              size: 2048,
              contentType: "image/png",
              userId: null,
              createdAt: "2026-01-02T00:00:00Z",
              updatedAt: null,
            },
          ],
          totalItems: 2,
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });

    const uploaded = await client.storage.upload("uploads", new Blob(["hello"]), "document.pdf");
    expect(uploaded.id).toBe("file_abc123");
    expect(uploaded.userId).toBe("usr_1");

    const listed = await client.storage.list("uploads");
    expect(listed.totalItems).toBe(2);
    expect(listed.items[0].userId).toBe("usr_1");
    expect(listed.items[0].updatedAt).toBeUndefined();
    expect(listed.items[1].userId).toBeUndefined();
    expect(listed.items[1].updatedAt).toBeUndefined();
  });

  it("magic-link request fixture matches canonical response wire shape", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: loadContractFixture("magic_link_request_response.json"),
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const response = await client.auth.requestMagicLink("dev@allyourbase.io");

    expect(response).toEqual({ message: "If an account exists, a magic link has been sent." });
  });

  it("magic-link confirm success fixture normalizes auth response aliases", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: loadContractFixture("magic_link_confirm_success_response.json"),
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const response = await client.auth.confirmMagicLink("magic-link-token");

    expect("token" in response).toBe(true);
    if ("token" in response) {
      expect(response.token).toBe("jwt_magic_link");
      expect(response.refreshToken).toBe("refresh_magic_link");
      expect(response.user.emailVerified).toBe(true);
      expect(response.user.createdAt).toBe("2026-05-01T12:00:00Z");
      expect(response.user.updatedAt).toBeUndefined();
    }
  });

  it("magic-link confirm pending-mfa fixture normalizes MFA challenge shape", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: loadContractFixture("magic_link_confirm_pending_mfa_response.json"),
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const response = await client.auth.confirmMagicLink("magic-link-token");

    expect(response).toEqual({
      mfaPending: true,
      mfaToken: "mfa_pending_token_stage1",
    });
  });

  it("first-factor webauthn begin response fixture normalizes challenge_id", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: loadContractFixture("webauthn_login_begin_response.json"),
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const response = await client.auth.beginWebAuthnLogin("dev@allyourbase.io");

    expect(response).toEqual({
      challengeId: "webauthn_challenge_fixture",
      options: {
        allowCredentials: [
          {
            id: "webauthn_login_begin_credential_a",
            type: "public-key",
          },
        ],
        challenge: "webauthn_login_begin_challenge",
        rpId: "127.0.0.1",
        timeout: 300000,
      },
    });
    expect(response.options.challenge).toBe("webauthn_login_begin_challenge");
    expect(response.options.allowCredentials).toEqual([
      { id: "webauthn_login_begin_credential_a", type: "public-key" },
    ]);
  });

  it("discoverable webauthn begin response fixture normalizes challenge_id", () => {
    const fixture = loadContractFixture("webauthn_discover_begin_response.json");
    const response = normalizeWebAuthnLoginBeginResponse(fixture);

    expect(response.challengeId).toBe("webauthn_discover_challenge_fixture");
    expect(response.options.challenge).toBe("webauthn_discover_challenge");
    expect(response.options.rpId).toBe("127.0.0.1");
    expect(response.options.timeout).toBe(300000);
    expect(response.options.allowCredentials ?? []).toEqual([]);
  });

  it("webauthn mfa enroll begin fixture pins creation options", () => {
    const fixture = loadContractFixture(
      "webauthn_enroll_begin_response.json",
    ) as WebAuthnEnrollBeginResponse;

    expect(fixture.challenge).toBe("webauthn_enroll_begin_challenge");
    expect(fixture.rp).toEqual({ id: "127.0.0.1", name: "Allyourbase" });
    expect(fixture.user).toEqual({
      id: "webauthn_enroll_user_id",
      name: "webauthn-e2e@example.com",
      displayName: "webauthn-e2e@example.com",
    });
    expect(fixture.pubKeyCredParams).toEqual([
      { type: "public-key", alg: -7 },
      { type: "public-key", alg: -35 },
      { type: "public-key", alg: -36 },
      { type: "public-key", alg: -257 },
      { type: "public-key", alg: -258 },
      { type: "public-key", alg: -259 },
      { type: "public-key", alg: -37 },
      { type: "public-key", alg: -38 },
      { type: "public-key", alg: -39 },
      { type: "public-key", alg: -8 },
    ]);
    expect(fixture.timeout).toBe(300000);
    expect(fixture.attestation).toBe("none");
    expect(fixture.excludeCredentials ?? []).toEqual([]);
  });

  it("webauthn mfa enroll confirm fixtures pin request and response envelopes", () => {
    const request = loadContractFixture(
      "webauthn_enroll_confirm_request.json",
    ) as {
      display_name: string;
      attestation_response: Record<string, unknown>;
    };
    const response = loadContractFixture("webauthn_enroll_confirm_response.json");

    expect(request.display_name).toBe("Primary security key");
    expect(request.attestation_response).toEqual({
      clientExtensionResults: {},
      id: "webauthn_enroll_credential",
      rawId: "webauthn_enroll_credential",
      response: {
        attestationObject: "webauthn_enroll_attestation_object",
        clientDataJSON:
          "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIiwiY2hhbGxlbmdlIjoid2ViYXV0aG5fZW5yb2xsX2JlZ2luX2NoYWxsZW5nZSIsIm9yaWdpbiI6Imh0dHA6Ly8xMjcuMC4wLjE6ODA5MCJ9",
        transports: ["internal"],
      },
      type: "public-key",
    });
    expect(response).toEqual({ message: "WebAuthn MFA enrollment confirmed" });
  });

  it("webauthn mfa challenge fixture normalizes challenge_id", () => {
    const fixture = loadContractFixture("webauthn_mfa_challenge_response.json");
    const response = normalizeWebAuthnMFAChallengeResponse(fixture);

    expect(response).toEqual({
      challengeId: "webauthn_mfa_challenge_fixture",
      options: {
        allowCredentials: [
          {
            id: "webauthn_mfa_credential_a",
            type: "public-key",
          },
        ],
        challenge: "webauthn_mfa_challenge",
        rpId: "127.0.0.1",
        timeout: 300000,
      },
    });
  });

  it("webauthn mfa verify fixtures pin request and auth response normalization", () => {
    const wireRequest = loadContractFixture("webauthn_mfa_verify_request.json") as {
      challenge_id: string;
      assertion_response: Record<string, unknown>;
    };
    const verifyRequest: WebAuthnMFAVerifyRequest = {
      challengeId: wireRequest.challenge_id,
      assertionResponse: wireRequest.assertion_response,
    };

    expect(JSON.parse(serializeWebAuthnMFAVerifyRequest(verifyRequest))).toEqual(wireRequest);
    expect(wireRequest).toEqual({
      challenge_id: "webauthn_mfa_challenge_fixture",
      assertion_response: {
        clientExtensionResults: {},
        id: "webauthn_mfa_credential",
        rawId: "webauthn_mfa_credential",
        response: {
          authenticatorData: "EsoXtJryKJQ28wPgFmAwoh5SXSZuIJJnQzgBqP1AcaAFAAAAAQ",
          clientDataJSON:
            "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0IiwiY2hhbGxlbmdlIjoid2ViYXV0aG5fbWZhX2NoYWxsZW5nZSIsIm9yaWdpbiI6Imh0dHA6Ly8xMjcuMC4wLjE6ODA5MCJ9",
          signature: "webauthn_mfa_signature",
          userHandle: "webauthn_mfa_user_handle",
        },
        type: "public-key",
      },
    });

    const response = normalizeAuthResponse(
      loadContractFixture("webauthn_mfa_verify_response.json") as AuthResponse,
    );
    expect(response).toEqual({
      token: "jwt_webauthn_mfa",
      refreshToken: "refresh_webauthn_mfa",
      user: {
        id: "usr_webauthn_mfa",
        email: "webauthn-e2e@example.com",
        emailVerified: undefined,
        createdAt: "2026-07-11T00:00:00Z",
        updatedAt: "2026-07-11T00:00:00Z",
      },
    });
  });

  it("discoverable webauthn finish request fixture pins login serialization", () => {
    const wireRequest = loadContractFixture("webauthn_discover_finish_request.json") as {
      challenge_id: string;
      assertion_response: {
        clientExtensionResults: Record<string, never>;
        id: string;
        rawId: string;
        response: {
          authenticatorData: string;
          clientDataJSON: string;
          signature: string;
          userHandle: string;
        };
        type: string;
      };
    };
    const finishRequest: WebAuthnLoginFinishRequest = {
      challengeId: wireRequest.challenge_id,
      assertionResponse: wireRequest.assertion_response,
    };

    expect(JSON.parse(serializeWebAuthnLoginFinishRequest(finishRequest))).toEqual(wireRequest);
    expect(wireRequest.challenge_id).toBe("webauthn_discover_challenge_fixture");
    expect(wireRequest.assertion_response.id).toBe("webauthn_discover_credential");
    expect(wireRequest.assertion_response.rawId).toBe("webauthn_discover_credential");
    expect(wireRequest.assertion_response.type).toBe("public-key");
    expect(wireRequest.assertion_response.response.clientDataJSON).toBe(
      "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0IiwiY2hhbGxlbmdlIjoid2ViYXV0aG5fZGlzY292ZXJfY2hhbGxlbmdlIiwib3JpZ2luIjoiaHR0cDovLzEyNy4wLjAuMTo4MDkwIn0",
    );
    expect(wireRequest.assertion_response.response.authenticatorData).toBe(
      "EsoXtJryKJQ28wPgFmAwoh5SXSZuIJJnQzgBqP1AcaAFAAAAAw",
    );
    expect(wireRequest.assertion_response.response.signature).toBe(
      "webauthn_discover_signature",
    );
    expect(wireRequest.assertion_response.response.userHandle).toBe(
      "webauthn_discover_user_handle",
    );
    expect(wireRequest.assertion_response.clientExtensionResults).toEqual({});
  });

  it("search synonym fixtures pin PUT request and normalized response envelopes", async () => {
    const requestFixture = loadContractFixture(
      "search_synonyms_request.json",
    ) as SearchSynonymsRequest;
    const responseFixture = loadContractFixture(
      "search_synonyms_response.json",
    ) as SearchSynonymsResponse;
    const fetchFn = mockFetchSequence([{ status: 200, body: responseFixture }]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    client.setApiKey("admin-token");

    const response = await client.searchSettings.setSynonyms(
      "sdk_contract_synonyms_fixture",
      requestFixture,
    );

    expect(response).toEqual(responseFixture);
    expect(response.groups.map((group) => group.terms)).toEqual([
      ["new york", "nyc"],
      ["science fiction", "scifi"],
    ]);

    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe(
      "https://api.example.com/api/collections/sdk_contract_synonyms_fixture/synonyms/",
    );
    expect(call[1]?.method).toBe("PUT");
    expect(call[1]?.headers).toMatchObject({
      Authorization: "Bearer admin-token",
      "Content-Type": "application/json",
    });
    expect(call[1]?.body).toBe(JSON.stringify(requestFixture));
  });

  it("search synonym response fixture decodes through the GET owner", async () => {
    const responseFixture = loadContractFixture(
      "search_synonyms_response.json",
    ) as SearchSynonymsResponse;
    const fetchFn = mockFetchSequence([{ status: 200, body: responseFixture }]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    client.setApiKey("admin-token");

    const response = await client.searchSettings.getSynonyms(
      "sdk_contract_synonyms_fixture",
    );

    expect(response).toEqual({
      groups: [
        { terms: ["new york", "nyc"] },
        { terms: ["science fiction", "scifi"] },
      ],
    });

    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe(
      "https://api.example.com/api/collections/sdk_contract_synonyms_fixture/synonyms/",
    );
    expect(call[1]?.headers).toMatchObject({
      Authorization: "Bearer admin-token",
    });
  });
});
