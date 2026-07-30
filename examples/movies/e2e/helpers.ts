import { expect, type Page, type Route } from "@playwright/test";

export const DEMO_EMAIL = "alice@demo.test";

interface MovieSearchResponse {
  items: Array<{
    slug: string;
    title: string;
    overview: string;
    release_year: number;
    primary_genre: string;
    _highlight?: string;
  }>;
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
  facets: {
    primary_genre: Array<{ value: string; count: number }>;
  };
}

const EMPTY_MOVIES_RESPONSE: MovieSearchResponse = {
  items: [],
  page: 1,
  perPage: 10,
  totalItems: 0,
  totalPages: 0,
  facets: { primary_genre: [] },
};

const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";
const MOVIE_COLLECTION_ROUTE = "**/api/collections/movies**";
const MOVIES_BUNDLE_ROUTE = "**/assets/index-*.js";
const AUTH_ME_ROUTE = "**/api/auth/me";

type BlockedRequestGate = {
  wasBlocked: () => boolean;
  release: () => void;
};

type BlockedRequestController = {
  gate: BlockedRequestGate;
  holdNextRequest: () => Promise<boolean>;
};

function createBlockedRequestController(): BlockedRequestController {
  let blocked = false;
  let released = false;
  let releaseHeldRequest = () => {};
  const heldUntilReleased = new Promise<void>((resolve) => {
    releaseHeldRequest = resolve;
  });

  return {
    gate: {
      wasBlocked: () => blocked,
      release: () => {
        released = true;
        releaseHeldRequest();
      },
    },
    holdNextRequest: async () => {
      if (blocked || released) {
        return false;
      }
      blocked = true;
      await heldUntilReleased;
      return true;
    },
  };
}

const MAGIC_LINK_ROUTE = "**/api/auth/magic-link";

type MagicLinkRequestEvidence = {
  method: string;
  path: string;
  jsonBody: unknown;
  responseStatus: number;
};

type MagicLinkRequestCapture = {
  evidence: () => Promise<MagicLinkRequestEvidence>;
};

type RouteFailure = {
  status: number;
  message: string;
};

type MovieSearchRequestEvidence = {
  url: string;
  search: string;
};

type MovieSearchRequestCapture = {
  evidence: () => Promise<MovieSearchRequestEvidence>;
};

async function failNextRequest(page: Page, routePattern: string, failure: RouteFailure): Promise<void> {
  let failed = false;
  await page.route(routePattern, async (route) => {
    if (failed) {
      await route.continue();
      return;
    }
    failed = true;
    await route.fulfill({
      status: failure.status,
      contentType: "application/json",
      body: JSON.stringify({ message: failure.message }),
    });
  });
}

