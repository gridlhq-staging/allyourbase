import {
  arrangePendingFirstSearch,
  arrangeSearchApiFailure,
  expect,
  loadSeededCatalog,
  test,
  ZERO_HIT_QUERY,
} from "../fixtures";

test("first load shows a loading state while the first search request is pending", async ({
  page,
  appURL,
}) => {
  const pendingSearch = await arrangePendingFirstSearch(page);

  await page.goto(appURL);

  // Arrange precondition: the search request was intercepted and is being held.
  await pendingSearch.waitForIntercepted;

  await expect(page.getByRole("status")).toHaveText(
    "Loading product search...",
  );

  // Release the held request(s) and confirm the demo leaves the loading state
  // and renders the seeded catalog once the search resolves.
  await pendingSearch.release();

  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
});

test("API failure shows an error state instead of an empty widget shell", async ({
  page,
  appURL,
}) => {
  await arrangeSearchApiFailure(page);

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
