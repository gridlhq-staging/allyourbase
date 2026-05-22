import { expect, test } from "@playwright/test";
import { loginWithDemoAccount, runDeterministicSearch } from "./helpers";

test("movies seeded search order and note submission", async ({ page }) => {
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  const resultTitles = page.locator("h3.font-medium.text-white");
  await expect(resultTitles.first()).toHaveText("Inception");

  await page.getByRole("button", { name: "Inception" }).click();
  const embedResponsePromise = page.waitForResponse((res) => {
    return res.request().method() === "POST" && res.url().includes("/api/admin/movies/notes/embed");
  });
  await page.getByPlaceholder("Add a note about this movie...").fill("Deterministic note for seeded movie");
  await page.getByRole("button", { name: "Save Note" }).click();
  const embedResponse = await embedResponsePromise;
  expect(embedResponse.status()).toBe(200);
  const embedPayload = (await embedResponse.json()) as { movie_slug?: string; embedding?: number[] };
  expect(embedPayload.movie_slug).toBe("inception");
  expect(Array.isArray(embedPayload.embedding)).toBeTruthy();
  expect(embedPayload.embedding?.length).toBeGreaterThan(0);
  await expect(page.getByText("Saved")).toBeVisible();
});

test("movies BYOK and chat stay local deterministic", async ({ page }) => {
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  await page.getByPlaceholder("Vault secret name...").fill("nonexistent_local_secret");
  await page.getByRole("button", { name: "Set" }).click();
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 15000 });

  await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
  const chatResponsePromise = page.waitForResponse((res) => {
    return res.request().method() === "POST" && res.url().includes("/api/admin/movies/chat/stream");
  });
  await page.getByRole("button", { name: "Send" }).click();
  const chatResponse = await chatResponsePromise;
  expect(chatResponse.status()).toBe(200);
  const chatStreamBody = await chatResponse.text();
  expect(chatStreamBody).toContain("assistant");
  expect(chatStreamBody).toContain("Local stub response: Summarize inception");
});
