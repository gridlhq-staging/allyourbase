import { test, expect } from "@playwright/test";
import {
  uniqueEmail,
  TEST_PASSWORD,
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  loginUser,
} from "./helpers";

test.describe("Row-Level Security", () => {
  test("user A cannot see user B boards", async ({ browser }) => {
    // User A creates a board
    const contextA = await browser.newContext();
    const pageA = await contextA.newPage();
    const emailA = await registerUser(pageA);
    await createBoard(pageA, "User A Private Board");
    await expect(pageA.getByText("User A Private Board")).toBeVisible();
    await contextA.close();

    // User B registers and should NOT see User A's board
    const contextB = await browser.newContext();
    const pageB = await contextB.newPage();
    await registerUser(pageB);

    // User B should see empty state, not User A's board
    await expect(pageB.getByText("No boards yet")).toBeVisible();
    await expect(
      pageB.getByText("User A Private Board"),
    ).not.toBeVisible();
    await contextB.close();
  });

  test("each user sees only their own boards", async ({ browser }) => {
    // User A
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    const emailA = await registerUser(pageA);
    await createBoard(pageA, "Board by A");

    // User B
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    const emailB = await registerUser(pageB);
    await createBoard(pageB, "Board by B");

    // User A should see only their board
    await expect(pageA.getByText("Board by A")).toBeVisible();
    await expect(pageA.getByText("Board by B")).not.toBeVisible();

    // User B should see only their board
    await expect(pageB.getByText("Board by B")).toBeVisible();
    await expect(pageB.getByText("Board by A")).not.toBeVisible();

    await ctxA.close();
    await ctxB.close();
  });

  test("user A cannot see user B columns and cards", async ({ browser }) => {
    // User A creates a board with a column and card
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await registerUser(pageA);
    await createBoard(pageA, "RLS Board A");
    await openBoard(pageA, "RLS Board A");
    await addColumn(pageA, "A Column");
    await addCard(pageA, "A Column", "A Secret Card");
    await expect(pageA.getByText("A Secret Card")).toBeVisible();
    await ctxA.close();

    // User B registers — should not see A's board at all
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await registerUser(pageB);
    await expect(pageB.getByText("No boards yet")).toBeVisible();
    await expect(pageB.getByText("RLS Board A")).not.toBeVisible();
    await expect(pageB.getByText("A Column")).not.toBeVisible();
    await expect(pageB.getByText("A Secret Card")).not.toBeVisible();
    await ctxB.close();
  });

  test("user sees own boards after re-login", async ({ browser }) => {
    // Register and create a board
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    const email = await registerUser(page);
    await createBoard(page, "My Persistent Board");
    await expect(page.getByText("My Persistent Board")).toBeVisible();

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Login again
    await loginUser(page, email);
    await expect(page.getByText("My Persistent Board")).toBeVisible();

    await ctx.close();
  });

  test("deleted board is not visible after re-login", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    const email = await registerUser(page);
    await createBoard(page, "Will Delete");
    await expect(page.getByText("Will Delete")).toBeVisible();

    // Delete the board
    page.on("dialog", (dialog) => dialog.accept());
    const boardCard = page.getByRole("heading", { name: "Will Delete" }).locator("../..");
    await boardCard.hover();
    await boardCard.getByRole("button", { name: "Delete board" }).click();
    await expect(page.getByText("Will Delete")).not.toBeVisible();

    // Logout and re-login
    await page.getByText("Sign out").click();
    await loginUser(page, email);

    // Should still be gone
    await expect(page.getByText("Will Delete")).not.toBeVisible();
    await expect(page.getByText("No boards yet")).toBeVisible();

    await ctx.close();
  });
});
