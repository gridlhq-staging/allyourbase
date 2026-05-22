import { expect, type Page } from "@playwright/test";

export const DEMO_EMAIL = "alice@demo.test";

export async function wireDeterministicMoviesAPI(page: Page): Promise<void> {
  await page.route("**/api/admin/movies/search", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        rows: [
          {
            slug: "inception",
            title: "Inception",
            overview: "A thief enters dreams to steal secrets and perform a final heist inside layered realities.",
            release_year: 2010,
            similarity: 1.0,
          },
          {
            slug: "arrival",
            title: "Arrival",
            overview: "A linguist helps decode alien language after mysterious ships appear around the world.",
            release_year: 2016,
            similarity: 0.8,
          },
          {
            slug: "moonlight",
            title: "Moonlight",
            overview: "A young man navigates identity, family, and belonging across three defining chapters of life.",
            release_year: 2016,
            similarity: 0.6,
          },
        ],
      }),
    });
  });

  await page.route("**/api/admin/movies/notes/embed", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "44444444-4444-4444-4444-444444444444",
        movie_slug: "inception",
        embedding: [0.9, 0.1, 0.2],
      }),
    });
  });
}

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

export async function runDeterministicSearch(page: Page, query: string): Promise<void> {
  await page.getByPlaceholder("Search movies...").fill(query);
  await page.getByRole("button", { name: "Search" }).click();
  await expect(page.getByText("Inception")).toBeVisible({ timeout: 15000 });
}
