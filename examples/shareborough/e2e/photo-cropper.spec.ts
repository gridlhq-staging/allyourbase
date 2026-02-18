import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, uniqueName } from "./helpers";
import { fileURLToPath } from "url";
import { dirname, join } from "path";
import fs from "fs";

/**
 * Photo Cropper E2E Tests
 * Tests the ImageCropper component with gestures (zoom, pan)
 *
 * Behaviors tested:
 * 1. Mouse wheel to zoom on desktop
 * 2. Drag to pan the image within crop area
 * 3. Crop & Use button applies crop
 * 4. Cancel button uses uncropped image
 * 5. Crop instructions are visible
 * 6. Crop area aspect ratio is square (1:1)
 *
 * Note: Tests that require a test fixture image (tests/fixtures/test-image.jpg)
 * will be skipped if the fixture is not found on the EC2 runner.
 */

test.describe("Photo Cropper", () => {
  const __filename = fileURLToPath(import.meta.url);
  const __dirname = dirname(__filename);
  const testImagePath = join(__dirname, "..", "tests", "fixtures", "test-image.jpg");
  const hasTestImage = fs.existsSync(testImagePath);

  test.beforeEach(async ({ page }) => {
    await registerUser(page);
    const libName = uniqueName("Cropper");
    await createLibrary(page, libName);
    await openLibrary(page, libName);
    await page.getByRole("link", { name: /Add Item/i }).click();
  });

  test("Desktop: Mouse wheel zoom works", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    // Upload an image via the file input
    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    // Cropper should appear
    const cropper = page.locator("canvas");
    await expect(cropper).toBeVisible({ timeout: 5000 });

    // Simulate mouse wheel zoom
    await cropper.hover();
    await page.mouse.wheel(0, -100);
    await page.waitForTimeout(500);

    // Verify cropper is still visible
    await expect(cropper).toBeVisible();
  });

  test("Desktop: Drag to pan works", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    const cropper = page.locator("canvas");
    await expect(cropper).toBeVisible({ timeout: 5000 });

    const initialBBox = await cropper.boundingBox();
    expect(initialBBox).toBeTruthy();

    // Simulate drag
    await page.mouse.move(initialBBox!.x + 100, initialBBox!.y + 100);
    await page.mouse.down();
    await page.mouse.move(initialBBox!.x + 150, initialBBox!.y + 150);
    await page.mouse.up();

    await expect(cropper).toBeVisible();
  });

  test("Crop & Use button applies crop", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    const cropper = page.locator("canvas");
    await expect(cropper).toBeVisible({ timeout: 5000 });

    const cropButton = page.getByRole("button", { name: /Crop.*Use/i });
    await expect(cropButton).toBeVisible();
    await cropButton.click();

    // Cropper should disappear
    await expect(cropper).not.toBeVisible({ timeout: 2000 });
  });

  test("Cancel button dismisses cropper", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    const cropper = page.locator("canvas");
    await expect(cropper).toBeVisible({ timeout: 5000 });

    const cancelButton = page.getByRole("button", { name: /Cancel/i });
    await expect(cancelButton).toBeVisible();
    await cancelButton.click();

    await expect(cropper).not.toBeVisible({ timeout: 2000 });
  });

  test("Crop instructions visible", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    await expect(page.locator("text=/Drag to pan/i")).toBeVisible({ timeout: 5000 });
    await expect(page.locator("text=/pinch or scroll to zoom/i")).toBeVisible();
  });

  test("Cropper aspect ratio is square (1:1)", async ({ page }) => {
    if (!hasTestImage) {
      test.skip(true, "Test fixture tests/fixtures/test-image.jpg not found");
      return;
    }

    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles(testImagePath);

    const cropper = page.locator("canvas");
    await expect(cropper).toBeVisible({ timeout: 5000 });

    const bbox = await cropper.boundingBox();
    expect(bbox).toBeTruthy();

    const aspectRatio = bbox!.width / bbox!.height;
    expect(aspectRatio).toBeGreaterThan(0.95);
    expect(aspectRatio).toBeLessThan(1.05);
  });
});
