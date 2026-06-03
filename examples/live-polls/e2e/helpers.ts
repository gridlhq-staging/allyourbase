/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun02_pm_2_demos_green_browser_standards/allyourbase_dev/examples/live-polls/e2e/helpers.ts.
 */
import { type Page, type Locator, type CDPSession, type Request, expect } from "@playwright/test";

let userCounter = 0;
export const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique test user email to avoid collisions between test runs. */
export function uniqueEmail(): string {
  return `test-${runId}-${Date.now()}-${++userCounter}@example.com`;
}

export const TEST_PASSWORD = "testpassword123";

/** Demo account credentials. */
export const DEMO_ACCOUNTS = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
  { email: "charlie@demo.test", password: "password123" },
];

const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";
const ensuredDemoAccounts = new Set<string>();
interface VirtualAuthenticatorHandle {
  authenticatorId: string;
  remove: () => Promise<void>;
}
type ObservedAuthRequest = {
  pathname: string;
  method: string;
  payload: unknown;
};

const PASSKEY_BEGIN_ENDPOINT = "/api/auth/webauthn/login/begin";
const PASSKEY_FINISH_ENDPOINT = "/api/auth/webauthn/login/finish";
const PASSWORD_LOGIN_ENDPOINT = "/api/auth/login";

export interface PasskeyFirstFactorRequestObserver {
  stop: () => void;
  expectPasskeyOnlySignIn: (email: string) => void;
}

async function ensureDemoAccountExists(
  page: Page,
  acct: { email: string; password: string },
): Promise<void> {
  if (ensuredDemoAccounts.has(acct.email)) {
    return;
  }
  const register = await page.request.post("/api/auth/register", {
    data: { email: acct.email, password: acct.password },
  });
  if (register.status() === 201 || register.status() === 409) {
    ensuredDemoAccounts.add(acct.email);
    return;
  }
  throw new Error(
    `failed to provision demo account ${acct.email}: ${register.status()} ${await register.text()}`,
  );
}

/**
 * Ensure the explicit auth form is visible even when the app lands on the
 * anonymous shell by default.
 */
export async function openExplicitAuth(page: Page): Promise<void> {
  await page.addInitScript((optOutKey) => {
    localStorage.setItem(optOutKey, "1");
  }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
  await page.goto("/");
  const authFormReady = page.getByRole("button", { name: "Sign In", exact: true });
  const anonymousShellReady = page.getByRole("button", { name: "Sign out" });
  await Promise.race([
    authFormReady.waitFor({ state: "visible", timeout: 15000 }),
    anonymousShellReady.waitFor({ state: "visible", timeout: 15000 }),
  ]);
  if (await authFormReady.isVisible()) {
    return;
  }
  await anonymousShellReady.click();
  await expect(authFormReady).toBeVisible();
  await expect(page.getByPlaceholder("Email")).toBeVisible();
}

/**
 * Create one scoped Chromium virtual authenticator and return an explicit
 * teardown handle. Mirrors the CDP setup/cleanup contract used by
 * ui/browser-tests-unmocked/fixtures/auth.ts::createVirtualAuthenticator.
 */
export async function attachVirtualAuthenticator(
  page: Page,
): Promise<VirtualAuthenticatorHandle> {
  const session: CDPSession = await page.context().newCDPSession(page);
  await session.send("WebAuthn.enable");
  const response = (await session.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })) as { authenticatorId?: string };
  const authenticatorId = response?.authenticatorId;
  if (typeof authenticatorId !== "string" || authenticatorId.length === 0) {
    await session.send("WebAuthn.disable").catch(() => {});
    await session.detach().catch(() => {});
    throw new Error("CDP virtual authenticator setup succeeded but no authenticatorId was returned");
  }
  return {
    authenticatorId,
    remove: async () => {
      await session
        .send("WebAuthn.removeVirtualAuthenticator", { authenticatorId })
        .catch(() => {});
      await session.send("WebAuthn.disable").catch(() => {});
      await session.detach().catch(() => {});
    },
  };
}

