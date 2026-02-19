import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
} from "./helpers";

// NOTE: Drag-and-drop tests require low-level mouse APIs (boundingBox, mouse.move/down/up)
// and library-specific data attributes ([data-rfd-*]) because Playwright has no native
// DnD API that works with @hello-pangea/dnd. These patterns are pragmatically necessary
// and documented as exceptions to BROWSER_TESTING_STANDARDS_2.md Rule 4/5.

test.describe("Drag and Drop", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
    await createBoard(page, "DnD Board");
    await openBoard(page, "DnD Board");
    await addColumn(page, "To Do");
    await addColumn(page, "Done");
    await addCard(page, "To Do", "Drag Me");
  });

  test("card is draggable", async ({ page }) => {
    // Verify the card exists in the first column
    await expect(page.getByText("Drag Me")).toBeVisible();

    // The card wrapper should have drag handle attributes (from @hello-pangea/dnd)
    // NOTE: data-rfd-* attributes are library-generated, not CSS selectors we control
    const draggable = page.locator("[data-rfd-draggable-id]").filter({ hasText: "Drag Me" });
    await expect(draggable).toBeVisible();
  });

  test("columns are drop targets", async ({ page }) => {
    // Both columns should be droppable zones
    const droppables = page.locator("[data-rfd-droppable-id]");
    await expect(droppables).toHaveCount(2);
  });

  test("can drag a card between columns", async ({ page }) => {
    const card = page.getByText("Drag Me");

    // Get the source and destination droppable positions
    // NOTE: boundingBox() is required for programmatic drag — no Playwright alternative
    const destColumn = page.locator("[data-rfd-droppable-id]").nth(1);

    const cardBox = await card.boundingBox();
    const destBox = await destColumn.boundingBox();

    if (!cardBox || !destBox) {
      test.fail(true, "Could not get bounding boxes");
      return;
    }

    // Perform the drag operation
    await page.mouse.move(
      cardBox.x + cardBox.width / 2,
      cardBox.y + cardBox.height / 2,
    );
    await page.mouse.down();

    // Move to the destination column
    await page.mouse.move(
      destBox.x + destBox.width / 2,
      destBox.y + destBox.height / 2,
      { steps: 10 },
    );
    await page.mouse.up();

    // After drop, verify the card is still visible
    await expect(page.getByText("Drag Me")).toBeVisible();

    // Verify card count changed via data-testid
    const todoCount = page.getByTestId("column-To Do").getByTestId("card-count");
    const doneCount = page.getByTestId("column-Done").getByTestId("card-count");
    await expect(todoCount).toHaveText("0", { timeout: 5000 });
    await expect(doneCount).toHaveText("1", { timeout: 5000 });
  });

  test("multiple cards can be dragged to different columns", async ({ page }) => {
    await addCard(page, "To Do", "Card 2");
    await addCard(page, "To Do", "Card 3");

    const todoCount = page.getByTestId("column-To Do").getByTestId("card-count");
    await expect(todoCount).toHaveText("3");

    // Drag "Drag Me" (the first card from beforeEach) to Done
    const card = page.getByText("Drag Me");
    const destColumn = page.locator("[data-rfd-droppable-id]").nth(1);

    const cardBox = await card.boundingBox();
    const destBox = await destColumn.boundingBox();
    if (!cardBox || !destBox) {
      test.fail(true, "Could not get bounding boxes");
      return;
    }

    await page.mouse.move(cardBox.x + cardBox.width / 2, cardBox.y + cardBox.height / 2);
    await page.mouse.down();
    await page.mouse.move(destBox.x + destBox.width / 2, destBox.y + destBox.height / 2, { steps: 10 });
    await page.mouse.up();

    // Wait for counts to update
    await expect(todoCount).toHaveText("2", { timeout: 5000 });
    const doneCount = page.getByTestId("column-Done").getByTestId("card-count");
    await expect(doneCount).toHaveText("1", { timeout: 5000 });
  });

  test("each card has a unique draggable id", async ({ page }) => {
    await addCard(page, "To Do", "Second Card");

    const draggables = page.locator("[data-rfd-draggable-id]");
    await expect(draggables).toHaveCount(2);

    // Verify they have different IDs
    const id1 = await draggables.nth(0).getAttribute("data-rfd-draggable-id");
    const id2 = await draggables.nth(1).getAttribute("data-rfd-draggable-id");
    expect(id1).not.toEqual(id2);
  });

  test("droppable count matches column count", async ({ page }) => {
    // Initially 2 columns from beforeEach
    await expect(page.locator("[data-rfd-droppable-id]")).toHaveCount(2);

    // Add a third column
    await addColumn(page, "In Progress");
    await expect(page.locator("[data-rfd-droppable-id]")).toHaveCount(3);
  });
});
