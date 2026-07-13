/** @module Browser-test fixtures for OAuth client registration, authorization, consent, and token exchange. */
import type { APIRequestContext } from "@playwright/test";
import { createHash, randomBytes } from "crypto";
import { validateResponse } from "./core";

export interface OAuthPKCEPair {
  codeVerifier: string;
  codeChallenge: string;
  codeChallengeMethod: "S256";
}

export function generateOAuthPKCEPair(codeVerifier?: string): OAuthPKCEPair {
  const verifier = codeVerifier ?? randomBytes(32).toString("base64url");
  const codeChallenge = createHash("sha256").update(verifier).digest("base64url");
  return {
    codeVerifier: verifier,
    codeChallenge,
    codeChallengeMethod: "S256",
  };
}

export interface ParsedOAuthRedirectURL {
  code?: string;
  state?: string;
  error?: string;
  errorDescription?: string;
}

export function parseOAuthRedirectURL(redirectTo: string): ParsedOAuthRedirectURL {
  const redirectURL = new URL(redirectTo);
  const searchParams = redirectURL.searchParams;
  return {
    code: searchParams.get("code") || undefined,
    state: searchParams.get("state") || undefined,
    error: searchParams.get("error") || undefined,
    errorDescription: searchParams.get("error_description") || undefined,
  };
}

/** Creates an OAuth client via the admin API and returns its id, clientId, name, and secret. */
export async function seedOAuthClient(
  request: APIRequestContext,
  token: string,
  options: {
    appId: string;
    name: string;
    clientType?: "confidential" | "public";
    redirectUris?: string[];
    scopes?: string[];
  },
): Promise<{ id: string; clientId: string; name: string; clientSecret?: string }> {
  const res = await request.post("/api/admin/oauth/clients", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      appId: options.appId,
      name: options.name,
      clientType: options.clientType || "confidential",
      redirectUris: options.redirectUris || ["https://example.test/callback"],
      scopes: options.scopes || ["readonly"],
    },
  });
  await validateResponse(res, `Create OAuth client ${options.name}`);
  const body = await res.json();
  const id = body?.client?.id;
  const clientId = body?.client?.clientId;
  const name = body?.client?.name;
  const clientSecret = body?.clientSecret;
  if (typeof id !== "string" || typeof clientId !== "string" || typeof name !== "string") {
    throw new Error(`Expected OAuth client id/clientId/name for ${options.name}`);
  }
  if (clientSecret !== undefined && typeof clientSecret !== "string") {
    throw new Error(`Expected OAuth client secret to be a string when present for ${options.name}`);
  }
  return { id, clientId, name, clientSecret };
}

/** Lists OAuth clients and deletes all that match the given name. */
export async function cleanupOAuthClientByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.get("/api/admin/oauth/clients?perPage=200", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List OAuth clients for cleanup ${name}`);
  const body = await res.json();
  const clients = Array.isArray(body?.items) ? body.items : [];
  for (const client of clients) {
    if (client?.name === name && typeof client?.clientId === "string") {
      const deleteRes = await request.delete(
        `/api/admin/oauth/clients/${encodeURIComponent(client.clientId)}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      await validateResponse(deleteRes, `Revoke OAuth client ${name}`);
    }
  }
}

export interface OAuthAuthorizeRequestOptions {
  responseType: "code";
  clientId: string;
  redirectURI: string;
  scope: string;
  state: string;
  codeChallenge: string;
  codeChallengeMethod: "S256";
  allowedTables?: string[];
}

export interface OAuthConsentPromptResult {
  kind: "requires_consent";
  requiresConsent: true;
  clientID: string;
  clientName: string;
  redirectURI: string;
  scope: string;
  state: string;
  codeChallenge: string;
  codeChallengeMethod: string;
  allowedTables: string[];
}

export interface OAuthRedirectReadyResult {
  kind: "redirect_ready";
  requiresConsent: false;
  redirectTo: string;
}

export type OAuthAuthorizeResult = OAuthConsentPromptResult | OAuthRedirectReadyResult;

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

