import { describe, it, expect, expectTypeOf, vi, beforeEach, afterEach } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import type { RpcNotifyOption, RpcOptions, SearchHit } from "./index";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";

// --- EventSource mock for realtime tests ---

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close() {
    this.closed = true;
  }

  // Test helper: send a message event.
  _sendMessage(data: string) {
    if (this.onmessage) {
      this.onmessage({ data } as MessageEvent);
    }
  }

  // Test helper: send a parsed object as JSON.
  _sendJSON(obj: unknown) {
    this._sendMessage(JSON.stringify(obj));
  }
}

const OriginalEventSource = globalThis.EventSource;

function mockFetch(
  status: number,
  body: unknown,
  headers?: Record<string, string>,
): typeof globalThis.fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    headers: new Headers(headers),
    json: () => Promise.resolve(body),
  }) as unknown as typeof globalThis.fetch;
}

describe("AYBClient", () => {
  it("constructs with baseURL", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("strips trailing slash from baseURL", () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090/", { fetch: fetchFn });
    client.records.list("posts");
    expect(fetchFn).toHaveBeenCalledWith(
      "http://localhost:8090/api/collections/posts",
      expect.anything(),
    );
  });

  it("setTokens / clearTokens", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("access", "refresh");
    expect(client.token).toBe("access");
    expect(client.refreshToken).toBe("refresh");
    client.clearTokens();
    expect(client.token).toBeNull();
  });
});
describe("auth persistence", () => {
  it("restores session from injected persistence at construction", async () => {
    const load = vi.fn().mockResolvedValue({ token: "restored-token", refreshToken: "restored-refresh" });
    const client = new AYBClient("http://localhost:8090", {
      authPersistence: { load },
    });

    await Promise.resolve();

    expect(load).toHaveBeenCalledTimes(1);
    expect(client.token).toBe("restored-token");
    expect(client.refreshToken).toBe("restored-refresh");
  });

  it("exposes a restore promise for consumers that need startup auth sync", async () => {
    const load = vi.fn().mockResolvedValue({ token: "restored-token", refreshToken: "restored-refresh" });
    const client = new AYBClient("http://localhost:8090", {
      authPersistence: { load },
    });

    await client.waitForSessionRestore();

    expect(load).toHaveBeenCalledTimes(1);
    expect(client.token).toBe("restored-token");
    expect(client.refreshToken).toBe("restored-refresh");
  });

  it("persists tokens on setTokens and successful sign-in paths", async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    const fetchFn = mockFetch(200, {
      token: "signed-in-token",
      refreshToken: "signed-in-refresh",
      user: { id: "u-1", email: "demo@example.com" },
    });
    const client = new AYBClient("http://localhost:8090", {
      fetch: fetchFn,
      authPersistence: { save },
    });

    client.setTokens("manual-token", "manual-refresh");
    await client.auth.login("demo@example.com", "pass");

    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenNthCalledWith(1, {
      token: "manual-token",
      refreshToken: "manual-refresh",
    });
    expect(save).toHaveBeenNthCalledWith(2, {
      token: "signed-in-token",
      refreshToken: "signed-in-refresh",
    });
  });

  it("clears persistence on clearTokens and sign-out paths", async () => {
    const clear = vi.fn().mockResolvedValue(undefined);
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", {
      fetch: fetchFn,
      authPersistence: { clear },
    });

    client.clearTokens();
    client.setTokens("t", "r");
    await client.auth.logout();

    expect(clear).toHaveBeenCalledTimes(2);
  });

  it("treats persistence callback failures as non-fatal for token state and auth fanout", async () => {
    const save = vi.fn().mockRejectedValue(new Error("save failed"));
    const clear = vi.fn().mockRejectedValue(new Error("clear failed"));
    const events: string[] = [];
    const fetchFn = mockFetch(200, {
      token: "signed-in-token",
      refreshToken: "signed-in-refresh",
      user: { id: "u-1", email: "demo@example.com" },
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: fetchFn,
      authPersistence: { save, clear },
    });
    client.onAuthStateChange((event) => events.push(event));

    expect(() => client.setTokens("manual-token", "manual-refresh")).not.toThrow();
    expect(client.token).toBe("manual-token");

    await expect(client.auth.login("demo@example.com", "pass")).resolves.toMatchObject({
      token: "signed-in-token",
      refreshToken: "signed-in-refresh",
    });
    expect(events).toContain("SIGNED_IN");

    expect(() => client.clearTokens()).not.toThrow();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });
});


