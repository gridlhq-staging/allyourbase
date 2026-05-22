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
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout: vi.fn(),
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
    mockSearchMovies.mockResolvedValue({
      rows: [
        {
          slug: "the-matrix",
          title: "The Matrix",
          overview: "A computer hacker learns about the true nature of reality.",
          release_year: 1999,
          similarity: 0.95,
        },
      ],
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

    const searchInput = screen.getByPlaceholderText(/search movies/i);
    fireEvent.change(searchInput, { target: { value: "matrix" } });
    fireEvent.submit(searchInput.closest("form")!);

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
