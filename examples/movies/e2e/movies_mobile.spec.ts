import { expect, test, type Page } from "@playwright/test";
import {
  expectLocalChatStream,
  loginWithDemoAccount,
  searchForMovie,
} from "./helpers";

async function expectNoHorizontalOverflow(page: Page, screenName: string): Promise<void> {
  // Horizontal overflow is the most common mobile regression this spec guards.
  // eslint-disable-next-line no-restricted-syntax
  const hasNoHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
  expect(hasNoHorizontalOverflow, `${screenName} should not create horizontal document overflow`).toBe(true);
}

test("mobile signed-in search, detail, and local chat flow", async ({ page }) => {
  await loginWithDemoAccount(page);

  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15_000 });
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15_000 });
  await expectNoHorizontalOverflow(page, "initial movie results");

  await searchForMovie(page, "inception", "inception");
  await expect(page.getByTestId("search-result-title-inception")).toHaveText("Inception");
  await expect(page.getByTestId("search-result-year-inception")).toHaveText("2010");
  await expectNoHorizontalOverflow(page, "filtered movie results");

  await page.getByTestId("search-result-row-inception").click();
  await expect(page.getByTestId("selected-result-notes-panel")).toContainText("Notes");
  await expect(page.getByTestId("chat-section")).toContainText("Chat");
  await expectNoHorizontalOverflow(page, "selected movie detail");

  await expectLocalChatStream(page, async () => {
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
  });
  await expect(page.getByText("Local stub response: Summarize inception")).toBeVisible();
  await expectNoHorizontalOverflow(page, "completed movie chat");
});