describe("health", () => {
  it("returns health response on 200", async () => {
    const fetchFn = mockFetch(200, { status: "ok", database: "ok" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.health();

    expect(result).toEqual({ status: "ok", database: "ok" });
  });

  it("throws AYBError with status 503 on degraded health", async () => {
    const fetchFn = mockFetch(503, { status: "degraded", database: "unreachable" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await expect(client.health()).rejects.toBeInstanceOf(AYBError);
    await expect(client.health()).rejects.toMatchObject({ status: 503 });
  });

  it("omits auth header even when tokens are set", async () => {
    const fetchFn = mockFetch(200, { status: "ok", database: "ok" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");

    await client.health();

    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBeUndefined();
  });
});

describe("records", () => {
  let fetchFn: ReturnType<typeof mockFetch>;
  let client: AYBClient;

  beforeEach(() => {
    fetchFn = mockFetch(200, { items: [{ id: "1" }], page: 1, perPage: 20, totalItems: 1, totalPages: 1 });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
  });

  it("list sends correct URL", async () => {
    await client.records.list("posts", { page: 2, sort: "-created_at", filter: "active=true" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts?");
    expect(url).toContain("page=2");
    expect(url).toContain("sort=-created_at");
    expect(url).toContain("filter=active%3Dtrue");
  });

  it("list with no params", async () => {
    await client.records.list("posts");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/collections/posts");
  });

  it("list encodes collection as one path segment", async () => {
    await client.records.list("posts/../../admin");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/collections/posts%2F..%2F..%2Fadmin");
  });

  it("get sends correct URL", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "hello" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.get("posts", "42", { expand: "author" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts/42");
    expect(url).toContain("expand=author");
  });

  it("create sends POST with body", async () => {
    fetchFn = mockFetch(201, { id: "new", title: "test" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.create("posts", { title: "test" });
    expect(result).toEqual({ id: "new", title: "test" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ title: "test" });
  });

  it("update sends PATCH", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "updated" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.update("posts", "42", { title: "updated" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("PATCH");
  });

  it("delete sends DELETE", async () => {
    fetchFn = mockFetch(204, undefined);
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.delete("posts", "42");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("DELETE");
  });

  it("batch sends POST to /batch with operations array", async () => {
    const results = [
      { index: 0, status: 201, body: { id: "1", title: "A" } },
      { index: 1, status: 200, body: { id: "2", title: "B" } },
      { index: 2, status: 204 },
    ];
    fetchFn = mockFetch(200, results);
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const ops = [
      { method: "create" as const, body: { title: "A" } },
      { method: "update" as const, id: "2", body: { title: "B" } },
      { method: "delete" as const, id: "3" },
    ];
    const res = await client.records.batch("posts", ops);
    expect(res).toEqual(results);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/collections/posts/batch");
    expect(call[1].method).toBe("POST");
    const sent = JSON.parse(call[1].body as string);
    expect(sent.operations).toHaveLength(3);
    expect(sent.operations[0].method).toBe("create");
  });
});

describe("auth", () => {
  it("login stores tokens", async () => {
    const fetchFn = mockFetch(200, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const res = await client.auth.login("a@b.com", "pass");
    expect(res.token).toBe("tok");
    expect(client.token).toBe("tok");
    expect(client.refreshToken).toBe("ref");
  });

  it("register stores tokens and sends correct request", async () => {
    const fetchFn = mockFetch(201, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.register("a@b.com", "pass");
    expect(client.token).toBe("tok");
    expect(client.refreshToken).toBe("ref");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/register");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "a@b.com", password: "pass" });
  });

  it("logout clears tokens and sends refresh token", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.logout();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/logout");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ refreshToken: "ref" });
  });

  it("refresh sends current refresh token and updates tokens", async () => {
    const fetchFn = mockFetch(200, { token: "new-tok", refreshToken: "new-ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("old-tok", "old-ref");
    await client.auth.refresh();
    expect(client.token).toBe("new-tok");
    expect(client.refreshToken).toBe("new-ref");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/refresh");
    expect(JSON.parse(call[1].body as string)).toEqual({ refreshToken: "old-ref" });
  });

  it("confirmPasswordReset sends token and new password", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.confirmPasswordReset("reset-tok", "newpass123");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/password-reset/confirm");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "reset-tok", password: "newpass123" });
  });

  it("verifyEmail sends token", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.verifyEmail("verify-tok");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/verify");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "verify-tok" });
  });

  it("resendVerification sends POST", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.resendVerification();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/verify/resend");
    expect(call[1].method).toBe("POST");
  });

  it("sends auth header when token is set", async () => {
    const fetchFn = mockFetch(200, { id: "1", email: "a@b.com" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("my-token", "my-refresh");
    await client.auth.me();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer my-token");
  });

  it("requestPasswordReset sends POST with email", async () => {
    const fetchFn = mockFetch(200, { message: "ok" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.requestPasswordReset("a@b.com");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/password-reset");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "a@b.com" });
  });

  it("deleteAccount sends DELETE to /me and clears tokens", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.deleteAccount();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/me");
    expect(call[1].method).toBe("DELETE");
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("deleteAccount emits SIGNED_OUT event", async () => {
    expect.assertions(1);
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_OUT");
    });
    await client.auth.deleteAccount();
  });

  it("signInAnonymously sends POST to /anonymous and stores tokens", async () => {
    expect.assertions(5);
    const fetchFn = mockFetch(201, {
      token: "anon-token",
      refreshToken: "anon-refresh",
      user: { id: "anon-1", is_anonymous: true },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_IN");
    });

    const result = await client.auth.signInAnonymously();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/anonymous");
    expect(call[1].method).toBe("POST");
    expect(client.token).toBe("anon-token");
    expect(result.user.isAnonymous).toBe(true);
  });

  it("requestMagicLink sends POST with email body", async () => {
    const fetchFn = mockFetch(200, { message: "if valid, a login link has been sent" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const result = await client.auth.requestMagicLink("demo@example.com");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/magic-link");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "demo@example.com" });
    expect(result).toEqual({ message: "if valid, a login link has been sent" });
  });

  it("confirmMagicLink stores tokens and emits SIGNED_IN for full auth response", async () => {
    expect.assertions(8);
    const fetchFn = mockFetch(200, {
      token: "magic-token",
      refreshToken: "magic-refresh",
      user: { id: "u-1", email: "magic@example.com", email_verified: true },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_IN");
    });

    const result = await client.auth.confirmMagicLink("magic-confirm-token");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/magic-link/confirm");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "magic-confirm-token" });
    expect(client.token).toBe("magic-token");
    expect(client.refreshToken).toBe("magic-refresh");
    expect("token" in result).toBe(true);
    if ("token" in result) {
      expect(result.user.emailVerified).toBe(true);
    }
  });

  it("confirmMagicLink returns MFA pending response without token persistence", async () => {
    const fetchFn = mockFetch(200, { mfa_pending: true, mfa_token: "pending-token" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const eventSpy = vi.fn();
    client.onAuthStateChange(eventSpy);

    const result = await client.auth.confirmMagicLink("pending-confirm-token");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/magic-link/confirm");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "pending-confirm-token" });
    expect(result).toEqual({ mfaPending: true, mfaToken: "pending-token" });
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
    expect(eventSpy).not.toHaveBeenCalled();
  });

  it("beginWebAuthnLogin sends POST with email and normalizes challenge shape", async () => {
    const fetchFn = mockFetch(200, {
      challenge_id: "challenge-first-factor",
      options: {
        challenge: "Zmlyc3QtZmFjdG9yLWNoYWxsZW5nZQ",
        rpId: "localhost",
        allowCredentials: [],
      },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const result = await client.auth.beginWebAuthnLogin("passkey@example.com");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/webauthn/login/begin");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "passkey@example.com" });
    expect(result).toEqual({
      challengeId: "challenge-first-factor",
      options: {
        challenge: "Zmlyc3QtZmFjdG9yLWNoYWxsZW5nZQ",
        rpId: "localhost",
        allowCredentials: [],
      },
    });
  });

  it("finishWebAuthnLogin sends challenge/assertion payload and stores tokens", async () => {
    expect.assertions(8);
    const fetchFn = mockFetch(200, {
      token: "passkey-token",
      refreshToken: "passkey-refresh",
      user: { id: "u-passkey", email: "passkey@example.com", email_verified: true },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_IN");
    });

    const assertionResponse = {
      id: "cred-1",
      rawId: "cmF3LWlk",
      type: "public-key",
      response: {
        clientDataJSON: "Y2xpZW50LWRhdGE",
        authenticatorData: "YXV0aC1kYXRh",
        signature: "c2ln",
        userHandle: null,
      },
      clientExtensionResults: {},
    };
    const result = await client.auth.finishWebAuthnLogin("challenge-first-factor", assertionResponse);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/webauthn/login/finish");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({
      challenge_id: "challenge-first-factor",
      assertion_response: assertionResponse,
    });
    expect(result.token).toBe("passkey-token");
    expect(client.token).toBe("passkey-token");
    expect(client.refreshToken).toBe("passkey-refresh");
    expect(result.user.emailVerified).toBe(true);
  });

  it("linkEmail requires auth header and stores tokens", async () => {
    expect.assertions(7);
    const fetchFn = mockFetch(200, {
      token: "linked-token",
      refreshToken: "linked-refresh",
      user: {
        id: "linked-user",
        email: "linked@example.com",
        is_anonymous: false,
        linked_at: "2026-05-20T10:11:12Z",
      },
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("anon-token", "anon-refresh");
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_IN");
    });

    const result = await client.auth.linkEmail("linked@example.com", "S3cretPass!");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];

    expect(call[0]).toContain("/api/auth/link/email");
    expect(call[1].method).toBe("POST");
    expect(call[1].headers.Authorization).toBe("Bearer anon-token");
    expect(JSON.parse(call[1].body as string)).toEqual({
      email: "linked@example.com",
      password: "S3cretPass!",
    });
    expect(client.token).toBe("linked-token");
    expect(result.user.linkedAt).toBe("2026-05-20T10:11:12Z");
  });
});

describe("auth webauthn MFA", () => {
  function utf8Bytes(value: string): ArrayBuffer {
    return new TextEncoder().encode(value).buffer;
  }

  class MockAuthenticatorAttestationResponse {
    clientDataJSON: ArrayBuffer;
    attestationObject: ArrayBuffer;
    constructor(clientDataJSON: ArrayBuffer, attestationObject: ArrayBuffer) {
      this.clientDataJSON = clientDataJSON;
      this.attestationObject = attestationObject;
    }
  }

  class MockAuthenticatorAssertionResponse {
    clientDataJSON: ArrayBuffer;
    authenticatorData: ArrayBuffer;
    signature: ArrayBuffer;
    userHandle: ArrayBuffer | null;
    constructor(
      clientDataJSON: ArrayBuffer,
      authenticatorData: ArrayBuffer,
      signature: ArrayBuffer,
      userHandle: ArrayBuffer | null,
    ) {
      this.clientDataJSON = clientDataJSON;
      this.authenticatorData = authenticatorData;
      this.signature = signature;
      this.userHandle = userHandle;
    }
  }

  class MockPublicKeyCredential {
    id: string;
    rawId: ArrayBuffer;
    type: string;
    response: object;
    constructor(id: string, rawId: ArrayBuffer, type: string, response: object) {
      this.id = id;
      this.rawId = rawId;
      this.type = type;
      this.response = response;
    }
    getClientExtensionResults() {
      return {};
    }
  }

  function stubWebAuthnGlobals(
    credentialsApi: { create?: ReturnType<typeof vi.fn>; get?: ReturnType<typeof vi.fn> },
  ): void {
    vi.stubGlobal("navigator", { credentials: credentialsApi });
    vi.stubGlobal("PublicKeyCredential", MockPublicKeyCredential);
    vi.stubGlobal("AuthenticatorAttestationResponse", MockAuthenticatorAttestationResponse);
    vi.stubGlobal("AuthenticatorAssertionResponse", MockAuthenticatorAssertionResponse);
  }

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("enrollPasskey POSTs enroll, runs ceremony, then POSTs enroll/confirm with display_name and serialized attestation", async () => {
    const creationOptions = {
      challenge: "Y2hhbGxlbmdl",
      rp: { id: "localhost", name: "Allyourbase" },
      user: { id: "dXNlci1pZA", name: "user@example.com", displayName: "User" },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    };
    const attestationCredential = new MockPublicKeyCredential(
      "new-cred",
      utf8Bytes("raw-id"),
      "public-key",
      new MockAuthenticatorAttestationResponse(
        utf8Bytes('{"type":"webauthn.create"}'),
        utf8Bytes("attestation-bytes"),
      ),
    );
    const credentialsCreate = vi.fn().mockResolvedValue(attestationCredential);
    stubWebAuthnGlobals({ create: credentialsCreate });

    const fetchFn = mockFetchSequence([
      { status: 200, body: creationOptions },
      { status: 204, body: undefined },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("session-token", "session-refresh");

    const result = await client.auth.enrollPasskey("My YubiKey");
    expect(result).toBeUndefined();

    const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls).toHaveLength(2);

    expect(calls[0][0]).toContain("/api/auth/mfa/webauthn/enroll");
    expect(calls[0][0]).not.toContain("/confirm");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toBeUndefined();
    expect(calls[0][1].headers.Authorization).toBe("Bearer session-token");

    expect(credentialsCreate).toHaveBeenCalledTimes(1);

    expect(calls[1][0]).toContain("/api/auth/mfa/webauthn/enroll/confirm");
    expect(calls[1][1].method).toBe("POST");
    expect(calls[1][1].headers.Authorization).toBe("Bearer session-token");
    expect(JSON.parse(calls[1][1].body as string)).toEqual({
      display_name: "My YubiKey",
      attestation_response: {
        id: "new-cred",
        rawId: "cmF3LWlk",
        type: "public-key",
        response: {
          clientDataJSON: "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIn0",
          attestationObject: "YXR0ZXN0YXRpb24tYnl0ZXM",
        },
        clientExtensionResults: {},
      },
    });
  });

  it("verifyPasskey sends mfa bearer on both challenge and verify, persists session from verify response", async () => {
    const challengeBody = {
      challenge_id: "mfa-challenge-1",
      options: {
        challenge: "bWZhLWNoYWxsZW5nZQ",
        rpId: "localhost",
        allowCredentials: [{ id: "Y3JlZA", type: "public-key" }],
      },
    };
    const verifyBody = {
      token: "stepped-up-token",
      refreshToken: "stepped-up-refresh",
      user: { id: "u-mfa", email: "mfa@example.com", email_verified: true },
    };

    const assertionCredential = new MockPublicKeyCredential(
      "mfa-cred",
      utf8Bytes("raw-mfa"),
      "public-key",
      new MockAuthenticatorAssertionResponse(
        utf8Bytes('{"type":"webauthn.get"}'),
        utf8Bytes("auth-data"),
        utf8Bytes("sig"),
        null,
      ),
    );
    const credentialsGet = vi.fn().mockResolvedValue(assertionCredential);
    stubWebAuthnGlobals({ get: credentialsGet });

    const fetchFn = mockFetchSequence([
      { status: 200, body: challengeBody },
      { status: 200, body: verifyBody },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_IN");
    });

    const result = await client.auth.verifyPasskey("pending-mfa-token");

    const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls).toHaveLength(2);

    expect(calls[0][0]).toContain("/api/auth/mfa/webauthn/challenge");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toBeUndefined();
    expect(calls[0][1].headers.Authorization).toBe("Bearer pending-mfa-token");

    expect(credentialsGet).toHaveBeenCalledTimes(1);

    expect(calls[1][0]).toContain("/api/auth/mfa/webauthn/verify");
    expect(calls[1][1].method).toBe("POST");
    expect(calls[1][1].headers.Authorization).toBe("Bearer pending-mfa-token");
    const verifyPayload = JSON.parse(calls[1][1].body as string);
    expect(verifyPayload.challenge_id).toBe("mfa-challenge-1");
    expect(verifyPayload.assertion_response).toMatchObject({
      id: "mfa-cred",
      type: "public-key",
    });

    expect(result.token).toBe("stepped-up-token");
    expect(client.token).toBe("stepped-up-token");
    expect(client.refreshToken).toBe("stepped-up-refresh");
  });

  it("verifyPasskey sends the mfa token (not the session token) when a session is already set", async () => {
    const challengeBody = {
      challenge_id: "mfa-challenge-2",
      options: { challenge: "Y2gy" },
    };
    const verifyBody = {
      token: "stepped-up-2",
      refreshToken: "stepped-up-refresh-2",
      user: { id: "u-mfa-2" },
    };
    const assertionCredential = new MockPublicKeyCredential(
      "mfa-cred-2",
      utf8Bytes("raw-mfa-2"),
      "public-key",
      new MockAuthenticatorAssertionResponse(
        utf8Bytes('{"t":"g"}'),
        utf8Bytes("ad"),
        utf8Bytes("sg"),
        null,
      ),
    );
    stubWebAuthnGlobals({ get: vi.fn().mockResolvedValue(assertionCredential) });

    const fetchFn = mockFetchSequence([
      { status: 200, body: challengeBody },
      { status: 200, body: verifyBody },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("session-token", "session-refresh");

    await client.auth.verifyPasskey("pending-mfa-token");

    const calls = (fetchFn as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls[0][1].headers.Authorization).toBe("Bearer pending-mfa-token");
    expect(calls[1][1].headers.Authorization).toBe("Bearer pending-mfa-token");
  });
});

describe("storage", () => {
  it("downloadURL builds correct URL with bucket and name", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.storage.downloadURL("avatars", "photo.jpg")).toBe(
      "http://localhost:8090/api/storage/avatars/photo.jpg",
    );
  });

  it("downloadURL encodes unsafe characters but preserves nested path separators", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.storage.downloadURL("avatars", "nested/path to/image?.jpg")).toBe(
      "http://localhost:8090/api/storage/avatars/nested/path%20to/image%3F.jpg",
    );
  });

  it("upload sends POST to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(201, { id: "1", bucket: "avatars", name: "photo.jpg" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const file = new Blob(["test"], { type: "image/jpeg" });
    await client.storage.upload("avatars", file, "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars");
    expect(call[1].method).toBe("POST");
  });

  it("delete sends DELETE to /api/storage/{bucket}/{name}", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.delete("avatars", "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg");
    expect(call[1].method).toBe("DELETE");
  });

  it("delete encodes bucket and object path safely", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.delete("avatars/../private", "nested/path to/image?.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe(
      "http://localhost:8090/api/storage/avatars%2F..%2Fprivate/nested/path%20to/image%3F.jpg",
    );
    expect(call[1].method).toBe("DELETE");
  });

  it("list sends GET to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { prefix: "user_", limit: 10 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/storage/avatars?");
    expect(url).toContain("prefix=user_");
    expect(url).toContain("limit=10");
  });

  it("getSignedURL sends POST to /api/storage/{bucket}/{name}/sign", async () => {
    const fetchFn = mockFetch(200, { url: "/api/storage/avatars/photo.jpg?exp=123&sig=abc" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.getSignedURL("avatars", "photo.jpg", 7200);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg/sign");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ expiresIn: 7200 });
  });

  it("getSignedURL defaults expiresIn to 3600", async () => {
    const fetchFn = mockFetch(200, { url: "/signed" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.getSignedURL("avatars", "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(JSON.parse(call[1].body as string)).toEqual({ expiresIn: 3600 });
  });

  it("list with no params sends clean URL", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/storage/avatars");
  });

  it("list includes offset param", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { offset: 10 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("offset=10");
  });

  it("list preserves offset=0", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { offset: 0 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("offset=0");
  });
});

describe("records params coverage", () => {
  it("list includes perPage, fields, expand, skipTotal", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 5, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { perPage: 5, fields: "id,title", expand: "author", skipTotal: true });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("perPage=5");
    expect(url).toContain("fields=id%2Ctitle");
    expect(url).toContain("expand=author");
    expect(url).toContain("skipTotal=true");
  });

  it("get includes fields param", async () => {
    const fetchFn = mockFetch(200, { id: "1", title: "hello" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.get("posts", "1", { fields: "id,title" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("fields=id%2Ctitle");
  });

  it("list preserves page=0 (falsy but valid)", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 0, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { page: 0 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("page=0");
  });

  it("list includes search param", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "postgres database" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("search=postgres+database");
  });

  it("list combines search with filter", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "postgres", filter: "status='active'" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("search=postgres");
    expect(url).toContain("filter=status%3D%27active%27");
  });

  it("delete returns undefined for 204", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.delete("posts", "42");
    expect(result).toBeUndefined();
  });

  it("list encodes fuzzy=true (omits when false/undefined)", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "hello", fuzzy: true });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("fuzzy=true");

    const fetchFn2 = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client2 = new AYBClient("http://localhost:8090", { fetch: fetchFn2 });
    await client2.records.list("posts", { search: "hello", fuzzy: false });
    const url2 = (fetchFn2 as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url2).not.toContain("fuzzy");
  });

  it("list encodes typo_threshold (omits when undefined)", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "hello", fuzzy: true, typoThreshold: 0.3 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("typo_threshold=0.3");

    const fetchFn2 = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client2 = new AYBClient("http://localhost:8090", { fetch: fetchFn2 });
    await client2.records.list("posts", { search: "hello", fuzzy: true, typoThreshold: undefined });
    const url2 = (fetchFn2 as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url2).not.toContain("typo_threshold");
  });

  it("list encodes highlight column (omits when absent)", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "hello", highlight: "title" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("highlight=title");

    const fetchFn2 = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client2 = new AYBClient("http://localhost:8090", { fetch: fetchFn2 });
    await client2.records.list("posts", { search: "hello" });
    const url2 = (fetchFn2 as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url2).not.toContain("highlight");
  });

  it("list exposes optional _highlight on the default item shape and via SearchHit<T>", async () => {
    const fetchFn = mockFetch(200, {
      items: [{ id: "1", title: "Hello world", _highlight: "<b>Hello</b> world" }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.list("posts", { search: "hello", highlight: "title" });
    expect(result.items[0]._highlight).toBe("<b>Hello</b> world");

    const fetchFn2 = mockFetch(200, {
      items: [{ id: "1", title: "Hello world", _highlight: "<b>Hello</b> world" }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    const client2 = new AYBClient("http://localhost:8090", { fetch: fetchFn2 });
    const typed = await client2.records.list<SearchHit<{ id: string; title: string }>>(
      "posts",
      { search: "hello", highlight: "title" },
    );
    expect(typed.items[0].title).toBe("Hello world");
    expect(typed.items[0]._highlight).toBe("<b>Hello</b> world");
  });

  it("list encodes facets as comma-separated single param", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { facets: ["col_a", "col_b"] });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("facets=col_a%2Ccol_b");
  });

  it("list encodes semantic search params", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", {
      semantic: true,
      semanticQuery: "find similar articles",
      vectorColumn: "embedding",
      distance: "cosine",
    });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("semantic=true");
    expect(url).toContain("semantic_query=find+similar+articles");
    expect(url).toContain("vector_column=embedding");
    expect(url).toContain("distance=cosine");
  });

  it("list encodes nearest as JSON-stringified number array", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { nearest: [0.1, 0.2, 0.3] });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    const params = new URL(url).searchParams;
    expect(params.get("nearest")).toBe("[0.1,0.2,0.3]");
  });

  it("list response includes typed facets envelope", async () => {
    const facetsPayload = { status: [{ value: "published", count: 2 }] };
    const fetchFn = mockFetch(200, {
      items: [{ id: "1" }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      facets: facetsPayload,
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.list("posts", { facets: ["status"] });
    expect(result.facets).toEqual(facetsPayload);
    expect(result.facets?.status[0].value).toBe("published");
    expect(result.facets?.status[0].count).toBe(2);
  });
});

describe("API keys", () => {
  it("setApiKey stores key as token", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.token).toBe("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.refreshToken).toBeNull();
  });

  it("setApiKey clears existing refresh token", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("jwt-token", "refresh-token");
    expect(client.refreshToken).toBe("refresh-token");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.refreshToken).toBeNull();
  });

  it("clearApiKey removes API key", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    client.clearApiKey();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("API key sends Bearer header on requests", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer ayb_abc123def456abc123def456abc123def456abc123def456");
  });

  it("API key is used for all request types", async () => {
    const apiKey = "ayb_abc123def456abc123def456abc123def456abc123def456";
    const fetchFn = mockFetch(200, { id: "1", title: "test" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey(apiKey);

    await client.records.get("posts", "1");
    const getCall = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(getCall[1].headers.Authorization).toBe(`Bearer ${apiKey}`);

    await client.records.create("posts", { title: "new" });
    const createCall = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[1];
    expect(createCall[1].headers.Authorization).toBe(`Bearer ${apiKey}`);
  });

  it("setApiKey replaces JWT token", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("jwt-token", "refresh-token");
    client.setApiKey("ayb_newkey123456789012345678901234567890123456");
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer ayb_newkey123456789012345678901234567890123456");
  });

  it("no auth header when no API key or token set", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBeUndefined();
  });
});

describe("error handling", () => {
  it("throws AYBError on non-2xx", async () => {
    const fetchFn = mockFetch(404, { message: "collection not found: missing" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.records.list("missing")).rejects.toThrow(AYBError);
    await expect(client.records.list("missing")).rejects.toThrow("collection not found: missing");
  });

  it("AYBError has status", async () => {
    const fetchFn = mockFetch(401, { message: "unauthorized" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(2);
    try {
      await client.auth.me();
    } catch (e) {
      expect(e).toBeInstanceOf(AYBError);
      expect((e as AYBError).status).toBe(401);
    }
  });

  it("AYBError includes data and docUrl from server response", async () => {
    const fetchFn = mockFetch(409, {
      message: "unique constraint violation",
      data: {
        users_email_key: { code: "unique_violation", message: "Key (email)=(a@b.com) already exists." },
      },
      doc_url: "https://allyourbase.io/guide/api-reference#error-format",
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(4);
    try {
      await client.records.create("users", { email: "a@b.com" });
    } catch (e) {
      expect(e).toBeInstanceOf(AYBError);
      const err = e as AYBError;
      expect(err.status).toBe(409);
      expect(err.data).toEqual({
        users_email_key: { code: "unique_violation", message: "Key (email)=(a@b.com) already exists." },
      });
      expect(err.docUrl).toBe("https://allyourbase.io/guide/api-reference#error-format");
    }
  });

  it("AYBError data and docUrl are undefined when server omits them", async () => {
    const fetchFn = mockFetch(404, { message: "record not found" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(2);
    try {
      await client.records.get("posts", "999");
    } catch (e) {
      const err = e as AYBError;
      expect(err.data).toBeUndefined();
      expect(err.docUrl).toBeUndefined();
    }
  });

  it("AYBError falls back to statusText when json parse fails", async () => {
    const fetchFn = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      statusText: "Bad Gateway",
      headers: new Headers(),
      json: () => Promise.reject(new Error("not json")),
    }) as unknown as typeof globalThis.fetch;
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.records.list("posts")).rejects.toThrow("Bad Gateway");
  });
});

describe("rpc", () => {
  it("calls POST /api/rpc/{function} with args", async () => {
    const fetchFn = mockFetch(200, 42);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc("get_total", { user_id: "abc" });
    expect(result).toBe(42);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/rpc/get_total");
    expect(call[1].method).toBe("POST");
    expect(call[1].headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(call[1].body as string)).toEqual({ user_id: "abc" });
  });

  it("encodes rpc function name as one path segment", async () => {
    const fetchFn = mockFetch(200, 42);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.rpc("dangerous/name with spaces");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/rpc/dangerous%2Fname%20with%20spaces");
  });

  it("calls without body when no args", async () => {
    const fetchFn = mockFetch(200, "PostgreSQL 16.2");
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc<string>("pg_version");
    expect(result).toBe("PostgreSQL 16.2");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/rpc/pg_version");
    expect(call[1].body).toBeUndefined();
    expect(call[1].headers["Content-Type"]).toBeUndefined();
  });

  it("calls without body when args is empty object", async () => {
    const fetchFn = mockFetch(200, "ok");
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.rpc("no_args_fn", {});
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].body).toBeUndefined();
  });

  it("returns undefined for void functions (204)", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc("cleanup_old_data", { days: 30 });
    expect(result).toBeUndefined();
  });

  it("returns array for set-returning functions", async () => {
    const rows = [
      { id: "1", name: "Alice" },
      { id: "2", name: "Bob" },
    ];
    const fetchFn = mockFetch(200, rows);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc<{ id: string; name: string }[]>("search_users", { query: "a" });
    expect(result).toEqual(rows);
    expect(result).toHaveLength(2);
  });

  it("sends auth header when authenticated", async () => {
    const fetchFn = mockFetch(200, 1);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("my-jwt", "my-refresh");
    await client.rpc("my_func", { id: "1" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers["Authorization"]).toBe("Bearer my-jwt");
    expect(call[1].headers["Content-Type"]).toBe("application/json");
  });

  it("throws AYBError on function not found", async () => {
    const fetchFn = mockFetch(404, { message: "function not found: nope" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.rpc("nope")).rejects.toThrow(AYBError);
    await expect(client.rpc("nope")).rejects.toThrow("function not found: nope");
  });

  it("sends notify headers when options.notify is present", async () => {
    const fetchFn = mockFetch(200, 1);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.rpc("upsert_post", { id: "1" }, {
      notify: { table: "posts", action: "update" },
    });

    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers["X-Notify-Table"]).toBe("posts");
    expect(call[1].headers["X-Notify-Action"]).toBe("update");
  });

  it("sends no notify headers when notify option is absent", async () => {
    const fetchFn = mockFetch(200, 1);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.rpc("upsert_post", { id: "1" });

    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers["X-Notify-Table"]).toBeUndefined();
    expect(call[1].headers["X-Notify-Action"]).toBeUndefined();
  });

  it("supports notify option with args present", async () => {
    const fetchFn = mockFetch(200, { ok: true });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.rpc("create_post", { title: "hello" }, {
      notify: { table: "posts", action: "create" },
    });

    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers["Content-Type"]).toBe("application/json");
    expect(call[1].headers["X-Notify-Table"]).toBe("posts");
    expect(call[1].headers["X-Notify-Action"]).toBe("create");
    expect(JSON.parse(call[1].body as string)).toEqual({ title: "hello" });
  });

  it("supports notify option without args", async () => {
    const fetchFn = mockFetch(200, "ok");
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await client.rpc("cleanup", undefined, {
      notify: { table: "jobs", action: "delete" },
    });

    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].body).toBeUndefined();
    expect(call[1].headers["X-Notify-Table"]).toBe("jobs");
    expect(call[1].headers["X-Notify-Action"]).toBe("delete");
  });

  it("keeps 2-arg rpc call signature and types third arg as RpcOptions", () => {
    const fetchFn = mockFetch(200, 1);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    void client.rpc("my_func", { id: "1" });

    type RpcThirdArg = Parameters<AYBClient["rpc"]>[2];
    expectTypeOf<RpcThirdArg>().toEqualTypeOf<RpcOptions | undefined>();
    expectTypeOf<RpcOptions["notify"]>().toEqualTypeOf<RpcNotifyOption | undefined>();
  });
});

describe("realtime", () => {
  beforeEach(() => {
    MockEventSource.instances = [];
    globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
  });

  afterEach(() => {
    globalThis.EventSource = OriginalEventSource;
  });

  it("subscribe creates EventSource with correct URL for single table", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts"], () => {});
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe(
      "http://localhost:8090/api/realtime?tables=posts",
    );
  });

  it("subscribe creates EventSource with comma-separated tables", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts", "comments", "users"], () => {});
    expect(MockEventSource.instances).toHaveLength(1);
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("tables=posts%2Ccomments%2Cusers");
  });

  it("subscribe includes auth token in URL when set", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("my-jwt-token", "refresh");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("token=my-jwt-token");
    expect(url).toContain("tables=posts");
  });

  it("subscribe omits token param when no auth", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).not.toContain("token=");
  });

  it("dispatches parsed JSON events to callback", () => {
    expect.assertions(3);
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1", title: "Hello" } });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "create",
      table: "posts",
      record: { id: "1", title: "Hello" },
    });
    expect(callback.mock.calls[0][0].action).toBe("create");
  });

  it("dispatches multiple events sequentially", () => {
    const client = new AYBClient("http://localhost:8090");
    const events: unknown[] = [];
    client.realtime.subscribe(["posts"], (e) => events.push(e));

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    es._sendJSON({ action: "update", table: "posts", record: { id: "1", title: "Updated" } });
    es._sendJSON({ action: "delete", table: "posts", record: { id: "1" } });

    expect(events).toHaveLength(3);
    expect((events[0] as { action: string }).action).toBe("create");
    expect((events[1] as { action: string }).action).toBe("update");
    expect((events[2] as { action: string }).action).toBe("delete");
  });

  it("ignores non-JSON messages (heartbeats)", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    // Send a non-JSON heartbeat message
    es._sendMessage("ping");
    es._sendMessage("");
    es._sendMessage(":");

    expect(callback).not.toHaveBeenCalled();
  });

  it("ignores malformed JSON gracefully", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendMessage("{invalid json}");
    // Valid event should still work after malformed one
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "create",
      table: "posts",
      record: { id: "1" },
    });
  });

  it("unsubscribe closes EventSource", () => {
    const client = new AYBClient("http://localhost:8090");
    const unsub = client.realtime.subscribe(["posts"], () => {});

    const es = MockEventSource.instances[0];
    expect(es.closed).toBe(false);

    unsub();
    expect(es.closed).toBe(true);
  });

  it("no callbacks after unsubscribe", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const unsub = client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    expect(callback).toHaveBeenCalledTimes(1);

    unsub();
    // onmessage is still assigned but EventSource is closed — in real impl
    // the browser wouldn't deliver more events. We verify close was called.
    expect(es.closed).toBe(true);
  });

  it("multiple independent subscriptions", () => {
    const client = new AYBClient("http://localhost:8090");
    const cb1 = vi.fn();
    const cb2 = vi.fn();
    const unsub1 = client.realtime.subscribe(["posts"], cb1);
    client.realtime.subscribe(["comments"], cb2);

    expect(MockEventSource.instances).toHaveLength(2);
    expect(MockEventSource.instances[0].url).toContain("tables=posts");
    expect(MockEventSource.instances[1].url).toContain("tables=comments");

    // Events to first subscription
    MockEventSource.instances[0]._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    expect(cb1).toHaveBeenCalledTimes(1);
    expect(cb2).not.toHaveBeenCalled();

    // Unsubscribe first doesn't affect second
    unsub1();
    expect(MockEventSource.instances[0].closed).toBe(true);
    expect(MockEventSource.instances[1].closed).toBe(false);
  });

  it("subscribe with API key includes token in URL", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("token=ayb_abc123def456abc123def456abc123def456abc123def456");
  });

  it("subscribe returns a function", () => {
    const client = new AYBClient("http://localhost:8090");
    const unsub = client.realtime.subscribe(["posts"], () => {});
    expect(typeof unsub).toBe("function");
  });
});

