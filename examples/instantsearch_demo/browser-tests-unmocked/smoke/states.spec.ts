import { test, expect } from "../fixtures";

const SEARCH_API_PATTERN = "**/api/collections/instantsearch_products**";
const ZERO_HIT_QUERY = "zzzz-no-products";

// Wait for the seeded catalog to render using the same signal the passing
// search/facets specs rely on: the first seeded hit plus the "20 results"
// summary copy. Gating on this rendered state — rather than InstantSearch's
// internal status — keeps the wait deterministic even though the demo runs
// under React StrictMode, which double-mounts in dev and leaves a superseded
// search request pending.
async function loadSeededCatalog(
  page: import("@playwright/test").Page,
  appURL: string,
) {
  await page.goto(appURL);
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
}

test("first load shows a loading state while the first search request is pending", async ({
  page,
  appURL,
}) => {
  // Hold every intercepted search request and record it, so we can prove a
  // real request is pending before trusting the loading copy. Without this
  // proof the assertion could pass on InstantSearch's initial artificial
  // results frame, which renders the same loading state before any request.
  const heldRoutes: import("@playwright/test").Route[] = [];
  let released = false;
  let signalIntercepted: () => void;
  const firstRequestIntercepted = new Promise<void>((resolve) => {
    signalIntercepted = resolve;
  });

  await page.route(SEARCH_API_PATTERN, async (route) => {
    if (released) {
      await route.continue();
      return;
    }
    heldRoutes.push(route);
    signalIntercepted();
  });

  await page.goto(appURL);

  // Arrange precondition: the search request was intercepted and is being held.
  await firstRequestIntercepted;

  await expect(page.getByRole("status")).toHaveText(
    "Loading product search...",
  );

  // Release the held request(s) and confirm the demo leaves the loading state
  // and renders the seeded catalog once the search resolves.
  released = true;
  for (const route of heldRoutes) {
    await route.continue();
  }

  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
});

test("API failure shows an error state instead of an empty widget shell", async ({
  page,
  appURL,
}) => {
  await page.route(SEARCH_API_PATTERN, (route) => route.abort("failed"));

  await page.goto(appURL);

  await expect(page.getByRole("alert")).toHaveText(
    "We could not load products. Check the API connection and retry.",
  );
  await expect(page.getByTestId("hit-red-notebook")).toBeHidden();
});

test("zero-hit query shows result and facet empty states", async ({
  page,
  appURL,
}) => {
  await loadSeededCatalog(page, appURL);

  await page.getByRole("searchbox").fill(ZERO_HIT_QUERY);

  await expect(page.getByRole("status")).toHaveText(
    "No products match this search.",
  );
  await expect(page.getByText("No category filters available.")).toBeVisible();
  await expect(page.getByText("No brand filters available.")).toBeVisible();
});
