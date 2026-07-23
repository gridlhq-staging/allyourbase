import { expect, test, type Page } from "@playwright/test";
import {
  addCard,
  ownedBoardId,
  uniqueName,
  waitForAnonymousBoardShell,
} from "./helpers";

async function expectNoHorizontalOverflow(page: Page, screenName: string): Promise<void> {
  // Horizontal overflow is the most common mobile regression this spec guards.
  // eslint-disable-next-line no-restricted-syntax
  const hasNoHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
  expect(hasNoHorizontalOverflow, `${screenName} should not create horizontal document overflow`).toBe(true);
}

test("mobile anonymous starter board supports visible card creation", async ({ page }) => {
  await waitForAnonymousBoardShell(page);

  await expect(page.getByText("Your Boards")).toBeVisible();
  await expectNoHorizontalOverflow(page, "board list");

  await expect.poll(async () => (await ownedBoardId(page)) ?? "", { timeout: 15_000 }).not.toBe("");
  const boardId = (await ownedBoardId(page)) ?? "";
  expect(boardId).not.toBe("");
  const starterBoard = page.getByTestId(`board-${boardId}`);
  await expect(starterBoard).toContainText("My First Board");

  await starterBoard.click();
  await expect(page.getByRole("heading", { name: "My First Board" })).toBeVisible();
  const toDoColumn = page.getByTestId("column-To Do");
  await expect(toDoColumn).toBeVisible();
  await expect(toDoColumn.getByText("Welcome to your board")).toBeVisible();
  await expectNoHorizontalOverflow(page, "opened starter board");

  const cardTitle = uniqueName("Mobile card");
  await addCard(page, "To Do", cardTitle);
  await expect(toDoColumn.getByText(cardTitle, { exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page, "board after mobile card create");
});
