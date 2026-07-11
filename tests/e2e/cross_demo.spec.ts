import { test, expect } from "@playwright/test";
import {
  DEMO_TARGETS,
  managedPortsForTest,
  managedPortsForDemoTargetForTest,
  orchestrateDemoRoundtrip,
  resolveApiHealthUrlForTest,
  resolveDemoTargetForTest,
  runtimePlanForTest,
} from "./fixtures";

// SSOT: examples/kanban/tests/helpers.ts — cross-project import blocked by
// separate Playwright node_modules trees (dual-package ESM resolution).
const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";
const DEMO_SIGNIN_EMAIL = "alice@demo.test";
const DEMO_SIGNIN_PASSWORD = "password123";
const LIVE_MAGIC_LINK_EMAIL = "e2e-livecheck@demo.test";
const DEFAULT_LIVE_APEX_URL = "https://demo.allyourbase.io";

const SIGN_IN_TIMEOUT_MS = 15_000;

let nameCounter = 0;
const runId = Math.random().toString(36).slice(2, 8);
function uniqueName(base: string): string {
  return `${base} ${runId}-${++nameCounter}`;
}

function liveApexUrlFromEnv(): string {
  return process.env.DEMO_APEX_URL?.trim() || DEFAULT_LIVE_APEX_URL;
}

function requiredEnvValue(key: string): string {
  const value = process.env[key]?.trim() ?? "";
  if (value === "") {
    throw new Error(`Missing required environment variable: ${key}`);
  }
  return value;
}

async function assertAuthAffordancesWithOptOut(page: import("@playwright/test").Page, url: string): Promise<void> {
  await page.addInitScript((key) => {
    localStorage.setItem(key, "1");
  }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
  await page.goto(url);

  await expect(page.getByLabel("Email")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByLabel("Password")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: /magic link/i })).toBeVisible();
  expect(await page.getByRole("button", { name: /^Continue with / }).count()).toBeGreaterThan(0);
}

function uniqueLiveAccountEmail(scope: string): string {
  const safeScope = scope.replace(/[^a-z0-9]/gi, "").toLowerCase();
  return `e2e-live-${safeScope}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@demo.test`;
}

