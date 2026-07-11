import type { APIRequestContext, Page, TestInfo } from "@playwright/test";
import {
  buildParallelSafeRunID,
  cleanupAdminAppByName,
  cleanupAuthUser,
  cleanupOAuthClientByName,
  createLinkedEmailAuthSessionToken,
  ensureAuthSettings,
  expect,
  fetchAuthSettings,
  generateOAuthPKCEPair,
  getAuthSettingsUnavailableSkipReason,
  parseOAuthRedirectURL,
  probeEndpoint,
  resolveAuthUserIdByEmail,
  seedAdminApp,
  seedOAuthClient,
  test,
} from "../fixtures";

interface CleanupState {
  anonymousAuthEnabled?: boolean;
  appName?: string;
  clientName?: string;
  email?: string;
}

interface OAuthConsentContext {
  clientID: string;
  clientName: string;
  redirectURI: string;
  userToken: string;
}

interface OAuthConsentOptions {
  scope?: "readonly" | "readwrite";
  allowedTables?: string[];
}

interface OAuthAuthorizeRequest {
  path: string;
  state: string;
}

const cleanupByTestID = new Map<string, CleanupState>();

function requireBaseURL(baseURL: string | undefined): string {
  if (!baseURL) {
    throw new Error("PLAYWRIGHT_BASE_URL is required for OAuth consent callback assertions");
  }
  return baseURL;
}

function buildAuthorizeRequest(
  context: OAuthConsentContext,
  options: OAuthConsentOptions = {},
): OAuthAuthorizeRequest {
  const pkce = generateOAuthPKCEPair();
  const state = `state-${crypto.randomUUID()}`;
  const params = new URLSearchParams({
    response_type: "code",
    client_id: context.clientID,
    redirect_uri: context.redirectURI,
    scope: options.scope || "readonly",
    state,
    code_challenge: pkce.codeChallenge,
    code_challenge_method: pkce.codeChallengeMethod,
  });
  for (const tableName of options.allowedTables || []) {
    params.append("allowed_tables", tableName);
  }
  return {
    path: `/oauth/authorize?${params.toString()}`,
    state,
  };
}

async function installOAuthPageToken(page: Page, token: string): Promise<void> {
  await page.addInitScript((authToken: string) => {
    window.localStorage.setItem("ayb_admin_token", authToken);
    window.localStorage.removeItem("ayb_auth_token");
  }, token);
}

async function setupOAuthConsentContext(
  request: APIRequestContext,
  adminToken: string,
  testInfo: TestInfo,
  baseURL: string | undefined,
): Promise<OAuthConsentContext> {
  const runID = buildParallelSafeRunID(testInfo);
  const originalAuthSettings = await fetchAuthSettings(request, adminToken);
  const email = `oauth-consent-${runID}@example.com`;
  const password = `OAuthConsent!${runID}`;
  const appName = `oauth-consent-app-${runID}`;
  const clientName = `oauth-consent-client-${runID}`;
  const redirectURI = new URL(`/oauth-consent-callback/${runID}`, requireBaseURL(baseURL)).toString();

  cleanupByTestID.set(testInfo.testId, {
    anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
    appName,
    clientName,
    email,
  });

  await ensureAuthSettings(request, adminToken, { anonymous_auth_enabled: true });
  const userToken = await createLinkedEmailAuthSessionToken(request, email, password);
  const userID = await resolveAuthUserIdByEmail(request, adminToken, email);
  const app = await seedAdminApp(request, adminToken, {
    name: appName,
    ownerUserId: userID,
    description: `OAuth consent page proof ${runID}`,
  });
  const client = await seedOAuthClient(request, adminToken, {
    appId: app.id,
    name: clientName,
    clientType: "public",
    redirectUris: [redirectURI],
    scopes: ["readonly", "readwrite"],
  });

  return {
    clientID: client.clientId,
    clientName,
    redirectURI,
    userToken,
  };
}

async function skipWhenOAuthConsentDependenciesUnavailable(
  request: APIRequestContext,
  adminToken: string,
): Promise<void> {
  const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/oauth/clients");
  test.skip(
    probeStatus === 404 || probeStatus === 501,
    `OAuth clients service not configured (status ${probeStatus})`,
  );

  const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
  test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "");
}

