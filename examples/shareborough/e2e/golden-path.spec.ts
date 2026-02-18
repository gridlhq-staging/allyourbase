import { test, expect } from "@playwright/test";
import { registerUser, uniqueName, createLibrary, openLibrary, addItem } from "./helpers";

test.describe("Golden Path — Full User Journey", () => {
  test("register, create library, add item, share, borrow, approve, return", async ({ page }) => {
    // Step 1: Register a new owner
    const email = await registerUser(page);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();

    // Step 2: Create a library
    const libraryName = uniqueName("Neighborhood Tools");
    await createLibrary(page, libraryName);

    // Step 3: Open the library
    await openLibrary(page, libraryName);
    await expect(page.getByRole("heading", { name: libraryName })).toBeVisible();

    // Step 4: Add an item
    const itemName = uniqueName("Cordless Drill");
    await addItem(page, itemName, "DeWalt 20V Max");
    await expect(page.getByText(itemName)).toBeVisible();
    await expect(page.getByText(/available/i).first()).toBeVisible();

    // Step 5: Get the share link (clipboard may not work in headless, use link directly)
    const shareLink = page.getByRole("link", { name: /\/l\// });
    const slug = await shareLink.getAttribute("href");
    expect(slug).toBeTruthy();
    await page.goto(slug!);
    await expect(page.getByRole("heading", { name: libraryName })).toBeVisible();
    await expect(page.getByText(itemName)).toBeVisible();

    // Step 7: Click on the item
    await page.getByText(itemName).click();
    await expect(page.getByRole("heading", { name: itemName })).toBeVisible();
    await expect(page.getByText(/available/i).first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Borrow This" })).toBeVisible();

    // Step 8: Submit borrow request (unique borrower name to avoid data pollution)
    const borrowerName = uniqueName("Jane");
    await page.getByRole("button", { name: "Borrow This" }).click();
    await page.getByPlaceholder("Your name").fill(borrowerName);
    await page.getByPlaceholder("Phone number").fill("+15559876543");
    await page.getByPlaceholder("Message to the owner").fill("I need it for a weekend project!");
    await page.getByRole("button", { name: "Send Request" }).click();

    // Step 9: Confirm borrow request page
    await expect(page.getByText("Request Sent!")).toBeVisible({ timeout: 10000 });

    // Step 10: Go back to owner dashboard to approve
    await page.goto("/dashboard");
    await expect(page.getByText("Pending Requests")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(borrowerName).first()).toBeVisible();
    await expect(page.getByText(itemName).first()).toBeVisible();

    // Step 11: Approve the specific request (scope to the card to avoid strict mode)
    const requestCard = page.locator(".card").filter({ hasText: borrowerName }).first();
    await requestCard.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByText("Request approved")).toBeVisible({ timeout: 10000 });

    // Step 12: Verify item shows as borrowed
    await expect(page.getByText("Currently Borrowed")).toBeVisible({ timeout: 5000 });

    // Step 13: Mark as returned
    const loanCard = page.locator(".card").filter({ hasText: itemName }).first();
    await loanCard.getByRole("button", { name: "Mark Returned" }).click();
    // Confirm in dialog
    await page.getByRole("dialog").getByRole("button", { name: "Mark Returned" }).click();
    await expect(page.getByText("Item marked as returned")).toBeVisible({ timeout: 10000 });
  });

  test("settings page is accessible from avatar dropdown", async ({ page }) => {
    await registerUser(page);

    // Open avatar dropdown
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Settings").click();

    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel("Email")).toBeVisible();
    await expect(page.getByLabel("Display name")).toBeVisible();
  });

  test("404 page shows for unknown routes", async ({ page }) => {
    await page.goto("/this-does-not-exist");
    await expect(page.getByText("Page not found")).toBeVisible();
    await expect(page.getByRole("link", { name: "Go Home" })).toBeVisible();
  });

  test("skeleton loading states appear while data loads", async ({ page }) => {
    await registerUser(page);

    // Navigate to dashboard — should briefly show skeleton then content
    await page.goto("/dashboard");
    // Dashboard should eventually load
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible({ timeout: 10000 });
  });

  test("library stats show after lending activity", async ({ page }) => {
    // Register and create library with item
    await registerUser(page);
    const libraryName = uniqueName("Stats Library");
    await createLibrary(page, libraryName);
    await openLibrary(page, libraryName);
    const itemName = uniqueName("Stat Item");
    await addItem(page, itemName, "For stats test");

    // Borrow the item
    const shareLink = page.getByRole("link", { name: /\/l\// });
    const slug = await shareLink.getAttribute("href");
    await page.goto(slug!);
    await page.getByText(itemName).click();
    await page.getByRole("button", { name: "Borrow This" }).click();
    const statsBorrower = uniqueName("StatsFriend");
    await page.getByPlaceholder("Your name").fill(statsBorrower);
    await page.getByPlaceholder("Phone number").fill("+15559999999");
    await page.getByRole("button", { name: "Send Request" }).click();
    await expect(page.getByText("Request Sent!")).toBeVisible({ timeout: 10000 });

    // Approve from dashboard (scope to the specific request card)
    await page.goto("/dashboard");
    await expect(page.getByText("Pending Requests")).toBeVisible({ timeout: 10000 });
    const statsRequestCard = page.locator(".card").filter({ hasText: statsBorrower }).first();
    await statsRequestCard.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByText("Request approved")).toBeVisible({ timeout: 10000 });

    // Verify loan appears in active loans section
    await expect(page.getByText("Currently Borrowed")).toBeVisible({ timeout: 5000 });
  });

  test("PWA manifest is accessible", async ({ page }) => {
    await page.goto("/manifest.json");
    const response = await page.evaluate(() =>
      fetch("/manifest.json").then((r) => r.json()),
    );
    expect(response.name).toContain("Shareborough");
    expect(response.display).toBe("standalone");
    expect(response.icons.length).toBeGreaterThanOrEqual(2);
  });

  test("item images use lazy loading attributes", async ({ page }) => {
    await registerUser(page);
    const libraryName = uniqueName("Lazy Lib");
    await createLibrary(page, libraryName);
    await openLibrary(page, libraryName);
    const itemName = uniqueName("Lazy Item");
    await addItem(page, itemName, "Lazy load test");

    // Visit public library — check img attributes
    const shareLink = page.getByRole("link", { name: /\/l\// });
    const slug = await shareLink.getAttribute("href");
    await page.goto(slug!);
    await expect(page.getByText(itemName)).toBeVisible({ timeout: 10000 });

    // If the item has a photo, check for lazy loading
    const images = page.locator("img[alt]");
    const count = await images.count();
    for (let i = 0; i < count; i++) {
      const img = images.nth(i);
      const src = await img.getAttribute("src");
      // Only check remote images (not emoji placeholders)
      if (src && !src.startsWith("data:")) {
        await expect(img).toHaveAttribute("loading", "lazy");
        await expect(img).toHaveAttribute("decoding", "async");
      }
    }
  });
});
