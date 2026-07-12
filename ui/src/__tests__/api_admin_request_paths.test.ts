import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  callRpc,
  checkOAuthAuthorize,
  executeApiExplorer,
  getCollectionSearchSettings,
  getCollectionSearchSynonyms,
  getRealtimeInspectorSnapshot,
  submitOAuthConsent,
  updateCollectionSearchSettings,
  updateCollectionSearchSynonyms,
} from "../api";

describe("admin API request helpers", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("uses the shared admin request path for callRpc while preserving 204 responses", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(callRpc("sync_users", { dryRun: true })).resolves.toEqual({
      status: 204,
      data: null,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/rpc/sync_users", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify({ dryRun: true }),
    });
  });

  it("preserves unauthorized handling for callRpc failures", async () => {
    const unauthorizedListener = vi.fn();
    window.addEventListener("ayb:unauthorized", unauthorizedListener);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ message: "rpc denied" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(callRpc("sync_users")).rejects.toThrow("rpc denied");

    expect(localStorage.getItem("ayb_admin_token")).toBeNull();
    expect(unauthorizedListener).toHaveBeenCalledTimes(1);
    window.removeEventListener("ayb:unauthorized", unauthorizedListener);
  });

  it("uses the shared admin request path for Api Explorer while keeping raw response handling", async () => {
    const nowSpy = vi.spyOn(performance, "now");
    nowSpy.mockReturnValueOnce(100).mockReturnValueOnce(145);
    fetchMock.mockResolvedValueOnce(
      new Response("raw text response", {
        status: 418,
        statusText: "I'm a teapot",
        headers: { "X-Trace": "trace-123" },
      }),
    );

    await expect(
      executeApiExplorer("POST", "/api/admin/sql/", '{"query":"select 1"}'),
    ).resolves.toEqual({
      status: 418,
      statusText: "I'm a teapot",
      headers: expect.objectContaining({
        "content-type": "text/plain;charset=UTF-8",
        "x-trace": "trace-123",
      }),
      body: "raw text response",
      durationMs: 45,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/sql/", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: '{"query":"select 1"}',
    });
    nowSpy.mockRestore();
  });

  it("calls /api/admin/realtime/stats and normalizes the live snapshot", async () => {
    const rawSnapshot = {
      version: "v1",
      timestamp: "2026-03-15T00:00:00Z",
      connections: { sse: 3, ws: 7, total: 10 },
      subscriptions: {
        tables: { public_posts: 4 },
        channels: {
          broadcast: { "room:lobby": 2 },
          presence: { "room:lobby": 1 },
        },
      },
      counters: { dropped_messages: 5, heartbeat_failures: 1 },
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(rawSnapshot), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const result = await getRealtimeInspectorSnapshot();

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/realtime/stats", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(result).toEqual({
      version: "v1",
      timestamp: "2026-03-15T00:00:00Z",
      connections: { sse: 3, ws: 7, total: 10 },
      subscriptions: [
        { name: "public_posts", type: "table", count: 4 },
        { name: "room:lobby", type: "broadcast", count: 2 },
        { name: "room:lobby", type: "presence", count: 1 },
      ],
      counters: { droppedMessages: 5, heartbeatFailures: 1 },
    });
  });

  it("gets collection search synonyms through the shared admin request path", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ groups: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getCollectionSearchSynonyms("posts")).resolves.toEqual({
      groups: [],
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/collections/posts/synonyms", {
      headers: { Authorization: "Bearer admin-token" },
    });
  });

  it("encodes unsafe unqualified table names for collection search synonyms", async () => {
    const table = "draft posts/2026?x=1#frag";
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ groups: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await getCollectionSearchSynonyms(table);

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/collections/${encodeURIComponent(table)}/synonyms`,
      { headers: { Authorization: "Bearer admin-token" } },
    );
  });

  it("updates collection search synonyms with the canonical groups payload", async () => {
    const payload = {
      groups: [{ terms: ["scifi", "science fiction"] }],
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      updateCollectionSearchSynonyms("posts", payload),
    ).resolves.toEqual(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/collections/posts/synonyms", {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify(payload),
    });
  });

  it("gets collection search settings through the shared admin request path", async () => {
    const response = {
      attributes: [
        { column: "title", weight: "high" },
        { column: "summary", weight: "medium" },
        { column: "description", weight: "low" },
        { column: "notes", weight: "lowest" },
      ],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "title", order: "asc" },
      ],
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getCollectionSearchSettings("posts")).resolves.toEqual(response);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/collections/posts/search-settings", {
      headers: { Authorization: "Bearer admin-token" },
    });
  });

  it("normalizes omitted custom ranking arrays from collection search settings responses", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ attributes: [{ column: "title", weight: "high" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getCollectionSearchSettings("posts")).resolves.toEqual({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [],
    });
  });

  it("encodes unsafe unqualified table names for collection search settings", async () => {
    const table = "draft posts/2026?x=1#frag";
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ attributes: [], customRanking: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await getCollectionSearchSettings(table);

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/collections/${encodeURIComponent(table)}/search-settings`,
      { headers: { Authorization: "Bearer admin-token" } },
    );
  });

  it("updates collection search settings with the canonical settings payload", async () => {
    const payload = {
      attributes: [
        { column: "title", weight: "high" },
        { column: "summary", weight: "medium" },
        { column: "description", weight: "low" },
        { column: "notes", weight: "lowest" },
      ],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "title", order: "asc" },
      ],
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      updateCollectionSearchSettings("posts", payload),
    ).resolves.toEqual(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/collections/posts/search-settings", {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify(payload),
    });
  });

  it("requests the OAuth authorize JSON contract instead of following redirects", async () => {
    const prompt = {
      requires_consent: true,
      client_id: "client-1",
      client_name: "Client One",
      redirect_uri: "http://localhost:8090/oauth-callback",
      scope: "read",
      state: "state-1",
      code_challenge: "challenge",
      code_challenge_method: "S256",
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(prompt), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const params = new URLSearchParams({
      response_type: "code",
      client_id: "client-1",
    });
    await expect(checkOAuthAuthorize(params)).resolves.toEqual(prompt);

    expect(fetchMock).toHaveBeenCalledWith("/api/auth/authorize?response_type=code&client_id=client-1", {
      headers: {
        Accept: "application/json",
        Authorization: "Bearer admin-token",
      },
    });
  });

  it("submits OAuth consent with the JSON redirect contract", async () => {
    const result = { redirect_to: "http://localhost:8090/oauth-callback?code=abc" };
    const payload = {
      decision: "approve" as const,
      response_type: "code",
      client_id: "client-1",
      redirect_uri: "http://localhost:8090/oauth-callback",
      scope: "read",
      state: "state-1",
      code_challenge: "challenge",
      code_challenge_method: "S256",
      allowed_tables: ["posts"],
    };
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify(result), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(submitOAuthConsent(payload)).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith("/api/auth/authorize/consent", {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify(payload),
    });
  });
});