/** Parses an authorize response into a consent-prompt or redirect-ready discriminated union. */
function decodeAuthorizeResult(body: unknown): OAuthAuthorizeResult {
  const responseBody = body as Record<string, unknown>;
  if (responseBody.requires_consent === true) {
    if (
      typeof responseBody.client_id !== "string" ||
      typeof responseBody.client_name !== "string" ||
      typeof responseBody.redirect_uri !== "string" ||
      typeof responseBody.scope !== "string" ||
      typeof responseBody.state !== "string" ||
      typeof responseBody.code_challenge !== "string" ||
      typeof responseBody.code_challenge_method !== "string"
    ) {
      throw new Error("Invalid OAuth authorize consent response shape");
    }
    return {
      kind: "requires_consent",
      requiresConsent: true,
      clientID: responseBody.client_id,
      clientName: responseBody.client_name,
      redirectURI: responseBody.redirect_uri,
      scope: responseBody.scope,
      state: responseBody.state,
      codeChallenge: responseBody.code_challenge,
      codeChallengeMethod: responseBody.code_challenge_method,
      allowedTables: toStringArray(responseBody.allowed_tables),
    };
  }

  if (typeof responseBody.redirect_to !== "string") {
    throw new Error("Invalid OAuth authorize redirect response shape");
  }

  return {
    kind: "redirect_ready",
    requiresConsent: false,
    redirectTo: responseBody.redirect_to,
  };
}

