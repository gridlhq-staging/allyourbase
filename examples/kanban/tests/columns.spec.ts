import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
} from "./helpers";

test.describe("Columns", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
    await createBoard(page, "Column Test Board");
    await openBoard(page, "Column Test Board");
  });

  test("shows add column input on empty board", async ({ page }) => {
    await expect(page.getByPlaceholder("+ Add column...")).toBeVisible();
  });

  test("can add a column", async ({ page }) => {
    await addColumn(page, "To Do");
    await expect(page.getByText("To Do")).toBeVisible();
  });

  test("can add multiple columns", async ({ page }) => {
    await addColumn(page, "To Do");
    await addColumn(page, "In Progress");
    await addColumn(page, "Done");

    await expect(page.getByText("To Do")).toBeVisible();
    await expect(page.getByText("In Progress")).toBeVisible();
    await expect(page.getByText("Done")).toBeVisible();
  });

  test("shows card count in column header", async ({ page }) => {
    await addColumn(page, "To Do");
    // Initially 0 cards — scoped to the column header's count span
    const columnHeader = page.locator("h3").filter({ hasText: "To Do" });
    await expect(columnHeader.locator("span")).toHaveText("0");
  });

  test("can delete a column", async ({ page }) => {
    await addColumn(page, "Delete Me");
    await expect(page.getByText("Delete Me")).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click the X button in the column header
    const columnHeader = page.getByText("Delete Me").locator("..");
    await columnHeader.locator("button").click();

    await expect(page.getByText("Delete Me")).not.toBeVisible();
  });

  test("cancel delete keeps column", async ({ page }) => {
    await addColumn(page, "Keep This");
    await expect(page.getByText("Keep This")).toBeVisible();

    // Dismiss the confirm dialog
    page.on("dialog", (dialog) => dialog.dismiss());

    const columnHeader = page.getByText("Keep This").locator("..");
    await columnHeader.locator("button").click();

    // Column should still be visible
    await expect(page.getByText("Keep This")).toBeVisible();
  });

  test("Add Column button only appears when text is typed", async ({ page }) => {
    // Initially no button
    await expect(page.getByRole("button", { name: "Add Column" })).not.toBeVisible();

    // Type a column name
    await page.getByPlaceholder("+ Add column...").fill("New Col");
    await expect(page.getByRole("button", { name: "Add Column" })).toBeVisible();

    // Clear the input — button should disappear
    await page.getByPlaceholder("+ Add column...").fill("");
    await expect(page.getByRole("button", { name: "Add Column" })).not.toBeVisible();
  });

  test("card count updates after adding cards", async ({ page }) => {
    await addColumn(page, "Counting");

    // Start at 0
    const columnHeader = page.locator("h3").filter({ hasText: "Counting" });
    await expect(columnHeader.locator("span")).toHaveText("0");

    await addCard(page, "Counting", "Card 1");
    await expect(columnHeader.locator("span")).toHaveText("1");

    await addCard(page, "Counting", "Card 2");
    await expect(columnHeader.locator("span")).toHaveText("2");
  });

  test("deleting column removes its cards too", async ({ page }) => {
    await addColumn(page, "Doomed");
    await addCard(page, "Doomed", "Card X");
    await addCard(page, "Doomed", "Card Y");

    await expect(page.getByText("Card X")).toBeVisible();
    await expect(page.getByText("Card Y")).toBeVisible();

    // Delete the column
    page.on("dialog", (dialog) => dialog.accept());
    const columnHeader = page.getByText("Doomed").locator("..");
    await columnHeader.locator("button").click();

    // Column and its cards should be gone
    await expect(page.getByText("Doomed")).not.toBeVisible();
    await expect(page.getByText("Card X")).not.toBeVisible();
    await expect(page.getByText("Card Y")).not.toBeVisible();
  });

  test("columns persist after page reload", async ({ page }) => {
    await addColumn(page, "Persistent Col");
    await expect(page.getByText("Persistent Col")).toBeVisible();

    await page.reload();
    // App uses client-side routing; reload returns to board list
    await openBoard(page, "Column Test Board");
    await expect(page.getByText("Persistent Col")).toBeVisible({ timeout: 5000 });
  });
});
