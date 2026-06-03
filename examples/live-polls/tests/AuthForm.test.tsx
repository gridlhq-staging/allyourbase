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
      emailPlaceholder = "you@example.com",
      passwordPlaceholder = "At least 8 characters",
      registerToggleLabel = "Sign up",
      loginToggleLabel = "Sign in",
      oauthProviders,
      methods,
      onEmailChange,
      onPasswordChange,
      onModeChange,
      onSubmit,
      onOAuthProvider,
      onAnonymous,
      onPasskey,
      onRequestMagicLink,
      onUpgradeAnonymous,
    }: {
      mode?: "login" | "register";
      loading: boolean;
      email: string;
      password: string;
      error: string | null;
      emailPlaceholder?: string;
      passwordPlaceholder?: string;
      registerToggleLabel?: string;
      loginToggleLabel?: string;
      oauthProviders?: string[];
      methods: { password: boolean; oauth: boolean; anonymous: boolean; canUpgradeAnonymous: boolean; magicLink?: boolean; passkey?: boolean };
      onEmailChange: (value: string) => void;
      onPasswordChange: (value: string) => void;
      onModeChange?: (mode: "login" | "register") => void;
      onSubmit: () => Promise<void>;
      onOAuthProvider?: (provider: string) => Promise<void>;
      onAnonymous?: () => Promise<void>;
      onPasskey?: (email: string) => Promise<void>;
      onRequestMagicLink?: (email: string) => Promise<void>;
      onUpgradeAnonymous?: () => Promise<void>;
    }) => (
      <div>
        <input aria-label="Email" placeholder={emailPlaceholder} value={email} onChange={(event) => onEmailChange(event.target.value)} />
        <input aria-label="Password" placeholder={passwordPlaceholder} value={password} onChange={(event) => onPasswordChange(event.target.value)} />
        <button type="button" disabled={loading} onClick={() => void onSubmit()}>
          {mode === "register" ? "Create Account" : "Sign In"}
        </button>
        {onModeChange && (
          <button type="button" onClick={() => onModeChange(mode === "register" ? "login" : "register")}>
            {mode === "register" ? loginToggleLabel : registerToggleLabel}
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
        {methods.passkey && onPasskey && (
          <button type="button" onClick={() => void onPasskey(email)}>Sign in with a passkey</button>
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

describe("AuthForm", () => {
  const login = vi.fn();
  const register = vi.fn();

  const signInAnonymously = vi.fn();
  const signInWithPasskey = vi.fn();
  const signInWithOAuth = vi.fn();
  const requestMagicLink = vi.fn();
  const linkEmail = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      loading: false,
      user: null,
      error: null,
      token: null,
      refreshToken: null,
      login,
      register,
      signInAnonymously,
      signInWithPasskey,
      requestMagicLink,
      confirmMagicLink: vi.fn(),
      linkEmail,
      signInWithOAuth,
      logout: vi.fn(),
      refresh: vi.fn(),
    });
  });

  it("renders login mode by default", () => {
    render(<AuthForm onAuth={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Sign In" })).toBeInTheDocument();
    expect(screen.getByText("alice@demo.test")).toBeInTheDocument();
  });

  it("switches to register mode and hides demo account quick picks", () => {
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Register" }));

    expect(screen.getByRole("button", { name: "Create Account" })).toBeInTheDocument();
    expect(screen.queryByText("alice@demo.test")).not.toBeInTheDocument();
  });

  it("submits with register() in register mode", async () => {
    register.mockResolvedValueOnce(undefined);
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "new@test.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "Create Account" }));

    await waitFor(() => expect(register).toHaveBeenCalledOnce());
    expect(register).toHaveBeenCalledWith("new@test.com", "password123");
    expect(login).not.toHaveBeenCalled();
  });

  it("uses live-polls placeholders and register toggle copy", () => {
    render(<AuthForm onAuth={vi.fn()} />);

    expect(screen.getByPlaceholderText("Email")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Register" })).toBeInTheDocument();
  });

  it("switches subtitle copy in register mode", () => {
    render(<AuthForm onAuth={vi.fn()} />);

    expect(screen.getByText("Sign in to create and vote on polls")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(screen.getByText("Create your account")).toBeInTheDocument();
  });

  it("persists tokens and notifies app after successful login", async () => {
    login.mockResolvedValueOnce(undefined);
    const onAuth = vi.fn();
    render(<AuthForm onAuth={onAuth} />);

    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "user@test.com" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => expect(login).toHaveBeenCalledOnce());
    expect(mockPersistTokens).toHaveBeenCalledWith("user@test.com");
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(onAuth).toHaveBeenCalledWith("user@test.com");
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

  it("renders a passkey CTA wired to signInWithPasskey and preserves auth side effects", async () => {
    signInWithPasskey.mockResolvedValue(undefined);
    const onAuth = vi.fn();
    render(<AuthForm onAuth={onAuth} />);

    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "passkey@test.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in with a passkey" }));

    await waitFor(() => expect(signInWithPasskey).toHaveBeenCalledWith("passkey@test.com"));
    expect(mockPersistTokens).toHaveBeenCalledWith("passkey@test.com");
    expect(mockClearAnonymousBootstrapOptOut).toHaveBeenCalledOnce();
    expect(onAuth).toHaveBeenCalledWith("passkey@test.com");
  });
});