/** Sends a PKCE authorize request and returns the parsed consent-or-redirect result. */
export async function authorizeOAuthRequest(
  request: APIRequestContext,
  token: string,
  options: OAuthAuthorizeRequestOptions,
): Promise<OAuthAuthorizeResult> {
  const query = new URLSearchParams({
    response_type: options.responseType,
    client_id: options.clientId,
    redirect_uri: options.redirectURI,
    scope: options.scope,
    state: options.state,
    code_challenge: options.codeChallenge,
    code_challenge_method: options.codeChallengeMethod,
  });
  for (const table of options.allowedTables || []) {
    query.append("allowed_tables", table);
  }

  const response = await request.get(`/api/auth/authorize?${query.toString()}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
    },
  });
  await validateResponse(response, "OAuth authorize request");
  return decodeAuthorizeResult(await response.json());
}

export interface OAuthConsentRequestOptions extends OAuthAuthorizeRequestOptions {
  decision: "approve" | "deny";
}

/** Posts an approval or denial to the consent endpoint and returns the redirect URL. */
export async function submitOAuthConsent(
  request: APIRequestContext,
  token: string,
  options: OAuthConsentRequestOptions,
): Promise<OAuthRedirectReadyResult> {
  const data: Record<string, unknown> = {
    decision: options.decision,
    response_type: options.responseType,
    client_id: options.clientId,
    redirect_uri: options.redirectURI,
    scope: options.scope,
    state: options.state,
    code_challenge: options.codeChallenge,
    code_challenge_method: options.codeChallengeMethod,
  };
  if (options.allowedTables !== undefined) {
    data.allowed_tables = options.allowedTables;
  }

  const response = await request.post("/api/auth/authorize/consent", {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    data,
  });
  await validateResponse(response, "OAuth consent request");
  const body = await response.json();
  if (typeof body?.redirect_to !== "string") {
    throw new Error("Invalid OAuth consent redirect response shape");
  }
  return {
    kind: "redirect_ready",
    requiresConsent: false,
    redirectTo: body.redirect_to,
  };
}

export interface OAuthTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope: string;
}

interface OAuthClientAuthBasic {
  method: "basic";
  clientId: string;
  clientSecret: string;
}

interface OAuthClientAuthBody {
  method: "body";
  clientId: string;
  clientSecret?: string;
}

export type OAuthClientAuth = OAuthClientAuthBasic | OAuthClientAuthBody;

interface TokenRequestCommonOptions {
  clientAuth: OAuthClientAuth;
}

/** Applies Basic auth headers or sets client credentials as form body params depending on auth method. */
function applyOAuthClientAuth(
  form: URLSearchParams,
  headers: Record<string, string>,
  clientAuth: OAuthClientAuth,
): void {
  if (clientAuth.method === "basic") {
    const encodedCredentials = Buffer.from(
      `${clientAuth.clientId}:${clientAuth.clientSecret}`,
      "utf-8",
    ).toString("base64");
    headers.Authorization = `Basic ${encodedCredentials}`;
    return;
  }

  form.set("client_id", clientAuth.clientId);
  if (clientAuth.clientSecret !== undefined) {
    form.set("client_secret", clientAuth.clientSecret);
  }
}

/** Validates and extracts the access_token, token_type, expires_in, scope, and optional refresh_token from a token response. */
function decodeOAuthTokenResponse(body: unknown): OAuthTokenResponse {
  const responseBody = body as Record<string, unknown>;
  if (
    typeof responseBody.access_token !== "string" ||
    typeof responseBody.token_type !== "string" ||
    typeof responseBody.expires_in !== "number" ||
    typeof responseBody.scope !== "string"
  ) {
    throw new Error("Invalid OAuth token response shape");
  }
  if (
    responseBody.refresh_token !== undefined &&
    typeof responseBody.refresh_token !== "string"
  ) {
    throw new Error("Invalid OAuth token response shape");
  }
  return responseBody as OAuthTokenResponse;
}

/** Posts a URL-encoded token form with client auth to /api/auth/token and returns the decoded token response. */
async function submitOAuthTokenForm(
  request: APIRequestContext,
  form: URLSearchParams,
  clientAuth: OAuthClientAuth,
): Promise<OAuthTokenResponse> {
  const headers: Record<string, string> = {
    "Content-Type": "application/x-www-form-urlencoded",
  };
  applyOAuthClientAuth(form, headers, clientAuth);
  const response = await request.post("/api/auth/token", {
    headers,
    data: form.toString(),
  });
  await validateResponse(response, "OAuth token exchange");
  return decodeOAuthTokenResponse(await response.json());
}

export interface OAuthAuthorizationCodeTokenOptions extends TokenRequestCommonOptions {
  code: string;
  redirectURI: string;
  codeVerifier: string;
}

export async function exchangeOAuthAuthorizationCode(
  request: APIRequestContext,
  options: OAuthAuthorizationCodeTokenOptions,
): Promise<OAuthTokenResponse> {
  if (options.codeVerifier === "") {
    throw new Error("codeVerifier is required");
  }
  const form = new URLSearchParams({
    grant_type: "authorization_code",
    code: options.code,
    redirect_uri: options.redirectURI,
    code_verifier: options.codeVerifier,
  });
  return submitOAuthTokenForm(request, form, options.clientAuth);
}

export interface OAuthAuthCodeTokenOptions {
  clientID: string;
  clientSecret: string;
  redirectURI: string;
  userToken: string;
  scope: string;
  state: string;
  codeVerifier: string;
  allowedTables?: string[];
}

/** Runs the full auth-code flow (authorize → consent → redirect → token exchange) and returns the access token. */
export async function mintOAuthAuthCodeToken(
  request: APIRequestContext,
  options: OAuthAuthCodeTokenOptions,
): Promise<string> {
  const pkce = generateOAuthPKCEPair(options.codeVerifier);

  const authorizeResult = await authorizeOAuthRequest(request, options.userToken, {
    responseType: "code",
    clientId: options.clientID,
    redirectURI: options.redirectURI,
    scope: options.scope,
    state: options.state,
    codeChallenge: pkce.codeChallenge,
    codeChallengeMethod: pkce.codeChallengeMethod,
    allowedTables: options.allowedTables,
  });

  const redirectTo =
    authorizeResult.kind === "requires_consent"
      ? (
          await submitOAuthConsent(request, options.userToken, {
            decision: "approve",
            responseType: "code",
            clientId: options.clientID,
            redirectURI: options.redirectURI,
            scope: options.scope,
            state: options.state,
            codeChallenge: pkce.codeChallenge,
            codeChallengeMethod: pkce.codeChallengeMethod,
            allowedTables: options.allowedTables,
          })
        ).redirectTo
      : authorizeResult.redirectTo;

  const redirect = parseOAuthRedirectURL(redirectTo);
  if (redirect.error) {
    throw new Error(`OAuth authorization failed: ${redirect.error}`);
  }
  if (!redirect.code) {
    throw new Error("OAuth authorization did not return a code");
  }
  if (redirect.state !== options.state) {
    throw new Error("OAuth authorization returned unexpected state");
  }

  const tokenResponse = await exchangeOAuthAuthorizationCode(request, {
    code: redirect.code,
    redirectURI: options.redirectURI,
    codeVerifier: pkce.codeVerifier,
    clientAuth: {
      method: "body",
      clientId: options.clientID,
      clientSecret: options.clientSecret,
    },
  });
  if (tokenResponse.access_token.length === 0) {
    throw new Error("OAuth authorization code exchange returned an empty access token");
  }
  return tokenResponse.access_token;
}

export interface OAuthClientCredentialsTokenOptions extends TokenRequestCommonOptions {
  scope: string;
  allowedTables?: string[];
}

export async function exchangeOAuthClientCredentials(
  request: APIRequestContext,
  options: OAuthClientCredentialsTokenOptions,
): Promise<OAuthTokenResponse> {
  const form = new URLSearchParams({
    grant_type: "client_credentials",
    scope: options.scope,
  });
  for (const table of options.allowedTables || []) {
    form.append("allowed_tables", table);
  }
  return submitOAuthTokenForm(request, form, options.clientAuth);
}

export interface OAuthRefreshTokenOptions extends TokenRequestCommonOptions {
  refreshToken: string;
}

export async function exchangeOAuthRefreshToken(
  request: APIRequestContext,
  options: OAuthRefreshTokenOptions,
): Promise<OAuthTokenResponse> {
  const form = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: options.refreshToken,
  });
  return submitOAuthTokenForm(request, form, options.clientAuth);
}