test.describe("OAuth Consent Page (Full E2E)", () => {
  test.afterEach(async ({ request, adminToken }, testInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }
    if (cleanup.clientName) {
      await cleanupOAuthClientByName(request, adminToken, cleanup.clientName).catch(() => {});
    }
    if (cleanup.appName) {
      await cleanupAdminAppByName(request, adminToken, cleanup.appName).catch(() => {});
    }
    if (cleanup.email) {
      await cleanupAuthUser(request, adminToken, cleanup.email).catch(() => {});
    }
    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }
    cleanupByTestID.delete(testInfo.testId);
  });

  test("shows authorization error when required parameters are missing", async ({ page }) => {
    await page.goto("/oauth/authorize");

    await expect(page.getByRole("heading", { name: "Authorization Error" })).toBeVisible();
    await expect(page.getByText("Missing required parameters")).toBeVisible();
  });

  test("redirects unauthenticated authorize requests to the login handoff", async (
    { page, request, adminToken, baseURL },
    testInfo,
  ) => {
    await skipWhenOAuthConsentDependenciesUnavailable(request, adminToken);
    const context = await setupOAuthConsentContext(request, adminToken, testInfo, baseURL);
    const authorizeRequest = buildAuthorizeRequest(context);

    await page.context().addInitScript(() => {
      window.localStorage.removeItem("ayb_admin_token");
      window.localStorage.removeItem("ayb_auth_token");
    });

    await page.goto(authorizeRequest.path);
    await page.waitForURL(/\/\?return_to=/);

    const redirectedURL = new URL(page.url());
    expect(redirectedURL.pathname).toBe("/");
    const returnTo = redirectedURL.searchParams.get("return_to");
    expect(returnTo).toBe(authorizeRequest.path);
  });

  test("renders authenticated consent prompt with requested permissions", async (
    { page, request, adminToken, baseURL },
    testInfo,
  ) => {
    await skipWhenOAuthConsentDependenciesUnavailable(request, adminToken);
    const context = await setupOAuthConsentContext(request, adminToken, testInfo, baseURL);
    await installOAuthPageToken(page, context.userToken);

    await page.goto(buildAuthorizeRequest(context, {
      scope: "readwrite",
      allowedTables: ["orders", "profiles"],
    }).path);

    await expect(page.getByRole("heading", { name: "Authorization Request" })).toBeVisible();
    await expect(page.getByText(context.clientName)).toBeVisible();
    await expect(page.getByText("Permissions requested")).toBeVisible();
    await expect(page.getByText("Read and modify your data")).toBeVisible();
    await expect(page.getByText("Tables: orders, profiles")).toBeVisible();
    await expect(page.getByRole("button", { name: "Deny" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Approve" })).toBeVisible();
  });

  test("redirects denied consent with access_denied and original state", async (
    { page, request, adminToken, baseURL },
    testInfo,
  ) => {
    await skipWhenOAuthConsentDependenciesUnavailable(request, adminToken);
    const context = await setupOAuthConsentContext(request, adminToken, testInfo, baseURL);
    await installOAuthPageToken(page, context.userToken);
    const authorizeRequest = buildAuthorizeRequest(context);
    await page.goto(authorizeRequest.path);

    await page.getByRole("button", { name: "Deny" }).click();
    await page.waitForURL(new RegExp(`${new URL(context.redirectURI).pathname}.*error=access_denied`));

    const redirect = parseOAuthRedirectURL(page.url());
    expect(redirect.error).toBe("access_denied");
    expect(redirect.state).toBe(authorizeRequest.state);
  });

  test("redirects approved consent with authorization code and original state", async (
    { page, request, adminToken, baseURL },
    testInfo,
  ) => {
    await skipWhenOAuthConsentDependenciesUnavailable(request, adminToken);
    const context = await setupOAuthConsentContext(request, adminToken, testInfo, baseURL);
    await installOAuthPageToken(page, context.userToken);
    const authorizeRequest = buildAuthorizeRequest(context);
    await page.goto(authorizeRequest.path);

    await page.getByRole("button", { name: "Approve" }).click();
    await page.waitForURL(new RegExp(`${new URL(context.redirectURI).pathname}.*code=`));

    const redirect = parseOAuthRedirectURL(page.url());
    expect(redirect.error).toBeUndefined();
    expect(redirect.code).toBeTruthy();
    expect(redirect.state).toBe(authorizeRequest.state);
  });
});
