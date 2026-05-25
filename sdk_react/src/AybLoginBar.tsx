import { useMemo } from "react";
import { DemoSuggestionChip } from "./DemoSuggestionChip";
import type { AybLoginBarProps, OAuthProvider } from "./types";

const OAUTH_PROVIDER_LABEL: Record<OAuthProvider, string> = {
  github: "Continue with GitHub",
  google: "Continue with Google",
};

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
    oauthProviders,
    onEmailChange,
    onPasswordChange,
    onModeChange,
    onSubmit,
    onOAuth,
    onAnonymous,
    onOAuthProvider,
    onRequestMagicLink,
    onUpgradeAnonymous,
  } = props;
  const canSubmitPassword = useMemo(
    () => methods.password && email.length > 0 && password.length > 0,
    [methods.password, email, password],
  );
  const canRequestMagicLink = useMemo(
    () => Boolean(methods.magicLink) && email.length > 0,
    [methods.magicLink, email],
  );
  const showEmailInput = methods.password || (methods.magicLink && onRequestMagicLink);

  return (
    <div>
      {showEmailInput && (
        <>
          <input
            aria-label="Email"
            placeholder={emailPlaceholder}
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
          />
        </>
      )}
      {methods.password && (
        <>
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
      {methods.magicLink && onRequestMagicLink && (
        <button
          type="button"
          disabled={loading || !canRequestMagicLink}
          onClick={() => void onRequestMagicLink(email)}
        >
          Email me a magic link
        </button>
      )}
      {methods.oauth && (
        oauthProviders && oauthProviders.length > 0 && onOAuthProvider ? (
          oauthProviders.map((provider) => (
            <button
              key={provider}
              type="button"
              disabled={loading}
              onClick={() => void onOAuthProvider(provider)}
            >
              {OAUTH_PROVIDER_LABEL[provider]}
            </button>
          ))
        ) : (
          <button type="button" disabled={loading} onClick={() => void onOAuth()}>
            Continue with OAuth
          </button>
        )
      )}
      {methods.anonymous && (
        <button type="button" disabled={loading} onClick={() => void onAnonymous()}>
          Continue as Guest
        </button>
      )}
      {methods.canUpgradeAnonymous && (
        onUpgradeAnonymous ? (
          <button
            type="button"
            disabled={loading || !canSubmitPassword}
            onClick={() => void onUpgradeAnonymous()}
          >
            Upgrade Account
          </button>
        ) : (
          <p>Upgrade your guest account</p>
        )
      )}
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
