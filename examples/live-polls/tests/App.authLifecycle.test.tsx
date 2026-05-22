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
    records: {
      list: vi.fn().mockResolvedValue({ items: [], page: 1, perPage: 100, totalItems: 0, totalPages: 0 }),
    },
  },
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
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

describe("App auth lifecycle", () => {
  const logout = vi.fn();

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
      signInAnonymously: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
  });

  it("honors persisted anonymous bootstrap opt-out on mount", () => {
    mockIsAnonymousBootstrapEnabled.mockReturnValueOnce(false);
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: false });
  });

  it("does not clear local auth state when logout() fails", async () => {
    logout.mockRejectedValueOnce(new Error("logout failed"));
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    expect(mockClearPersistedTokens).not.toHaveBeenCalled();
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(await screen.findByRole("alert")).toHaveTextContent("Sign out failed");
  });

  it("disables anonymous bootstrap after explicit logout", async () => {
    logout.mockResolvedValueOnce(undefined);
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: true });

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    await waitFor(() => {
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: false });
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
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: false });
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
      signInAnonymously: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    render(<App />);

    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: false });

    fireEvent.click(screen.getByRole("button", { name: "Auth Form" }));

    await waitFor(() => {
      expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: true });
    });
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
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });

    render(<App />);

    expect(screen.getByRole("button", { name: "Auth Form" })).toBeInTheDocument();
    expect(mockListRecords).not.toHaveBeenCalled();
  });

  it("surfaces an auth-boundary error when initial poll load fails", async () => {
    mockListRecords.mockRejectedValueOnce(new Error("load failed"));
    render(<App />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load polls. Please refresh.");
  });
});
