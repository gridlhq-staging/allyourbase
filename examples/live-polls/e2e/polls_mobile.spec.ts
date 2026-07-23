import { expect, test, type Page } from "@playwright/test";
import {
  createPoll,
  openCreatePoll,
  pollCard,
  registerUser,
  runId,
} from "./helpers";

async function expectNoHorizontalOverflow(page: Page, screenName: string): Promise<void> {
  // Horizontal overflow is the most common mobile regression this spec guards.
  // eslint-disable-next-line no-restricted-syntax
  const hasNoHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
  expect(hasNoHorizontalOverflow, `${screenName} should not create horizontal document overflow`).toBe(true);
}

test("mobile registered user creates and votes in a poll", async ({ page }) => {
  await registerUser(page);

  await expect(page.getByRole("button", { name: "+ New Poll" })).toBeVisible();
  await expectNoHorizontalOverflow(page, "poll list");

  await openCreatePoll(page);
  await expect(page.getByRole("form", { name: "New Poll" })).toBeVisible();
  await expectNoHorizontalOverflow(page, "open create poll form");
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByRole("heading", { name: "New Poll" })).toBeHidden();

  const question = `Mobile poll ${runId}?`;
  await createPoll(page, question, ["First mobile option", "Second mobile option"]);
  const card = pollCard(page, question);
  await expect(card.getByRole("heading", { name: question })).toBeVisible();
  await expect(card.getByText("0 total votes")).toBeVisible();
  await expectNoHorizontalOverflow(page, "created poll");

  await card.getByRole("button", { name: /First mobile option/ }).click();
  await expect(card.getByText("1 total vote")).toBeVisible({ timeout: 5_000 });
  await expect(card.getByRole("button", { name: /First mobile option/ }).getByText("1 vote (100%)")).toBeVisible();
  await expectNoHorizontalOverflow(page, "voted poll");
});
