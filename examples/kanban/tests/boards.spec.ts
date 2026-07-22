import { test, expect } from "@playwright/test";
import { registerUser, createBoard, openBoard, uniqueName } from "./helpers";

test.describe("Boards", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
  });

  test("can create boards and empty state disappears", async ({ page }) => {
    const board1 = uniqueName("Board 1");
    const board2 = uniqueName("Board 2");
    const board3 = uniqueName("Board 3");

    await createBoard(page, board1);
    await expect(page.getByText(board1)).toBeVisible();
    await expect(page.getByText("No boards yet")).toBeHidden();

    await createBoard(page, board2);
    await createBoard(page, board3);

    await expect(page.getByText(board1)).toBeVisible();
    await expect(page.getByText(board2)).toBeVisible();
    await expect(page.getByText(board3)).toBeVisible();
  });

  test("can navigate into a board", async ({ page }) => {
    const boardTitle = uniqueName("Navigate Test");

    await createBoard(page, boardTitle);
    await openBoard(page, boardTitle);
    // Should see the board header with the board title
    await expect(
      page.getByRole("heading", { name: boardTitle }),
    ).toBeVisible();
    // Should see the "Live" badge
    await expect(page.getByText("Live")).toBeVisible();
  });

  test("can navigate back from a board", async ({ page }) => {
    const boardTitle = uniqueName("Back Test");

    await createBoard(page, boardTitle);
    await openBoard(page, boardTitle);

    // Click the back arrow (has aria-label)
    await page.getByRole("button", { name: "Back to boards" }).click();

    // Should be back on the board list
    await expect(page.getByText("Your Boards")).toBeVisible();
  });

  test("can delete a board", async ({ page }) => {
    const boardTitle = uniqueName("Delete Me");

    await createBoard(page, boardTitle);
    await expect(page.getByText(boardTitle)).toBeVisible();

    // Hover to reveal delete button, then click
    page.on("dialog", (dialog) => dialog.accept());
    const boardCard = page.getByRole("button", { name: `Open board ${boardTitle}` });
    await boardCard.hover();
    await page.getByRole("button", { name: `Delete board ${boardTitle}` }).click();

    await expect(page.getByText(boardTitle)).toBeHidden();
  });

  test("cancel delete keeps board", async ({ page }) => {
    const boardTitle = uniqueName("Keep Me");

    await createBoard(page, boardTitle);
    await expect(page.getByText(boardTitle)).toBeVisible();

    // Dismiss the confirm dialog
    page.on("dialog", (dialog) => dialog.dismiss());
    const boardCard = page.getByRole("button", { name: `Open board ${boardTitle}` });
    await boardCard.hover();
    await page.getByRole("button", { name: `Delete board ${boardTitle}` }).click();

    // Board should still be there
    await expect(page.getByText(boardTitle)).toBeVisible();
  });

  test("create button is disabled when title is empty", async ({ page }) => {
    const createBtn = page.getByRole("button", { name: "Create" });
    await expect(createBtn).toBeDisabled();

    // Type something — button should be enabled
    await page.getByPlaceholder("New board name...").fill("Test");
    await expect(createBtn).toBeEnabled();

    // Clear — button should be disabled again
    await page.getByPlaceholder("New board name...").fill("");
    await expect(createBtn).toBeDisabled();
  });

  test("boards persist after page reload", async ({ page }) => {
    const boardTitle = uniqueName("Persistent Board");

    await createBoard(page, boardTitle);
    await expect(page.getByText(boardTitle).first()).toBeVisible();

    await page.reload();
    await expect(page.getByText(boardTitle).first()).toBeVisible({ timeout: 5000 });
  });

  test("board shows creation date", async ({ page }) => {
    const boardTitle = uniqueName("Dated Board");

    await createBoard(page, boardTitle);

    // The board card should display today's date
    const today = new Date().toLocaleDateString("en-US");
    const boardCard = page.getByRole("button", { name: `Open board ${boardTitle}` });
    await expect(boardCard.getByText(today)).toBeVisible();
  });

  test("boards disappear after logout", async ({ page }) => {
    const boardTitle = uniqueName("Private Board");

    await createBoard(page, boardTitle);
    await expect(page.getByText(boardTitle)).toBeVisible();

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Should not see board data on login page
    await expect(page.getByText(boardTitle)).toBeHidden();
  });

  test("can navigate back and forth between boards", async ({ page }) => {
    const boardAlpha = uniqueName("Board Alpha");
    const boardBeta = uniqueName("Board Beta");

    await createBoard(page, boardAlpha);
    await createBoard(page, boardBeta);

    // Open first board
    await openBoard(page, boardAlpha);
    await expect(page.getByRole("heading", { name: boardAlpha })).toBeVisible();

    // Go back
    await page.getByRole("button", { name: "Back to boards" }).click();
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Open second board
    await openBoard(page, boardBeta);
    await expect(page.getByRole("heading", { name: boardBeta })).toBeVisible();

    // Go back again
    await page.getByRole("button", { name: "Back to boards" }).click();
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Both boards should still be listed
    await expect(page.getByText(boardAlpha)).toBeVisible();
    await expect(page.getByText(boardBeta)).toBeVisible();
  });
});
