import { AYBClient } from "@allyourbase/js";

const TOKEN_KEY = "ayb_token";
const REFRESH_KEY = "ayb_refresh_token";
const EMAIL_KEY = "ayb_email";
const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";

export const ayb = new AYBClient(
  import.meta.env.VITE_AYB_URL ?? "http://localhost:8090",
  {
    authPersistence: {
      load: () => {
        const token = sessionStorage.getItem(TOKEN_KEY);
        const refreshToken = sessionStorage.getItem(REFRESH_KEY);
        if (!token || !refreshToken) {
          return null;
        }
        return { token, refreshToken };
      },
      save: ({ token, refreshToken }) => {
        // Keep demo auth tokens scoped to the current browser tab.
        sessionStorage.setItem(TOKEN_KEY, token);
        sessionStorage.setItem(REFRESH_KEY, refreshToken);
      },
      clear: () => {
        sessionStorage.removeItem(TOKEN_KEY);
        sessionStorage.removeItem(REFRESH_KEY);
      },
    },
  },
);

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
  sessionStorage.removeItem(REFRESH_KEY);
  localStorage.removeItem(EMAIL_KEY);
  ayb.clearTokens();
}

export function getPersistedEmail(): string | null {
  return localStorage.getItem(EMAIL_KEY);
}

export function isLoggedIn(): boolean {
  return ayb.token !== null;
}