async function exposeMoviesClientFromBundle(page: Page): Promise<void> {
  await page.route(MOVIES_BUNDLE_ROUTE, async (route) => {
    const response = await route.fetch();
    const body = await response.text();
    const clientMatch = body.match(/(?:\bconst\s+|,)([$A-Z_a-z][$\w]*)=new\s+[$A-Z_a-z][$\w]*\([^)]*,\{fetch:/);
    if (!clientMatch) {
      await route.fulfill({
        response,
        body: `${body}\nglobalThis.__aybMoviesE2EClientInstallError = "AYB client construction was not found in the movies bundle";`,
      });
      return;
    }
    await route.fulfill({
      response,
      body: `${body}\nglobalThis.__aybMoviesE2EClient = ${clientMatch[1]};`,
    });
  });
}

async function refreshMoviesClientInPage(page: Page): Promise<void> {
  await page.addScriptTag({
    content: `
      (async () => {
        if (globalThis.__aybMoviesE2EClientInstallError) {
          throw new Error(globalThis.__aybMoviesE2EClientInstallError);
        }
        if (!globalThis.__aybMoviesE2EClient) {
          throw new Error("Movies E2E AYB client handle is not installed");
        }
        await globalThis.__aybMoviesE2EClient.auth.refresh();
      })();
    `,
  });
}

export async function optOutAnonymousBootstrap(page: Page): Promise<void> {
  await page.addInitScript((key) => {
    localStorage.setItem(key, "1");
  }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
}

export async function loginWithDemoAccount(page: Page): Promise<void> {
  await optOutAnonymousBootstrap(page);
  await page.goto("/");
  await page.waitForSelector("input[placeholder='you@example.com'], button:has-text('Sign out')", { timeout: 20000 });
  if (await page.getByRole("button", { name: "Sign out" }).isVisible()) {
    return;
  }
  await page.getByPlaceholder("you@example.com").fill(DEMO_EMAIL);
  await page.getByPlaceholder("At least 8 characters").fill("password123");
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible({ timeout: 15000 });
}

export async function loginWithDemoAccountAndExposedClient(page: Page): Promise<void> {
  await exposeMoviesClientFromBundle(page);
  await loginWithDemoAccount(page);
}

export async function triggerSameAccountAuthedReadyDip(page: Page): Promise<void> {
  await failNextRequest(page, AUTH_ME_ROUTE, {
    status: 500,
    message: "forced retained-token auth readiness failure",
  });

  await refreshMoviesClientInPage(page);
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({ timeout: 15000 });

  await refreshMoviesClientInPage(page);
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible({ timeout: 15000 });
}

/**
 * Hold the next movie-search response open until the caller releases it.
 *
 * Use this whenever a test asserts a transient in-flight state ("Loading
 * movies..." / "Searching movies..."). The previous approach delayed the
 * response by a fixed 900ms, which only widens the window the assertion is
 * racing; under union load the two can drift far enough apart that the state is
 * never observed. Holding the response makes the window last exactly as long as
 * the test needs, so the assertion cannot flake.
 *
 * Mirrors blockCollectionRequest in examples/kanban/tests/helpers.ts.
 */
export async function blockNextMovieSearch(page: Page): Promise<BlockedRequestGate> {
  const controller = createBlockedRequestController();

  await page.route(MOVIE_COLLECTION_ROUTE, async (route) => {
    await controller.holdNextRequest();
    await route.continue();
  });

  return controller.gate;
}

export async function failNextMovieSearch(page: Page): Promise<void> {
  await failNextRequest(page, MOVIE_COLLECTION_ROUTE, {
    status: 500,
    message: "forced movies search failure",
  });
}

export async function failNextAuthLogin(page: Page): Promise<void> {
  await failNextRequest(page, "**/api/auth/login", {
    status: 401,
    message: "Forced auth failure",
  });
}

export async function recordNextMovieSearchRequest(page: Page): Promise<MovieSearchRequestCapture> {
  let settled = false;
  let resolveEvidence: (evidence: MovieSearchRequestEvidence) => void = () => {};
  let rejectEvidence: (error: Error) => void = () => {};
  const evidencePromise = new Promise<MovieSearchRequestEvidence>((resolve, reject) => {
    resolveEvidence = resolve;
    rejectEvidence = reject;
  });

  const handler = async (route: Route) => {
    if (settled) {
      await route.continue();
      return;
    }
    settled = true;
    clearTimeout(timeoutID);
    const requestURL = route.request().url();
    const url = new URL(requestURL);
    try {
      await route.continue();
      resolveEvidence({
        url: requestURL,
        search: url.searchParams.get("search") ?? "",
      });
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error);
      rejectEvidence(new Error(`Failed to capture movie search request: ${detail}`));
    } finally {
      await page.unroute(MOVIE_COLLECTION_ROUTE, handler).catch(() => {});
    }
  };

  await page.route(MOVIE_COLLECTION_ROUTE, handler);
  const timeoutID = setTimeout(() => {
    settled = true;
    rejectEvidence(new Error("movie search request was not sent"));
    void page.unroute(MOVIE_COLLECTION_ROUTE, handler).catch(() => {});
  }, 15000);
  return { evidence: () => evidencePromise };
}

/** Capture the next magic-link request while allowing it to reach the backend. */
export async function recordNextMagicLinkRequest(page: Page): Promise<MagicLinkRequestCapture> {
  let settled = false;
  let resolveEvidence: (evidence: MagicLinkRequestEvidence) => void = () => {};
  let rejectEvidence: (error: Error) => void = () => {};
  const evidencePromise = new Promise<MagicLinkRequestEvidence>((resolve, reject) => {
    resolveEvidence = resolve;
    rejectEvidence = reject;
  });

  const handler = async (route: Route) => {
    if (settled) {
      await route.continue();
      return;
    }
    settled = true;
    clearTimeout(timeoutID);
    const request = route.request();
    let evidence: MagicLinkRequestEvidence | undefined;
    let captureError: unknown;
    try {
      const jsonBody = request.postDataJSON();
      const response = await route.fetch();
      await route.fulfill({ response });
      evidence = {
        method: request.method(),
        path: new URL(request.url()).pathname,
        jsonBody,
        responseStatus: response.status(),
      };
    } catch (error) {
      captureError = error;
      await route.continue().catch(() => {});
    }
    try {
      await page.unroute(MAGIC_LINK_ROUTE, handler);
    } catch (error) {
      captureError ??= error;
    }
    if (captureError) {
      const detail = captureError instanceof Error ? captureError.message : String(captureError);
      rejectEvidence(new Error(`Failed to capture magic-link request: ${detail}`));
      return;
    }
    if (!evidence) {
      rejectEvidence(new Error("Failed to capture magic-link request: no evidence was recorded"));
      return;
    }
    resolveEvidence(evidence);
  };

  await page.route(MAGIC_LINK_ROUTE, handler);
  const timeoutID = setTimeout(() => {
    settled = true;
    rejectEvidence(new Error("magic-link request was not sent"));
    void page.unroute(MAGIC_LINK_ROUTE, handler).catch(() => {});
  }, 15000);
  return { evidence: () => evidencePromise };
}

export async function failNextLogout(page: Page): Promise<void> {
  await failNextRequest(page, "**/api/auth/logout", {
    status: 503,
    message: "forced logout failure",
  });
}

export async function returnEmptyMovieCorpus(page: Page): Promise<void> {
  await page.route(MOVIE_COLLECTION_ROUTE, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(EMPTY_MOVIES_RESPONSE),
    });
  });
}

