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
      onEmailChange,
      onPasswordChange,
      onModeChange,
      onSubmit,
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
      onEmailChange: (value: string) => void;
      onPasswordChange: (value: string) => void;
      onModeChange?: (mode: "login" | "register") => void;
      onSubmit: () => Promise<void>;
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
      signInAnonymously: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
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
});
