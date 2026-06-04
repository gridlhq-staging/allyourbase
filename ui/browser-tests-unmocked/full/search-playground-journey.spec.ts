import {
  test,
  expect,
  buildParallelSafeRunID,
  dropTableIfExists,
  execSQL,
  seedRecord,
  waitForDashboard,
} from "../fixtures";

test.describe("Search Playground Journey (Full E2E)", () => {
  let tableName = "";

  test.afterEach(async ({ request, adminToken }) => {
    if (!tableName) {
      return;
    }
    await dropTableIfExists(request, adminToken, tableName, "search playground journey table").catch(() => {});
    tableName = "";
  });

  test("exact, fuzzy toggle, and filter expression return deterministic results", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    tableName = `search_playground_${runID}`;

    await execSQL(
      request,
      adminToken,
      `
        CREATE EXTENSION IF NOT EXISTS pg_trgm;
        DROP TABLE IF EXISTS ${tableName};
        CREATE TABLE ${tableName} (
          id BIGSERIAL PRIMARY KEY,
          name TEXT NOT NULL,
          status TEXT NOT NULL,
          category TEXT NOT NULL,
          rank INTEGER NOT NULL
        );
      `,
    );

    const exactName = `JourneySignal_${runID}`;
    const fuzzyTarget = `Notification_${runID}`;

    await seedRecord(request, adminToken, tableName, {
      name: exactName,
      status: "active",
      category: "alpha",
      rank: 1,
    });
    await seedRecord(request, adminToken, tableName, {
      name: fuzzyTarget,
      status: "active",
      category: "alpha",
      rank: 1,
    });
    await seedRecord(request, adminToken, tableName, {
      name: `FilterControl_${runID}`,
      status: "inactive",
      category: "beta",
      rank: 2,
    });

    await page.goto("/admin/");
    await waitForDashboard(page);

    const searchNavControl = page.locator("aside").getByText("Search", { exact: true }).first();
    await expect(searchNavControl).toBeVisible({ timeout: 10000 });
    await searchNavControl.click();

    await expect(page.getByRole("heading", { name: "Search" })).toBeVisible({ timeout: 10000 });

    await page.getByLabel("Collection").selectOption(tableName);
    await expect(page.getByText(exactName, { exact: true })).toBeVisible({ timeout: 10000 });

    const searchInput = page.getByLabel("Search query");
    const fuzzyToggle = page.getByLabel("Use fuzzy matching");
    const filterInput = page.getByLabel("Filter expression");
    const searchButton = page.getByRole("main").getByRole("button", { name: /^Search$/i });

    await searchInput.fill(exactName);
    await filterInput.fill("");
    await searchButton.click();

    await expect(page.getByText(exactName, { exact: true })).toBeVisible({ timeout: 10000 });

    const misspelled = `Notificaton_${runID}`;
    await searchInput.fill(misspelled);
    await filterInput.fill("");
    await fuzzyToggle.setChecked(false);
    await searchButton.click();

    await expect(page.getByText("No results matched this search")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(fuzzyTarget, { exact: true })).toBeHidden();

    await fuzzyToggle.setChecked(true);
    await expect(page.getByText(fuzzyTarget, { exact: true })).toBeVisible({ timeout: 10000 });

    await searchInput.fill("");
    await filterInput.fill("rank=1");
    await fuzzyToggle.setChecked(false);
    await searchButton.click();

    await expect(page.getByText(fuzzyTarget, { exact: true })).toBeVisible();
    await expect(page.getByText(`FilterControl_${runID}`, { exact: true })).toBeHidden();
  });

  test("facet counts render exactly and clicking a bucket narrows the result set", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    tableName = `search_facets_${runID}`;

    await execSQL(
      request,
      adminToken,
      `
        DROP TABLE IF EXISTS ${tableName};
        CREATE TABLE ${tableName} (
          id BIGSERIAL PRIMARY KEY,
          title TEXT NOT NULL,
          status TEXT,
          rank INTEGER NOT NULL
        );
      `,
    );

    const publishedA = `Facet post A ${runID}`;
    const publishedB = `Facet post B ${runID}`;
    const draft = `Facet draft ${runID}`;
    const outsideSearch = `Outside topic ${runID}`;

    await seedRecord(request, adminToken, tableName, {
      title: publishedA,
      status: "published",
      rank: 1,
    });
    await seedRecord(request, adminToken, tableName, {
      title: publishedB,
      status: "published",
      rank: 2,
    });
    await seedRecord(request, adminToken, tableName, {
      title: draft,
      status: "draft",
      rank: 3,
    });
    await seedRecord(request, adminToken, tableName, {
      title: outsideSearch,
      status: "archived",
      rank: 4,
    });

    await page.goto("/admin/");
    await waitForDashboard(page);

    const searchNavControl = page.locator("aside").getByText("Search", { exact: true }).first();
    await expect(searchNavControl).toBeVisible({ timeout: 10000 });
    await searchNavControl.click();

    await expect(page.getByRole("heading", { name: "Search" })).toBeVisible({ timeout: 10000 });
    await page.getByLabel("Collection").selectOption(tableName);

    await expect(page.getByText(publishedA, { exact: true })).toBeVisible({ timeout: 10000 });

    await page.getByRole("checkbox", { name: "status" }).click();
    await page.getByLabel("Search query").fill("facet");
    await page.getByRole("main").getByRole("button", { name: /^Search$/i }).click();

    const statusPanel = page.getByTestId("search-facet-panel-status");
    const publishedBucket = statusPanel.getByTestId("search-facet-bucket-status-published");
    const draftBucket = statusPanel.getByTestId("search-facet-bucket-status-draft");

    await expect(page.getByText(publishedA, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(publishedB, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(draft, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(outsideSearch, { exact: true })).toBeHidden();
    await expect(publishedBucket).toHaveText(/^published\s*2$/);
    await expect(draftBucket).toHaveText(/^draft\s*1$/);

    await publishedBucket.click();

    await expect(page.getByLabel("Filter expression")).toHaveValue("status='published'");
    await expect(page.getByText(publishedA, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(publishedB, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(draft, { exact: true })).toBeHidden();
    await expect(page.getByText(outsideSearch, { exact: true })).toBeHidden();
  });

  test("highlight snippets render emphasized match terms from the real backend", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    tableName = `search_highlight_${runID}`;

    await execSQL(
      request,
      adminToken,
      `
        DROP TABLE IF EXISTS ${tableName};
        CREATE TABLE ${tableName} (
          id BIGSERIAL PRIMARY KEY,
          description TEXT NOT NULL,
          category TEXT NOT NULL
        );
      `,
    );

    const matchTerm = "Xylophone";
    const matchRow = `${matchTerm} orchestral demo ${runID}`;
    const controlRow = `Drumkit percussion demo ${runID}`;

    await seedRecord(request, adminToken, tableName, {
      description: matchRow,
      category: "music",
    });
    await seedRecord(request, adminToken, tableName, {
      description: controlRow,
      category: "music",
    });

    await page.goto("/admin/");
    await waitForDashboard(page);

    const searchNavControl = page.locator("aside").getByText("Search", { exact: true }).first();
    await expect(searchNavControl).toBeVisible({ timeout: 10000 });
    await searchNavControl.click();

    await expect(page.getByRole("heading", { name: "Search" })).toBeVisible({ timeout: 10000 });
    await page.getByLabel("Collection").selectOption(tableName);
    await expect(page.getByText(matchRow, { exact: true })).toBeVisible({ timeout: 10000 });

    const highlightToggle = page.getByLabel("Show highlighted matches");
    await expect(highlightToggle).toBeChecked();

    await page.getByLabel("Search query").fill(matchTerm);
    await page.getByRole("main").getByRole("button", { name: /^Search$/i }).click();

    const highlightSection = page.getByTestId("search-highlight-results");
    await expect(highlightSection).toBeVisible({ timeout: 10000 });

    const firstSnippet = page.getByTestId("search-highlight-snippet-0");
    await expect(firstSnippet).toBeVisible();
    await expect(firstSnippet).toContainText(matchTerm);

    const emphasizedMark = firstSnippet.getByRole("mark");
    await expect(emphasizedMark).toBeVisible();
    await expect(emphasizedMark).toHaveText(matchTerm);

    await expect(firstSnippet).not.toContainText(controlRow);
    await expect(firstSnippet).not.toContainText("Drumkit");
    await expect(firstSnippet).not.toContainText("percussion demo");

    // Highlight-mode search must also filter the rendered Search result set: the
    // non-matching control row should be removed from the grid below the snippet
    // section, and no second highlight snippet should survive for it. Without
    // these guards, the snippet-local assertions above would still pass if the
    // backend stopped filtering rows but happened to return the matched row's
    // snippet first.
    await expect(page.getByText(controlRow, { exact: true })).toBeHidden();
    await expect(page.getByTestId("search-highlight-snippet-1")).toHaveCount(0);

    await highlightToggle.setChecked(false);
    await expect(highlightSection).toBeHidden();
  });
});
