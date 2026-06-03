/**
 * @module OAuth-capable authentication client supporting email/password login, token management, and both popup and redirect-based OAuth flows.
 */
import { AYBError } from "./errors";
import {
  normalizeAuthResponse,
  normalizeMagicLinkConfirmResponse,
  normalizeUser,
  normalizeWebAuthnLoginBeginResponse,
  normalizeWebAuthnMFAChallengeResponse,
  openPopup,
} from "./helpers";
import { createPasskeyAssertion, createPasskeyAttestation } from "./webauthn";
import type {
  AuthResponse,
  MagicLinkConfirmResponse,
  MagicLinkRequestResponse,
  OAuthOptions,
  OAuthProvider,
  User,
  WebAuthnEnrollBeginResponse,
  WebAuthnLoginBeginResponse,
  WebAuthnLoginFinishRequest,
} from "./types";

interface AuthClientRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
  refreshToken: string | null;
  setTokens(token: string, refreshToken: string): void;
  clearTokens(): void;
  emitAuthEvent(event: "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED"): void;
  getBaseURL(): string;
}

/**
 * Manages email/password authentication, OAuth sign-in, token refresh, password reset, email verification, and account operations.
 */
export class AuthClient {
  constructor(private client: AuthClientRuntime) {}

  /** Register a new user account. */
  async register(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    return this.applySignedInSession(res);
  }

  /** Log in with email and password. */
  async login(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    return this.applySignedInSession(res);
  }

