import { test, expect } from "@playwright/test";
import {
  registerUser,
  createBoard,
  openBoard,
  addColumn,
  addCard,
  loginUser,
  TEST_PASSWORD,
} from "./helpers";

test.describe("Realtime SSE", () => {
  test("card created in one tab appears in another", async ({ browser }) => {
    // Open two tabs with the same user
    const context = await browser.newContext();
    const page1 = await context.newPage();
    const email = await registerUser(page1);
    await createBoard(page1, "Realtime Board");
    await openBoard(page1, "Realtime Board");
    await addColumn(page1, "Column A");

    // Second tab: login with same user and open the same board
    const page2 = await context.newPage();
    await page2.goto("/");
    // Should already be authed via shared context (same localStorage)
    await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await page2.getByText("Realtime Board").click();
    await expect(
      page2.getByRole("heading", { name: "Realtime Board" }),
    ).toBeVisible({ timeout: 5000 });

    // Add a card in tab 1
    await addCard(page1, "Column A", "Realtime Card");
    await expect(page1.getByText("Realtime Card")).toBeVisible();

    // The card should appear in tab 2 via SSE (poll for it)
    await expect(page2.getByText("Realtime Card")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card deleted in one tab disappears in another", async ({
    browser,
  }) => {
    const context = await browser.newContext();
    const page1 = await context.newPage();
    await registerUser(page1);
    await createBoard(page1, "Delete Sync Board");
    await openBoard(page1, "Delete Sync Board");
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Will Be Deleted");

    // Second tab
    const page2 = await context.newPage();
    await page2.goto("/");
    await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await page2.getByText("Delete Sync Board").click();
    await expect(
      page2.getByRole("heading", { name: "Delete Sync Board" }),
    ).toBeVisible({ timeout: 5000 });

    // Verify card is visible in tab 2
    await expect(page2.getByText("Will Be Deleted")).toBeVisible({
      timeout: 5000,
    });

    // Delete the card in tab 1 via the modal
    await page1.getByText("Will Be Deleted").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByText("Delete card").click();
    await expect(page1.getByText("Will Be Deleted")).not.toBeVisible();

    // The card should disappear in tab 2 via SSE
    await expect(page2.getByText("Will Be Deleted")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("column created in one tab appears in another", async ({ browser }) => {
    const context = await browser.newContext();
    const page1 = await context.newPage();
    await registerUser(page1);
    await createBoard(page1, "Col Sync Board");
    await openBoard(page1, "Col Sync Board");

    // Second tab
    const page2 = await context.newPage();
    await page2.goto("/");
    await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await page2.getByText("Col Sync Board").click();
    await expect(
      page2.getByRole("heading", { name: "Col Sync Board" }),
    ).toBeVisible({ timeout: 5000 });

    // Add a column in tab 1
    await addColumn(page1, "Realtime Column");
    await expect(page1.getByText("Realtime Column")).toBeVisible();

    // Column should appear in tab 2 via SSE
    await expect(page2.getByText("Realtime Column")).toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });

  test("card updated in one tab updates in another", async ({ browser }) => {
    const context = await browser.newContext();
    const page1 = await context.newPage();
    await registerUser(page1);
    await createBoard(page1, "Update Sync Board");
    await openBoard(page1, "Update Sync Board");
    await addColumn(page1, "Col");
    await addCard(page1, "Col", "Original Name");

    // Second tab
    const page2 = await context.newPage();
    await page2.goto("/");
    await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await page2.getByText("Update Sync Board").click();
    await expect(
      page2.getByRole("heading", { name: "Update Sync Board" }),
    ).toBeVisible({ timeout: 5000 });
    await expect(page2.getByText("Original Name")).toBeVisible({ timeout: 5000 });

    // Edit the card in tab 1 (use role="dialog" and label)
    await page1.getByText("Original Name").click();
    await expect(page1.getByText("Edit Card")).toBeVisible();
    const modal = page1.getByRole("dialog");
    const titleInput = modal.getByLabel("Title");
    await titleInput.clear();
    await titleInput.fill("Renamed Card");
    await page1.getByRole("button", { name: "Save" }).click();
    await expect(page1.getByText("Renamed Card")).toBeVisible();

    // The updated name should appear in tab 2 via SSE
    await expect(page2.getByText("Renamed Card")).toBeVisible({
      timeout: 10000,
    });
    await expect(page2.getByText("Original Name")).not.toBeVisible();

    await context.close();
  });

  test("column deleted in one tab disappears in another", async ({ browser }) => {
    const context = await browser.newContext();
    const page1 = await context.newPage();
    await registerUser(page1);
    await createBoard(page1, "Col Delete Sync");
    await openBoard(page1, "Col Delete Sync");
    await addColumn(page1, "Ephemeral");

    // Second tab
    const page2 = await context.newPage();
    await page2.goto("/");
    await expect(page2.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
    await page2.getByText("Col Delete Sync").click();
    await expect(
      page2.getByRole("heading", { name: "Col Delete Sync" }),
    ).toBeVisible({ timeout: 5000 });
    await expect(page2.getByText("Ephemeral")).toBeVisible({ timeout: 5000 });

    // Delete column in tab 1 (use aria-label on delete button)
    page1.on("dialog", (dialog) => dialog.accept());
    await page1.getByRole("button", { name: "Delete column Ephemeral" }).click();
    await expect(page1.getByText("Ephemeral")).not.toBeVisible();

    // Column should disappear from tab 2 via SSE
    await expect(page2.getByText("Ephemeral")).not.toBeVisible({
      timeout: 10000,
    });

    await context.close();
  });
});
