import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, addItem, getShareSlug, uniqueName } from "./helpers";

test.describe("Public Pages", () => {
  test("non-existent library shows not found", async ({ page }) => {
    await page.goto("/l/this-library-does-not-exist-xyz");
    await expect(page.getByText("Library not found")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("link", { name: "Go Home" })).toBeVisible();
  });

  test("public library shows name and description", async ({ page }) => {
    const libName = uniqueName("Camping Gear");
    // Create a library as an owner with a description
    await registerUser(page);
    await page.getByRole("button", { name: "+ New Library" }).click();
    await page.getByPlaceholder(/Power Tools/).fill(libName);
    await page.getByPlaceholder("What kind of stuff").fill("Everything you need for the great outdoors");
    await page.getByRole("button", { name: "Create Library" }).click();
    await expect(page.getByRole("button", { name: "Create Library" })).not.toBeVisible({ timeout: 10000 });

    // Find the slug from the library detail page share link
    await page.getByRole("link", { name: libName }).first().click();
    await expect(page.getByRole("heading", { name: libName })).toBeVisible({ timeout: 10000 });
    const shareLinkEl = page.locator("a").filter({ hasText: /\/l\// });
    const href = await shareLinkEl.getAttribute("href");
    const slug = href!.replace("/l/", "");

    // Visit public page
    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await expect(publicPage.getByRole("heading", { name: libName })).toBeVisible({ timeout: 5000 });
    await expect(publicPage.getByText("Everything you need for the great outdoors")).toBeVisible();
    await publicPage.close();
  });

  test("search filters items by name", async ({ page }) => {
    const libName = uniqueName("Searchable");
    await registerUser(page);
    await createLibrary(page, libName);
    await openLibrary(page, libName);
    await addItem(page, "Red Hammer");
    await addItem(page, "Blue Wrench");
    await addItem(page, "Red Pliers");

    // Get slug from share link
    const slug = await getShareSlug(page);

    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await expect(publicPage.getByText("Red Hammer")).toBeVisible({ timeout: 5000 });
    await expect(publicPage.getByText("Blue Wrench")).toBeVisible();
    await expect(publicPage.getByText("Red Pliers")).toBeVisible();

    // Search for "Red"
    await publicPage.getByPlaceholder("Search items").fill("Red");
    await expect(publicPage.getByText("Red Hammer")).toBeVisible();
    await expect(publicPage.getByText("Red Pliers")).toBeVisible();
    await expect(publicPage.getByText("Blue Wrench")).not.toBeVisible();

    // Clear search
    await publicPage.getByPlaceholder("Search items").fill("");
    await expect(publicPage.getByText("Blue Wrench")).toBeVisible();
    await publicPage.close();
  });

  test("empty library shows appropriate message", async ({ page }) => {
    const libName = uniqueName("Empty Public");
    await registerUser(page);
    await createLibrary(page, libName);
    await openLibrary(page, libName);

    const slug = await getShareSlug(page);

    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await expect(publicPage.getByText("This library is empty")).toBeVisible({ timeout: 5000 });
    await publicPage.close();
  });
});
