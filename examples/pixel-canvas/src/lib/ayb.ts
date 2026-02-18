import { AYBClient } from "@allyourbase/js";

const url = import.meta.env.VITE_AYB_URL ?? "http://localhost:8090";
export const ayb = new AYBClient(url);

// Restore persisted tokens on load.
const token = localStorage.getItem("ayb_token");
const refresh = localStorage.getItem("ayb_refresh_token");
if (token && refresh) {
  ayb.setTokens(token, refresh);
}

export function persistTokens() {
  if (ayb.token) localStorage.setItem("ayb_token", ayb.token);
  if (ayb.refreshToken) localStorage.setItem("ayb_refresh_token", ayb.refreshToken);
}

export function clearPersistedTokens() {
  localStorage.removeItem("ayb_token");
  localStorage.removeItem("ayb_refresh_token");
  ayb.clearTokens();
}

export function isLoggedIn(): boolean {
  return !!ayb.token;
}
