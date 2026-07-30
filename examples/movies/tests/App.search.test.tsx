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

  function authState(overrides: Partial<ReturnType<typeof useAuth>> = {}) {
    return {
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
      ...overrides,
    };
  }

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isAnonymousBootstrapEnabled).mockReturnValue(true);
    logout = vi.fn().mockResolvedValue(undefined);
    mockUseAuth.mockReturnValue(authState());
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

  async function setMovieSearchControls(query: string, decade: string) {
    fireEvent.change(screen.getByPlaceholderText(/search movies/i), { target: { value: query } });
    fireEvent.click(screen.getByTestId("genre-facet-Sci-Fi"));
    fireEvent.change(screen.getByTestId("decade-filter"), { target: { value: decade } });
    await advanceDebounce();
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

  it("renders highlight text with accessible label", async () => {
    render(<App />);
    await advanceDebounce();

    await screen.findByText("Inception");
    const highlight = screen.getByLabelText("Highlighted match");
    expect(highlight).toHaveTextContent(
      "A thief who steals corporate secrets through <b>dream</b>-sharing technology.",
    );
    expect(highlight.querySelector("b")).toBeNull();
  });

  it("renders server-provided highlight HTML as inert text", async () => {
    mockSearchMovies.mockResolvedValue(
      listResponse([
        {
          slug: "hostile-highlight",
          title: "Hostile Highlight",
          overview: "A crafted highlight should not create DOM nodes.",
          release_year: 2026,
          primary_genre: "Thriller",
          _highlight: 'Owned <img src=x onerror="window.__aybHighlightXss = true"> highlight',
        },
      ]),
    );

    render(<App />);
    await advanceDebounce();

    const overview = await screen.findByTestId("search-result-overview-hostile-highlight");
    expect(overview).toHaveTextContent(
      'Owned <img src=x onerror="window.__aybHighlightXss = true"> highlight',
    );
    expect(overview.querySelector("img")).toBeNull();
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

  it("clears query and filters after successful sign out", async () => {
    const postLogoutSearch = deferredSearchResponse();
    render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    await setMovieSearchControls("matrix", "1990");
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "matrix",
          filter: "primary_genre='Sci-Fi' AND release_year>=1990 AND release_year<2000",
        }),
      );
    });

    mockSearchMovies.mockClear();
    mockSearchMovies.mockReturnValueOnce(postLogoutSearch.promise);
    fireEvent.click(screen.getByRole("button", { name: /sign out/i }));
    await waitFor(() => {
      expect(logout).toHaveBeenCalled();
    });
    await advanceDebounce();

    expect(screen.getByPlaceholderText(/search movies/i)).toHaveValue("");
    expect(screen.getByTestId("decade-filter")).toHaveValue("");
    expect(mockSearchMovies).toHaveBeenCalledWith(expect.objectContaining({ search: "" }));
    expect(mockSearchMovies).toHaveBeenCalledWith(
      expect.not.objectContaining({ filter: expect.any(String) }),
    );
    expect(screen.queryByText("The Matrix")).not.toBeInTheDocument();
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Loading movies...");
  });

  it("clears query and filters after session invalidation before a different account signs in", async () => {
    const { rerender } = render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");

    await setMovieSearchControls("matrix", "1990");
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "matrix",
          filter: "primary_genre='Sci-Fi' AND release_year>=1990 AND release_year<2000",
        }),
      );
    });

    // SIGNED_OUT and an unauthorized /me reload both produce this invalidated
    // session shape before a later sign-in can establish a different account.
    mockUseAuth.mockReturnValue(authState({
      user: null,
      token: null,
      refreshToken: null,
    }));
    rerender(<App />);

    mockSearchMovies.mockClear();
    mockUseAuth.mockReturnValue(authState({
      user: { id: "user-2", email: "next@test.com", isAnonymous: false },
      token: "token-2",
      refreshToken: "refresh-2",
    }));
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

  it("ignores an in-flight search response after a retained-token authedReady dip", async () => {
    const pendingSearch = deferredSearchResponse();
    const recoveredSearch = deferredSearchResponse();
    const { rerender } = render(<App />);
    await advanceDebounce();
    await screen.findByText("The Matrix");
    mockSearchMovies.mockClear();
    mockSearchMovies.mockReturnValueOnce(pendingSearch.promise);

    await setMovieSearchControls("inception", "2010");
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "inception",
          filter: "primary_genre='Sci-Fi' AND release_year>=2010 AND release_year<2020",
        }),
      );
    });

    mockUseAuth.mockReturnValue(authState({
      user: null,
      error: new Error("temporary /me failure"),
      token: "token-2",
      refreshToken: "refresh-2",
    }));
    rerender(<App />);

    mockUseAuth.mockReturnValue(authState({
      token: "token-2",
      refreshToken: "refresh-2",
    }));
    mockSearchMovies.mockClear();
    mockSearchMovies.mockReturnValueOnce(recoveredSearch.promise);
    rerender(<App />);
    await advanceDebounce();

    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({
          search: "inception",
          filter: "primary_genre='Sci-Fi' AND release_year>=2010 AND release_year<2020",
        }),
      );
    });

    await act(async () => {
      pendingSearch.resolve(SEARCH_RESPONSE);
    });

    expect(screen.getByPlaceholderText(/search movies/i)).toHaveValue("inception");
    expect(screen.getByTestId("decade-filter")).toHaveValue("2010");
    expect(screen.queryByText("The Matrix")).not.toBeInTheDocument();
    expect(screen.queryByText("Inception")).not.toBeInTheDocument();
    expect(screen.getByTestId("results-summary")).toHaveTextContent("Loading movies...");
  });

  it("typed query survives an authedReady dip", async () => {
    const { rerender } = render(<App />);
    await advanceDebounce();
    mockSearchMovies.mockClear();

    fireEvent.change(screen.getByPlaceholderText(/search movies/i), {
      target: { value: "inception" },
    });
    await advanceDebounce();
    await waitFor(() => {
      expect(mockSearchMovies).toHaveBeenCalledWith(
        expect.objectContaining({ search: "inception" }),
      );
    });

    // TOKEN_REFRESHED retains the refreshed session. If its /me reload fails
    // with a non-authorization error, useAuth clears user but keeps this token.
    mockUseAuth.mockReturnValue(authState({
      user: null,
      error: new Error("temporary /me failure"),
      token: "token-2",
      refreshToken: "refresh-2",
    }));
    rerender(<App />);

    // A later successful refresh can restore the same account and readiness.
    mockSearchMovies.mockClear();
    mockUseAuth.mockReturnValue(authState({
      token: "token-2",
      refreshToken: "refresh-2",
    }));
    rerender(<App />);
    await advanceDebounce();

    const recoveredInput = screen.getByPlaceholderText(/search movies/i);
    const dispatchedTypedSearch = mockSearchMovies.mock.calls.some(
      ([options]) => options.search === "inception",
    );
    expect(
      {
        inputValue: (recoveredInput as HTMLInputElement).value,
        dispatchedTypedSearch,
      },
      "LOST_QUERY_REPRO",
    ).toEqual({
      inputValue: "inception",
      dispatchedTypedSearch: true,
    });
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