export function observePasskeyFirstFactorRequests(
  page: Page,
): PasskeyFirstFactorRequestObserver {
  const observedAuthRequests: ObservedAuthRequest[] = [];
  const requestListener = (request: Request) => {
    const pathname = new URL(request.url()).pathname;
    if (!pathname.startsWith("/api/auth/")) {
      return;
    }
    let payload: unknown = null;
    try {
      payload = request.postDataJSON();
    } catch {
      payload = null;
    }
    observedAuthRequests.push({
      pathname,
      method: request.method(),
      payload,
    });
  };
  page.on("request", requestListener);

  return {
    stop: () => page.off("request", requestListener),
    expectPasskeyOnlySignIn: (email: string) => {
      const passkeyBeginRequests = observedAuthRequests.filter(
        (request) => request.pathname === PASSKEY_BEGIN_ENDPOINT,
      );
      const passkeyFinishRequests = observedAuthRequests.filter(
        (request) => request.pathname === PASSKEY_FINISH_ENDPOINT,
      );
      const passwordLoginRequests = observedAuthRequests.filter(
        (request) => request.pathname === PASSWORD_LOGIN_ENDPOINT,
      );

      expect(passkeyBeginRequests).toHaveLength(1);
      expect(passkeyBeginRequests[0].method).toBe("POST");
      expect(passkeyBeginRequests[0].payload).toEqual({ email });
      expect(passkeyFinishRequests).toHaveLength(1);
      expect(passkeyFinishRequests[0].method).toBe("POST");
      expect(passwordLoginRequests).toHaveLength(0);
      expect(
        observedAuthRequests.findIndex((request) => request.pathname === PASSKEY_BEGIN_ENDPOINT),
      ).toBeLessThan(
        observedAuthRequests.findIndex((request) => request.pathname === PASSKEY_FINISH_ENDPOINT),
      );
    },
  };
}

/**
 * Enroll one passkey for the currently signed-in browser session by calling the
 * real enrollment endpoints and generating attestation in-browser where
 * WebAuthn constructors are available.
 */
export async function enrollPasskeyForCurrentSession(
  page: Page,
  displayName: string,
): Promise<void> {
  await page.evaluate(
    async ({ passkeyDisplayName }: { passkeyDisplayName: string }) => {
      const decodeBase64URL = (value: string): ArrayBuffer => {
        const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
        const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
        const binary = window.atob(padded);
        const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
        return bytes.buffer;
      };

      const token = window.sessionStorage.getItem("ayb_token");
      if (!token) {
        throw new Error("passkey enrollment requires ayb_token in sessionStorage");
      }

      const beginResponse = await window.fetch("/api/auth/mfa/webauthn/enroll", {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!beginResponse.ok) {
        throw new Error(
          `passkey enroll begin failed: ${beginResponse.status} ${await beginResponse.text()}`,
        );
      }
      const options = await beginResponse.json();

      const publicKey: PublicKeyCredentialCreationOptions = {
        ...options,
        challenge: decodeBase64URL(String((options as { challenge?: unknown }).challenge ?? "")),
        user: {
          ...((options as { user?: unknown }).user as Record<string, unknown>),
          id: decodeBase64URL(
            String((((options as { user?: unknown }).user as { id?: unknown })?.id ?? "")),
          ),
        },
        excludeCredentials: Array.isArray((options as { excludeCredentials?: unknown }).excludeCredentials)
          ? ((options as { excludeCredentials?: unknown }).excludeCredentials as Array<Record<string, unknown>>)
              .map((credential) => ({
                ...credential,
                id: decodeBase64URL(String((credential as { id?: unknown }).id ?? "")),
              }))
          : undefined,
      };

      const credential = await navigator.credentials.create({ publicKey });
      if (!(credential instanceof PublicKeyCredential)) {
        throw new Error("The browser did not return a WebAuthn attestation credential");
      }
      const response = credential.response;
      if (!(response instanceof AuthenticatorAttestationResponse)) {
        throw new Error("The browser returned an unexpected passkey attestation response");
      }
      const toJSON = (credential as { toJSON?: () => unknown }).toJSON;
      if (typeof toJSON !== "function") {
        throw new Error("The browser did not expose PublicKeyCredential.toJSON()");
      }
      const attestationResponse = toJSON.call(credential);
      if (typeof attestationResponse !== "object" || attestationResponse === null) {
        throw new Error("Passkey attestation serialization did not return an object payload");
      }
      const attestationRecord = attestationResponse as {
        response?: { transports?: unknown };
      };
      const transports = typeof response.getTransports === "function" ? response.getTransports() : [];
      if (
        attestationRecord.response
        && (attestationRecord.response.transports === undefined
          || attestationRecord.response.transports === null)
      ) {
        attestationRecord.response.transports = transports;
      }

      const confirmResponse = await window.fetch("/api/auth/mfa/webauthn/enroll/confirm", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          display_name: passkeyDisplayName,
          attestation_response: attestationResponse,
        }),
      });
      if (!confirmResponse.ok) {
        throw new Error(
          `passkey enroll confirm failed: ${confirmResponse.status} ${await confirmResponse.text()}`,
        );
      }

      const body = (await confirmResponse.json().catch(() => null)) as { token?: unknown } | null;
      if (typeof body?.token === "string" && body.token.length > 0) {
        window.sessionStorage.setItem("ayb_token", body.token);
      }
    },
    {
      passkeyDisplayName: displayName,
    },
  );
}

