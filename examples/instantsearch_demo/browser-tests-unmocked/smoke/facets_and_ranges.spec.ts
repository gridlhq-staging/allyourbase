import { test, expect } from "../fixtures";

test("multi-select category facets and price range filtering work against live AYB search", async ({
  page,
  appURL,
}) => {
  await page.goto(appURL);

  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });

  const kitchenFacet = page.getByRole("checkbox", { name: /Kitchen/i });
  await kitchenFacet.check();
  await expect(kitchenFacet).toBeChecked();
  await expect(kitchenFacet).toHaveAccessibleName(/Kitchen.*[1-9]\d*/i);
  await expect(page.getByTestId("hit-ceramic-coffee-mug")).toBeVisible();

  const travelFacet = page.getByRole("checkbox", { name: /Travel/i });
  await expect(travelFacet).toBeVisible();
  await expect(travelFacet).toHaveAccessibleName(/Travel.*[1-9]\d*/i);
  await travelFacet.check();

  await expect(kitchenFacet).toBeChecked();
  await expect(travelFacet).toBeChecked();
  await expect(kitchenFacet).toHaveAccessibleName(/Kitchen.*[1-9]\d*/i);
  await expect(travelFacet).toHaveAccessibleName(/Travel.*[1-9]\d*/i);
  await expect(page.getByTestId("hit-ceramic-coffee-mug")).toBeVisible();
  await expect(page.getByTestId("hit-canvas-weekender")).toBeVisible();

  await kitchenFacet.uncheck();
  await travelFacet.uncheck();
  await expect(page.getByTestId("results-summary")).toContainText("14 results");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();

  const priceRange = page.getByRole("region", { name: "Price range" });
  const rangeInputs = priceRange.getByRole("spinbutton");
  await rangeInputs.first().fill("4000");
  await rangeInputs.nth(1).fill("5000");
  await priceRange.getByRole("button", { name: "Go" }).click();

  await expect(page.getByTestId("results-summary")).toContainText("1 result");
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeVisible();
  await expect(page.getByTestId("hit-ceramic-coffee-mug")).toBeHidden();
});
