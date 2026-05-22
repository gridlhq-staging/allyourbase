import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider, AybLoginBar, DemoSuggestionChip, useAuth, useAybAnonymousBootstrap, useQuery } from "../src";
import type { AYBClientLike } from "../src/types";
import { AYBClient } from "../../sdk/src/client";
import type { AuthResponse, ListResponse } from "../../sdk/src/types";
import { mockFetchSequence } from "../../sdk/src/test_utils/mockFetchSequence";

class FakeEventSource {
  private listeners = new Map<string, ((event: MessageEvent) => void)[]>();

  constructor(_url: string) {
    queueMicrotask(() => {
      this.emit("connected", { clientId: "state_123" });
    });
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    const list = this.listeners.get(type) ?? [];
    list.push(listener);
    this.listeners.set(type, list);
  }

  close() {}

  emit(type: string, data: unknown) {
    const list = this.listeners.get(type) ?? [];
    const event = { data: JSON.stringify(data) } as MessageEvent;
    for (const listener of list) {
      listener(event);
    }
  }
}

describe("react contract parity", () => {
  it("public barrel exports shared movies-demo react surface", () => {
    expect(typeof AYBProvider).toBe("function");
    expect(typeof AybLoginBar).toBe("function");
    expect(typeof useAuth).toBe("function");
    expect(typeof useQuery).toBe("function");
    expect(typeof useAybAnonymousBootstrap).toBe("function");
    expect(typeof DemoSuggestionChip).toBe("function");
  });

  it("useAuth consumes canonical auth fixture shape parsed by core SDK", async () => {
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
      {
        status: 200,
        body: {
          id: "usr_1",
          email: "dev@allyourbase.io",
          emailVerified: true,
          createdAt: "2026-01-01T00:00:00Z",
          updatedAt: null,
        },
      },
    ]);

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const auth: AuthResponse = await core.auth.login("dev@allyourbase.io", "secret");
    expect(auth.user.emailVerified).toBe(true);

    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={core as unknown as AYBClientLike}>{children}</AYBProvider>;

    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.user?.id).toBe("usr_1");
      expect(result.current.user?.email).toBe("dev@allyourbase.io");
    });
  });

  it("useQuery consumes canonical list fixture shape parsed by core SDK", async () => {
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

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const list: ListResponse<Record<string, unknown>> = await core.records.list("posts");
    expect(list.totalItems).toBe(2);

    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={core as unknown as AYBClientLike}>{children}</AYBProvider>;

    const { result } = renderHook(() => useQuery<Record<string, unknown>>("posts"), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.data?.items[0]?.title).toBe("First");
      expect(result.current.data?.items[1]?.title).toBe("Second");
    });
  });

  it("useAuth drives canonical anonymous/link-email/oauth methods with core token persistence", async () => {
    const persistence = { save: vi.fn(), clear: vi.fn() };
    let eventSource: FakeEventSource | null = null;
    vi.stubGlobal(
      "EventSource",
      class {
        constructor(url: string) {
          eventSource = new FakeEventSource(url);
          return eventSource;
        }
      },
    );

    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          token: "anon_token",
          refreshToken: "anon_refresh",
          user: { id: "anon_1", is_anonymous: true },
        },
      },
      {
        status: 200,
        body: {
          id: "anon_1",
          is_anonymous: true,
        },
      },
      {
        status: 200,
        body: {
          token: "link_token",
          refreshToken: "link_refresh",
          user: { id: "usr_2", email: "alice@demo.test", linked_at: "2026-01-02T00:00:00Z" },
        },
      },
      {
        status: 200,
        body: {
          id: "usr_2",
          email: "alice@demo.test",
          linked_at: "2026-01-02T00:00:00Z",
        },
      },
      {
        status: 200,
        body: {
          id: "usr_oauth",
          email: "oauth@demo.test",
        },
      },
    ]);

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn, authPersistence: persistence });
    const wrapper = ({ children }: { children: React.ReactNode }) => <AYBProvider client={core as unknown as AYBClientLike}>{children}</AYBProvider>;
    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.signInAnonymously();
    });
    await waitFor(() => expect(result.current.token).toBe("anon_token"));

    await act(async () => {
      await result.current.linkEmail("alice@demo.test", "password123");
    });
    await waitFor(() => expect(result.current.user?.email).toBe("alice@demo.test"));

    await act(async () => {
      const oauth = result.current.signInWithOAuth("google", {
        urlCallback: async () => {
          setTimeout(() => {
            eventSource?.emit("oauth", {
              token: "oauth_token",
              refreshToken: "oauth_refresh",
              user: { id: "usr_oauth", email: "oauth@demo.test" },
            });
          }, 0);
        },
      });
      await oauth;
    });

    await waitFor(() => expect(result.current.user?.email).toBe("oauth@demo.test"));
    expect(persistence.save).toHaveBeenCalledWith({ token: "anon_token", refreshToken: "anon_refresh" });
    expect(persistence.save).toHaveBeenCalledWith({ token: "link_token", refreshToken: "link_refresh" });
    expect(persistence.save).toHaveBeenCalledWith({ token: "oauth_token", refreshToken: "oauth_refresh" });
  });
});
