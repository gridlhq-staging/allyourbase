import { expect, test } from "@playwright/test";
import {
  expectInceptionNoteEmbedding,
  expectLocalChatStream,
  loginWithDemoAccount,
  runDeterministicSearch,
} from "./helpers";

test("movies seeded search order and note submission", async ({ page }) => {
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  const inceptionRow = page.getByTestId("search-result-row-inception");
  await expect(inceptionRow).toBeVisible();
  await expect(page.getByTestId("search-result-title-inception")).toHaveText("Inception");
  await expect(page.getByTestId("search-result-year-inception")).toHaveText("2010");

  await page.getByRole("button", { name: "Inception" }).click();
  await expectInceptionNoteEmbedding(page, async () => {
    await page.getByPlaceholder("Add a note about this movie...").fill("Deterministic note for seeded movie");
    await page.getByRole("button", { name: "Save Note" }).click();
  });
  await expect(page.getByText("Saved")).toBeVisible();
});

test("movies BYOK and chat stay local deterministic", async ({ page }) => {
  await loginWithDemoAccount(page);
  await runDeterministicSearch(page, "inception");

  await page.getByPlaceholder("Vault secret name...").fill("nonexistent_local_secret");
  await page.getByRole("button", { name: "Set" }).click();
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 15000 });

  await expectLocalChatStream(page, async () => {
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
  });
  await expect(page.getByText("Local stub response: Summarize inception")).toBeVisible();
});
