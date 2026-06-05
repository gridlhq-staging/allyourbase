import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import AuthForm from "../src/components/AuthForm";
import { clearAnonymousBootstrapOptOut, persistTokens } from "../src/lib/ayb";
import { useAuth } from "@allyourbase/react";

vi.mock("../src/lib/ayb", () => ({
  persistTokens: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
}));

vi.mock("@allyourbase/react", () => {
  return {
    AybLoginBar: ({
      mode = "login",
      loading,
      email,
      password,
      error,
      oauthProviders,
      methods,
      onEmailChange,
      onPasswordChange,
      onModeChange,
      onSubmit,
      onOAuthProvider,
      onAnonymous,
      onRequestMagicLink,
      onUpgradeAnonymous,
    }: {
      mode?: "login" | "register";
      loading: boolean;
      email: string;
      password: string;
      error: string | null;
      oauthProviders?: string[];
      methods: { password: boolean; oauth: boolean; anonymous: boolean; canUpgradeAnonymous: boolean; magicLink?: boolean };
      onEmailChange: (value: string) => void;
      onPasswordChange: (value: string) => void;
      onModeChange?: (mode: "login" | "register") => void;
      onSubmit: () => Promise<void>;
      onOAuthProvider?: (provider: string) => Promise<void>;
      onAnonymous?: () => Promise<void>;
      onRequestMagicLink?: (email: string) => Promise<void>;
      onUpgradeAnonymous?: () => Promise<void>;
    }) => (
      <div>
        <input aria-label="Email" value={email} onChange={(event) => onEmailChange(event.target.value)} />
        <input aria-label="Password" value={password} onChange={(event) => onPasswordChange(event.target.value)} />
        <button type="button" disabled={loading} onClick={() => void onSubmit()}>
          {mode === "register" ? "Create Account" : "Sign In"}
        </button>
        {onModeChange && (
          <button type="button" onClick={() => onModeChange(mode === "register" ? "login" : "register")}>
            {mode === "register" ? "Sign in" : "Sign up"}
          </button>
        )}
        {methods.oauth && oauthProviders && onOAuthProvider && oauthProviders.map((p) => (
          <button key={p} type="button" onClick={() => void onOAuthProvider(p)}>{`Continue with ${p}`}</button>
        ))}
        {methods.anonymous && onAnonymous && (
          <button type="button" onClick={() => void onAnonymous()}>Continue as Guest</button>
        )}
        {methods.magicLink && onRequestMagicLink && (
          <button type="button" onClick={() => void onRequestMagicLink(email)}>Email me a magic link</button>
        )}
        {methods.canUpgradeAnonymous && onUpgradeAnonymous && (
          <button type="button" onClick={() => void onUpgradeAnonymous()}>Upgrade Account</button>
        )}
        {error && <p role="alert">{error}</p>}
      </div>
    ),
    DemoSuggestionChip: ({
      suggestion,
      onSelect,
    }: {
      suggestion: { label: string; email: string; password: string };
      onSelect: (value: { email: string; password: string }) => void;
    }) => (
      <button type="button" onClick={() => onSelect({ email: suggestion.email, password: suggestion.password })}>
        {suggestion.label}
      </button>
    ),
    useAuth: vi.fn(),
  };
});

const mockPersistTokens = vi.mocked(persistTokens);
const mockClearAnonymousBootstrapOptOut = vi.mocked(clearAnonymousBootstrapOptOut);
const mockUseAuth = vi.mocked(useAuth);

describe("movies AuthForm", () => {
  const login = vi.fn();
  const register = vi.fn();
  const signInAnonymously = vi.fn();
  const signInWithOAuth = vi.fn();
  const requestMagicLink = vi.fn();
  const linkEmail = vi.fn();

  function setAuth(user: { id: string; email?: string; isAnonymous?: boolean } | null) {
    mockUseAuth.mockReturnValue({
      loading: false,
      user,
      error: null,
      token: user ? "token" : null,
      refreshToken: user ? "refresh" : null,
      login,
      register,
      signInAnonymously,
      requestMagicLink,
      confirmMagicLink: vi.fn(),
      linkEmail,
      signInWithOAuth,
      signInWithPasskey: vi.fn(),
      logout: vi.fn(),
      refresh: vi.fn(),
    });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    setAuth(null);
  });

  it("submits login via the password path and notifies parent", async () => {
    login.mockResolvedValueOnce(undefined);
    const onAuth = vi.fn();
    render(<AuthForm onAuth={onAuth} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "u@test.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));
    await waitFor(() => expect(login).toHaveBeenCalledWith("u@test.com", "password123"));
    expect(mockPersistTokens).toHaveBeenCalledWith("u@test.com");
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(onAuth).toHaveBeenCalledWith("u@test.com");
  });

  it("renders per-provider OAuth buttons wired to signInWithOAuth", async () => {
    signInWithOAuth.mockResolvedValue(undefined);
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Continue with github" }));
    fireEvent.click(screen.getByRole("button", { name: "Continue with google" }));
    await waitFor(() => expect(signInWithOAuth).toHaveBeenCalledTimes(2));
    expect(signInWithOAuth).toHaveBeenNthCalledWith(1, "github");
    expect(signInWithOAuth).toHaveBeenNthCalledWith(2, "google");
  });

  it("renders a guest sign-in button wired to signInAnonymously", async () => {
    signInAnonymously.mockResolvedValue(undefined);
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Continue as Guest" }));
    await waitFor(() => expect(signInAnonymously).toHaveBeenCalledOnce());
  });

  it("renders a magic-link trigger wired to requestMagicLink with the typed email", async () => {
    requestMagicLink.mockResolvedValue(undefined);
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "magic@test.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Email me a magic link" }));
    await waitFor(() => expect(requestMagicLink).toHaveBeenCalledWith("magic@test.com"));
  });

  it("renders the Upgrade Account button when current user is anonymous and wires it to linkEmail", async () => {
    linkEmail.mockResolvedValueOnce(undefined);
    setAuth({ id: "anon-1", isAnonymous: true });
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "claim@test.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "Upgrade Account" }));
    await waitFor(() => expect(linkEmail).toHaveBeenCalledWith("claim@test.com", "password123"));
  });

  it("does not render the Upgrade Account button for non-anonymous (logged-out) state", () => {
    setAuth(null);
    render(<AuthForm onAuth={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Upgrade Account" })).not.toBeInTheDocument();
  });
});
