import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import { searchMovies, isAnonymousBootstrapEnabled } from "../src/lib/ayb";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import type { MovieSearchResponse } from "../src/types";

vi.mock("../src/components/AuthForm", () => ({
  default: () => null,
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

const mockSearchMovies = vi.fn<typeof searchMovies>();

vi.mock("../src/lib/ayb", () => ({
  ayb: {},
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
  getPersistedEmail: vi.fn(() => null),
  persistTokens: vi.fn(),
  searchMovies: (...args: Parameters<typeof searchMovies>) => mockSearchMovies(...args),
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

const SEARCH_RESPONSE: MovieSearchResponse = {
  rows: [
    {
      slug: "the-matrix",
      title: "The Matrix",
      overview: "A computer hacker learns about the true nature of reality.",
      release_year: 1999,
      similarity: 0.95,
    },
    {
      slug: "inception",
      title: "Inception",
      overview: "A thief who steals corporate secrets through dream-sharing technology.",
      release_year: 2010,
      similarity: 0.88,
    },
  ],
};

describe("App search", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isAnonymousBootstrapEnabled).mockReturnValue(true);
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
      logout: vi.fn(),
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
    mockSearchMovies.mockResolvedValue(SEARCH_RESPONSE);
  });

  it("calls searchMovies from lib/ayb on form submit", async () => {
    render(<App />);

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "sci-fi" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith("sci-fi");
    });
  });

  it("renders search results with canonical response shape", async () => {
    render(<App />);

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "sci-fi" } });
    fireEvent.submit(input.closest("form")!);

    expect(await screen.findByText("The Matrix")).toBeInTheDocument();
    expect(screen.getByText("1999")).toBeInTheDocument();
    expect(screen.getByText("Inception")).toBeInTheDocument();
    expect(screen.getByText("2010")).toBeInTheDocument();
  });

  it("shows an error when search fails", async () => {
    mockSearchMovies.mockRejectedValueOnce(new Error("Search failed: 500"));
    render(<App />);

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "broken" } });
    fireEvent.submit(input.closest("form")!);

    expect(await screen.findByRole("alert")).toHaveTextContent(/search failed/i);
  });

  it("does not issue ad-hoc fetches from search results component", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());
    render(<App />);

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "sci-fi" } });
    fireEvent.submit(input.closest("form")!);

    await screen.findByText("The Matrix");
    expect(mockSearchMovies).toHaveBeenCalledTimes(1);
    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });
});