/** Register a new user via the UI and return the email. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await openExplicitAuth(page);
  await page.getByRole("button", { name: "Register" }).click();

  // Fill form.
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();

  await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeHidden({
    timeout: 15000,
  });
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  return email;
}

/** Login with a demo account by clicking it in the demo accounts list. */
export async function loginWithDemoAccount(
  page: Page,
  email: string = DEMO_ACCOUNTS[0].email,
): Promise<void> {
  const acct = DEMO_ACCOUNTS.find((a) => a.email === email)!;
  await ensureDemoAccountExists(page, acct);
  await openExplicitAuth(page);

  // Click the demo account button to fill credentials.
  await page.getByText(acct.email).click();

  // Submit the form.
  await page.getByRole("button", { name: "Sign In", exact: true }).click();

  await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeHidden({
    timeout: 10000,
  });
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}

/** Login with existing credentials. */
export async function loginUser(
  page: Page,
  email: string,
  password: string = TEST_PASSWORD,
): Promise<void> {
  await openExplicitAuth(page);
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(password);
  await page.getByRole("button", { name: "Sign In", exact: true }).click();
  await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeHidden({
    timeout: 10000,
  });
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}

/** Open the create poll form. */
export async function openCreatePoll(page: Page): Promise<void> {
  await page.getByRole("button", { name: "+ New Poll" }).click();
  await expect(
    page.getByRole("heading", { name: "New Poll" }),
  ).toBeVisible();
}

/**
 * Attempt to INSERT a vote on the named closed poll via a direct fetch call,
 * bypassing the UI's disabled-button guard. Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * votes_insert policy that rejects writes to closed polls. The test cannot
 * be written with UI interactions alone because the UI prevents the action
 * before it reaches the server. Per BROWSER_TESTING_STANDARDS_3.md, API
 * shortcuts belong here in helpers.ts, never in spec files.
 *
 * Returns 0 on any setup failure (token missing, poll not found, etc.) so
 * that `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of
 * silently producing a false pass.
 */
export async function attemptDirectVoteOnClosedPoll(
  page: Page,
  pollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = sessionStorage.getItem("ayb_token");
    if (!token) return 0;

    const meRes = await fetch("/api/auth/me", {
      headers: { Authorization: `Bearer ${token}` },
    });
    const me = (await meRes.json()) as { id: string };

    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      is_closed: boolean;
      question: string;
    }>;
    const closed = polls.find((p) => p.question === question);
    if (!closed || !closed.is_closed) return 0;

    // Paginate through ALL poll_options — a shared CI DB accumulates options
    // across test runs and a single 500-row page misses recent entries.
    const PAGE_SIZE = 500;
    const allOpts: Array<{ id: string; poll_id: string }> = [];
    for (let pg = 1; ; pg++) {
      const optsRes = await fetch(
        `/api/collections/poll_options?perPage=${PAGE_SIZE}&page=${pg}&sort=position`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      const body = (await optsRes.json()) as {
        items?: Array<{ id: string; poll_id: string }>;
      };
      const items = body.items ?? [];
      allOpts.push(...items);
      if (items.length < PAGE_SIZE) break;
    }
    const opt = allOpts.find((o) => o.poll_id === closed.id);
    if (!opt) return 0;

    const voteRes = await fetch("/api/collections/votes", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        poll_id: closed.id,
        option_id: opt.id,
        user_id: me.id,
      }),
    });
    return voteRes.status;
  }, pollQuestion);
}

