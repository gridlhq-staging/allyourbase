/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun04_pm_2_movies_instant_search_showcase/allyourbase_dev/examples/movies/src/lib/ayb.ts.
 */
import { AYBClient } from "@allyourbase/js";
import type { ListResponse } from "@allyourbase/js";
import type {
  MovieListItem,
  NoteEmbedResponse,
  ChatMessage,
  BYOKProvider,
} from "../types";

const url = import.meta.env.VITE_AYB_URL ?? "";
const TOKEN_KEY = "ayb_token";
const REFRESH_TOKEN_KEY = "ayb_refresh_token";
const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";

export const ayb = new AYBClient(url, {
  // Indirect through globalThis so test code can spy on fetch after the
  // client is constructed; without this the SDK captures the original fetch
  // reference at construction time and module-scoped tests can't intercept.
  fetch: (...args) => globalThis.fetch(...args),
  authPersistence: {
    load: () => {
      const token = sessionStorage.getItem(TOKEN_KEY);
      const refreshToken = sessionStorage.getItem(REFRESH_TOKEN_KEY);
      if (!token || !refreshToken) {
        return null;
      }
      return { token, refreshToken };
    },
    save: ({ token, refreshToken }) => {
      sessionStorage.setItem(TOKEN_KEY, token);
      sessionStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
    },
    clear: () => {
      sessionStorage.removeItem(TOKEN_KEY);
      sessionStorage.removeItem(REFRESH_TOKEN_KEY);
    },
  },
});

const EMAIL_KEY = "ayb_email";

export function persistTokens(email?: string) {
  if (email) localStorage.setItem(EMAIL_KEY, email);
}

export function isAnonymousBootstrapEnabled(): boolean {
  return localStorage.getItem(ANONYMOUS_BOOTSTRAP_OPTOUT_KEY) !== "1";
}

export function disableAnonymousBootstrap() {
  localStorage.setItem(ANONYMOUS_BOOTSTRAP_OPTOUT_KEY, "1");
}

export function clearAnonymousBootstrapOptOut() {
  localStorage.removeItem(ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
}

export function clearPersistedTokens() {
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(EMAIL_KEY);
  ayb.clearTokens();
}

export function getPersistedEmail(): string | null {
  return localStorage.getItem(EMAIL_KEY);
}

function apiBase(): string {
  return url;
}

function apiHeaders(): HeadersInit {
  const token = sessionStorage.getItem(TOKEN_KEY);
  if (!token) {
    return { "Content-Type": "application/json" };
  }
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  };
}

export interface SearchMoviesParams {
  search: string;
  filter?: string;
  perPage?: number;
}

export async function searchMovies(
  params: SearchMoviesParams,
): Promise<ListResponse<MovieListItem>> {
  // Backend rejects fuzzy + highlight when search is empty (400
  // "fuzzy parameter requires non-empty search"), so the default
  // corpus load and filter-only views must omit those options and
  // only enable them once the user types a query.
  const hasSearch = params.search.trim().length > 0;
  return ayb.records.list<MovieListItem>("movies", {
    search: params.search,
    sort: "title",
    perPage: params.perPage ?? 10,
    // typoThreshold is a pg_trgm similarity ratio (0..1), not edit distance.
    // 0.3 is permissive enough to forgive single-character typos like
    // "Inceptoin" -> "Inception" while still excluding unrelated matches.
    ...(hasSearch ? { fuzzy: true, typoThreshold: 0.3, highlight: true } : {}),
    facets: ["primary_genre"],
    filter: params.filter,
  });
}

export async function embedNote(
  text: string,
  movieSlug: string,
): Promise<NoteEmbedResponse> {
  const res = await fetch(`${apiBase()}/api/admin/movies/notes/embed`, {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({ text, movie_slug: movieSlug }),
  });
  if (!res.ok) {
    throw new Error(`Embed failed: ${res.status}`);
  }
  return res.json();
}

export async function streamChat(
  messages: ChatMessage[],
  onChunk: (text: string) => void,
  opts?: { provider?: string; model?: string; sessionId?: string },
): Promise<{ sessionId: string; fullText: string }> {
  const res = await fetch(`${apiBase()}/api/admin/movies/chat/stream`, {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({
      messages,
      provider: opts?.provider ?? "",
      model: opts?.model ?? "",
      session_id: opts?.sessionId ?? "",
    }),
  });
  if (!res.ok) {
    throw new Error(`Chat failed: ${res.status}`);
  }

  const reader = res.body?.getReader();
  if (!reader) {
    throw new Error("No response body");
  }

  const decoder = new TextDecoder();
  let fullText = "";
  let sessionId = "";
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.startsWith("data: ")) continue;
      const data = JSON.parse(line.slice(6));
      if (data.text && !data.session_id) {
        fullText += data.text;
        onChunk(data.text);
      }
      if (data.session_id && data.text !== undefined) {
        sessionId = data.session_id;
        fullText = data.text;
      }
      if (data.session_id && data.text === undefined) {
        sessionId = data.session_id;
      }
    }
  }

  return { sessionId, fullText };
}

export async function setBYOKKey(
  provider: BYOKProvider,
  secretName: string,
): Promise<void> {
  const res = await fetch(`${apiBase()}/api/admin/movies/byok`, {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({ provider, secret_name: secretName }),
  });
  if (!res.ok) {
    throw new Error(`BYOK set failed: ${res.status}`);
  }
}

export async function clearBYOKKey(provider: BYOKProvider): Promise<void> {
  const res = await fetch(`${apiBase()}/api/admin/movies/byok/${provider}`, {
    method: "DELETE",
    headers: apiHeaders(),
  });
  if (!res.ok) {
    throw new Error(`BYOK clear failed: ${res.status}`);
  }
}
