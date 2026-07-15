import { beforeEach, describe, expect, it, vi } from "vitest";
import * as apiClient from "../api_client";

declare const jsdom: {
  reconfigure(options: { url: string }): void;
};

const ADMIN_TOKEN = "admin-origin-token";
const AUTH_TOKEN = "auth-origin-token";
const PAGE_URL = "https://console.example.test/admin/dashboard";
const CROSS_ORIGIN_ERROR = "Cross-origin API requests are not allowed";

type ApiCaller = "admin" | "auth";
type ConfigureConsoleApiOrigin = (origin: unknown) => void;
type ResetConsoleApiOrigin = () => void;
type RuntimeConfigurableApiClient = typeof apiClient & {
  configureConsoleApiOrigin?: ConfigureConsoleApiOrigin;
  resetConsoleApiOrigin?: ResetConsoleApiOrigin;
};

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function setPageURL(url: string): void {
  jsdom.reconfigure({ url });
}

async function requestVia(caller: ApiCaller, path: string): Promise<unknown> {
  if (caller === "admin") {
    return apiClient.fetchAdmin(path);
  }
  return apiClient.requestAuth<unknown>(path);
}

function bearerFor(caller: ApiCaller): string {
  return caller === "admin" ? `Bearer ${ADMIN_TOKEN}` : `Bearer ${AUTH_TOKEN}`;
}

function runtimeConfigurableApiClient(): RuntimeConfigurableApiClient {
  return apiClient as RuntimeConfigurableApiClient;
}

