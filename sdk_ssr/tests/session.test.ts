import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { confirmMagicLinkServer, loadServerSession } from "../src";
import type { SSRClientLike } from "../src";

const magicLinkConfirmFixture = JSON.parse(
  readFileSync(
    resolve(__dirname, "../../tests/contract/fixtures/sdk_contract/magic_link_confirm_success_response.json"),
    "utf8",
  ),
) as {
  token: string;
  refreshToken: string;
  user: {
    id: string;
    email: string;
  };
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

function makeClient() {
  const client: SSRClientLike = {
    setTokens: vi.fn(),
    clearTokens: vi.fn(),
    auth: {
      me: vi.fn(async () => {
        throw Object.assign(new Error("unauthorized"), { status: 401 });
      }),
      refresh: vi.fn(async () => ({
        token: "new-token",
        refreshToken: "new-refresh",
        user: { id: "u1", email: "u@example.com" },
      })),
        confirmMagicLink: vi.fn(async () => magicLinkConfirmFixture),
    },
  };
  return client;
}

describe("loadServerSession", () => {
  it("refreshes expired session and rotates cookies", async () => {
    const client = makeClient();
    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client,
    });

    expect(client.auth.refresh).toHaveBeenCalledTimes(1);
    expect(result.session?.token).toBe("new-token");
    expect(result.setCookieHeaders.length).toBe(2);
  });

  it("clears cookies when refresh fails", async () => {
    const client = makeClient();
    (client.auth.refresh as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("refresh failed"),
    );

    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client,
    });

    expect(result.session).toBeNull();
    expect(result.setCookieHeaders.length).toBe(2);
    expect(result.setCookieHeaders[0]).toContain("Max-Age=0");
  });

  it("confirms a magic link and returns session cookies for SSR landing pages", async () => {
    const client = makeClient();

      const result = await confirmMagicLinkServer({
        token: "magic-link-token",
        client,
      });

      expect(client.auth.confirmMagicLink).toHaveBeenCalledWith("magic-link-token");
      expect(client.setTokens).toHaveBeenCalledWith(
        magicLinkConfirmFixture.token,
        magicLinkConfirmFixture.refreshToken,
      );
      expect(result.session).toEqual({
        token: magicLinkConfirmFixture.token,
        refreshToken: magicLinkConfirmFixture.refreshToken,
        user: magicLinkConfirmFixture.user,
      });
      expect(result.setCookieHeaders).toHaveLength(2);
    });

    it("rejects malformed confirm responses that do not include signed-in tokens", async () => {
      const client = makeClient();
      (client.auth.confirmMagicLink as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        token: "",
        refreshToken: "",
        user: { id: "usr_bad", email: "bad@example.com" },
      });

      const result = await confirmMagicLinkServer({
        token: "magic-link-token",
        client,
      });

      expect(client.auth.confirmMagicLink).toHaveBeenCalledWith("magic-link-token");
      expect(client.setTokens).not.toHaveBeenCalled();
      expect(client.clearTokens).toHaveBeenCalledTimes(1);
      expect(result.session).toBeNull();
      expect(result.setCookieHeaders).toHaveLength(2);
      expect(result.setCookieHeaders[0]).toContain("Max-Age=0");
      expect(result.mfaPending).toBeUndefined();
      expect(result.mfaToken).toBeUndefined();
    });

    it("returns canonical pending-MFA confirm payload while clearing stale session cookies", async () => {
      const client = makeClient();
      (client.auth.confirmMagicLink as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
        magicLinkConfirmPendingFixture,
      );

      const result = await confirmMagicLinkServer({
        token: "magic-link-token",
        client,
      });

      expect(client.auth.confirmMagicLink).toHaveBeenCalledWith("magic-link-token");
      expect(client.setTokens).not.toHaveBeenCalled();
      expect(client.clearTokens).toHaveBeenCalledTimes(1);
      expect(result.session).toBeNull();
      expect(result.setCookieHeaders).toHaveLength(2);
      expect(result.setCookieHeaders[0]).toContain("Max-Age=0");
      expect(result.mfaPending).toBe(true);
      expect(result.mfaToken).toBe(magicLinkConfirmPendingFixture.mfa_token);
    });
  });
