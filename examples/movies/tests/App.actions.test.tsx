import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import {
  embedNote,
  streamChat,
  isAnonymousBootstrapEnabled,
  searchMovies,
} from "../src/lib/ayb";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";

vi.mock("../src/components/AuthForm", () => ({
  default: () => null,
}));

const mockSearchMovies = vi.fn<typeof searchMovies>();
const mockEmbedNote = vi.fn<typeof embedNote>();
const mockStreamChat = vi.fn<typeof streamChat>();

vi.mock("../src/lib/ayb", () => ({
  ayb: {},
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
  getPersistedEmail: vi.fn(() => null),
  persistTokens: vi.fn(),
  searchMovies: (...args: Parameters<typeof searchMovies>) => mockSearchMovies(...args),
  embedNote: (...args: Parameters<typeof embedNote>) => mockEmbedNote(...args),
  streamChat: (...args: Parameters<typeof streamChat>) => mockStreamChat(...args),
  setBYOKKey: vi.fn(),
  clearBYOKKey: vi.fn(),
}));

vi.mock("@allyourbase/react", () => ({
  useAuth: vi.fn(),
  useAybAnonymousBootstrap: vi.fn(),
}));

const mockUseAuth = vi.mocked(useAuth);
const mockUseAybAnonymousBootstrap = vi.mocked(useAybAnonymousBootstrap);

function loggedOutAuthState() {
  return {
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
    logout: vi.fn(),
    refresh: vi.fn(),
  };
}

function authenticatedAuthState(id: string, email: string, token: string, refreshToken: string) {
  return {
    loading: false,
    user: { id, email, isAnonymous: false },
    error: null,
    token,
    refreshToken,
    login: vi.fn(),
    register: vi.fn(),
    signInAnonymously: vi.fn(),
    requestMagicLink: vi.fn(),
    confirmMagicLink: vi.fn(),
    linkEmail: vi.fn(),
    signInWithOAuth: vi.fn(),
    signInWithPasskey: vi.fn(),
    logout: vi.fn(),
    refresh: vi.fn(),
  };
}

function deferredChatResponse() {
  let resolve!: (value: { sessionId: string; fullText: string }) => void;
  const promise = new Promise<{ sessionId: string; fullText: string }>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("App actions delegate to lib/ayb", () => {
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
      signInWithPasskey: vi.fn(),
      logout: vi.fn(),
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
    mockSearchMovies.mockResolvedValue({
      items: [
        {
          slug: "the-matrix",
          title: "The Matrix",
          overview: "A computer hacker learns about the true nature of reality.",
          release_year: 1999,
          primary_genre: "Sci-Fi",
        },
      ],
      page: 1,
      perPage: 10,
      totalItems: 1,
      totalPages: 1,
      facets: { primary_genre: [{ value: "Sci-Fi", count: 1 }] },
    });
    mockEmbedNote.mockResolvedValue({
      id: "note-1",
      movie_slug: "the-matrix",
      embedding: [0.1, 0.2, 0.3],
    });
    mockStreamChat.mockResolvedValue({
      sessionId: "sess-1",
      fullText: "The Matrix is great.",
    });
  });

  it("calls embedNote from lib/ayb when adding a note", async () => {
    render(<App />);

    const movieButton = await screen.findByText("The Matrix");
    fireEvent.click(movieButton.closest("button")!);

    const noteInput = await screen.findByPlaceholderText(/add a note/i);
    fireEvent.change(noteInput, { target: { value: "Great movie" } });
    fireEvent.submit(noteInput.closest("form")!);

    await waitFor(() => {
      expect(mockEmbedNote).toHaveBeenCalledWith("Great movie", "the-matrix");
    });
  });

  it("calls streamChat from lib/ayb when sending a chat message", async () => {
    render(<App />);

    const chatInput = screen.getByPlaceholderText(/ask about movies/i);
    fireEvent.change(chatInput, { target: { value: "Tell me about The Matrix" } });
    fireEvent.submit(chatInput.closest("form")!);

    await waitFor(() => {
      expect(mockStreamChat).toHaveBeenCalledTimes(1);
      const [messages] = mockStreamChat.mock.calls[0];
      expect(messages).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ role: "user", content: "Tell me about The Matrix" }),
        ]),
      );
    });
  });

  it("renders the assistant response after chat completes", async () => {
    mockStreamChat.mockResolvedValue({
      sessionId: "sess-1",
      fullText: "Local stub response: Summarize inception",
    });
    render(<App />);

    const chatInput = screen.getByPlaceholderText(/ask about movies/i);
    fireEvent.change(chatInput, { target: { value: "Summarize inception" } });
    fireEvent.submit(chatInput.closest("form")!);

    await waitFor(() => {
      expect(screen.getByText("Local stub response: Summarize inception")).toBeInTheDocument();
    });
  });

  it("clears chat transcript across non-logout auth transitions", async () => {
    mockStreamChat.mockResolvedValue({
      sessionId: "sess-1",
      fullText: "Previous session answer",
    });
    const { rerender } = render(<App />);

    const chatInput = screen.getByPlaceholderText(/ask about movies/i);
    fireEvent.change(chatInput, { target: { value: "What did I watch?" } });
    fireEvent.submit(chatInput.closest("form")!);

    await waitFor(() => {
      expect(screen.getByText("Previous session answer")).toBeInTheDocument();
    });

    mockUseAuth.mockReturnValue(loggedOutAuthState());
    rerender(<App />);

    mockUseAuth.mockReturnValue(authenticatedAuthState("user-2", "next@test.com", "token-2", "refresh-2"));
    rerender(<App />);

    expect(screen.queryByText("Previous session answer")).not.toBeInTheDocument();
    expect(screen.getByText(/ask a question about movies/i)).toBeInTheDocument();
  });

  it("ignores in-flight chat stream updates across non-logout auth transitions", async () => {
    const pendingChat = deferredChatResponse();
    mockStreamChat.mockImplementation(async (_messages, onChunk) => {
      onChunk("partial previous answer");
      return pendingChat.promise;
    });
    const { rerender } = render(<App />);

    const chatInput = screen.getByPlaceholderText(/ask about movies/i);
    fireEvent.change(chatInput, { target: { value: "Keep streaming?" } });
    fireEvent.submit(chatInput.closest("form")!);

    await waitFor(() => {
      expect(screen.getByText("partial previous answer")).toBeInTheDocument();
    });

    mockUseAuth.mockReturnValue(loggedOutAuthState());
    rerender(<App />);

    mockUseAuth.mockReturnValue(authenticatedAuthState("user-2", "next@test.com", "token-2", "refresh-2"));
    rerender(<App />);

    expect(screen.queryByText("partial previous answer")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText(/ask about movies/i)).toBeEnabled();

    pendingChat.resolve({ sessionId: "sess-1", fullText: "resolved previous answer" });

    await waitFor(() => {
      expect(screen.queryByText("resolved previous answer")).not.toBeInTheDocument();
    });
  });

  it("does not call fetch directly from components", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response());
    render(<App />);

    const chatInput = screen.getByPlaceholderText(/ask about movies/i);
    fireEvent.change(chatInput, { target: { value: "Hello" } });
    fireEvent.submit(chatInput.closest("form")!);

    await waitFor(() => {
      expect(mockStreamChat).toHaveBeenCalledTimes(1);
    });
    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });
});
