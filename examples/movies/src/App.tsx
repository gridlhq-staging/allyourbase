import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import type { FacetCounts } from "@allyourbase/js";
import {
  clearAnonymousBootstrapOptOut,
  clearPersistedTokens,
  disableAnonymousBootstrap,
  getPersistedEmail,
  isAnonymousBootstrapEnabled,
  searchMovies,
  embedNote,
  streamChat,
  setBYOKKey,
  clearBYOKKey,
} from "./lib/ayb";
import type { MovieListItem, ChatMessage, BYOKProvider } from "./types";
import AuthForm from "./components/AuthForm";
import SearchResults from "./components/SearchResults";
import NoteComposer from "./components/NoteComposer";
import ChatPanel from "./components/ChatPanel";
import ProviderKeyForm from "./components/ProviderKeyForm";

const SEARCH_DEBOUNCE_MS = 300;

interface DecadeOption {
  label: string;
  // null = no decade filter
  start: number | null;
}

const DECADE_OPTIONS: DecadeOption[] = [
  { label: "All decades", start: null },
  { label: "1980s", start: 1980 },
  { label: "1990s", start: 1990 },
  { label: "2000s", start: 2000 },
  { label: "2010s", start: 2010 },
  { label: "2020s", start: 2020 },
];

function escapeGenreLiteral(value: string): string {
  // backend filter grammar uses single-quoted string literals; escape embedded quotes
  return value.replace(/'/g, "''");
}

function buildFilter(genre: string | null, decadeStart: number | null): string | undefined {
  const clauses: string[] = [];
  if (genre) clauses.push(`primary_genre='${escapeGenreLiteral(genre)}'`);
  if (decadeStart != null) {
    clauses.push(`release_year>=${decadeStart} AND release_year<${decadeStart + 10}`);
  }
  return clauses.length ? clauses.join(" AND ") : undefined;
}

export default function App() {
  const { user, token, loading, logout } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  const { bootstrapping } = useAybAnonymousBootstrap({ enabled: anonymousBootstrapEnabled });
  const [email, setEmail] = useState<string | null>(getPersistedEmail());
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);

  const [searchQuery, setSearchQuery] = useState("");
  const [items, setItems] = useState<MovieListItem[]>([]);
  const [totalItems, setTotalItems] = useState(0);
  const [facets, setFacets] = useState<FacetCounts | undefined>(undefined);
  const [selectedGenre, setSelectedGenre] = useState<string | null>(null);
  const [selectedDecadeStart, setSelectedDecadeStart] = useState<number | null>(null);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);
  const [hasLoadedOnce, setHasLoadedOnce] = useState(false);
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null);

  const [streamedText, setStreamedText] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [chatHistory, setChatHistory] = useState<ChatMessage[]>([]);

  const requestSeqRef = useRef(0);
  const chatSeqRef = useRef(0);

  const resetMoviesSearchState = useCallback(() => {
    requestSeqRef.current += 1;
    setItems([]);
    setTotalItems(0);
    setFacets(undefined);
    setSearchError(null);
    setSearching(false);
    setHasLoadedOnce(false);
    setSelectedSlug(null);
  }, []);

  const resetMoviesSearchControls = useCallback(() => {
    setSearchQuery("");
    setSelectedGenre(null);
    setSelectedDecadeStart(null);
  }, []);

  const resetChatState = useCallback(() => {
    chatSeqRef.current += 1;
    setChatHistory([]);
    setStreamedText("");
    setStreaming(false);
  }, []);

  useEffect(() => {
    if (user?.email) {
      setEmail(user.email);
      return;
    }
    if (!token) {
      setEmail(getPersistedEmail());
    }
  }, [user, token]);

  const runSearch = useCallback(
    async (query: string, genre: string | null, decadeStart: number | null) => {
      const seq = ++requestSeqRef.current;
      setSearching(true);
      setSearchError(null);
      try {
        const res = await searchMovies({
          search: query.trim(),
          filter: buildFilter(genre, decadeStart),
        });
        if (seq !== requestSeqRef.current) return;
        setItems(res.items);
        setTotalItems(res.totalItems);
        setFacets(res.facets);
        setHasLoadedOnce(true);
      } catch {
        if (seq !== requestSeqRef.current) return;
        setSearchError("Movie search failed");
      } finally {
        if (seq === requestSeqRef.current) setSearching(false);
      }
    },
    [],
  );

  // Authenticated users: debounce search-as-you-type across query, genre, and
  // decade. We re-run the SDK list call on every change (including empty
  // string) so the corpus default + filter-only views share one code path.
  const authedReady = Boolean(token && user && !user.isAnonymous);
  useEffect(() => {
    if (!authedReady) return;
    const handle = setTimeout(() => {
      void runSearch(searchQuery, selectedGenre, selectedDecadeStart);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(handle);
  }, [authedReady, searchQuery, selectedGenre, selectedDecadeStart, runSearch]);

  useEffect(() => {
    if (authedReady) return;
    resetMoviesSearchState();
    resetMoviesSearchControls();
    resetChatState();
  }, [authedReady, resetChatState, resetMoviesSearchControls, resetMoviesSearchState]);

  async function handleLogout() {
    const bootstrapEnabledBeforeLogout = anonymousBootstrapEnabled;
    setLogoutPending(true);
    setLogoutError(null);
    setAnonymousBootstrapEnabled(false);
    disableAnonymousBootstrap();
    try {
      await logout();
      resetMoviesSearchState();
      resetMoviesSearchControls();
      clearPersistedTokens();
      setEmail(null);
      resetChatState();
    } catch {
      if (bootstrapEnabledBeforeLogout) {
        setAnonymousBootstrapEnabled(true);
        clearAnonymousBootstrapOptOut();
      }
      setLogoutError("Sign out failed. Please try again.");
    } finally {
      setLogoutPending(false);
    }
  }

  function handleAuth(emailValue: string) {
    setAnonymousBootstrapEnabled(true);
    setEmail(emailValue);
  }

  function handleClearFilters() {
    setSelectedGenre(null);
    setSelectedDecadeStart(null);
    setSearchQuery("");
  }

  function handleRetry() {
    void runSearch(searchQuery, selectedGenre, selectedDecadeStart);
  }

  async function handleEmbedNote(text: string, movieSlug: string) {
    await embedNote(text, movieSlug);
  }

  async function handleChat(messages: ChatMessage[]) {
    const seq = ++chatSeqRef.current;
    setStreaming(true);
    setStreamedText("");
    try {
      const result = await streamChat(messages, (chunk) => {
        if (seq !== chatSeqRef.current) return;
        setStreamedText((prev) => prev + chunk);
      });
      if (seq !== chatSeqRef.current) return;
      setChatHistory([...messages, { role: "assistant", content: result.fullText }]);
    } catch {
      if (seq !== chatSeqRef.current) return;
      setChatHistory([...messages, { role: "assistant", content: "Error: chat request failed." }]);
    } finally {
      if (seq === chatSeqRef.current) setStreaming(false);
    }
  }

  async function handleSetBYOK(provider: BYOKProvider, secretName: string) {
    await setBYOKKey(provider, secretName);
  }

  async function handleClearBYOK(provider: BYOKProvider) {
    await clearBYOKKey(provider);
  }

  if (bootstrapping || loading) {
    return <div className="min-h-screen flex items-center justify-center text-gray-500">Loading...</div>;
  }

  if (!token || !user || user.isAnonymous) {
    return <AuthForm onAuth={handleAuth} />;
  }

  const genreBuckets = facets?.primary_genre ?? [];
  const corpusEmpty = hasLoadedOnce && totalItems === 0 && !selectedGenre && selectedDecadeStart == null && !searchQuery.trim();
  const filtersActive = Boolean(selectedGenre) || selectedDecadeStart != null || Boolean(searchQuery.trim());
  const noMatches = hasLoadedOnce && items.length === 0 && filtersActive;
  const loadingInitialCorpus = !hasLoadedOnce && !searchError;
  const statusMessage = loadingInitialCorpus
    ? "Loading movies..."
    : searching
      ? "Searching movies..."
      : `Showing ${items.length} of ${totalItems} movies`;

  return (
    <div className="min-h-screen bg-gray-950 text-white">
      <header className="bg-gray-900 border-b border-gray-800 px-6 py-3 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold">Movies Demo</h1>
          <span className="text-xs text-gray-500">powered by Allyourbase</span>
        </div>
        <div className="flex items-center gap-3">
          {logoutError && (
            <span role="alert" className="text-sm text-red-400">{logoutError}</span>
          )}
          {email && (
            <span data-testid="user-email" className="text-sm text-gray-400">{email}</span>
          )}
          <button
            onClick={() => void handleLogout()}
            disabled={logoutPending}
            className="text-sm text-gray-400 hover:text-white transition-colors"
          >
            {logoutPending ? "Signing out..." : "Sign out"}
          </button>
        </div>
      </header>

      <main className="max-w-4xl mx-auto p-6 space-y-8">
        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Search the movie corpus</h2>
          <p className="text-sm text-gray-400 mb-3">Search across 250 movies seeded into the demo.</p>
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search movies..."
            aria-label="Search movies"
            className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 text-sm focus:outline-none focus:border-purple-500"
          />

          {!corpusEmpty && (
            <div className="mt-4 space-y-3">
              <div>
                <span className="text-xs uppercase tracking-wider text-gray-500">Primary genre</span>
                <div
                  role="group"
                  aria-label="Primary genre"
                  data-testid="genre-facet-group"
                  className="flex flex-wrap gap-2 mt-1"
                >
                  {genreBuckets.map((bucket) => {
                    const value = String(bucket.value);
                    const active = selectedGenre === value;
                    return (
                      <button
                        key={value}
                        type="button"
                        data-testid={`genre-facet-${value}`}
                        aria-pressed={active}
                        onClick={() => setSelectedGenre(active ? null : value)}
                        className={`px-3 py-1 rounded-full border text-xs transition-colors ${
                          active
                            ? "border-purple-500 bg-purple-950/40 text-white"
                            : "border-gray-700 bg-gray-900 text-gray-300 hover:border-gray-500"
                        }`}
                      >
                        {value} ({bucket.count})
                      </button>
                    );
                  })}
                </div>
              </div>

              <div>
                <label htmlFor="decade-filter" className="text-xs uppercase tracking-wider text-gray-500">
                  Decade
                </label>
                <select
                  id="decade-filter"
                  data-testid="decade-filter"
                  value={selectedDecadeStart == null ? "" : String(selectedDecadeStart)}
                  onChange={(e) =>
                    setSelectedDecadeStart(e.target.value === "" ? null : Number(e.target.value))
                  }
                  className="block mt-1 px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:border-purple-500"
                >
                  {DECADE_OPTIONS.map((opt) => (
                    <option key={opt.label} value={opt.start == null ? "" : String(opt.start)}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              <button
                type="button"
                data-testid="clear-filters"
                onClick={handleClearFilters}
                disabled={!filtersActive}
                className="text-xs text-gray-400 hover:text-white disabled:opacity-40"
              >
                Clear filters
              </button>
            </div>
          )}

          <div
            role="status"
            aria-live="polite"
            data-testid="results-summary"
            className="mt-4 text-sm text-gray-400"
          >
            {statusMessage}
          </div>

          {searchError && (
            <div className="mt-2 flex items-center gap-3">
              <p role="alert" className="text-sm text-red-400">{searchError}</p>
              <button
                type="button"
                data-testid="retry-search"
                onClick={handleRetry}
                className="px-3 py-1 bg-purple-600 hover:bg-purple-700 rounded text-xs text-white transition-colors"
              >
                Retry search
              </button>
            </div>
          )}

          {corpusEmpty && (
            <p data-testid="corpus-empty" className="mt-3 text-sm text-gray-400">
              No seeded movies found
            </p>
          )}

          {noMatches && !corpusEmpty && (
            <p data-testid="no-matches" className="mt-3 text-sm text-gray-400">
              No movies match your filters
            </p>
          )}

          {hasLoadedOnce && items.length > 0 && (
            <div className="mt-4">
              <SearchResults items={items} selectedSlug={selectedSlug} onSelect={setSelectedSlug} />
            </div>
          )}
        </section>

        {selectedSlug && (
          <section data-testid="selected-result-notes-panel">
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Notes</h2>
            <NoteComposer movieSlug={selectedSlug} onSubmit={handleEmbedNote} />
          </section>
        )}

        <section data-testid="chat-section">
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Chat</h2>
          <ChatPanel
            history={chatHistory}
            onHistoryChange={setChatHistory}
            onSend={handleChat}
            streamedText={streamedText}
            streaming={streaming}
          />
        </section>

        <section data-testid="provider-keys-section">
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Provider Keys (BYOK)</h2>
          <ProviderKeyForm onSet={handleSetBYOK} onClear={handleClearBYOK} />
        </section>
      </main>
    </div>
  );
}
