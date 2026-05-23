import React from "react";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider, useAuth } from "../src";
import type { AYBClientLike, AuthStateListener } from "../src/types";

const magicLinkRequestFixture = JSON.parse(
  readFileSync(
    resolve(__dirname, "../../tests/contract/fixtures/sdk_contract/magic_link_request_response.json"),
    "utf8",
  ),
) as {
  message: string;
};

const magicLinkConfirmSuccessFixture = JSON.parse(
  readFileSync(
    resolve(__dirname, "../../tests/contract/fixtures/sdk_contract/magic_link_confirm_success_response.json"),
    "utf8",
  ),
) as {
  token: string;
  refreshToken: string;
  user: {
    email?: string;
  };
};

function createAuthClient() {
  let listener: AuthStateListener | null = null;
  const unsub = vi.fn();
  const waitForSessionRestore = vi.fn(async () => {});
  const clearTokens = vi.fn(() => {
    client.token = null;
    client.refreshToken = null;
  });

  const client: AYBClientLike = {
    token: "t1",
    refreshToken: "r1",
    waitForSessionRestore,
    clearTokens,
    onAuthStateChange: (cb) => {
      listener = cb;
      return unsub;
    },
    auth: {
      login: vi.fn(async () => ({ token: "t2", refreshToken: "r2", user: { id: "u1", email: "u@example.com" } })),
      register: vi.fn(async () => ({ token: "t2", refreshToken: "r2", user: { id: "u1", email: "u@example.com" } })),
      signInAnonymously: vi.fn(async () => ({ token: "t3", refreshToken: "r3", user: { id: "guest", isAnonymous: true } })),
        requestMagicLink: vi.fn(async () => magicLinkRequestFixture),
        confirmMagicLink: vi.fn(async () => magicLinkConfirmSuccessFixture),
      linkEmail: vi.fn(async () => ({ token: "t4", refreshToken: "r4", user: { id: "u2", email: "next@example.com" } })),
      signInWithOAuth: vi.fn(async () => ({ token: "t5", refreshToken: "r5", user: { id: "u3", email: "oauth@example.com" } })),
      logout: vi.fn(async () => {}),
      refresh: vi.fn(async () => ({ token: "t6", refreshToken: "r6", user: { id: "u4" } })),
      me: vi.fn(async () => ({ id: "u1", email: "u@example.com" })),
    },
    records: {
      list: vi.fn(async () => ({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 })),
    },
    realtime: { subscribe: vi.fn(() => () => {}) },
  };

    return {
      client,
      emit: (event: Parameters<AuthStateListener>[0], session: Parameters<AuthStateListener>[1]) => listener?.(event, session),
      clearTokens,
      unsub,
      waitForSessionRestore,
    };
  }

describe("useAuth", () => {
  it("loads current user, reacts to auth events, unsubscribes on unmount", async () => {
    const { client, emit, unsub, waitForSessionRestore } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { result, unmount } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.user?.id).toBe("u1");
    });
    expect(waitForSessionRestore).toHaveBeenCalledTimes(1);

    await act(async () => {
      emit("SIGNED_OUT", null);
    });

    await waitFor(() => {
      expect(result.current.user).toBeNull();
    });

    unmount();
    expect(unsub).toHaveBeenCalledTimes(1);
  });

  it("waits for session restore before loading current user", async () => {
    let resolveRestore: (() => void) | null = null;
    const { client } = createAuthClient();
    client.waitForSessionRestore = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveRestore = resolve;
        }),
    );
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    expect(result.current.loading).toBe(true);
    expect(client.auth.me).not.toHaveBeenCalled();
    if (!resolveRestore) {
      throw new Error("session restore resolver was not captured");
    }

    await act(async () => {
      resolveRestore();
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(client.auth.me).toHaveBeenCalledTimes(1);
      expect(result.current.loading).toBe(false);
    });
  });

  it("clears unauthorized restored sessions instead of keeping stale auth state", async () => {
    const { client, clearTokens } = createAuthClient();
    client.auth.me = vi.fn(async () => {
      throw Object.assign(new Error("unauthorized"), { status: 401 });
    });
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
      expect(result.current.refreshToken).toBeNull();
    });
    expect(clearTokens).toHaveBeenCalledTimes(1);
  });

  it("delegates requestMagicLink without forcing a user reload", async () => {
    const { client } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.requestMagicLink("fixture@example.com");
    });

    expect(client.auth.requestMagicLink).toHaveBeenCalledWith("fixture@example.com");
    expect(client.auth.me).toHaveBeenCalledTimes(1);
  });

  it("delegates confirmMagicLink and reloads the current user", async () => {
    const { client } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.confirmMagicLink("magic-link-token");
    });

    expect(client.auth.confirmMagicLink).toHaveBeenCalledWith("magic-link-token");
    expect(client.auth.me).toHaveBeenCalledTimes(2);
  });

  it("delegates stage1 auth methods through client.auth", async () => {
    const { client } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.signInAnonymously();
      await result.current.linkEmail("next@example.com", "password123");
      await result.current.signInWithOAuth("google");
    });

    expect(client.auth.signInAnonymously).toHaveBeenCalledTimes(1);
    expect(client.auth.linkEmail).toHaveBeenCalledWith("next@example.com", "password123");
    expect(client.auth.signInWithOAuth).toHaveBeenCalledWith("google", undefined);
  });

  // MAY22-OAUTH-RETURN-TO follow-up: confirms `useAuth().signInWithOAuth`
  // forwards the new `redirectTo` option through to the underlying client
  // unchanged. React is purely passthrough — the server is the security
  // owner (`internal/auth/handler_oauth.go` host-allowlist validation), and
  // the JS SDK builds the actual URL. This test guards against React's
  // wrapper accidentally dropping options.
  it("forwards redirectTo through useAuth.signInWithOAuth to the underlying client", async () => {
    const { client } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));

    const options = { redirectTo: "https://app.example.com/post-oauth" };
    await act(async () => {
      await result.current.signInWithOAuth("github", options);
    });

    // Asserts the exact options object reached the client mock — covers both
    // that React keeps the option set AND that it doesn't overwrite or
    // synthesize a different one. `toHaveBeenCalledWith` does a structural
    // match, so the redirectTo value is checked, not just presence.
    expect(client.auth.signInWithOAuth).toHaveBeenCalledWith("github", options);
  });
});
