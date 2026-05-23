import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { confirmMagicLinkServer, loadServerSession } from "../src";
import type { SSRClientLike } from "../src";
import { AYBClient } from "../../sdk/src/client";
import { mockFetchSequence } from "../../sdk/src/test_utils/mockFetchSequence";

const magicLinkConfirmFixture = JSON.parse(
  readFileSync(
    resolve(__dirname, "../../tests/contract/fixtures/sdk_contract/magic_link_confirm_success_response.json"),
    "utf8",
  ),
) as {
  token: string;
  refreshToken: string;
  user: Record<string, unknown>;
};

const magicLinkConfirmPendingFixture = JSON.parse(
  readFileSync(
    resolve(__dirname, "../../tests/contract/fixtures/sdk_contract/magic_link_confirm_pending_mfa_response.json"),
    "utf8",
  ),
) as {
  mfa_pending: boolean;
  mfa_token: string;
};

describe("ssr contract parity", () => {
  it("loadServerSession consumes canonical auth response via core SDK refresh", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 401,
        body: { message: "unauthorized" },
      },
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

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const ssrClient: SSRClientLike = {
      setTokens: (token, refreshToken) => core.setTokens(token, refreshToken),
      clearTokens: () => core.clearTokens(),
      auth: {
        me: async () => core.auth.me() as Promise<Record<string, unknown>>,
        refresh: async () => core.auth.refresh() as Promise<{
          token: string;
          refreshToken: string;
          user?: Record<string, unknown>;
        }>,
        confirmMagicLink: async (token: string) => core.auth.confirmMagicLink(token),
      },
    };

    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client: ssrClient,
    });

    expect(result.session?.token).toBe("jwt_stage3");
    expect(result.session?.refreshToken).toBe("refresh_stage3");
    expect(result.session?.user.id).toBe("usr_1");
    expect(result.session?.user.email).toBe("dev@allyourbase.io");
    expect(result.session?.user.emailVerified).toBe(true);
    expect(result.session?.user.createdAt).toBe("2026-01-01T00:00:00Z");
    expect(result.session?.user.updatedAt).toBeUndefined();
  });

  it("confirmMagicLinkServer consumes canonical success fixture shape via core SDK parser", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: magicLinkConfirmFixture,
      },
    ]);
    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const ssrClient: SSRClientLike = {
      setTokens: (token, refreshToken) => core.setTokens(token, refreshToken),
      clearTokens: () => core.clearTokens(),
      auth: {
        me: async () => core.auth.me() as Promise<Record<string, unknown>>,
        refresh: async () =>
          core.auth.refresh() as Promise<{ token: string; refreshToken: string; user?: Record<string, unknown> }>,
        confirmMagicLink: async (token: string) => core.auth.confirmMagicLink(token),
      },
    };

    const result = await confirmMagicLinkServer({
      token: "magic-token",
      client: ssrClient,
    });

    expect(result.session?.token).toBe(magicLinkConfirmFixture.token);
    expect(result.session?.refreshToken).toBe(magicLinkConfirmFixture.refreshToken);
    expect(result.session?.user.email).toBe("magic@allyourbase.io");
    expect(result.session?.user.emailVerified).toBe(true);
    expect(result.setCookieHeaders).toHaveLength(2);
  });

  it("confirmMagicLinkServer preserves canonical pending-MFA shape via core SDK parser", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: magicLinkConfirmPendingFixture,
      },
    ]);
    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const ssrClient: SSRClientLike = {
      setTokens: (token, refreshToken) => core.setTokens(token, refreshToken),
      clearTokens: () => core.clearTokens(),
      auth: {
        me: async () => core.auth.me() as Promise<Record<string, unknown>>,
        refresh: async () =>
          core.auth.refresh() as Promise<{ token: string; refreshToken: string; user?: Record<string, unknown> }>,
        confirmMagicLink: async (token: string) => core.auth.confirmMagicLink(token),
      },
    };

    const result = await confirmMagicLinkServer({
      token: "magic-token",
      client: ssrClient,
    });

    expect(result.session).toBeNull();
    expect(result.setCookieHeaders).toHaveLength(2);
    expect(result.setCookieHeaders[0]).toContain("Max-Age=0");
    expect(result.mfaPending).toBe(true);
    expect(result.mfaToken).toBe(magicLinkConfirmPendingFixture.mfa_token);
  });
});
