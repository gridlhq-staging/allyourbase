import { test, expect } from "../fixtures";

test("load-and-verify, query highlighting, facet filtering, and pagination all work against live AYB search", async ({
  page,
  appURL,
}) => {
  await page.goto(appURL);

  const seededHit = page.getByTestId("hit-red-notebook");
  await expect(seededHit).toBeVisible({ timeout: 20_000 });
  await expect(seededHit).toContainText("crimson ledger");
  await expect(page.getByTestId("results-summary")).toContainText("14 results");

  const searchBox = page.getByRole("searchbox");
  await searchBox.fill("red");
  await expect(page.getByTestId("results-summary")).toContainText("1 result");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeHidden();
  await expect(page.getByTestId("hit-red-notebook-title-highlight")).toContainText(
    "Red",
  );

  await searchBox.fill("");
  const pageTwoLink = page.getByRole("link", { name: "Page 2" });
  await expect(pageTwoLink).toBeVisible();
  await pageTwoLink.click();
  await expect(page.getByTestId("hit-steel-cable-tray")).toBeVisible();
  await expect(page.getByTestId("hit-red-notebook")).toBeHidden();

  const kitchenFacet = page.getByRole("checkbox", { name: /Kitchen/i });
  await kitchenFacet.check();
  await expect(page.getByTestId("results-summary")).toContainText("3 results");
  await expect(page.getByTestId("hit-ceramic-coffee-mug")).toBeVisible();
  await expect(page.getByTestId("hit-glass-water-bottle")).toBeVisible();
  await expect(page.getByTestId("hit-steel-cable-tray")).toBeHidden();
});