export async function failNextNoteEmbed(page: Page): Promise<void> {
  await failNextRequest(page, "**/api/admin/movies/notes/embed", {
    status: 500,
    message: "forced note embed failure",
  });
}

export async function failChatStream(page: Page): Promise<void> {
  await page.route("**/api/admin/movies/chat/stream", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ message: "forced chat backend failure" }),
    });
  });
}

export async function blockNextBYOKClear(page: Page): Promise<BlockedRequestGate> {
  const controller = createBlockedRequestController();

  await page.route("**/api/admin/movies/byok/*", async (route) => {
    if (route.request().method() !== "DELETE") {
      await route.continue();
      return;
    }
    if (await controller.holdNextRequest()) {
      // Let the disabled-control assertion own the in-flight window; the old fixed delay raced under union load.
      await route.fulfill({ status: 204, body: "" });
      return;
    }
    await route.continue();
  });

  return controller.gate;
}

export async function blockNextNoteEmbed(page: Page): Promise<BlockedRequestGate> {
  const controller = createBlockedRequestController();

  await page.route("**/api/admin/movies/notes/embed", async (route) => {
    // Let the Saving... assertion own the in-flight window; the old fixed delay raced under union load.
    await controller.holdNextRequest();
    await route.continue();
  });

  return controller.gate;
}

// UI-only search helper. Types the query and waits for the named result row
// to appear via Playwright auto-retry — no waitForResponse, no .json(), no
// API status assertions. The eslint config in eslint.config.mjs bans those
// patterns in spec files; this helper keeps the act+assert path UI-only.
export async function searchForMovie(page: Page, query: string, expectedSlug: string): Promise<void> {
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });
  const input = page.getByPlaceholder("Search movies...");
  await input.fill(query);
  await expect(page.getByTestId(`search-result-row-${expectedSlug}`)).toBeVisible({ timeout: 15000 });
}

export async function expectInceptionNoteEmbedding(
  page: Page,
  submitNote: () => Promise<void>,
): Promise<void> {
  const embedResponsePromise = page.waitForResponse((res) => {
    return res.request().method() === "POST" && res.url().includes("/api/admin/movies/notes/embed");
  });
  await submitNote();
  const embedResponse = await embedResponsePromise;
  expect(embedResponse.status()).toBe(200);
  const embedPayload = (await embedResponse.json()) as { movie_slug?: string; embedding?: number[] };
  expect(embedPayload.movie_slug).toBe("inception");
  expect(Array.isArray(embedPayload.embedding)).toBeTruthy();
  expect(embedPayload.embedding?.length).toBeGreaterThan(0);
}

export async function expectLocalChatStream(
  page: Page,
  sendMessage: () => Promise<void>,
): Promise<void> {
  const chatResponsePromise = page.waitForResponse((res) => {
    return res.request().method() === "POST" && res.url().includes("/api/admin/movies/chat/stream");
  });
  await sendMessage();
  const chatResponse = await chatResponsePromise;
  expect(chatResponse.status()).toBe(200);
  const chatStreamBody = await chatResponse.text();
  expect(chatStreamBody).toContain("event: chunk");
  expect(chatStreamBody).toContain("event: done");
  expect(chatStreamBody).toContain("Local stub response: Summarize inception");
}
