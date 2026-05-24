import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider } from "../src/provider";
import { useAybAnonymousBootstrap } from "../src/useAybAnonymousBootstrap";
import type { AYBClientLike, AuthStateListener } from "../src/types";

type TestClient = AYBClientLike & {
  emitAuthStateChange: (...args: Parameters<AuthStateListener>) => void;
};

function createClient(token: string | null): TestClient {
  let listener: AuthStateListener | null = null;
  return {
    token,
    refreshToken: token ? "r1" : null,
    onAuthStateChange: (cb) => {
      listener = cb;
      return () => {
        listener = null;
      };
    },
    auth: {
      login: vi.fn(async () => ({ token: "t", refreshToken: "r", user: { id: "u", email: "e@test" } })),
      register: vi.fn(async () => ({ token: "t", refreshToken: "r", user: { id: "u", email: "e@test" } })),
      signInAnonymously: vi.fn(async () => ({ token: "ta", refreshToken: "ra", user: { id: "guest", isAnonymous: true } })),
      linkEmail: vi.fn(async () => ({ token: "t2", refreshToken: "r2", user: { id: "u2", email: "x@test" } })),
      signInWithOAuth: vi.fn(async () => ({ token: "t3", refreshToken: "r3", user: { id: "u3", email: "y@test" } })),
      logout: vi.fn(async () => {}),
      refresh: vi.fn(async () => ({ token: "t4", refreshToken: "r4", user: { id: "u4" } })),
      me: vi.fn(async () => ({ id: "guest", isAnonymous: true })),
    },
    records: { list: vi.fn(async () => ({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 })) },
    realtime: { subscribe: vi.fn(() => () => {}) },
    emitAuthStateChange: (...args) => {
      listener?.(...args);
    },
  };
}

describe("useAybAnonymousBootstrap", () => {
  it("uses explicit bootstrap overrides without triggering extra auth-me loads", async () => {
    const client = createClient("existing-token");
    const overrideSignIn = vi.fn(async () => {});
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { result } = renderHook(
      () => useAybAnonymousBootstrap({ enabled: true, token: "", signInAnonymously: overrideSignIn }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.bootstrapping).toBe(false);
      expect(overrideSignIn).toHaveBeenCalledTimes(1);
    });
    expect(client.auth.me).not.toHaveBeenCalled();
  });

  it("boots anonymous auth once when client token is empty", async () => {
    const client = createClient(null);
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { result } = renderHook(() => useAybAnonymousBootstrap({ enabled: true }), { wrapper });

    await waitFor(() => {
      expect(result.current.bootstrapping).toBe(false);
      expect(client.auth.signInAnonymously).toHaveBeenCalledTimes(1);
    });
  });

  it("skips anonymous sign-in when disabled", async () => {
    const client = createClient(null);
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    renderHook(() => useAybAnonymousBootstrap({ enabled: false }), { wrapper });
    await act(async () => {
      await Promise.resolve();
    });

    expect(client.auth.signInAnonymously).not.toHaveBeenCalled();
  });

  it("does not bootstrap anonymously after enabled flips true with resolved token", async () => {
    const client = createClient(null);
    const bootstrapOverride = vi.fn(async () => {});
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { rerender } = renderHook(
      ({ enabled, token }: { enabled: boolean; token: string | null }) =>
        useAybAnonymousBootstrap({ enabled, token, signInAnonymously: bootstrapOverride }),
      { initialProps: { enabled: false, token: null }, wrapper },
    );

    rerender({ enabled: true, token: "registered-token" });
    await act(async () => {
      await Promise.resolve();
    });

    expect(bootstrapOverride).not.toHaveBeenCalled();
  });

  it("respects provider auth-state updates before enabling anonymous bootstrap", async () => {
    const client = createClient(null);
    const bootstrapOverride = vi.fn(async () => {});
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => useAybAnonymousBootstrap({ enabled, signInAnonymously: bootstrapOverride }),
      { initialProps: { enabled: false }, wrapper },
    );

    await act(async () => {
      client.token = "registered-token";
      client.refreshToken = "registered-refresh";
      client.emitAuthStateChange(
        ...([
          "signed_in",
          { token: "registered-token", refreshToken: "registered-refresh" },
        ] as unknown as Parameters<AuthStateListener>),
      );
      await Promise.resolve();
    });

    rerender({ enabled: true });
    await act(async () => {
      await Promise.resolve();
    });

    expect(bootstrapOverride).not.toHaveBeenCalled();
  });

  it("handles rejected anonymous sign-in without leaking an unhandled rejection", async () => {
    const client = createClient(null);
    client.auth.signInAnonymously = vi.fn(async () => {
      throw new Error("anonymous disabled");
    });
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={client}>{children}</AYBProvider>;

    const { result } = renderHook(() => useAybAnonymousBootstrap({ enabled: true }), { wrapper });

    await waitFor(() => {
      expect(client.auth.signInAnonymously).toHaveBeenCalledTimes(1);
      expect(result.current.bootstrapping).toBe(false);
    });
  });
});
