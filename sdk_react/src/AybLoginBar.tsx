import { useMemo } from "react";
import { DemoSuggestionChip } from "./DemoSuggestionChip";
import type { AybAuthMethods, DemoSuggestion } from "./types";

interface AybLoginBarProps {
  methods: AybAuthMethods;
  loading: boolean;
  mode?: "login" | "register";
  submitLabel?: string;
  registerToggleLabel?: string;
  loginToggleLabel?: string;
  email: string;
  emailPlaceholder?: string;
  password: string;
  passwordPlaceholder?: string;
  error: string | null;
  demoSuggestions: DemoSuggestion[];
  onEmailChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onModeChange?: (mode: "login" | "register") => void;
  onSubmit: () => Promise<void>;
  onOAuth: () => Promise<void>;
  onAnonymous: () => Promise<void>;
}

export function AybLoginBar(props: AybLoginBarProps) {
  const {
    methods,
    loading,
    mode = "login",
    submitLabel,
    registerToggleLabel = "Sign up",
    loginToggleLabel = "Sign in",
    email,
    emailPlaceholder = "you@example.com",
    password,
    passwordPlaceholder = "At least 8 characters",
    error,
    demoSuggestions,
    onEmailChange,
    onPasswordChange,
    onModeChange,
    onSubmit,
    onOAuth,
    onAnonymous,
  } = props;
  const canSubmitPassword = useMemo(
    () => methods.password && email.length > 0 && password.length > 0,
    [methods.password, email, password],
  );

  return (
    <div>
      {methods.password && (
        <>
          <input
            aria-label="Email"
            placeholder={emailPlaceholder}
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
          />
          <input
            aria-label="Password"
            placeholder={passwordPlaceholder}
            type="password"
            value={password}
            onChange={(e) => onPasswordChange(e.target.value)}
          />
          <button type="button" disabled={loading || !canSubmitPassword} onClick={() => void onSubmit()}>
            {submitLabel ?? (mode === "register" ? "Create Account" : "Sign In")}
          </button>
          {onModeChange && (
            <p>
              {mode === "register" ? "Already have an account? " : "Need an account? "}
              <button
                type="button"
                disabled={loading}
                onClick={() => onModeChange(mode === "register" ? "login" : "register")}
              >
                {mode === "register" ? loginToggleLabel : registerToggleLabel}
              </button>
            </p>
          )}
        </>
      )}
      {methods.oauth && (
        <button type="button" disabled={loading} onClick={() => void onOAuth()}>
          Continue with OAuth
        </button>
      )}
      {methods.anonymous && (
        <button type="button" disabled={loading} onClick={() => void onAnonymous()}>
          Continue as Guest
        </button>
      )}
      {methods.canUpgradeAnonymous && <p>Upgrade your guest account</p>}
      {error && <p role="alert">{error}</p>}
      <div>
        {demoSuggestions.map((suggestion) => (
          <DemoSuggestionChip
            key={suggestion.email}
            suggestion={suggestion}
            onSelect={(selected) => {
              onEmailChange(selected.email);
              onPasswordChange(selected.password);
            }}
          />
        ))}
      </div>
    </div>
  );
}