  /** Sign in with an anonymous account. */
  async signInAnonymously(): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/anonymous", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
    return this.applySignedInSession(res);
  }

  /** Send a passwordless sign-in email. */
  async requestMagicLink(email: string): Promise<MagicLinkRequestResponse> {
    return this.client.request<MagicLinkRequestResponse>("/api/auth/magic-link", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
  }

  /** Confirm a passwordless sign-in token. */
  async confirmMagicLink(token: string): Promise<MagicLinkConfirmResponse> {
    const res = await this.client.request<unknown>("/api/auth/magic-link/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
    const normalized = normalizeMagicLinkConfirmResponse(res);
    if ("token" in normalized) {
      return this.applySignedInSession(normalized);
    }
    return normalized;
  }

  /** Begin a first-factor WebAuthn login challenge for an email. */
  async beginWebAuthnLogin(email: string): Promise<WebAuthnLoginBeginResponse> {
    const response = await this.client.request<unknown>("/api/auth/webauthn/login/begin", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
    return normalizeWebAuthnLoginBeginResponse(response);
  }

  /** Verify a first-factor WebAuthn assertion and establish a signed-in session. */
  async finishWebAuthnLogin(
    challengeId: string,
    assertionResponse: Record<string, unknown>,
  ): Promise<AuthResponse> {
    const payload: WebAuthnLoginFinishRequest = { challengeId, assertionResponse };
    const response = await this.client.request<AuthResponse>("/api/auth/webauthn/login/finish", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        challenge_id: payload.challengeId,
        assertion_response: payload.assertionResponse,
      }),
    });
    return this.applySignedInSession(response);
  }

  /**
   * Run the full browser WebAuthn first-factor login ceremony.
   * This keeps browser assertion serialization in the SDK owner.
   */
  async signInWithPasskey(email: string): Promise<AuthResponse> {
    const begin = await this.beginWebAuthnLogin(email);
    const assertionResponse = await createPasskeyAssertion(begin.options);
    return this.finishWebAuthnLogin(begin.challengeId, assertionResponse);
  }

  /**
   * Enroll a WebAuthn passkey as a second factor for the currently signed-in user.
   * Requires an active session (the regular session bearer is attached by client.request).
   */
  async enrollPasskey(displayName?: string): Promise<void> {
    const creationOptions = await this.client.request<WebAuthnEnrollBeginResponse>(
      "/api/auth/mfa/webauthn/enroll",
      { method: "POST" },
    );
    const attestationResponse = await createPasskeyAttestation(creationOptions);
    await this.client.request<void>("/api/auth/mfa/webauthn/enroll/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        display_name: displayName ?? "",
        attestation_response: attestationResponse,
      }),
    });
  }

  /**
   * Complete the second-factor WebAuthn verification for an mfa-pending session.
   * The `mfaToken` is the short-lived token returned in a prior
   * `{mfa_pending: true, mfa_token}` envelope; it is sent as the Authorization
   * header for both /challenge and /verify, bypassing any current session token.
   */
  async verifyPasskey(mfaToken: string): Promise<AuthResponse> {
    const mfaAuthHeader = { Authorization: `Bearer ${mfaToken}` };
    const challengeRaw = await this.client.request<unknown>(
      "/api/auth/mfa/webauthn/challenge",
      { method: "POST", skipAuth: true, headers: mfaAuthHeader },
    );
    const challenge = normalizeWebAuthnMFAChallengeResponse(challengeRaw);
    const assertionResponse = await createPasskeyAssertion(challenge.options);
    const response = await this.client.request<AuthResponse>(
      "/api/auth/mfa/webauthn/verify",
      {
        method: "POST",
        skipAuth: true,
        headers: { ...mfaAuthHeader, "Content-Type": "application/json" },
        body: JSON.stringify({
          challenge_id: challenge.challengeId,
          assertion_response: assertionResponse,
        }),
      },
    );
    return this.applySignedInSession(response);
  }

  /** Convert an anonymous account to email/password auth. */
  async linkEmail(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/link/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    return this.applySignedInSession(res);
  }

  /** Refresh the access token using the stored refresh token. */
  async refresh(): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    const normalized = normalizeAuthResponse(res);
    this.client.setTokens(normalized.token, normalized.refreshToken);
    this.client.emitAuthEvent("TOKEN_REFRESHED");
    return normalized;
  }

  /** Log out (revoke the refresh token). */
  async logout(): Promise<void> {
    await this.client.request<void>("/api/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Get the current authenticated user. */
  async me(): Promise<User> {
    const user = await this.client.request<User>("/api/auth/me");
    return normalizeUser(user);
  }

  /** Delete the current authenticated user's account. */
  async deleteAccount(): Promise<void> {
    await this.client.request<void>("/api/auth/me", { method: "DELETE" });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Request a password reset email. */
  async requestPasswordReset(email: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
  }

  /** Confirm a password reset with a token. */
  async confirmPasswordReset(token: string, password: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password }),
    });
  }

  /** Verify an email address with a token. */
  async verifyEmail(token: string): Promise<void> {
    await this.client.request<void>("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
  }

  /** Resend the email verification (requires auth). */
  async resendVerification(): Promise<void> {
    await this.client.request<void>("/api/auth/verify/resend", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
  }

  /**
   * Sign in with an OAuth provider using a popup + SSE flow.
   * Opens a popup immediately to avoid browser popup blockers.
   */
  /**
   * Initiate OAuth sign-in via popup or custom redirect flow. Opens popup to bypass browser blockers.
   * @param provider - OAuth provider identifier
   * @param options - Optional scopes array and urlCallback for custom redirect handling
   * @returns AuthResponse with tokens and user data
   * @throws If popup is blocked by browser or OAuth flow fails
   */
  async signInWithOAuth(
    provider: OAuthProvider,
    options?: OAuthOptions,
  ): Promise<AuthResponse> {
    let popup: Window | null = null;
    if (!options?.urlCallback) {
      popup = openPopup();
      if (!popup) {
        throw new AYBError(
          403,
          "Popup was blocked by the browser. Use urlCallback for redirect flow.",
          "oauth/popup-blocked",
        );
      }
    }

    try {
      const { clientId, waitForAuth, close } = await this.connectOAuthSSE();
      let oauthURL = `${this.client.getBaseURL()}/api/auth/oauth/${provider}?state=${clientId}`;
      if (options?.scopes?.length) {
        oauthURL += `&scopes=${encodeURIComponent(options.scopes.join(","))}`;
      }
      // Per-request OAuth redirect target. Server-side validation lives in
      // internal/auth/handler_oauth.go (host-allowlist + scheme/userinfo
      // checks at both start and callback dispatch); the SDK is the
      // transport, not a validator. See OAuthOptions.redirectTo doc.
      if (options?.redirectTo) {
        oauthURL += `&redirect_to=${encodeURIComponent(options.redirectTo)}`;
      }

      if (options?.urlCallback) {
        await options.urlCallback(oauthURL);
      } else if (popup) {
        popup.location.href = oauthURL;
      }

      const result = await waitForAuth(popup);
      this.client.setTokens(result.token, result.refreshToken);
      this.client.emitAuthEvent("SIGNED_IN");
      close();
      return result;
    } catch (err) {
      popup?.close();
      throw err;
    }
  }

  /**
   * Parse OAuth tokens from a URL hash fragment after redirect-based OAuth.
   */
  /**
   * Parse OAuth response tokens from URL hash and apply them internally. Cleans up the URL history.
   * @returns AuthResponse with token and refreshToken if present, otherwise null
   */
  handleOAuthRedirect(): AuthResponse | null {
    if (typeof window === "undefined") return null;
    const hash = window.location.hash;
    if (!hash) return null;
    const params = new URLSearchParams(hash.slice(1));
    const token = params.get("token");
    const refreshToken = params.get("refreshToken");
    if (!token || !refreshToken) return null;
    this.client.setTokens(token, refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    window.history.replaceState(
      null,
      "",
      window.location.pathname + window.location.search,
    );
    return { token, refreshToken, user: {} as User };
  }

  /**
   * Establish an EventSource connection for OAuth authentication events.
   * @returns Object with clientId for state parameter, waitForAuth function to handle OAuth completion, and close function to clean up the connection
   * @throws If SSE connection fails to establish
   */
  private connectOAuthSSE(): Promise<{
    clientId: string;
    waitForAuth: (popup: Window | null) => Promise<AuthResponse>;
    close: () => void;
  }> {
    return new Promise((resolve, reject) => {
      const url = `${this.client.getBaseURL()}/api/realtime?oauth=true`;
      const es = new EventSource(url);
      let settled = false;

      const cleanup = () => {
        es.close();
      };

      es.addEventListener("connected", (e: MessageEvent) => {
        const data = JSON.parse(e.data) as { clientId: string };

        const waitForAuth = (popup: Window | null): Promise<AuthResponse> => {
          return new Promise<AuthResponse>((resolveAuth, rejectAuth) => {
            const timeout = setTimeout(() => {
              cleanup();
              rejectAuth(new AYBError(408, "OAuth sign-in timed out", "oauth/timeout"));
            }, 5 * 60 * 1000);

            let popupPoll: ReturnType<typeof setInterval> | undefined;
            if (popup) {
              popupPoll = setInterval(() => {
                if (popup.closed) {
                  clearInterval(popupPoll);
                  clearTimeout(timeout);
                  cleanup();
                  rejectAuth(
                    new AYBError(
                      499,
                      "OAuth popup was closed by the user",
                      "oauth/popup-closed",
                    ),
                  );
                }
              }, 500);
            }

            es.addEventListener("oauth", (oauthEvt: MessageEvent) => {
              clearTimeout(timeout);
              if (popupPoll) clearInterval(popupPoll);
              popup?.close();

              const result = JSON.parse(oauthEvt.data) as {
                token?: string;
                refreshToken?: string;
                user?: User;
                error?: string;
              };

              if (result.error) {
                cleanup();
                rejectAuth(new AYBError(401, result.error, "oauth/provider-error"));
                return;
              }

              if (!result.token || !result.refreshToken) {
                cleanup();
                rejectAuth(
                  new AYBError(500, "OAuth response missing tokens", "oauth/missing-tokens"),
                );
                return;
              }

              resolveAuth({
                token: result.token,
                refreshToken: result.refreshToken,
                user: result.user ? normalizeUser(result.user as User) : ({} as User),
              });
            });
          });
        };

        resolve({ clientId: data.clientId, waitForAuth, close: cleanup });
      });

      es.onerror = () => {
        if (!settled) {
          settled = true;
          cleanup();
          reject(
            new AYBError(
              503,
              "Failed to connect to OAuth SSE channel",
              "oauth/sse-failed",
            ),
          );
        }
      };
    });
  }

  private applySignedInSession(response: AuthResponse): AuthResponse {
    const normalized = normalizeAuthResponse(response);
    this.client.setTokens(normalized.token, normalized.refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    return normalized;
  }
}
