import { expect, type Page } from "@playwright/test";

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

export async function delayNextMovieSearch(page: Page, delayMs = 900): Promise<void> {
  let delayed = false;
  await page.route(MOVIE_COLLECTION_ROUTE, async (route) => {
    if (!delayed) {
      delayed = true;
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
    await route.continue();
  });
}

export async function failNextMovieSearch(page: Page): Promise<void> {
  let failed = false;
  await page.route(MOVIE_COLLECTION_ROUTE, async (route) => {
    if (!failed) {
      failed = true;
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ message: "forced movies search failure" }),
      });
      return;
    }
    await route.continue();
  });
}

export async function failNextAuthLogin(page: Page): Promise<void> {
  let failed = false;
  await page.route("**/api/auth/login", async (route) => {
    if (!failed) {
      failed = true;
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ message: "Forced auth failure" }),
      });
      return;
    }
    await route.continue();
  });
}

export async function failNextLogout(page: Page): Promise<void> {
  let failed = false;
  await page.route("**/api/auth/logout", async (route) => {
    if (!failed) {
      failed = true;
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ message: "forced logout failure" }),
      });
      return;
    }
    await route.continue();
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
  let failed = false;
  await page.route("**/api/admin/movies/notes/embed", async (route) => {
    if (!failed) {
      failed = true;
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ message: "forced note embed failure" }),
      });
      return;
    }
    await route.continue();
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

export async function delayNextBYOKClear(page: Page, delayMs = 900): Promise<void> {
  let delayed = false;
  await page.route("**/api/admin/movies/byok/*", async (route) => {
    if (route.request().method() !== "DELETE") {
      await route.continue();
      return;
    }
    if (!delayed) {
      delayed = true;
      await new Promise((resolve) => setTimeout(resolve, delayMs));
      await route.fulfill({ status: 204, body: "" });
      return;
    }
    await route.continue();
  });
}

export async function delayNextNoteEmbed(page: Page, delayMs = 900): Promise<void> {
  let delayed = false;
  await page.route("**/api/admin/movies/notes/embed", async (route) => {
    if (!delayed) {
      delayed = true;
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
    await route.continue();
  });
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
