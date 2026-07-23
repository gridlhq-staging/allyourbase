import { expect, loadSeededCatalog, test } from "../fixtures";
import { type Page } from "@playwright/test";

async function expectNoHorizontalOverflow(page: Page, screenName: string): Promise<void> {
  // Horizontal overflow is the most common mobile regression this spec guards.
  // eslint-disable-next-line no-restricted-syntax
  const hasNoHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
  expect(hasNoHorizontalOverflow, `${screenName} should not create horizontal document overflow`).toBe(true);
}

test("mobile seeded search query and category facet stay usable", async ({ page, appURL }) => {
  await loadSeededCatalog(page, appURL);

  await expect(page.getByTestId("hit-red-notebook")).toContainText("crimson ledger");
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
  await expectNoHorizontalOverflow(page, "initial seeded catalog");

  const searchBox = page.getByRole("searchbox");
  await searchBox.fill("red");
  await expect(page.getByTestId("results-summary")).toContainText("1 result");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeHidden();
  await expect(page.getByTestId("hit-red-notebook-title-highlight")).toContainText("Red");
  await expectNoHorizontalOverflow(page, "red query results");

  await searchBox.fill("");
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
  const kitchenFacet = page.getByRole("checkbox", { name: /Kitchen/i });
  await expect(kitchenFacet).toBeVisible();
  await kitchenFacet.check();
  await expect(kitchenFacet).toBeChecked();
  await expect(page.getByTestId("results-summary")).toContainText("4 results");
  await expect(page.getByTestId("hit-ceramic-coffee-mug")).toBeVisible();
  await expect(page.getByTestId("hit-glass-water-bottle")).toBeVisible();
  await expect(page.getByTestId("hit-red-notebook")).toBeHidden();
  await expectNoHorizontalOverflow(page, "kitchen facet results");
});
