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

export async function runDeterministicSearch(page: Page, query: string): Promise<void> {
  await page.getByPlaceholder("Search movies...").fill(query);
  const searchResponsePromise = page.waitForResponse((res) => {
    return res.request().method() === "POST" && res.url().includes("/api/admin/movies/search");
  });
  await page.getByRole("button", { name: "Search" }).click();
  const searchResponse = await searchResponsePromise;
  expect(searchResponse.status()).toBe(200);
  const payload = (await searchResponse.json()) as { rows?: Array<{ slug?: string }> };
  expect(Array.isArray(payload.rows)).toBeTruthy();
  expect(payload.rows?.[0]?.slug).toBe("inception");
  await expect(page.getByText("Inception")).toBeVisible({ timeout: 15000 });
}
