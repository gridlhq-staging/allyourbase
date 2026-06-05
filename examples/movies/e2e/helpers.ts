import { expect, type Page } from "@playwright/test";

export const DEMO_EMAIL = "alice@demo.test";

export async function loginWithDemoAccount(page: Page): Promise<void> {
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

// UI-only search helper. Types the query and waits for the named result row
// to appear via Playwright auto-retry — no waitForResponse, no .json(), no
// API status assertions. The eslint config in eslint.config.mjs bans those
// patterns in spec files; this helper keeps the act+assert path UI-only.
export async function searchForMovie(page: Page, query: string, expectedSlug: string): Promise<void> {
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
