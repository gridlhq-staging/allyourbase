import { expect, test } from "@playwright/test";
import { loginWithDemoAccount, runDeterministicSearch, wireDeterministicMoviesAPI } from "./helpers";

test("movies seeded search order and note submission", async ({ page }) => {
  await wireDeterministicMoviesAPI(page);
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  const resultTitles = page.locator("h3.font-medium.text-white");
  await expect(resultTitles.first()).toHaveText("Inception");

  await page.getByRole("button", { name: "Inception" }).click();
  await page.getByPlaceholder("Add a note about this movie...").fill("Deterministic note for seeded movie");
  await page.getByRole("button", { name: "Save Note" }).click();
  await expect(page.getByText("Saved")).toBeVisible();
});

test("movies BYOK and chat stay local deterministic", async ({ page }) => {
  await wireDeterministicMoviesAPI(page);
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  await page.getByPlaceholder("Vault secret name...").fill("nonexistent_local_secret");
  await page.getByRole("button", { name: "Set" }).click();
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 15000 });

  await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText("user:")).toBeVisible({ timeout: 15000 });
  await expect(page.getByText("Summarize inception")).toBeVisible({ timeout: 15000 });
});
