import { test, expect, type Browser, type Page, type BrowserContext } from "@playwright/test";
import {
  blockFirstColumnCreate,
  registerUser,
  loginUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  installEscapedBoardApiMock,
  uniqueName,
} from "./helpers";

/** Set up two tabs viewing the same board. Returns context, page1, page2. */
async function setupTwoTabs(
  browser: Browser,
  boardName: string,
): Promise<{ context: BrowserContext; page1: Page; page2: Page }> {
  const context = await browser.newContext();
  const page1 = await context.newPage();
  const email = await registerUser(page1);
  await createBoard(page1, boardName);
  await openBoard(page1, boardName);

  const page2 = await context.newPage();
  await loginUser(page2, email);
  await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
  await page2.getByText(boardName).first().click();
  await expect(
    page2.getByRole("heading", { name: boardName }),
  ).toBeVisible({ timeout: 5000 });

  return { context, page1, page2 };
}

test.describe("Realtime WS", () => {
  test("board data requests escape filter literals before sending them to the API", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    const boardId = String.raw`board-\demo's`;
    const boardTitle = uniqueName("EscapedBoard");
    const capturedColumnsFilter = await installEscapedBoardApiMock(page, boardId, boardTitle);

    await page.goto("/");
    await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(boardTitle).first()).toBeVisible({ timeout: 10000 });
    await page.getByText(boardTitle).first().click();

    await expect
      .poll(() => capturedColumnsFilter(), { timeout: 10000 })
      .toBe(`board_id='board-\\\\demo\\'s'`);

    await context.close();
  });

  test("board realtime transport connects via websocket", async ({ browser }) => {
    const boardName = uniqueName("WSTransport");
    const context = await browser.newContext();
    const page = await context.newPage();
    const wsUrls: string[] = [];
    page.on("websocket", (ws) => {
      wsUrls.push(ws.url());
    });

    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
    await addColumn(page, "Todo");

    await expect
      .poll(() => wsUrls.some((url) => url.includes("/realtime/ws")), { timeout: 10000 })
      .toBeTruthy();

    await context.close();
  });

  test("leaving and reopening a board keeps one remote update per action", async ({ browser }) => {
    const boardName = uniqueName("ReopenSync");
    const context = await browser.newContext();
    const actorPage = await context.newPage();
    const email = await registerUser(actorPage);
    await createBoard(actorPage, boardName);
    await openBoard(actorPage, boardName);
    await addColumn(actorPage, "Todo");

    const watcherPage = await context.newPage();
    await loginUser(watcherPage, email);
    await expect(watcherPage.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await watcherPage.getByText(boardName).first().click();
    await expect(
      watcherPage.getByRole("heading", { name: boardName }),
    ).toBeVisible({ timeout: 5000 });

    // Leave and reopen board to exercise hook teardown/setup lifecycle.
    await watcherPage.getByRole("button", { name: "Back to boards" }).click();
    await expect(watcherPage.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await watcherPage.getByText(boardName).first().click();
    await expect(
      watcherPage.getByRole("heading", { name: boardName }),
    ).toBeVisible({ timeout: 5000 });

    await addCard(actorPage, "Todo", "Action Card");
    await expect(watcherPage.getByText("Action Card")).toHaveCount(1, { timeout: 10000 });

    await actorPage.getByText("Action Card").click();
    await expect(actorPage.getByText("Edit Card")).toBeVisible();
    const modal = actorPage.getByRole("dialog");
    await modal.getByLabel("Title").clear();
    await modal.getByLabel("Title").fill("Action Card Renamed");
    await actorPage.getByRole("button", { name: "Save" }).click();

    await expect(watcherPage.getByText("Action Card Renamed")).toHaveCount(1, {
      timeout: 10000,
    });

    await context.close();
  });

  test("local column create preserves remote columns that arrive while the request is in flight", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("ColumnRace"));
    const localCreateGate = await blockFirstColumnCreate(page1);

    await page1.getByPlaceholder("+ Add column...").fill("Local Column");
    await page1.getByRole("button", { name: "Add Column" }).click();
    await addColumn(page2, "Remote Column");
    await expect(page1.getByText("Remote Column")).toBeVisible({ timeout: 10000 });
    await expect.poll(localCreateGate.wasBlocked).toBe(true);
    localCreateGate.release();

    await expect(page1.getByText("Local Column")).toBeVisible({ timeout: 10000 });
    await expect(page1.getByText("Remote Column")).toHaveCount(1);
    await expect(page1.getByText("Local Column")).toHaveCount(1);

    await context.close();
  });

  test("card created in one tab appears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("RT Board"));
    await addColumn(page1, "Column A");

    await addCard(page1, "Column A", "Realtime Card");
    await expect(page1.getByText("Realtime Card")).toBeVisible();

    await expect(page2.getByText("Realtime Card")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card deleted in one tab disappears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("DelSync"));
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Will Be Deleted");

    await expect(page2.getByText("Will Be Deleted")).toBeVisible({
      timeout: 5000,
    });

    // Delete the card in tab 1.
    await page1.getByText("Will Be Deleted").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByText("Delete card").click();
    await expect(page1.getByText("Will Be Deleted")).toBeHidden();

    await expect(page2.getByText("Will Be Deleted")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("column created in one tab appears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("ColSync"));

    await addColumn(page1, "Realtime Column");
    await expect(page1.getByText("Realtime Column")).toBeVisible();

    await expect(page2.getByText("Realtime Column")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card updated in one tab updates in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("UpdSync"));
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Original Name");
    await expect(page2.getByText("Original Name")).toBeVisible({ timeout: 5000 });

    // Edit the card in tab 1.
    await page1.getByText("Original Name").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    const modal = page1.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();
    await titleInput.fill("Renamed Card");
    await page1.getByRole("button", { name: "Save" }).click();
    await expect(page1.getByText("Renamed Card")).toBeVisible();

    await expect(page2.getByText("Renamed Card")).toBeVisible({
      timeout: 10000,
    });
    await expect(page2.getByText("Original Name")).toBeHidden();

    await context.close();
  });

  test("column deleted in one tab disappears in another", async ({ browser }) => {
    const { context, page1, page2 } = await setupTwoTabs(browser, uniqueName("ColDelSync"));
    await addColumn(page1, "Ephemeral");
    await expect(page2.getByText("Ephemeral")).toBeVisible({ timeout: 5000 });

    // Delete column in tab 1.
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByRole("button", { name: "Delete column Ephemeral" }).click();
    await expect(page1.getByText("Ephemeral")).toBeHidden();

    await expect(page2.getByText("Ephemeral")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });
});
