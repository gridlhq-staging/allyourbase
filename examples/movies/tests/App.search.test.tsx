import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import { searchMovies, isAnonymousBootstrapEnabled } from "../src/lib/ayb";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import type { ListResponse } from "@allyourbase/js";
import type { MovieListItem } from "../src/types";

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

function listResponse(items: MovieListItem[], totalItems = items.length): ListResponse<MovieListItem> {
  return {
    items,
    page: 1,
    perPage: 10,
    totalItems,
    totalPages: 1,
    facets: {
      primary_genre: [
        { value: "Sci-Fi", count: 5 },
        { value: "Drama", count: 3 },
      ],
    },
  };
}

const SEARCH_RESPONSE = listResponse(
  [
    {
      slug: "the-matrix",
      title: "The Matrix",
      overview: "A computer hacker learns about the true nature of reality.",
      release_year: 1999,
      primary_genre: "Sci-Fi",
    },
    {
      slug: "inception",
      title: "Inception",
      overview: "A thief who steals corporate secrets through dream-sharing technology.",
      release_year: 2010,
      primary_genre: "Sci-Fi",
      _highlight: "A thief who steals corporate secrets through <b>dream</b>-sharing technology.",
    },
  ],
  250,
);

describe("App search", () => {
  let logout: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isAnonymousBootstrapEnabled).mockReturnValue(true);
    logout = vi.fn().mockResolvedValue(undefined);
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
      signInWithPasskey: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
    mockSearchMovies.mockResolvedValue(SEARCH_RESPONSE);
  });

  async function advanceDebounce() {
    // Debounce is 300ms; give a little buffer for the SDK promise to settle.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 400));
    });
  }

  function deferredSearchResponse() {
    let resolve!: (value: ListResponse<MovieListItem>) => void;
    const promise = new Promise<ListResponse<MovieListItem>>((res) => {
      resolve = res;
    });
    return { promise, resolve };
  }

  it("calls searchMovies via SDK with empty query on initial corpus load", async () => {
    render(<App />);
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalled();
    });
    expect(mockSearchMovies).toHaveBeenCalledWith(
      expect.objectContaining({ search: "" }),
    );
  });

  it("debounces search-as-you-type into a single SDK call", async () => {
    render(<App />);
    await advanceDebounce();
    mockSearchMovies.mockClear();

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "s" } });
    fireEvent.change(input, { target: { value: "sc" } });
    fireEvent.change(input, { target: { value: "sci" } });
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledTimes(1);
    });
    expect(mockSearchMovies).toHaveBeenCalledWith(
      expect.objectContaining({ search: "sci" }),
    );
  });

  it("renders search results with canonical response shape", async () => {
    render(<App />);
    await advanceDebounce();

    expect(await screen.findByText("The Matrix")).toBeInTheDocument();
    expect(screen.getByText("1999")).toBeInTheDocument();
    expect(screen.getByText("Inception")).toBeInTheDocument();
    expect(screen.getByText("2010")).toBeInTheDocument();
    expect(screen.getByTestId("search-result-row-inception")).toBeInTheDocument();
    expect(screen.getByTestId("search-result-title-inception")).toHaveTextContent("Inception");
    expect(screen.getByTestId("search-result-year-inception")).toHaveTextContent("2010");
    expect(screen.getByTestId("search-result-genre-inception")).toHaveTextContent("Sci-Fi");
  });

  it("renders highlight markup with accessible label", async () => {
    render(<App />);
    await advanceDebounce();

    await screen.findByText("Inception");
    const highlight = screen.getByLabelText("Highlighted match");
    expect(highlight.innerHTML).toContain("<b>dream</b>");
  });

  it("renders results summary with totalItems", async () => {
    render(<App />);
    await advanceDebounce();

    const summary = await screen.findByTestId("results-summary");
    await waitFor(() => {
      expect(summary).toHaveTextContent("Showing 2 of 250 movies");
    });
  });

  it("renders primary_genre facet buttons from the SDK response", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    expect(screen.getByTestId("genre-facet-Sci-Fi")).toHaveTextContent("Sci-Fi (5)");
    expect(screen.getByTestId("genre-facet-Drama")).toHaveTextContent("Drama (3)");
  });

  it("re-issues SDK call with primary_genre filter when a facet is selected", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");
    mockSearchMovies.mockClear();

    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({ filter: "primary_genre='Sci-Fi'" }),
      );
    });
  });

  it("re-issues SDK call with decade range when a decade is selected", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");
    mockSearchMovies.mockClear();

    fireEvent.change(screen.getByTestId("decade-filter"), { target: { value: "2010" } });
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({ filter: "release_year>=2010 AND release_year<2020" }),
      );
    });
  });

  it("combines genre and decade filters with AND", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");
    mockSearchMovies.mockClear();

    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    fireEvent.change(screen.getByTestId("decade-filter"), { target: { value: "2010" } });
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          filter: "primary_genre='Sci-Fi' AND release_year>=2010 AND release_year<2020",
        }),
      );
    });
  });

  it("clears filters when Clear filters is clicked", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    await advanceDebounce();
    mockSearchMovies.mockClear();

    fireEvent.click(screen.getByTestId("clear-filters"));
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.not.objectContaining({ filter: expect.any(String) }),
      );
    });
  });

  it("shows error alert and retry when search fails", async () => {
    mockSearchMovies.mockRejectedValueOnce(new Error("boom"));
    render(<App />);
    await advanceDebounce();

    expect(await screen.findByRole("alert")).toHaveTextContent(/movie search failed/i);
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Showing 0 of 0 movies");
    expect(screen.getByTestId("retry-search")).toBeInTheDocument();
  });

  it("clears stale failed search state when signing out", async () => {
    mockSearchMovies.mockRejectedValueOnce(new Error("boom"));
    render(<App />);
    await advanceDebounce();
    expect(await screen.findByRole("alert")).toHaveTextContent(/movie search failed/i);

    fireEvent.click(screen.getByRole("button", { name: /sign out/i }));
    await waitFor(() => {
      expect(logout).toHaveBeenCalled();
    });

    expect(screen.queryByText(/movie search failed/i)).not.toBeInTheDocument();
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Loading movies...");
  });

  it("preserves authenticated movie results when logout fails", async () => {
    logout.mockRejectedValueOnce(new Error("logout failed"));
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    fireEvent.click(screen.getByRole("button", { name: /sign out/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/sign out failed/i);
    expect(screen.getByText("The Matrix")).toBeInTheDocument();
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Showing 2 of 250 movies");
  });

  it("ignores an in-flight search response after signing out", async () => {
    const pendingSearch = deferredSearchResponse();
    mockSearchMovies.mockReturnValueOnce(pendingSearch.promise);
    render(<App />);
    await advanceDebounce();
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByRole("button", { name: /sign out/i }));
    await waitFor(() => {
      expect(logout).toHaveBeenCalled();
    });

    await act(async () => {
      pendingSearch.resolve(SEARCH_RESPONSE);
    });

    expect(screen.queryByText("The Matrix")).not.toBeInTheDocument();
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Loading movies...");
  });

  it("clears query and filters across non-logout auth transitions", async () => {
    const { rerender } = render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    fireEvent.change(screen.getByPlaceholderText(/search movies/i), { target: { value: "matrix" } });
    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    fireEvent.change(screen.getByTestId("decade-filter"), { target: { value: "1990" } });
    await advanceDebounce();
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "matrix",
          filter: "primary_genre='Sci-Fi' AND release_year>=1990 AND release_year<2000",
        }),
      );
    });

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
      signInWithPasskey: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    rerender(<App />);

    mockSearchMovies.mockClear();
    mockUseAuth.mockReturnValue({
      loading: false,
      user: { id: "user-2", email: "next@test.com", isAnonymous: false },
      error: null,
      token: "token-2",
      refreshToken: "refresh-2",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      requestMagicLink: vi.fn(),
      confirmMagicLink: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      signInWithPasskey: vi.fn(),
      logout,
      refresh: vi.fn(),
    });
    rerender(<App />);
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({ search: "" }),
      );
    });
    expect(mockSearchMovies).toHaveBeenCalledWith(
      expect.not.objectContaining({ filter: expect.any(String) }),
    );
    expect(screen.getByPlaceholderText(/search movies/i)).toHaveValue("");
    expect(screen.getByTestId("decade-filter")).toHaveValue("");
  });

  it("retry re-issues the SDK search call preserving query", async () => {
    // Keep the search rejecting so the error state (and Retry button) persist
    // through the typing+debounce cycle until the retry is explicitly clicked.
    mockSearchMovies.mockRejectedValue(new Error("boom"));
    render(<App />);
    await advanceDebounce();
    await screen.findByRole("alert");

    const input = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(input, { target: { value: "matrix" } });
    await advanceDebounce();

    mockSearchMovies.mockClear();
    mockSearchMovies.mockResolvedValue(SEARCH_RESPONSE);
    fireEvent.click(screen.getByTestId("retry-search"));

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({ search: "matrix" }),
      );
    });
  });

  it("shows no-results state when filters produce zero matches", async () => {
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    mockSearchMovies.mockResolvedValueOnce(listResponse([], 250));
    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    await advanceDebounce();

    await waitFor(() => {
      expect(screen.getByTestId("no-matches")).toHaveTextContent(/no movies match your filters/i);
    });
  });

  it("shows empty-corpus state when totalItems=0 with no filters", async () => {
    mockSearchMovies.mockResolvedValue({
      items: [],
      page: 1,
      perPage: 10,
      totalItems: 0,
      totalPages: 0,
      facets: undefined,
    });
    render(<App />);
    await advanceDebounce();

    await waitFor(() => {
      expect(screen.getByTestId("corpus-empty")).toHaveTextContent(/no seeded movies found/i);
    });
  });

});