describe("console API client origin routing", () => {
  const fetchMock = vi.fn<typeof fetch>();

  function resetConfiguredOriginForTest(): void {
    runtimeConfigurableApiClient().resetConsoleApiOrigin?.();
  }

  function getConfigureConsoleApiOrigin(): ConfigureConsoleApiOrigin {
    const configure = runtimeConfigurableApiClient().configureConsoleApiOrigin;
    expect(
      configure,
      "Stage 2 must export configureConsoleApiOrigin from ui/src/api_client.ts",
    ).toBeTypeOf("function");
    if (typeof configure !== "function") {
      throw new Error("configureConsoleApiOrigin is not exported");
    }
    return configure;
  }

  function getResetConsoleApiOrigin(): ResetConsoleApiOrigin {
    const reset = runtimeConfigurableApiClient().resetConsoleApiOrigin;
    expect(
      reset,
      "Stage 2 must export resetConsoleApiOrigin from ui/src/api_client.ts",
    ).toBeTypeOf("function");
    if (typeof reset !== "function") {
      throw new Error("resetConsoleApiOrigin is not exported");
    }
    return reset;
  }

  function configureConsoleApiOrigin(origin: unknown): void {
    getConfigureConsoleApiOrigin()(origin);
  }

  function authorizationHeadersSentToFetch(): string[] {
    return fetchMock.mock.calls.flatMap(([, init]) => {
      if (!init?.headers || init.headers instanceof Headers || Array.isArray(init.headers)) {
        return [];
      }
      const authorization = (init.headers as Record<string, string>).Authorization;
      return authorization ? [authorization] : [];
    });
  }

  async function expectRejectedBeforeFetch(caller: ApiCaller, path: string): Promise<void> {
    let caughtError: unknown;
    try {
      await requestVia(caller, path);
    } catch (error) {
      caughtError = error;
    }

    expect.soft(caughtError).toBeInstanceOf(Error);
    expect.soft((caughtError as Error | undefined)?.message).toBe(CROSS_ORIGIN_ERROR);
    expect.soft(fetchMock).not.toHaveBeenCalled();
    expect.soft(authorizationHeadersSentToFetch()).toEqual([]);
  }

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", ADMIN_TOKEN);
    localStorage.setItem("ayb_auth_token", AUTH_TOKEN);
    setPageURL(PAGE_URL);
    resetConfiguredOriginForTest();
    vi.stubGlobal("fetch", fetchMock);
  });

  it.each<ApiCaller>(["admin", "auth"])(
    "normalizes same-origin %s requests before attaching the bearer token",
    async (caller) => {
      fetchMock.mockImplementation(() => Promise.resolve(jsonResponse({ ok: true })));

      await requestVia(caller, "https://console.example.test/api/projects?limit=1#frag");
      await requestVia(caller, "//console.example.test/api/projects?limit=2#frag");
      await requestVia(caller, "/api/projects?limit=3#frag");

      expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/projects?limit=1#frag", {
        headers: { Authorization: bearerFor(caller) },
      });
      expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/projects?limit=2#frag", {
        headers: { Authorization: bearerFor(caller) },
      });
      expect(fetchMock).toHaveBeenNthCalledWith(3, "/api/projects?limit=3#frag", {
        headers: { Authorization: bearerFor(caller) },
      });
    },
  );

  it.each([
    ["admin", "https://attacker.example/collect"],
    ["admin", "//attacker.example/collect"],
    ["auth", "https://attacker.example/collect"],
    ["auth", "//attacker.example/collect"],
  ] as const)(
    "rejects ordinary foreign %s request %s before any bearer token is attached",
    async (caller, path) => {
      await expect(requestVia(caller, path)).rejects.toThrow(CROSS_ORIGIN_ERROR);

      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([
    [String.raw`/\evil.com/x`, "https://evil.com"],
    [String.raw`\/evil.com/x`, "https://evil.com"],
    [String.raw`\\evil.com/x`, "https://evil.com"],
  ])("rejects backslash-confused path %s as resolved foreign origin %s", async (path, resolvedOrigin) => {
    expect(new URL(path, window.location.href).origin).toBe(resolvedOrigin);

    await expectRejectedBeforeFetch("admin", path);
    vi.clearAllMocks();
    await expectRejectedBeforeFetch("auth", path);
  });

  it.each(["s3://bucket/x", "h2://example/x"])(
    "rejects non-page-origin scheme %s before credentials can reach fetch",
    async (path) => {
      expect(new URL(path, window.location.href).origin).not.toBe(window.location.origin);

      await expectRejectedBeforeFetch("admin", path);
      vi.clearAllMocks();
      await expectRejectedBeforeFetch("auth", path);
    },
  );

  it("exports configureConsoleApiOrigin as the runtime allowlist seam", () => {
    expect(
      runtimeConfigurableApiClient().configureConsoleApiOrigin,
      "Stage 2 must add the runtime configuration export on ui/src/api_client.ts",
    ).toBeTypeOf("function");
  });

  it.each<ApiCaller>(["admin", "auth"])(
    "permits an explicitly configured HTTPS remote origin for %s requests",
    async (caller) => {
      configureConsoleApiOrigin("https://api.example.test");
      fetchMock.mockImplementation(() => Promise.resolve(jsonResponse({ ok: true })));

      await requestVia(caller, "https://api.example.test/api/projects?limit=1#frag");

      expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/projects?limit=1#frag", {
        headers: { Authorization: bearerFor(caller) },
      });
    },
  );

  it.each([
    "https://api.example.test.evil.example/api/projects",
    "https://api.example.test@attacker.example/api/projects",
    "https://attacker.example/api/projects#https://api.example.test",
    "https://api.example.test:8443/api/projects",
  ])("rejects configured-origin lookalike %s before credentials can reach fetch", async (path) => {
    configureConsoleApiOrigin("https://api.example.test");

    await expectRejectedBeforeFetch("admin", path);
    vi.clearAllMocks();
    await expectRejectedBeforeFetch("auth", path);
  });

  it.each([
    "*",
    "",
    null,
    "api.example.test",
    "*.example.test",
    "https://api.example.test/path",
    "https://api.example.test?token=1",
    "https://api.example.test#frag",
    "s3://bucket",
    "h2://example",
    "https://[::1",
  ])("refuses invalid configured remote origin %s loudly", (origin) => {
    const configure = getConfigureConsoleApiOrigin();

    expect(() => configure(origin)).toThrow();
  });

  it("resets configured state back to same-origin-only routing", async () => {
    configureConsoleApiOrigin("https://api.example.test");
    getResetConsoleApiOrigin()();

    await expectRejectedBeforeFetch("admin", "https://api.example.test/api/projects");
    vi.clearAllMocks();
    await expectRejectedBeforeFetch("auth", "https://api.example.test/api/projects");
  });

  it("refuses an HTTP remote origin from an HTTPS page before any request can run", async () => {
    setPageURL("https://console.example.test/admin/dashboard");

    expect(() => configureConsoleApiOrigin("http://api.example.test")).toThrow();
    await expectRejectedBeforeFetch("admin", "http://api.example.test/api/projects");
  });

  it("permits an HTTP remote origin from an HTTP page for local consoles", async () => {
    setPageURL("http://console.example.test/admin/dashboard");
    configureConsoleApiOrigin("http://api.example.test");
    fetchMock.mockResolvedValue(jsonResponse({ ok: true }));

    await apiClient.fetchAdmin("http://api.example.test/api/projects");

    expect(fetchMock).toHaveBeenCalledWith("http://api.example.test/api/projects", {
      headers: { Authorization: `Bearer ${ADMIN_TOKEN}` },
    });
  });
});
