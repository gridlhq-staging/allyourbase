import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, addItem, uniqueName } from "./helpers";

test.describe("Items", () => {
  let libName: string;

  test.beforeEach(async ({ page }) => {
    libName = uniqueName("Tools");
    await registerUser(page);
    await createLibrary(page, libName);
    await openLibrary(page, libName);
  });

  test("can add an item to a library", async ({ page }) => {
    await addItem(page, "Cordless Drill", "18V with two batteries");
    await expect(page.getByText("Cordless Drill")).toBeVisible();
  });

  test("item shows available status", async ({ page }) => {
    await addItem(page, "Jigsaw");
    await expect(page.locator(".badge-available", { hasText: "available" })).toBeVisible();
  });

  test("can add multiple items", async ({ page }) => {
    await addItem(page, "Hammer");
    await addItem(page, "Wrench Set");
    await expect(page.getByText("Hammer")).toBeVisible();
    await expect(page.getByText("Wrench Set")).toBeVisible();
  });

  test("can navigate back to dashboard from library detail", async ({ page }) => {
    await page.getByRole("link", { name: "My Libraries", exact: true }).click();
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
  });

  test("can delete an item", async ({ page }) => {
    await addItem(page, "Disposable Item");
    await expect(page.getByText("Disposable Item")).toBeVisible();

    // Hover to reveal delete button and click it
    const itemCard = page.locator(".card").filter({ hasText: "Disposable Item" }).first();
    await itemCard.hover();

    // Click delete — may open a ConfirmDialog modal or browser dialog
    page.on("dialog", (dialog) => dialog.accept());
    await itemCard.getByRole("button", { name: "Delete" }).click();

    // If a modal dialog appeared, confirm it
    const dialogDeleteBtn = page.getByRole("dialog").getByRole("button", { name: "Delete" });
    if (await dialogDeleteBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
      await dialogDeleteBtn.click();
    }

    // Item should be gone — check heading specifically to avoid matching dialog text
    await expect(itemCard).not.toBeVisible({ timeout: 5000 });
  });
});
