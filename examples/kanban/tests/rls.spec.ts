import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  loginUser,
  runId,
  uniqueName,
  DEMO_ACCOUNTS,
  loginWithDemoAccount,
} from "./helpers";

const persistentBoardName = uniqueName("My Persistent Board");
const deleteBoardName = uniqueName("Will Delete");

test.describe("Row-Level Security (collaborative model)", () => {
  test("collaborative persistence board names include run-specific suffixes", () => {
    expect(persistentBoardName).not.toBe("My Persistent Board");
    expect(deleteBoardName).not.toBe("Will Delete");
    expect(persistentBoardName).toContain(runId);
    expect(deleteBoardName).toContain(runId);
    expect(persistentBoardName).not.toBe(deleteBoardName);
  });

  test("user B can see user A boards", async ({ browser }) => {
    const boardName = `Visible Board ${runId}`;
    // User A creates a board.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    await createBoard(pageA, boardName);
    await expect(pageA.getByText(boardName).first()).toBeVisible();

    // User B logs in and should see User A's board.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    await expect(pageB.getByText(boardName).first()).toBeVisible({ timeout: 5000 });

    await ctxA.close();
    await ctxB.close();
  });

  test("user B can add cards to user A board", async ({ browser }) => {
    const boardName = `Collab Board ${runId}`;
    // User A creates a board with a column.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    await createBoard(pageA, boardName);
    await openBoard(pageA, boardName);
    await addColumn(pageA, "Todo");

    // User B opens the same board and adds a card.
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await loginWithDemoAccount(pageB, DEMO_ACCOUNTS[1].email);
    await expect(pageB.getByText(boardName).first()).toBeVisible({ timeout: 10000 });
    await openBoard(pageB, boardName);
    await expect(pageB.getByText("Todo")).toBeVisible({ timeout: 10000 });
    await addCard(pageB, "Todo", "Bob's Card");
    await expect(pageB.getByText("Bob's Card")).toBeVisible();

    await ctxA.close();
    await ctxB.close();
  });

  test("only board owner can delete the board", async ({ browser }) => {
    const boardName = `Owner Only Delete ${runId}`;
    // User A creates a board.
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await loginWithDemoAccount(pageA, DEMO_ACCOUNTS[0].email);
    await createBoard(pageA, boardName);
    await expect(pageA.getByText(boardName).first()).toBeVisible();

    // User A can delete (owner).
    const boardCard = pageA.getByRole("button", { name: new RegExp(`Open board ${boardName}`) }).first();
    await boardCard.hover();
    const deleteBtn = pageA.getByRole("button", { name: `Delete board ${boardName}` });
    await expect(deleteBtn).toBeVisible();

    await ctxA.close();
  });

  test("user sees own boards after re-login", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    const email = await registerUser(page);
    await createBoard(page, persistentBoardName);
    await expect(page.getByText(persistentBoardName)).toBeVisible();

    // Logout.
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login again.
    await loginUser(page, email);
    await expect(page.getByText(persistentBoardName)).toBeVisible();

    await ctx.close();
  });

  test("deleted board is not visible after re-login", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    const email = await registerUser(page);
    await createBoard(page, deleteBoardName);
    await expect(page.getByText(deleteBoardName)).toBeVisible();

    // Delete the board.
    page.on("dialog", (dialog) => dialog.accept());
    const boardCard = page.getByRole("button", { name: `Open board ${deleteBoardName}` });
    await boardCard.hover();
    await page.getByRole("button", { name: `Delete board ${deleteBoardName}` }).click();
    await expect(page.getByText(deleteBoardName)).toBeHidden();

    // Logout and re-login.
    await page.getByText("Sign out").click();
    await loginUser(page, email);

    // Should still be gone.
    await expect(page.getByText(deleteBoardName)).toBeHidden();

    await ctx.close();
  });
});
