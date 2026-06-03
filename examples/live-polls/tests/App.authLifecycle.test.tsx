import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import {
  ayb,
  clearAnonymousBootstrapOptOut,
  clearPersistedTokens,
  disableAnonymousBootstrap,
  isAnonymousBootstrapEnabled,
} from "../src/lib/ayb";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";

vi.mock("../src/components/AuthForm", () => ({
  default: ({ onAuth }: { onAuth: (email: string) => void }) => (
    <button type="button" onClick={() => onAuth("reauthed@test.com")}>
      Auth Form
    </button>
  ),
}));

vi.mock("../src/components/CreatePoll", () => ({
  default: () => null,
}));

vi.mock("../src/components/PollCard", () => ({
  default: () => null,
}));

vi.mock("../src/hooks/useRealtime", () => ({
  useRealtime: vi.fn(),
}));

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    graphql: {
      query: vi.fn().mockResolvedValue({ polls: [] }),
    },
    records: {
      list: vi.fn().mockResolvedValue({ items: [], page: 1, perPage: 100, totalItems: 0, totalPages: 0 }),
    },
  },
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
  hasLivePollsBootstrapSeeded: vi.fn(() => false),
  markLivePollsBootstrapSeeded: vi.fn(),
  clearLivePollsBootstrapSeeded: vi.fn(),
}));

vi.mock("@allyourbase/react", () => ({
  useAuth: vi.fn(),
  useAybAnonymousBootstrap: vi.fn(),
}));

const mockUseAuth = vi.mocked(useAuth);
const mockUseAybAnonymousBootstrap = vi.mocked(useAybAnonymousBootstrap);
const mockClearPersistedTokens = vi.mocked(clearPersistedTokens);
const mockIsAnonymousBootstrapEnabled = vi.mocked(isAnonymousBootstrapEnabled);
const mockDisableAnonymousBootstrap = vi.mocked(disableAnonymousBootstrap);
const mockClearAnonymousBootstrapOptOut = vi.mocked(clearAnonymousBootstrapOptOut);
const mockListRecords = vi.mocked(ayb.records.list);
const mockGraphQLQuery = vi.mocked(ayb.graphql.query);

describe("App auth lifecycle", () => {
  const logout = vi.fn();
  const signInAnonymously = vi.fn();
  const signInWithPasskey = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockIsAnonymousBootstrapEnabled.mockReturnValue(true);
    mockUseAuth.mockReturnValue({
      loading: false,
      user: { id: "user-1", email: "me@test.com", isAnonymous: false },
      error: null,
      token: "token-1",
      refreshToken: "refresh-1",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously,
      signInWithPasskey,
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
  });

  it("passes resolved auth state into anonymous bootstrap to avoid post-register guest reentry", () => {
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
      enabled: true,
      token: "token-1",
      signInAnonymously,
    });
  });

  it("honors persisted anonymous bootstrap opt-out on mount", () => {
    mockIsAnonymousBootstrapEnabled.mockReturnValueOnce(false);
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
      enabled: false,
      token: "token-1",
      signInAnonymously,
    });
  });

  it("does not clear local auth state when logout() fails", async () => {
    logout.mockRejectedValueOnce(new Error("logout failed"));
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    expect(mockClearPersistedTokens).not.toHaveBeenCalled();
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(await screen.findByText("Sign out failed. Please try again.")).toBeInTheDocument();
  });

  it("disables anonymous bootstrap after explicit logout", async () => {
    logout.mockResolvedValueOnce(undefined);
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
      enabled: true,
      token: "token-1",
      signInAnonymously,
    });

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    await waitFor(() => {
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
        enabled: false,
        token: "token-1",
        signInAnonymously,
      });
    });
    expect(mockDisableAnonymousBootstrap).toHaveBeenCalledOnce();
  });

  it("disables anonymous bootstrap before logout resolves", async () => {
    const logoutResolution: { resolve: (() => void) | null } = { resolve: null };
    logout.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          logoutResolution.resolve = resolve;
        }),
    );

    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    expect(mockDisableAnonymousBootstrap).toHaveBeenCalledOnce();
    await waitFor(() => {
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
        enabled: false,
        token: "token-1",
        signInAnonymously,
      });
    });

    if (!logoutResolution.resolve) {
      throw new Error("logout resolution callback not captured");
    }
    logoutResolution.resolve();
    await waitFor(() => expect(mockClearPersistedTokens).toHaveBeenCalledOnce());
  });

  it("re-enables anonymous bootstrap after explicit auth succeeds", async () => {
    mockIsAnonymousBootstrapEnabled.mockReturnValueOnce(false);
    mockUseAuth.mockReturnValue({
      loading: false,
      user: null,
      error: null,
      token: null,
      refreshToken: null,
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously,
      signInWithPasskey,
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
      enabled: false,
      token: null,
      signInAnonymously,
    });

    fireEvent.click(screen.getByRole("button", { name: "Auth Form" }));

    await waitFor(() => {
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({
        enabled: true,
        token: null,
        signInAnonymously,
      });
    });
  });

  it("lands anonymous users on the main app without exposing poll creation", () => {
    mockUseAuth.mockReturnValue({
      loading: false,
      user: { id: "anon-1", isAnonymous: true },
      error: null,
      token: "anon-token",
      refreshToken: "anon-refresh",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      signInWithPasskey: vi.fn(),
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });

    render(<App />);

    expect(screen.queryByRole("button", { name: "Auth Form" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /New Poll/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });

  it("does not treat a token without a resolved user as authenticated", () => {
    mockUseAuth.mockReturnValue({
      loading: false,
      user: null,
      error: Object.assign(new Error("unauthorized"), { status: 401 }),
      token: "stale-token",
      refreshToken: "stale-refresh",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      signInWithPasskey: vi.fn(),
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });

    render(<App />);

    expect(screen.getByRole("button", { name: "Auth Form" })).toBeInTheDocument();
    expect(mockListRecords).not.toHaveBeenCalled();
    expect(mockGraphQLQuery).not.toHaveBeenCalled();
  });

  it("surfaces an auth-boundary error when initial poll load fails", async () => {
    mockListRecords.mockRejectedValueOnce(new Error("load failed"));
    render(<App />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load polls. Please refresh.");
  });

  it("loads bootstrap polls via one graphql polls+poll_options query and keeps vote loading on records.list", async () => {
    mockGraphQLQuery.mockResolvedValueOnce({
      polls: [
        {
          id: "poll-1",
          user_id: "user-1",
          question: "Q1",
          is_closed: false,
          created_at: "2026-01-01T00:00:00Z",
          poll_options: [],
        },
      ],
    });
    mockListRecords.mockResolvedValueOnce({ items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 });

    render(<App />);

    await waitFor(() => expect(mockGraphQLQuery).toHaveBeenCalledOnce());
    expect(mockListRecords).toHaveBeenCalledWith("votes", expect.objectContaining({ page: 1, perPage: 500 }));
    const [document] = mockGraphQLQuery.mock.calls[0] ?? [];
    expect(document).toContain("polls");
    expect(document).toContain("poll_options");
    expect(document).toContain("limit");
    expect(document).toContain("orderBy");
  });
});
