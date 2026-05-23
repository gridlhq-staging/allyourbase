import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import {
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

vi.mock("../src/components/SearchResults", () => ({
  default: () => <div data-testid="search-results" />,
}));

vi.mock("../src/components/NoteComposer", () => ({
  default: () => null,
}));

vi.mock("../src/components/ChatPanel", () => ({
  default: () => null,
}));

vi.mock("../src/components/ProviderKeyForm", () => ({
  default: () => null,
}));

vi.mock("../src/lib/ayb", () => ({
  ayb: {},
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
  getPersistedEmail: vi.fn(() => null),
  persistTokens: vi.fn(),
  searchMovies: vi.fn().mockResolvedValue({ rows: [] }),
  embedNote: vi.fn(),
  streamChat: vi.fn(),
  setBYOKKey: vi.fn(),
  clearBYOKKey: vi.fn(),
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
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
  });

  it("shows loading spinner while bootstrapping", () => {
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: true });
    render(<App />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows auth form when no user is authenticated", () => {
    mockUseAuth.mockReturnValue({
      loading: false,
      user: null,
      error: null,
      token: null,
      refreshToken: null,
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    render(<App />);
    expect(screen.getByRole("button", { name: "Auth Form" })).toBeInTheDocument();
  });

  it("shows auth form for anonymous users", () => {
    mockUseAuth.mockReturnValue({
      loading: false,
      user: { id: "anon-1", email: "", isAnonymous: true },
      error: null,
      token: "anon-token",
      refreshToken: "anon-refresh",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    render(<App />);
    expect(screen.getByRole("button", { name: "Auth Form" })).toBeInTheDocument();
  });

  it("honors persisted anonymous bootstrap opt-out on mount", () => {
    mockIsAnonymousBootstrapEnabled.mockReturnValueOnce(false);
    render(<App />);
    expect(mockUseAybAnonymousBootstrap).toHaveBeenLastCalledWith({ enabled: false });
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

  it("does not clear local auth state when logout fails", async () => {
    logout.mockRejectedValueOnce(new Error("logout failed"));
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));

    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    expect(mockClearPersistedTokens).not.toHaveBeenCalled();
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(await screen.findByRole("alert")).toHaveTextContent("Sign out failed");
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
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
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

  it("renders the main app UI when authenticated", () => {
    render(<App />);
    expect(screen.getByText("Movies Demo")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });
});