async function authenticateLiveAccount(
  page: import("@playwright/test").Page,
  scope: string,
): Promise<void> {
  const email = uniqueLiveAccountEmail(scope);
  const emailInput = page.getByLabel("Email");

  // useAuth fires two concurrent loadMe() calls after register/login (one
  // from the method, one from onAuthStateChange SIGNED_IN). If the second
  // hits a rate limit, user resets to null and the auth form flickers back.
  // Deduplicate concurrent /api/auth/me responses at the network layer so
  // both loadMe() calls resolve with the same successful response.
  type CachedResp = { body: string; status: number; headers: Record<string, string> };
  let meInflight: Promise<CachedResp> | null = null;
  await page.route("**/api/auth/me", async (route) => {
    if (meInflight) {
      const c = await meInflight;
      await route.fulfill({ body: c.body, status: c.status, headers: c.headers });
      return;
    }
    meInflight = route.fetch().then(async (resp) => {
      const r = { body: await resp.text(), status: resp.status(), headers: resp.headers() };
      setTimeout(() => { meInflight = null; }, 2_000);
      return r;
    });
    const r = await meInflight;
    await route.fulfill({ body: r.body, status: r.status, headers: r.headers });
  });

  const registerToggle = page.getByRole("button", { name: /sign up|register/i });
  if (await registerToggle.isVisible({ timeout: 2_000 }).catch(() => false)) {
    await registerToggle.click();
  }

  await emailInput.fill(email);
  await page.getByLabel("Password").fill(DEMO_SIGNIN_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();
  await emailInput.waitFor({ state: "hidden", timeout: SIGN_IN_TIMEOUT_MS });
  await page.waitForTimeout(1_000);
  await page.unroute("**/api/auth/me");
}

test("fixture leak-gate coverage includes fixture-declared runtime-managed ports", () => {
  const expectedRuntimeManagedPorts = DEMO_TARGETS.movies.runtime?.managedPorts ?? [];

  expect(expectedRuntimeManagedPorts.length).toBeGreaterThan(0);
  expect(managedPortsForTest()).toEqual(expect.arrayContaining([...expectedRuntimeManagedPorts]));
});

test("fixture AI runtime setup is scoped to demos that require it", () => {
  const kanbanPlan = runtimePlanForTest(DEMO_TARGETS.kanban);
  const livePollsPlan = runtimePlanForTest(DEMO_TARGETS.livePolls);
  const moviesPlan = runtimePlanForTest(DEMO_TARGETS.movies);

  expect(kanbanPlan.startFakeOllama).toBeFalsy();
  expect(livePollsPlan.startFakeOllama).toBeFalsy();
  expect(moviesPlan.startFakeOllama).toBeTruthy();

  expect(kanbanPlan.aybConfigPath).toBeUndefined();
  expect(livePollsPlan.aybConfigPath).toBeUndefined();
  expect(moviesPlan.aybConfigPath).toContain("ayb_movies_e2e.toml");
});

test("fixture orchestration port scope excludes movies runtime ports for non-movies demos", () => {
  const moviesRuntimeManagedPorts = DEMO_TARGETS.movies.runtime?.managedPorts ?? [];

  expect(moviesRuntimeManagedPorts.length).toBeGreaterThan(0);
  expect(managedPortsForDemoTargetForTest(DEMO_TARGETS.kanban)).not.toEqual(
    expect.arrayContaining([...moviesRuntimeManagedPorts]),
  );
  expect(managedPortsForDemoTargetForTest(DEMO_TARGETS.livePolls)).not.toEqual(
    expect.arrayContaining([...moviesRuntimeManagedPorts]),
  );
  expect(managedPortsForDemoTargetForTest(DEMO_TARGETS.movies)).toEqual(
    expect.arrayContaining([...moviesRuntimeManagedPorts]),
  );
});

test("fixture live mode resolves public URLs from environment and API health owner", () => {
  const env: NodeJS.ProcessEnv = {
    CROSS_DEMO_LIVE: "1",
    DEMO_KANBAN_URL: "https://kanban.demo.allyourbase.io",
    DEMO_POLLS_URL: "https://polls.demo.allyourbase.io",
    DEMO_MOVIES_URL: "https://movies.demo.allyourbase.io",
    DEMO_API_URL: "https://api.allyourbase.io",
  };

  expect(resolveDemoTargetForTest(DEMO_TARGETS.kanban.name, env).url).toBe(env.DEMO_KANBAN_URL);
  expect(resolveDemoTargetForTest(DEMO_TARGETS.livePolls.name, env).url).toBe(env.DEMO_POLLS_URL);
  expect(resolveDemoTargetForTest(DEMO_TARGETS.movies.name, env).url).toBe(env.DEMO_MOVIES_URL);
  expect(resolveApiHealthUrlForTest(env)).toBe("https://api.allyourbase.io/health");
});

test.describe("local orchestration", () => {
  // eslint-disable-next-line playwright/no-skipped-test
  test.skip(Boolean(process.env.CROSS_DEMO_LIVE), "local orchestration tests run only without CROSS_DEMO_LIVE");

  test("kanban roundtrip succeeds with Stage 3 orchestration", async ({ page }) => {
    test.setTimeout(120_000);
    await orchestrateDemoRoundtrip(DEMO_TARGETS.kanban.name, async ({ demoTarget }) => {
      const boardTitle = uniqueName("Stage3 Board");
      const columnTitle = "Stage3 Todo";
      const cardTitle = "Stage3 Card";

      // Opt out of anonymous bootstrap before navigation to prevent race
      // where auto-signin hides the auth form. Pattern: helpers.ts:ensureAuthFormVisible.
      await page.addInitScript((key) => {
        localStorage.setItem(key, "1");
      }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
      await page.goto(demoTarget.url);

      await page.getByText(DEMO_SIGNIN_EMAIL).click();
      await page.getByRole("button", { name: "Sign In", exact: true }).click();
      await expect(page.getByText("Your Boards")).toBeVisible({ timeout: SIGN_IN_TIMEOUT_MS });

      // Board/column/card interaction — selectors from helpers.ts:createBoard/openBoard/addColumn/addCard
      await page.getByPlaceholder("New board name...").fill(boardTitle);
      await page.getByRole("button", { name: "Create" }).click();
      await expect(page.getByText(boardTitle).first()).toBeVisible();

      await page.getByText(boardTitle).first().click();
      await expect(page.getByRole("heading", { name: boardTitle })).toBeVisible({ timeout: 5000 });

      await page.getByPlaceholder("+ Add column...").fill(columnTitle);
      await page.getByRole("button", { name: "Add Column" }).click();
      await expect(page.getByText(columnTitle)).toBeVisible();

      const column = page.getByTestId(`column-${columnTitle}`);
      await column.getByText("+ Add a card").click();
      await column.getByPlaceholder("Card title...").fill(cardTitle);
      await column.getByRole("button", { name: "Add", exact: true }).click();
      await expect(page.getByText(cardTitle)).toBeVisible();
    });
  });

  test("live-polls roundtrip succeeds with Stage 4 orchestration", async ({ page }) => {
    test.setTimeout(120_000);
    await orchestrateDemoRoundtrip(DEMO_TARGETS.livePolls.name, async ({ demoTarget }) => {
      const question = uniqueName("Stage4 Poll") + "?";
      const yesOption = "Yes";
      const noOption = "No";

      // Opt out of anonymous bootstrap before navigation to prevent the auto-signin
      // race that hides the auth form (same pattern as the kanban test above; see
      // examples/live-polls/e2e/helpers.ts:openExplicitAuth).
      await page.addInitScript((key) => {
        localStorage.setItem(key, "1");
      }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
      await page.goto(demoTarget.url);

      await page.getByText(DEMO_SIGNIN_EMAIL).click();
      await page.getByRole("button", { name: "Sign In", exact: true }).click();
      // Waiting for "+ New Poll" confirms auth success AND polls UI readiness in one assertion.
      const newPollButton = page.getByRole("button", { name: "+ New Poll" });
      await expect(newPollButton).toBeVisible({ timeout: SIGN_IN_TIMEOUT_MS });

      // Create poll — selectors mirror examples/live-polls/e2e/helpers.ts:createPoll.
      await newPollButton.click();
      await expect(page.getByRole("heading", { name: "New Poll" })).toBeVisible();
      await page.getByPlaceholder("Ask a question...").fill(question);
      await page.getByPlaceholder("Option 1").fill(yesOption);
      await page.getByPlaceholder("Option 2").fill(noOption);
      await page.getByRole("button", { name: "Create Poll" }).click();

      // Scope assertions to the just-created card so a shared demo DB with prior
      // polls cannot give a false positive. Matches pollCard() in helpers.ts.
      const card = page
        .getByTestId("poll-card")
        .filter({ has: page.getByRole("heading", { name: question }) });
      await expect(card).toBeVisible({ timeout: 10_000 });

      // Vote for Yes and assert the exact numeric contract from PollCard.tsx.
      const yesButton = card.getByRole("button", { name: new RegExp(yesOption) });
      const noButton = card.getByRole("button", { name: new RegExp(noOption) });
      await yesButton.click();

      await expect(card.getByText("1 total vote")).toBeVisible({ timeout: 5_000 });
      await expect(yesButton.getByText("1 vote (100%)")).toBeVisible();
      await expect(noButton.getByText("0 votes (0%)")).toBeVisible();
    });
  });

  test("movies roundtrip succeeds with Stage 5 orchestration", async ({ page }) => {
    test.setTimeout(120_000);
    await orchestrateDemoRoundtrip(DEMO_TARGETS.movies.name, async ({ demoTarget }) => {
      await page.addInitScript((key) => {
        localStorage.setItem(key, "1");
      }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
      await page.goto(demoTarget.url);

      await expect(
        page
          .getByPlaceholder("you@example.com")
          .or(page.getByRole("button", { name: "Sign out" })),
      ).toBeVisible({ timeout: 20_000 });
      const signOutButton = page.getByRole("button", { name: "Sign out" });
      if (!(await signOutButton.isVisible())) {
        await page.getByPlaceholder("you@example.com").fill(DEMO_SIGNIN_EMAIL);
        await page.getByPlaceholder("At least 8 characters").fill(DEMO_SIGNIN_PASSWORD);
        await page.getByRole("button", { name: "Sign In", exact: true }).click();
      }
      await expect(signOutButton).toBeVisible({ timeout: SIGN_IN_TIMEOUT_MS });

      await page.getByPlaceholder("Search movies...").fill("inception");
      const searchResponsePromise = page.waitForResponse((res) => {
        return res.request().method() === "POST" && res.url().includes("/api/admin/movies/search");
      });
      await page.getByRole("button", { name: "Search" }).click();

      const searchResponse = await searchResponsePromise;
      expect(searchResponse.status()).toBe(200);
      const payload = (await searchResponse.json()) as { rows?: Array<{ slug?: string }> };
      expect(Array.isArray(payload.rows)).toBeTruthy();
      expect(payload.rows?.[0]?.slug).toBe("inception");

      await expect(page.getByRole("heading", { level: 3 }).first()).toHaveText("Inception");
    });
  });
});

test.describe("live deployment", () => {
  // eslint-disable-next-line playwright/no-skipped-test
  test.skip(!process.env.CROSS_DEMO_LIVE, "live-target tests require CROSS_DEMO_LIVE=1");

  test("anonymous bootstrap default path renders each live demo shell without crash", async ({ page }) => {
    const liveKanban = resolveDemoTargetForTest(DEMO_TARGETS.kanban.name, process.env);
    const livePolls = resolveDemoTargetForTest(DEMO_TARGETS.livePolls.name, process.env);
    const liveMovies = resolveDemoTargetForTest(DEMO_TARGETS.movies.name, process.env);

    await page.goto(liveKanban.url);
    await expect(page.getByText("Kanban Board")).toBeVisible({ timeout: 20_000 });

    await page.goto(livePolls.url);
    await expect(page.getByText("Live Polls")).toBeVisible({ timeout: 20_000 });

    await page.goto(liveMovies.url);
    await expect(page.getByText("Movies Demo")).toBeVisible({ timeout: 20_000 });
  });

  test("opt-out path renders auth affordances across demos", async ({ page }) => {
    const liveKanban = resolveDemoTargetForTest(DEMO_TARGETS.kanban.name, process.env);
    const livePolls = resolveDemoTargetForTest(DEMO_TARGETS.livePolls.name, process.env);
    const liveMovies = resolveDemoTargetForTest(DEMO_TARGETS.movies.name, process.env);
    let validatedDemoCount = 0;

    await assertAuthAffordancesWithOptOut(page, liveKanban.url);
    validatedDemoCount += 1;
    await assertAuthAffordancesWithOptOut(page, livePolls.url);
    validatedDemoCount += 1;
    await assertAuthAffordancesWithOptOut(page, liveMovies.url);
    validatedDemoCount += 1;
    expect(validatedDemoCount).toBe(3);
  });

  test("kanban live authenticated action creates a board", async ({ page }) => {
    await orchestrateDemoRoundtrip(DEMO_TARGETS.kanban.name, async ({ demoTarget }) => {
      const boardTitle = uniqueName("Live Board");
      await page.addInitScript((key) => {
        localStorage.setItem(key, "1");
      }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
      await page.goto(demoTarget.url);
      await authenticateLiveAccount(page, "kanban");
      await expect(page.getByText("Your Boards")).toBeVisible({ timeout: SIGN_IN_TIMEOUT_MS });

      await page.getByPlaceholder("New board name...").fill(boardTitle);
      await page.getByRole("button", { name: "Create" }).click();
      await expect(page.getByText(boardTitle).first()).toBeVisible({ timeout: 10_000 });
    });
  });

  test("live-polls authenticated action creates a poll", async ({ page }) => {
    await orchestrateDemoRoundtrip(DEMO_TARGETS.livePolls.name, async ({ demoTarget }) => {
      const question = uniqueName("Live Poll") + "?";
      await page.addInitScript((key) => {
        localStorage.setItem(key, "1");
      }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
      await page.goto(demoTarget.url);
      await authenticateLiveAccount(page, "livepolls");

      const newPollButton = page.getByRole("button", { name: "+ New Poll" });
      await expect(newPollButton).toBeVisible({ timeout: SIGN_IN_TIMEOUT_MS });
      await newPollButton.click();
      await page.getByPlaceholder("Ask a question...").fill(question);
      await page.getByPlaceholder("Option 1").fill("Yes");
      await page.getByPlaceholder("Option 2").fill("No");
      await page.getByRole("button", { name: "Create Poll" }).click();

      const card = page
        .getByTestId("poll-card")
        .filter({ has: page.getByRole("heading", { name: question }) });
      await expect(card).toBeVisible({ timeout: 10_000 });
    });
  });

  test("movies authenticated search returns seeded inception row", async ({ page }) => {
    const apiHealthUrl = resolveApiHealthUrlForTest(process.env);
    const apiBaseUrl = apiHealthUrl.replace(/\/health$/, "");
    const adminPassword = process.env.DEMO_ADMIN_PASSWORD?.trim() ?? "";
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(adminPassword === "", "movies live search requires DEMO_ADMIN_PASSWORD");

    const adminAuthResponse = await page.request.post(`${apiBaseUrl}/api/admin/auth`, {
      data: { password: requiredEnvValue("DEMO_ADMIN_PASSWORD") },
    });
    expect(adminAuthResponse.status()).toBe(200);
    const adminAuthPayload = (await adminAuthResponse.json()) as { token?: string };
    const adminToken = adminAuthPayload.token ?? "";
    expect(adminToken.length).toBeGreaterThan(0);

    const searchResponse = await page.request.post(`${apiBaseUrl}/api/admin/movies/search`, {
      headers: { authorization: `Bearer ${adminToken}` },
      data: { query: "inception" },
    });
    // eslint-disable-next-line playwright/no-conditional-in-test
    if (searchResponse.status() === 503) {
      const maybeServicePayload = (await searchResponse.json().catch(() => ({}))) as { message?: string };
      // eslint-disable-next-line playwright/no-skipped-test
      test.skip(
        maybeServicePayload.message === "movies demo backend is not enabled",
        "movies live backend is disabled in deployed runtime",
      );
    }

    expect(searchResponse.status()).toBe(200);
    const payload = (await searchResponse.json()) as {
      rows?: Array<{ slug?: string; title?: string }>;
    };
    expect(payload.rows?.[0]?.slug).toBe("inception");
    expect(payload.rows?.[0]?.title).toBe("Inception");
  });

  test("apex landing renders demo index content", async ({ page }) => {
    const response = await page.request.get(liveApexUrlFromEnv());
    expect(response.status()).toBe(200);
    const body = (await response.text()).toLowerCase();
    expect(body).toContain("kanban");
    expect(body).toContain("poll");
    expect(body).toContain("movie");
  });

  test("oauth start returns 307 redirect to github without mismatch errors", async ({ page }) => {
    const apiHealthUrl = resolveApiHealthUrlForTest(process.env);
    const apiBaseUrl = apiHealthUrl.replace(/\/health$/, "");
    const redirectTo = liveApexUrlFromEnv();
    let response = await page.request.get(
      `${apiBaseUrl}/api/auth/oauth/github?redirect_to=${encodeURIComponent(redirectTo)}`,
      { maxRedirects: 0 },
    );
    // eslint-disable-next-line playwright/no-conditional-in-test
    for (let attempts = 1; response.status() === 429 && attempts < 5; attempts += 1) {
      await new Promise((resolve) => setTimeout(resolve, attempts * 1_000));
      response = await page.request.get(
        `${apiBaseUrl}/api/auth/oauth/github?redirect_to=${encodeURIComponent(redirectTo)}`,
        { maxRedirects: 0 },
      );
    }
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(response.status() === 429, "oauth start endpoint is rate-limited in live environment");
    expect(response.status()).toBe(307);

    const location = response.headers().location ?? "";
    expect(location).toContain("github.com");
    expect(location.toLowerCase()).not.toContain("redirect_uri_mismatch");
    const body = (await response.text()).toLowerCase();
    expect(body).not.toContain("redirect_uri_mismatch");
  });

  test("magic-link request returns 200", async ({ page }) => {
    const apiHealthUrl = resolveApiHealthUrlForTest(process.env);
    const apiBaseUrl = apiHealthUrl.replace(/\/health$/, "");
    const response = await page.request.post(`${apiBaseUrl}/api/auth/magic-link`, {
      data: { email: LIVE_MAGIC_LINK_EMAIL },
    });
    expect(response.status()).toBe(200);
  });
});