// --- MockWebSocket for WS realtime tests ---

type WSMessageHandler = ((e: { data: string }) => void) | null;

class MockWebSocket {
  static instances: MockWebSocket[] = [];

  url: string;
  onopen: (() => void) | null = null;
  onmessage: WSMessageHandler = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 0; // CONNECTING
  sent: string[] = [];
  closed = false;

  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.closed = true;
    this.readyState = MockWebSocket.CLOSED;
    if (this.onclose) this.onclose();
  }

  _open() {
    this.readyState = MockWebSocket.OPEN;
    if (this.onopen) this.onopen();
  }

  _receive(data: string) {
    if (this.onmessage) this.onmessage({ data });
  }

  _receiveJSON(obj: unknown) {
    this._receive(JSON.stringify(obj));
  }

  _sentJSON(): unknown[] {
    return this.sent.map((s) => JSON.parse(s));
  }
}

const OriginalWebSocket = globalThis.WebSocket;

describe("realtime WS", () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
  });

  afterEach(() => {
    globalThis.WebSocket = OriginalWebSocket;
  });

  function lastWS(): MockWebSocket {
    return MockWebSocket.instances[MockWebSocket.instances.length - 1];
  }

  // --- Connection and URL derivation ---

  it("subscribeWS derives ws:// URL from http:// baseURL", async () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribeWS(["posts"], () => {});
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(lastWS().url).toBe("ws://localhost:8090/api/realtime/ws");
  });

  it("subscribeWS derives wss:// URL from https:// baseURL", async () => {
    const client = new AYBClient("https://example.com");
    client.realtime.subscribeWS(["posts"], () => {});
    expect(lastWS().url).toBe("wss://example.com/api/realtime/ws");
  });

  // --- Auth + Subscribe handshake ---

  it("waits for connected then sends auth when token is set", async () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("test-jwt", "refresh");
    const promise = client.realtime.subscribeWS(["posts"], () => {});
    const ws = lastWS();

    ws._open();
    // Server sends connected
    ws._receiveJSON({ type: "connected", client_id: "ws-1" });
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1);
    });

    const authMsg = ws._sentJSON()[0] as Record<string, unknown>;
    expect(authMsg.type).toBe("auth");
    expect(authMsg.token).toBe("test-jwt");
    expect(authMsg.ref).toBeDefined();

    // Server replies ok to auth
    ws._receiveJSON({ type: "reply", ref: authMsg.ref, status: "ok" });
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(2);
    });

    const subMsg = ws._sentJSON()[1] as Record<string, unknown>;
    expect(subMsg.type).toBe("subscribe");
    expect(subMsg.tables).toEqual(["posts"]);
    expect(subMsg.ref).toBeDefined();

    // Server replies ok to subscribe
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    const unsub = await promise;
    expect(typeof unsub).toBe("function");
  });

  it("sends subscribe without auth when no token is set", async () => {
    const client = new AYBClient("http://localhost:8090");
    const promise = client.realtime.subscribeWS(["tasks"], () => {});
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-2" });
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1);
    });

    // First message should be subscribe (no auth)
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    expect(subMsg.type).toBe("subscribe");
    expect(subMsg.tables).toEqual(["tasks"]);

    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;
  });

  it("starts handshake from onopen without waiting for connected frame", async () => {
    const client = new AYBClient("http://localhost:8090");
    const promise = client.realtime.subscribeWS(["tasks"], () => {});
    const ws = lastWS();

    ws._open();
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1);
    });

    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    expect(subMsg.type).toBe("subscribe");
    expect(subMsg.tables).toEqual(["tasks"]);

    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;
  });

  it("rejects when the socket closes before subscribe ack arrives", async () => {
    const client = new AYBClient("http://localhost:8090");
    const promise = client.realtime.subscribeWS(["tasks"], () => {});
    const ws = lastWS();

    ws._open();
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1);
    });

    ws.close();

    await expect(promise).rejects.toThrow("WebSocket closed before subscription was ready");
  });

  it("does not send subscribe before auth reply arrives", async () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("tok", "ref");
    client.realtime.subscribeWS(["t"], () => {});
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-3" });
    await vi.waitFor(() => {
      expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1);
    });

    // Only auth sent so far — subscribe must not be sent yet
    expect(ws._sentJSON()).toHaveLength(1);
    expect((ws._sentJSON()[0] as Record<string, unknown>).type).toBe("auth");
  });

  // --- Event delivery ---

  it("forwards normalized event messages to callback", async () => {
    const client = new AYBClient("http://localhost:8090");
    const events: unknown[] = [];
    const promise = client.realtime.subscribeWS(["posts"], (e) => events.push(e));
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-4" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;

    ws._receiveJSON({
      type: "event",
      action: "INSERT",
      table: "posts",
      record: { id: 1, title: "hello" },
    });

    expect(events).toHaveLength(1);
    expect(events[0]).toEqual({
      action: "INSERT",
      table: "posts",
      record: { id: 1, title: "hello" },
    });
  });

  it("does not invoke callback for non-event frames", async () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const promise = client.realtime.subscribeWS(["posts"], callback);
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-5" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;

    // Non-event frames
    ws._receiveJSON({ type: "reply", ref: "x", status: "ok" });
    ws._receiveJSON({ type: "system", message: "hello" });
    ws._receive("not json at all");

    expect(callback).not.toHaveBeenCalled();
  });

  it("delivers event frames that arrive before subscribe ack", async () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const promise = client.realtime.subscribeWS(["posts"], callback);
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-5b" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;

    ws._receiveJSON({ type: "event", action: "INSERT", table: "posts", record: { id: 7 } });
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "INSERT",
      table: "posts",
      record: { id: 7 },
    });
  });

  it("delivers WS realtime frames that omit type and carry action/table/record", async () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const promise = client.realtime.subscribeWS(["posts"], callback);
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-5c" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    await promise;

    ws._receiveJSON({ action: "create", table: "posts", record: { id: 9 } });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "create",
      table: "posts",
      record: { id: 9 },
    });
  });

  // --- Unsubscribe ---

  it("unsubscribe sends unsubscribe frame and closes socket", async () => {
    const client = new AYBClient("http://localhost:8090");
    const promise = client.realtime.subscribeWS(["posts", "users"], () => {});
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-6" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    const unsub = await promise;

    const sentBefore = ws.sent.length;
    unsub();

    const unsubMsg = ws._sentJSON()[sentBefore] as Record<string, unknown>;
    expect(unsubMsg.type).toBe("unsubscribe");
    expect(unsubMsg.tables).toEqual(["posts", "users"]);
    expect(ws.closed).toBe(true);
  });

  it("no callbacks after unsubscribe", async () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const promise = client.realtime.subscribeWS(["posts"], callback);
    const ws = lastWS();

    ws._open();
    ws._receiveJSON({ type: "connected", client_id: "ws-7" });
    await vi.waitFor(() => expect(ws._sentJSON().length).toBeGreaterThanOrEqual(1));
    const subMsg = ws._sentJSON()[0] as Record<string, unknown>;
    ws._receiveJSON({ type: "reply", ref: subMsg.ref, status: "ok" });
    const unsub = await promise;

    unsub();
    ws._receiveJSON({ type: "event", action: "create", table: "posts", record: { id: 1 } });

    expect(callback).not.toHaveBeenCalled();
  });
});
