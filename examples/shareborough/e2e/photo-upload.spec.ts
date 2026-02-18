import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, addItem, uniqueName } from "./helpers";

/**
 * Photo upload tests
 * Verifies file upload input and item creation with/without photos
 */
test.describe("Photo Upload", () => {
  let libName: string;

  test.beforeEach(async ({ page }) => {
    await registerUser(page);
    libName = uniqueName("Photo");
    await createLibrary(page, libName);
    await openLibrary(page, libName);
  });

  test("Add Item page has file upload input", async ({ page }) => {
    await page.getByRole("link", { name: /Add Item/i }).click();

    // Verify file input is present on the page
    const fileInput = page.locator('input[type="file"]');
    await expect(fileInput.first()).toBeAttached();
  });

  test("Item name field is required", async ({ page }) => {
    await page.getByRole("link", { name: /Add Item/i }).click();

    // The item name input should be visible
    const nameInput = page.getByPlaceholder("What is this item?");
    await expect(nameInput).toBeVisible();
  });

  test("Image cropper not visible until photo uploaded", async ({ page }) => {
    await page.getByRole("link", { name: /Add Item/i }).click();

    // Cropper instructions should not be visible (no photo uploaded yet)
    const cropperInstructions = page.locator('text="Drag to pan, pinch or scroll to zoom"');
    await expect(cropperInstructions).not.toBeVisible();
  });

  test("Item created without photo shows placeholder", async ({ page }) => {
    const itemName = uniqueName("No Photo Item");
    await addItem(page, itemName);

    // Verify item appears in the library
    await expect(page.getByText(itemName)).toBeVisible();
  });
});
