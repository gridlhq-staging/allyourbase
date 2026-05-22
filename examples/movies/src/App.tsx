import { useEffect, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
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
import type { MovieSearchRow, ChatMessage, BYOKProvider } from "./types";
import AuthForm from "./components/AuthForm";
import SearchResults from "./components/SearchResults";
import NoteComposer from "./components/NoteComposer";
import ChatPanel from "./components/ChatPanel";
import ProviderKeyForm from "./components/ProviderKeyForm";

export default function App() {
  const { user, token, loading, logout } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  const { bootstrapping } = useAybAnonymousBootstrap({ enabled: anonymousBootstrapEnabled });
  const [email, setEmail] = useState<string | null>(getPersistedEmail());
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);

  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<MovieSearchRow[]>([]);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null);

  const [streamedText, setStreamedText] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [, setChatHistory] = useState<ChatMessage[]>([]);

  useEffect(() => {
    if (user?.email) {
      setEmail(user.email);
      return;
    }
    if (!token) {
      setEmail(getPersistedEmail());
    }
  }, [user, token]);

  async function handleLogout() {
    const bootstrapEnabledBeforeLogout = anonymousBootstrapEnabled;
    setLogoutPending(true);
    setLogoutError(null);
    setAnonymousBootstrapEnabled(false);
    disableAnonymousBootstrap();
    try {
      await logout();
      clearPersistedTokens();
      setEmail(null);
      setSearchResults([]);
      setSelectedSlug(null);
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

  async function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    if (!searchQuery.trim()) return;
    setSearching(true);
    setSearchError(null);
    try {
      const res = await searchMovies(searchQuery.trim());
      setSearchResults(res.rows);
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : "Search failed");
    } finally {
      setSearching(false);
    }
  }

  async function handleEmbedNote(text: string, movieSlug: string) {
    await embedNote(text, movieSlug);
  }

  async function handleChat(messages: ChatMessage[]) {
    setStreaming(true);
    setStreamedText("");
    try {
      const result = await streamChat(messages, (chunk) => {
        setStreamedText((prev) => prev + chunk);
      });
      setChatHistory([...messages, { role: "assistant", content: result.fullText }]);
    } catch {
      setChatHistory([...messages, { role: "assistant", content: "Error: chat request failed." }]);
    } finally {
      setStreaming(false);
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
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Search</h2>
          <form onSubmit={handleSearch} className="flex gap-2">
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search movies..."
              className="flex-1 px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 text-sm focus:outline-none focus:border-purple-500"
            />
            <button
              type="submit"
              disabled={searching || !searchQuery.trim()}
              className="px-4 py-2 bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg text-sm text-white transition-colors"
            >
              {searching ? "Searching..." : "Search"}
            </button>
          </form>
          {searchError && <p role="alert" className="text-sm text-red-400 mt-2">{searchError}</p>}
          {searchResults.length > 0 && (
            <div className="mt-4">
              <SearchResults rows={searchResults} selectedSlug={selectedSlug} onSelect={setSelectedSlug} />
            </div>
          )}
        </section>

        {selectedSlug && (
          <section>
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Notes</h2>
            <NoteComposer movieSlug={selectedSlug} onSubmit={handleEmbedNote} />
          </section>
        )}

        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Chat</h2>
          <ChatPanel onSend={handleChat} streamedText={streamedText} streaming={streaming} />
        </section>

        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Provider Keys (BYOK)</h2>
          <ProviderKeyForm onSet={handleSetBYOK} onClear={handleClearBYOK} />
        </section>
      </main>
    </div>
  );
}
