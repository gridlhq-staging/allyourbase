import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, uniqueName } from "./helpers";

test.describe("Libraries", () => {
  test.beforeEach(async ({ page }) => {
    await registerUser(page);
  });

  test("dashboard loads for new user", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
    await expect(page.getByRole("button", { name: /New Library/ })).toBeVisible();
  });

  test("can create a library", async ({ page }) => {
    const name = uniqueName("Power Tools");
    await createLibrary(page, name);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
  });

  test("library shows slug link after creation", async ({ page }) => {
    const name = uniqueName("Board Games");
    await createLibrary(page, name);
    // The library card shows the slug as "shareborough.com/l/board-games-..."
    await expect(page.locator("text=/\\/l\\/board-games/").first()).toBeVisible();
  });

  test("can navigate to library detail", async ({ page }) => {
    const name = uniqueName("Kitchen Gadgets");
    await createLibrary(page, name);
    await openLibrary(page, name);
    await expect(page.getByRole("heading", { name })).toBeVisible();
  });

  test("library detail shows empty state", async ({ page }) => {
    const name = uniqueName("Empty Lib");
    await createLibrary(page, name);
    await openLibrary(page, name);
    await expect(page.getByText("No items yet")).toBeVisible();
    await expect(page.getByRole("link", { name: "Add Your First Item" })).toBeVisible();
  });

  test("can create multiple libraries", async ({ page }) => {
    const name1 = uniqueName("Books");
    const name2 = uniqueName("Camping Gear");
    await createLibrary(page, name1);
    await createLibrary(page, name2);
    await expect(page.getByText(name1)).toBeVisible();
    await expect(page.getByText(name2)).toBeVisible();
  });

  test("library detail shows share link and copy button", async ({ page }) => {
    const name = uniqueName("Music Stuff");
    await createLibrary(page, name);
    await openLibrary(page, name);
    await expect(page.getByText("Share link:")).toBeVisible();
    await expect(page.getByRole("button", { name: "Copy Link" })).toBeVisible();
  });
});
