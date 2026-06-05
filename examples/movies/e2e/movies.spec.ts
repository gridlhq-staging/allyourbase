import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  expectInceptionNoteEmbedding,
  expectLocalChatStream,
  loginWithDemoAccount,
  searchForMovie,
} from "./helpers";

async function expectReachableByTab(page: Page, target: Locator): Promise<void> {
  for (let i = 0; i < 60; i += 1) {
    try {
      await expect(target).toBeFocused({ timeout: 50 });
      await expect(target).toBeVisible();
      return;
    } catch {
      await page.keyboard.press("Tab");
    }
  }

  await expect(target).toBeFocused();
}

test("default load shows full corpus with structured rows", async ({ page }) => {
  await loginWithDemoAccount(page);

  // Wait for the corpus-default load to settle (status flips from
  // "Loading movies..." / "Searching movies..." to "Showing N of M movies").
  const summary = page.getByTestId("results-summary");
  await expect(summary).toHaveText(/Showing \d+ of \d{3,} movies/, { timeout: 15000 });
  await expect(summary).toContainText(/of 250 movies/);

  const rows = page.getByTestId(/^search-result-row-/);
  await expect(rows.first()).toBeVisible();
  const rowCount = await rows.count();
  expect(rowCount).toBeGreaterThan(0);

  // Inspect the first visible result — all four structured fields must
  // be present with non-empty text content.
  const firstRow = rows.first();
  const firstSlug = await firstRow.getAttribute("data-testid");
  expect(firstSlug).toMatch(/^search-result-row-/);
  const slug = firstSlug!.replace("search-result-row-", "");

  const title = page.getByTestId(`search-result-title-${slug}`);
  const year = page.getByTestId(`search-result-year-${slug}`);
  const genre = page.getByTestId(`search-result-genre-${slug}`);
  const overview = page.getByTestId(`search-result-overview-${slug}`);
  await expect(title).toBeVisible();
  await expect(year).toBeVisible();
  await expect(genre).toBeVisible();
  await expect(overview).toBeVisible();
  expect((await title.textContent())?.trim()).toBeTruthy();
  expect((await year.textContent())?.trim()).toBeTruthy();
  expect((await genre.textContent())?.trim()).toBeTruthy();
  expect((await overview.textContent())?.trim()).toBeTruthy();
});

test("instant search by title prefix surfaces Inception with highlight", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });

  // Type partial title prefix without pressing Enter — search-as-you-type
  // (300ms debounce) plus Playwright auto-retry handle the timing.
  await page.getByPlaceholder("Search movies...").fill("incep");
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });

  // The server-rendered `_highlight` field is non-empty for matched rows,
  // so SearchResults renders the row's overview inside a span labelled
  // "Highlighted match". (For fuzzy/prefix matches like this one the
  // backend's ts_headline does not wrap matches in <b> — only full-token
  // tsquery matches get those delimiters — so we only assert that the
  // highlight span renders with non-empty content, proving the
  // `_highlight` data path is engaged.)
  const overview = page.getByTestId("search-result-overview-inception");
  await expect(overview).toBeVisible();
  const highlightSpan = overview.getByLabel("Highlighted match");
  await expect(highlightSpan).toBeVisible();
  const highlightText = (await highlightSpan.textContent())?.trim() ?? "";
  expect(highlightText.length).toBeGreaterThan(0);
  expect(highlightText.toLowerCase()).toContain("inception");
});

test("typo-tolerant search still matches Inception", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });

  // pg_trgm is enabled by default in AYB's managed Postgres
  // (internal/config/config_defaults_sections.go:29); fuzzy + typoThreshold
  // in the SDK search call should resolve this transposition.
  await page.getByPlaceholder("Search movies...").fill("Inceptoin");
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });
});

