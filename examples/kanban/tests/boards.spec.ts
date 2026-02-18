import { test, expect } from "@playwright/test";
import { registerUser, createBoard, openBoard } from "./helpers";

test.describe("Boards", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
  });

  test("shows empty state when no boards exist", async ({ page }) => {
    await expect(page.getByText("No boards yet")).toBeVisible();
    await expect(page.getByText("Create your first board above")).toBeVisible();
  });

  test("can create a board", async ({ page }) => {
    await createBoard(page, "My Test Board");
    await expect(page.getByText("My Test Board")).toBeVisible();
    // Empty state should be gone
    await expect(page.getByText("No boards yet")).not.toBeVisible();
  });

  test("can create multiple boards", async ({ page }) => {
    await createBoard(page, "Board 1");
    await createBoard(page, "Board 2");
    await createBoard(page, "Board 3");

    await expect(page.getByText("Board 1")).toBeVisible();
    await expect(page.getByText("Board 2")).toBeVisible();
    await expect(page.getByText("Board 3")).toBeVisible();
  });

  test("can navigate into a board", async ({ page }) => {
    await createBoard(page, "Navigate Test");
    await openBoard(page, "Navigate Test");
    // Should see the board header with the board title
    await expect(
      page.getByRole("heading", { name: "Navigate Test" }),
    ).toBeVisible();
    // Should see the "Live" badge
    await expect(page.getByText("Live")).toBeVisible();
  });

  test("can navigate back from a board", async ({ page }) => {
    await createBoard(page, "Back Test");
    await openBoard(page, "Back Test");

    // Click the back arrow
    await page.locator("button").filter({ has: page.locator("svg") }).first().click();

    // Should be back on the board list
    await expect(page.getByText("Your Boards")).toBeVisible();
  });

  test("can delete a board", async ({ page }) => {
    await createBoard(page, "Delete Me");
    await expect(page.getByText("Delete Me")).toBeVisible();

    // Hover to reveal delete button, then click
    page.on("dialog", (dialog) => dialog.accept());
    const boardCard = page.getByRole("heading", { name: "Delete Me" }).locator("../..");
    await boardCard.hover();
    await boardCard.getByRole("button", { name: "Delete board" }).click();

    await expect(page.getByText("Delete Me")).not.toBeVisible();
  });

  test("cancel delete keeps board", async ({ page }) => {
    await createBoard(page, "Keep Me");
    await expect(page.getByText("Keep Me")).toBeVisible();

    // Dismiss the confirm dialog
    page.on("dialog", (dialog) => dialog.dismiss());
    const boardCard = page.getByRole("heading", { name: "Keep Me" }).locator("../..");
    await boardCard.hover();
    await boardCard.getByRole("button", { name: "Delete board" }).click();

    // Board should still be there
    await expect(page.getByText("Keep Me")).toBeVisible();
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
    await createBoard(page, "Persistent Board");
    await expect(page.getByText("Persistent Board")).toBeVisible();

    await page.reload();
    await expect(page.getByText("Persistent Board")).toBeVisible({ timeout: 5000 });
  });

  test("board shows creation date", async ({ page }) => {
    await createBoard(page, "Dated Board");

    // The board card should display today's date, scoped to the specific card
    const today = new Date().toLocaleDateString("en-US");
    const boardCard = page.getByRole("heading", { name: "Dated Board" }).locator("../..");
    await expect(boardCard.getByText(today)).toBeVisible();
  });

  test("boards disappear after logout", async ({ page }) => {
    await createBoard(page, "Private Board");
    await expect(page.getByText("Private Board")).toBeVisible();

    // Logout
    await page.getByText("Sign out").click();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();

    // Should not see board data on login page
    await expect(page.getByText("Private Board")).not.toBeVisible();
  });

  test("can navigate back and forth between boards", async ({ page }) => {
    await createBoard(page, "Board Alpha");
    await createBoard(page, "Board Beta");

    // Open first board
    await openBoard(page, "Board Alpha");
    await expect(page.getByRole("heading", { name: "Board Alpha" })).toBeVisible();

    // Go back
    await page.locator("button").filter({ has: page.locator("svg") }).first().click();
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Open second board
    await openBoard(page, "Board Beta");
    await expect(page.getByRole("heading", { name: "Board Beta" })).toBeVisible();

    // Go back again
    await page.locator("button").filter({ has: page.locator("svg") }).first().click();
    await expect(page.getByText("Your Boards")).toBeVisible();

    // Both boards should still be listed
    await expect(page.getByText("Board Alpha")).toBeVisible();
    await expect(page.getByText("Board Beta")).toBeVisible();
  });
});
