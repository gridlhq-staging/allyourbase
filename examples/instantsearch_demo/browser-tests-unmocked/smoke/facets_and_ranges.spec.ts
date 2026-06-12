import { test, expect } from "../fixtures";

test("query, brand facet OR behavior, and price range filtering work against live AYB search", async ({
  page,
  appURL,
}) => {
  await page.goto(appURL);

  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByTestId("results-summary")).toContainText("20 results");

  const searchBox = page.getByRole("searchbox");
  await searchBox.fill("red");
  await expect(page.getByTestId("results-summary")).toContainText("1 result");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeHidden();
  await expect(page.getByTestId("hit-red-notebook-title-highlight")).toContainText(
    "Red",
  );

  await searchBox.fill("");
  await expect(page.getByTestId("results-summary")).toContainText("20 results");

  const apexFacet = page.getByRole("checkbox", { name: /Apex/i });
  await apexFacet.check();
  await expect(apexFacet).toBeChecked();
  await expect(apexFacet).toHaveAccessibleName(/Apex.*[1-9]\d*/i);

  const beaconFacet = page.getByRole("checkbox", { name: /Beacon/i });
  await expect(beaconFacet).toBeVisible();
  await expect(beaconFacet).toHaveAccessibleName(/Beacon.*[1-9]\d*/i);
  await beaconFacet.check();

  await expect(apexFacet).toBeChecked();
  await expect(beaconFacet).toBeChecked();
  await expect(apexFacet).toHaveAccessibleName(/Apex.*[1-9]\d*/i);
  await expect(beaconFacet).toHaveAccessibleName(/Beacon.*[1-9]\d*/i);
  await expect(page.getByTestId("results-summary")).toContainText("8 results");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeVisible();

  await apexFacet.uncheck();
  await beaconFacet.uncheck();
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible();

  const priceRange = page.getByRole("region", { name: "Price range" });
  const rangeInputs = priceRange.getByRole("spinbutton");
  await expect(rangeInputs.first()).toHaveAttribute("placeholder", "899");
  await expect(rangeInputs.nth(1)).toHaveAttribute("placeholder", "12999");
  await rangeInputs.first().fill("2000");
  await rangeInputs.nth(1).fill("6000");
  await priceRange.getByRole("button", { name: "Go" }).click();

  await expect(page.getByTestId("results-summary")).toContainText("9 results");
  await expect(page.getByTestId("hit-archival-pen-case")).toBeVisible();
  await expect(page.getByTestId("hit-brass-desk-lamp")).toBeVisible();
  await expect(page.getByTestId("hit-red-notebook")).toBeHidden();
  await expect(page.getByTestId("hit-aluminum-carry-on")).toBeHidden();

  await page.goto(appURL);
  await expect(page.getByTestId("results-summary")).toContainText("20 results");

  const resetPriceRange = page.getByRole("region", { name: "Price range" });
  const resetRangeInputs = resetPriceRange.getByRole("spinbutton");
  await resetRangeInputs.first().fill("12000");
  await resetRangeInputs.nth(1).fill("12999");
  await resetPriceRange.getByRole("button", { name: "Go" }).click();

  await expect(page.getByTestId("results-summary")).toContainText("1 result");
  await expect(page.getByTestId("hit-aluminum-carry-on")).toBeVisible();
});