test("genre and decade facets filter and compose then clear", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });

  const allRows = page.getByTestId(/^search-result-row-/);
  const baselineCount = await allRows.count();
  expect(baselineCount).toBeGreaterThan(0);

  // Confirm Sci-Fi is present somewhere in the facets (sanity-check seed
  // data shape) before picking the first visible facet button.
  await expect(page.getByTestId("genre-facet-Sci-Fi")).toBeVisible();

  // Scope to buttons inside the genre-facet group — the regex
  // `/^genre-facet-/` would also match the parent group div, which has
  // no aria-pressed attribute.
  const facetButtons = page.getByTestId("genre-facet-group").getByRole("button");
  const firstFacet = facetButtons.first();
  const firstFacetTestId = await firstFacet.getAttribute("data-testid");
  expect(firstFacetTestId).toMatch(/^genre-facet-/);
  const chosenGenre = firstFacetTestId!.replace("genre-facet-", "");

  await firstFacet.click();
  await expect(firstFacet).toHaveAttribute("aria-pressed", "true");

  // Poll until the visible result set has settled to the filtered subset.
  // A one-shot allTextContents() read can otherwise observe pre-refetch
  // rows because the debounced refetch hasn't completed yet.
  await expect(async () => {
    const genres = await page.getByTestId(/^search-result-genre-/).allTextContents();
    expect(genres.length).toBeGreaterThan(0);
    for (const g of genres) expect(g.trim()).toBe(chosenGenre);
  }).toPass({ timeout: 15000 });

  // Add the 2010s decade filter on top — the two filters must compose.
  await page.getByTestId("decade-filter").selectOption("2010");
  await expect(async () => {
    const genres = await page.getByTestId(/^search-result-genre-/).allTextContents();
    const years = await page.getByTestId(/^search-result-year-/).allTextContents();
    expect(genres.length).toBeGreaterThan(0);
    expect(years).toHaveLength(genres.length);
    for (const g of genres) expect(g.trim()).toBe(chosenGenre);
    for (const y of years) {
      const yearNum = Number(y.trim());
      expect(yearNum).toBeGreaterThanOrEqual(2010);
      expect(yearNum).toBeLessThanOrEqual(2019);
    }
  }).toPass({ timeout: 15000 });
  const composedRowCount = await allRows.count();

  // Clear filters returns the broader unfiltered set.
  await page.getByTestId("clear-filters").click();
  await expect(page.getByTestId("clear-filters")).toBeDisabled();
  await expect(async () => {
    const clearedRowCount = await allRows.count();
    expect(clearedRowCount).toBeGreaterThan(composedRowCount);
  }).toPass({ timeout: 15000 });
});

test("no-results state renders when search query matches nothing", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });

  // Guaranteed no-match search query — `filtersActive` is true whenever the
  // search input is non-empty, so the App renders the `no-matches` state.
  // (Seed corpus saturates every genre × decade cell, so we drive the same
  // UI state with a search query instead of a filter combo.)
  await page.getByPlaceholder("Search movies...").fill("xyzzyqwertynomatchxyzzy");

  await expect(page.getByTestId("no-matches")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTestId("no-matches")).toHaveText("No movies match your filters");
  // clear-filters stays enabled because the search query counts as an
  // active filter even though no genre/decade is selected.
  await expect(page.getByTestId("clear-filters")).toBeEnabled();
});

test("mobile layout keeps the signed-in search surface reachable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await loginWithDemoAccount(page);

  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });
  const searchInput = page.getByPlaceholder("Search movies...");
  const firstGenreFacet = page.getByTestId("genre-facet-group").getByRole("button").first();
  const decadeFilter = page.getByTestId("decade-filter");
  const clearFilters = page.getByTestId("clear-filters");

  await expectReachableByTab(page, searchInput);
  await expectReachableByTab(page, firstGenreFacet);
  await page.keyboard.press("Space");
  await expect(firstGenreFacet).toHaveAttribute("aria-pressed", "true");
  await expect(clearFilters).toBeEnabled();
  await expectReachableByTab(page, decadeFilter);
  await expectReachableByTab(page, clearFilters);

  const rows = page.getByTestId(/^search-result-row-/);
  await expect(rows.first()).toBeVisible();
  await expectReachableByTab(page, rows.first());
  await page.keyboard.press("Enter");

  const notesPanel = page.getByTestId("selected-result-notes-panel");
  const chatSection = page.getByTestId("chat-section");
  const providerKeysSection = page.getByTestId("provider-keys-section");
  const noteInput = page.getByPlaceholder("Add a note about this movie...");
  const chatInput = page.getByPlaceholder("Ask about movies...");
  const providerKeyInput = page.getByPlaceholder("Vault secret name...");
  const signOutButton = page.getByRole("button", { name: "Sign out" });

  await expect(notesPanel).toBeVisible();
  await expectReachableByTab(page, noteInput);
  await expect(chatSection).toBeVisible();
  await expectReachableByTab(page, chatInput);
  await expect(providerKeysSection).toBeVisible();
  await expectReachableByTab(page, providerKeyInput);
  await expectReachableByTab(page, signOutButton);
});

test("note submission embeds against the selected movie", async ({ page }) => {
  await loginWithDemoAccount(page);
  await searchForMovie(page, "inception", "inception");

  await page.getByTestId("search-result-row-inception").click();
  await expectInceptionNoteEmbedding(page, async () => {
    await page.getByPlaceholder("Add a note about this movie...").fill("Deterministic note for seeded movie");
    await page.getByRole("button", { name: "Save Note" }).click();
  });
  await expect(page.getByText("Saved")).toBeVisible();
});

test("BYOK error and chat stay local-deterministic", async ({ page }) => {
  await loginWithDemoAccount(page);
  await searchForMovie(page, "inception", "inception");

  await page.getByPlaceholder("Vault secret name...").fill("nonexistent_local_secret");
  await page.getByRole("button", { name: "Set" }).click();
  await expect(page.getByRole("alert")).toBeVisible({ timeout: 15000 });

  await expectLocalChatStream(page, async () => {
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
  });
  await expect(page.getByText("Local stub response: Summarize inception")).toBeVisible();
});