/**
 * Attempt to PATCH (close) a poll owned by another user via a direct fetch call,
 * bypassing the UI's hidden-button guard. Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * polls_update policy that only allows the poll owner to update their poll.
 * The test cannot be written with UI interactions alone because the UI never
 * renders the "Close poll" button for non-owners. Per BROWSER_TESTING_STANDARDS_3.md,
 * API shortcuts belong here in helpers.ts, never in spec files.
 *
 * Returns 0 on any setup failure (token missing, poll not found, etc.) so that
 * `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of silently
 * producing a false pass.
 */
export async function attemptDirectClosePoll(
  page: Page,
  pollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = sessionStorage.getItem("ayb_token");
    if (!token) return 0;

    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      is_closed: boolean;
      question: string;
    }>;
    const target = polls.find((p) => p.question === question);
    if (!target || target.is_closed) return 0;

    // Attempt to close the poll (set is_closed=true) as a non-owner.
    const patchRes = await fetch(`/api/collections/polls/${target.id}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ is_closed: true }),
    });
    return patchRes.status;
  }, pollQuestion);
}

/**
 * Attempt to INSERT a poll on behalf of another user by spoofing their user_id
 * in the request body, bypassing the UI which always sets user_id from the JWT.
 * Returns the HTTP response status.
 *
 * This helper exists to test server-side RLS enforcement — specifically the
 * polls_insert WITH CHECK policy that rejects inserts where user_id ≠ the
 * authenticated user's ID. The test cannot be written with UI interactions alone
 * because the frontend always sends the correct user_id from the JWT. Per
 * BROWSER_TESTING_STANDARDS_3.md, API shortcuts belong here in helpers.ts, never
 * in spec files.
 *
 * Returns 0 on any setup failure (token missing, owner poll not found) so that
 * `expect(result).toBeGreaterThanOrEqual(400)` fails loudly instead of silently
 * producing a false pass.
 */
export async function attemptDirectInsertPollForOtherUser(
  page: Page,
  existingOwnerPollQuestion: string,
): Promise<number> {
  return page.evaluate(async (question: string) => {
    const token = sessionStorage.getItem("ayb_token");
    if (!token) return 0;

    // Find the target poll (owned by another user) to extract their user_id.
    const pollsRes = await fetch(
      "/api/collections/polls?perPage=500&sort=-created_at",
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const polls = ((await pollsRes.json()).items ?? []) as Array<{
      id: string;
      user_id: string;
      question: string;
    }>;
    const ownerPoll = polls.find((p) => p.question === question);
    if (!ownerPoll) return 0;

    // Attempt to INSERT a new poll using the other user's user_id, bypassing
    // the frontend which always supplies the current user's own ID.
    const insertRes = await fetch("/api/collections/polls", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        question: `Spoofed insert ${Date.now()}?`,
        user_id: ownerPoll.user_id,
      }),
    });
    return insertRes.status;
  }, existingOwnerPollQuestion);
}

/** Return a Locator for the poll card containing the given question. */
export function pollCard(page: Page, question: string): Locator {
  return page
    .getByTestId("poll-card")
    .filter({ has: page.getByRole("heading", { name: question }) });
}

/** Create a poll with the given question and options. Returns a Locator for the poll card. */
export async function createPoll(
  page: Page,
  question: string,
  options: string[],
): Promise<Locator> {
  await openCreatePoll(page);

  // Fill question.
  await page.getByPlaceholder("Ask a question...").fill(question);

  // Fill options (2 inputs exist by default).
  for (let i = 0; i < options.length; i++) {
    if (i >= 2) {
      // Add more option inputs as needed.
      await page.getByRole("button", { name: "+ Add option" }).click();
    }
    await page.getByPlaceholder(`Option ${i + 1}`).fill(options[i]);
  }

  // Submit.
  await page.getByRole("button", { name: "Create Poll" }).click();

  // Wait for poll to appear in the list.
  await expect(page.getByText(question)).toBeVisible({ timeout: 10000 });

  return pollCard(page, question);
}
