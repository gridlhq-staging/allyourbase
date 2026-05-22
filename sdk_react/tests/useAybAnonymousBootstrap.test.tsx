import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider } from "../src/provider";
import { useAybAnonymousBootstrap } from "../src/useAybAnonymousBootstrap";
import type { AYBClientLike, AuthStateListener } from "../src/types";

function createClient(token: string | null): AYBClientLike {
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
  };
}

describe("useAybAnonymousBootstrap", () => {
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
