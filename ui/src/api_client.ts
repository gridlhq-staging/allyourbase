/**
 * @module Dashboard API client helpers for token-backed requests and origin validation.
 */
const ADMIN_TOKEN_KEY = "ayb_admin_token";
const AUTH_TOKEN_KEY = "ayb_auth_token";
const CROSS_ORIGIN_API_REQUEST_ERROR = "Cross-origin API requests are not allowed";
const INVALID_API_REQUEST_URL_ERROR = "API request URL is invalid";

let configuredConsoleApiOrigin: string | null = null;

/**
 * Converts various header formats to a plain object record.
 * @param headersInit - Headers to normalize, supporting Headers instances, header tuples, or plain objects.
 * @returns A plain object mapping header names to values.
 */
function normalizeHeaders(headersInit?: HeadersInit): Record<string, string> {
  if (!headersInit) {
    return {};
  }
  if (headersInit instanceof Headers) {
    const headers: Record<string, string> = {};
    headersInit.forEach((value, key) => {
      headers[key] = value;
    });
    return headers;
  }
  if (Array.isArray(headersInit)) {
    return Object.fromEntries(headersInit);
  }
  return { ...headersInit };
}

export function getAdminToken(): string | null {
  return localStorage.getItem(ADMIN_TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(ADMIN_TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(ADMIN_TOKEN_KEY);
}

export function getAuthToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function setAuthToken(token: string) {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function clearAuthToken() {
  localStorage.removeItem(AUTH_TOKEN_KEY);
}

function dispatchUnauthorizedEvent(eventName: string, clearTokenFn: () => void) {
  clearTokenFn();
  window.dispatchEvent(new Event(eventName));
}

export function emitUnauthorized() {
  dispatchUnauthorizedEvent("ayb:unauthorized", clearToken);
}

export function resetConsoleApiOrigin(): void {
  configuredConsoleApiOrigin = null;
}

export function configureConsoleApiOrigin(origin: unknown): void {
  resetConsoleApiOrigin();
  configuredConsoleApiOrigin = parseConsoleApiOrigin(origin);
}

/**
 * Validates and normalizes a configured cross-origin console API origin.
 * @param origin - Candidate origin supplied by runtime configuration.
 * @returns The normalized origin string safe to compare against request URLs.
 */
function parseConsoleApiOrigin(origin: unknown): string {
  if (typeof origin !== "string" || origin.length === 0 || origin.trim() !== origin) {
    throw new Error("Console API origin must be an HTTP(S) URL origin");
  }

  let apiURL: URL;
  try {
    apiURL = new URL(origin);
  } catch {
    throw new Error("Console API origin must be an HTTP(S) URL origin");
  }

  if (apiURL.protocol !== "http:" && apiURL.protocol !== "https:") {
    throw new Error("Console API origin must use HTTP or HTTPS");
  }
  if (apiURL.username || apiURL.password || apiURL.pathname !== "/" || apiURL.search || apiURL.hash) {
    throw new Error("Console API origin must not include credentials, path, query, or fragment");
  }
  if (window.location.protocol === "https:" && apiURL.protocol === "http:") {
    throw new Error("Console API origin cannot downgrade HTTPS consoles to HTTP APIs");
  }

  return apiURL.origin;
}

function authorizedHeaders(
  headersInit: HeadersInit | undefined,
  token: string | null,
): Record<string, string> {
  const headers = normalizeHeaders(headersInit);
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  return headers;
}

function sameOriginRequestPath(path: string): string {
  let requestURL: URL;
  try {
    requestURL = new URL(path, window.location.href);
  } catch {
    throw new Error(INVALID_API_REQUEST_URL_ERROR);
  }
  if (requestURL.origin === window.location.origin) {
    return pageOriginRequestPath(path, requestURL);
  }
  if (requestURL.origin === configuredConsoleApiOrigin) {
    return requestURL.href;
  }
  throw new Error(CROSS_ORIGIN_API_REQUEST_ERROR);
}

function pageOriginRequestPath(path: string, requestURL: URL): string {
  if (!path.startsWith("//") && !isAbsoluteURL(path)) {
    return path;
  }
  return `${requestURL.pathname}${requestURL.search}${requestURL.hash}`;
}

function isAbsoluteURL(path: string): boolean {
  try {
    new URL(path);
  } catch {
    return false;
  }
  return true;
}

function fetchWithToken(path: string, init: RequestInit | undefined, token: string | null): Promise<Response> {
  return fetch(sameOriginRequestPath(path), {
    ...init,
    headers: authorizedHeaders(init?.headers, token),
  });
}

async function throwResponseError(
  res: Response,
  onUnauthorized?: () => void,
): Promise<never> {
  if (res.status === 401) {
    onUnauthorized?.();
  }
  const body = await res.json().catch(() => ({ message: res.statusText }));
  const retryAfterHeader = res.headers.get("Retry-After");
  const parsedRetryAfterSeconds =
    retryAfterHeader !== null ? Number.parseInt(retryAfterHeader, 10) : undefined;
  const retryAfterSeconds =
    parsedRetryAfterSeconds !== undefined && Number.isFinite(parsedRetryAfterSeconds) && parsedRetryAfterSeconds > 0
      ? parsedRetryAfterSeconds
      : undefined;
  throw new ApiError(
    res.status,
    body.message || res.statusText,
    retryAfterSeconds,
  );
}

export async function fetchAdmin(path: string, init?: RequestInit): Promise<Response> {
  return fetchWithToken(path, init, getAdminToken());
}

export async function throwApiError(res: Response): Promise<never> {
  if (!res.ok) {
    return throwResponseError(res, emitUnauthorized);
  }
  throw new Error("throwApiError called with an ok response");
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetchAdmin(path, init);
  if (!res.ok) {
    await throwApiError(res);
  }
  return res.json();
}

export async function requestNoBody(path: string, init?: RequestInit): Promise<void> {
  const res = await fetchAdmin(path, init);
  if (!res.ok) {
    await throwApiError(res);
  }
}

export async function requestAuth<T>(
  path: string,
  init?: RequestInit,
  includeAuthHeader = true,
): Promise<T> {
  const token = includeAuthHeader ? getAuthToken() : null;
  const res = await fetchWithToken(path, init, token);
  if (!res.ok) {
    return throwResponseError(res, () => dispatchUnauthorizedEvent("ayb:auth-unauthorized", clearAuthToken));
  }
  return res.json();
}

export async function requestAuthNoBody(
  path: string,
  init?: RequestInit,
  includeAuthHeader = true,
): Promise<void> {
  const token = includeAuthHeader ? getAuthToken() : null;
  const res = await fetchWithToken(path, init, token);
  if (!res.ok) {
    await throwResponseError(res, () => dispatchUnauthorizedEvent("ayb:auth-unauthorized", clearAuthToken));
  }
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public retryAfterSeconds?: number,
  ) {
    super(message);
  }
}
