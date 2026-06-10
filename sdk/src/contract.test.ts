import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import { AYBClient as PublicAYBClient } from "./index";
import { createInstantSearchClient } from "./instantsearch";
import type {
  AuthResponse as PublicAuthResponse,
  ListResponse as PublicListResponse,
  SearchHit as PublicSearchHit,
  StorageObject as PublicStorageObject,
  User as PublicUser,
  WebAuthnLoginBeginResponse as PublicWebAuthnLoginBeginResponse,
} from "./index";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";
import type {
  AuthResponse,
  ListResponse,
  SearchHit,
  StorageObject,
  User,
  WebAuthnLoginBeginResponse,
} from "./types";
import type {
  InstantSearchClient,
  InstantSearchResponse,
  InstantSearchSearchRequest,
} from "./instantsearch";

function loadContractFixture(name: string): unknown {
  const fixturePath = resolve(__dirname, "../../tests/contract/fixtures/sdk_contract", name);
  return JSON.parse(readFileSync(fixturePath, "utf8")) as unknown;
}

describe("SDK contract fixtures", () => {
  it("keeps magic-link fixtures canonical to sdk_contract tree", () => {
    const sdkParityFixtureDir = resolve(__dirname, "../../tests/contract/fixtures/sdk_parity");
    const duplicateMagicLinkFixtures = readdirSync(sdkParityFixtureDir).filter((fileName) =>
      fileName.startsWith("magic_link_"),
    );

    expect(duplicateMagicLinkFixtures).toEqual([]);
  });

  it("public barrel re-exports core client and canonical types", () => {
    const publicClient = new PublicAYBClient("https://api.example.com");
    expect(publicClient).toBeInstanceOf(AYBClient);

    const assertAuthType = (_value: AuthResponse): void => {};
    const assertListType = (_value: ListResponse<Record<string, unknown>>): void => {};
    const assertSearchHitType = (_value: ListResponse<SearchHit<{ id: string }>>): void => {};
    const assertStorageType = (_value: StorageObject): void => {};
    const assertUserType = (_value: User): void => {};
    const assertWebAuthnBeginType = (_value: WebAuthnLoginBeginResponse): void => {};

    assertAuthType({} as PublicAuthResponse);
    assertListType({} as PublicListResponse<Record<string, unknown>>);
    assertSearchHitType({} as PublicListResponse<PublicSearchHit<{ id: string }>>);
    assertStorageType({} as PublicStorageObject);
    assertUserType({} as PublicUser);
    assertWebAuthnBeginType({} as PublicWebAuthnLoginBeginResponse);
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

    assertSearchClient(searchClient);
    assertRequest({ indexName: "posts", params: { query: "postgres" } });
    assertResponse({ results: [] });
    expect(typeof searchClient.search).toBe("function");
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
        body: {
          challenge_id: "webauthn-challenge-stage3",
          options: {
            challenge: "Y2hhbGxlbmdl",
            rpId: "example.com",
            allowCredentials: [
              {
                id: "Y3JlZA",
                type: "public-key",
              },
            ],
          },
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const response = await client.auth.beginWebAuthnLogin("dev@allyourbase.io");

    expect(response).toEqual({
      challengeId: "webauthn-challenge-stage3",
      options: {
        challenge: "Y2hhbGxlbmdl",
        rpId: "example.com",
        allowCredentials: [{ id: "Y3JlZA", type: "public-key" }],
      },
    });
  });
});
