import { AYBClient } from "@allyourbase/js";

const TOKEN_KEY = "ayb_token";
const REFRESH_KEY = "ayb_refresh_token";

export const ayb = new AYBClient(
  import.meta.env.VITE_AYB_URL || "http://localhost:8090",
);

// Restore tokens from localStorage on load
const savedToken = localStorage.getItem(TOKEN_KEY);
const savedRefresh = localStorage.getItem(REFRESH_KEY);
if (savedToken && savedRefresh) {
  ayb.setTokens(savedToken, savedRefresh);
}

export function persistTokens() {
  if (ayb.token && ayb.refreshToken) {
    localStorage.setItem(TOKEN_KEY, ayb.token);
    localStorage.setItem(REFRESH_KEY, ayb.refreshToken);
  }
}

export function clearPersistedTokens() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
}

export function isLoggedIn(): boolean {
  return ayb.token !== null;
}
