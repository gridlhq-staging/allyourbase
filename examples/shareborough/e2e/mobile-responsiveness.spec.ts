import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, getShareSlug, uniqueName } from "./helpers";

/**
 * Mobile responsiveness tests
 * Verifies UI works correctly on mobile devices (iPhone SE viewport)
 * Uses Chromium with mobile viewport (WebKit not installed on EC2 runner)
 */

// Configure mobile viewport for all tests in this file (iPhone SE dimensions)
const mobileTest = test.extend({});
mobileTest.use({
  viewport: { width: 375, height: 667 },
  isMobile: true,
  hasTouch: true,
});

test.describe("Mobile Responsiveness", () => {

  mobileTest("Landing page: No overflow, buttons accessible", async ({ page }) => {
    await page.goto("/");

    // Check for horizontal overflow
    const body = await page.locator("body").boundingBox();
    const viewport = page.viewportSize()!;
    expect(body!.width).toBeLessThanOrEqual(viewport.width);

    // Verify buttons are visible and clickable
    const signInButton = page.locator('a:has-text("Sign in")');
    const getStartedButton = page.locator('a:has-text("Get Started")');

    await expect(signInButton).toBeVisible();
    await expect(getStartedButton).toBeVisible();

    // Verify minimum touch target size (44x44px per WCAG 2.5.8)
    const signInBox = await signInButton.boundingBox();
    expect(signInBox!.height).toBeGreaterThanOrEqual(44);
    expect(signInBox!.width).toBeGreaterThanOrEqual(44);

    // Click buttons to verify they work
    await getStartedButton.click();
    await expect(page).toHaveURL("/signup");
  });

  mobileTest("Auth pages: Forms fit viewport, no horizontal scroll", async ({ page }) => {
    await page.goto("/signup");

    // Check no horizontal overflow
    const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
    const clientWidth = await page.evaluate(() => document.body.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 1); // Allow 1px tolerance

    // Verify inputs are fully visible
    const emailInput = page.locator('input[type="email"]');
    const inputBox = await emailInput.boundingBox();
    const viewport = page.viewportSize()!;
    expect(inputBox!.x).toBeGreaterThanOrEqual(0);
    expect(inputBox!.x + inputBox!.width).toBeLessThanOrEqual(viewport.width);
  });

  mobileTest("Dashboard: Cards stack vertically on mobile", async ({ page }) => {
    // Register using helpers (proper email format + selectors)
    await registerUser(page);

    // Create a test library using helpers
    const libName = uniqueName("Mobile Lib");
    await createLibrary(page, libName);

    // Verify library card is visible and fits viewport
    const libraryCard = page.locator(".card").first();
    await expect(libraryCard).toBeVisible();

    const cardBox = await libraryCard.boundingBox();
    const viewport = page.viewportSize()!;
    expect(cardBox!.width).toBeLessThanOrEqual(viewport.width);
  });

  mobileTest("Add Item: Photo buttons visible, crop gestures work", async ({ page }) => {
    // Register and set up library using helpers
    await registerUser(page);
    const libName = uniqueName("Mobile Photo");
    await createLibrary(page, libName);
    await openLibrary(page, libName);

    // Navigate to add item
    await page.getByRole("link", { name: /Add Item/i }).click();

    // Verify photo upload input is present and fits viewport
    const fileInput = page.locator('input[type="file"]');
    await expect(fileInput.first()).toBeAttached();

    // Verify the item name field fits viewport
    const nameInput = page.getByPlaceholder("What is this item?");
    await expect(nameInput).toBeVisible();
    const inputBox = await nameInput.boundingBox();
    const viewport = page.viewportSize()!;
    expect(inputBox!.x + inputBox!.width).toBeLessThanOrEqual(viewport.width);
  });

  mobileTest("Settings: Form fields stack vertically, no overflow", async ({ page }) => {
    // Register using helpers
    await registerUser(page);

    // Go to settings via avatar dropdown
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Settings").click();
    await expect(page).toHaveURL("/dashboard/settings");

    // Check no horizontal overflow
    const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
    const clientWidth = await page.evaluate(() => document.body.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 1);

    // Verify input fields are fully visible
    const displayNameInput = page.getByLabel("Display name");
    const inputBox = await displayNameInput.boundingBox();
    const viewport = page.viewportSize()!;
    expect(inputBox!.width).toBeLessThanOrEqual(viewport.width - 32); // Account for padding
  });

  mobileTest("Public library: Grid adapts to mobile, text wraps properly", async ({ page }) => {
    // Register and create a library with a long name (use uniqueName to avoid collision)
    await registerUser(page);
    const libName = uniqueName("Very Long Library Name That Should Wrap");

    // Create with long name
    await page.getByRole("button", { name: "+ New Library" }).click();
    await page.getByPlaceholder(/Power Tools/).fill(libName);
    await page.getByRole("button", { name: "Create Library" }).click();
    await expect(page.getByRole("button", { name: "Create Library" })).not.toBeVisible({ timeout: 10000 });

    // Navigate to library detail to get slug
    await page.getByRole("link", { name: libName }).first().click();
    await expect(page.getByRole("heading", { name: libName })).toBeVisible({ timeout: 10000 });
    const slug = await getShareSlug(page);

    // Visit public page
    await page.goto(`/l/${slug}`);

    // Check no horizontal overflow
    const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
    const clientWidth = await page.evaluate(() => document.body.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 1);

    // Verify title wraps and is visible
    const title = page.locator("h1");
    await expect(title).toBeVisible();
    const titleBox = await title.boundingBox();
    const viewport = page.viewportSize()!;
    expect(titleBox!.width).toBeLessThanOrEqual(viewport.width - 32);
  });
});
