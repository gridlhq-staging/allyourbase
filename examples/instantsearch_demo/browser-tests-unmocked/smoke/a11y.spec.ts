import AxeBuilder from "@axe-core/playwright";
import { type Page } from "@playwright/test";
import {
  arrangePendingFirstSearch,
  arrangeSearchApiFailure,
  expect,
  loadSeededCatalog,
  test,
  ZERO_HIT_QUERY,
} from "../fixtures";

// Copied from ui/browser-tests-unmocked/smoke/accessibility.spec.ts::assertAccessible.
// Keep axe tags and critical/serious assertion semantics in sync; this demo omits the UI-only .cm-editor exclusion.
async function assertAccessible(page: Page, pageName: string): Promise<void> {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();

  for (const violation of results.violations) {
    console.log(
      `[a11y] ${pageName}: ${violation.id} impact=${violation.impact ?? "unknown"} nodes=${violation.nodes.length}`,
    );
  }

  const critical = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  const minor = results.violations.filter(
    (violation) => violation.impact === "moderate" || violation.impact === "minor",
  );

  if (minor.length > 0) {
    console.log(
      `[a11y] ${pageName}: ${minor.length} moderate/minor issue(s):`,
      minor.map((violation) => `${violation.id}: ${violation.help} (${violation.nodes.length} node(s))`),
    );
  }

  expect(
    critical,
    `${pageName}: ${critical.length} critical/serious a11y violation(s): ${critical
      .map((violation) => `${violation.id}: ${violation.help}`)
      .join("; ")}`,
  ).toHaveLength(0);
}

test.describe("InstantSearch accessibility states", () => {
  test("a11y: initial seeded results", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    await assertAccessible(page, "initial seeded results");
  });

  test("a11y: search results highlight", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    await page.getByRole("searchbox").fill("red");
    await expect(page.getByTestId("results-summary")).toContainText("1 result");
    await expect(page.getByTestId("hit-red-notebook-title-highlight")).toContainText("Red");
    await assertAccessible(page, "search results highlight");
  });

  test("a11y: pagination", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    await page.getByRole("link", { name: "Page 2" }).click();
    await expect(page.getByTestId("hit-steel-cable-tray")).toBeVisible();
    await assertAccessible(page, "pagination");
  });

  test("a11y: category facet", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    await page.getByRole("checkbox", { name: /Kitchen/i }).check();
    await expect(page.getByTestId("results-summary")).toContainText("4 results");
    await assertAccessible(page, "category facet");
  });

  test("a11y: brand OR facets", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    const apexFacet = page.getByRole("checkbox", { name: /Apex/i });
    const beaconFacet = page.getByRole("checkbox", { name: /Beacon/i });
    await apexFacet.check();
    await beaconFacet.check();
    await expect(apexFacet).toBeChecked();
    await expect(beaconFacet).toBeChecked();
    await expect(page.getByTestId("results-summary")).toContainText("8 results");
    await assertAccessible(page, "brand OR facets");
  });

  test("a11y: price range", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    const priceRange = page.getByRole("region", { name: "Price range" });
    const rangeInputs = priceRange.getByRole("spinbutton");
    await rangeInputs.first().fill("2000");
    await rangeInputs.nth(1).fill("6000");
    await priceRange.getByRole("button", { name: "Go" }).click();
    await expect(page.getByTestId("results-summary")).toContainText("9 results");
    await assertAccessible(page, "price range");
  });

  test("a11y: loading", async ({ page, appURL }) => {
    const pendingSearch = await arrangePendingFirstSearch(page);
    await page.goto(appURL);
    await pendingSearch.waitForIntercepted;
    await expect(page.getByRole("status")).toHaveText("Loading product search...");
    await assertAccessible(page, "loading");
    await pendingSearch.release();
  });

  test("a11y: API error", async ({ page, appURL }) => {
    await arrangeSearchApiFailure(page);
    await page.goto(appURL);
    await expect(page.getByRole("alert")).toHaveText(
      "We could not load products. Check the API connection and retry.",
    );
    await assertAccessible(page, "API error");
  });

  test("a11y: zero-hit facet-empty", async ({ page, appURL }) => {
    await loadSeededCatalog(page, appURL);
    await page.getByRole("searchbox").fill(ZERO_HIT_QUERY);
    await expect(page.getByRole("status")).toHaveText("No products match this search.");
    await expect(page.getByText("No category filters available.")).toBeVisible();
    await expect(page.getByText("No brand filters available.")).toBeVisible();
    await assertAccessible(page, "zero-hit facet-empty");
  });
});
