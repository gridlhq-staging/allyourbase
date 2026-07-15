import {
  adminPath,
  buildParallelSafeRunID,
  execSQL,
  expect,
  test,
  waitForDashboard,
} from "../fixtures";

test.describe("Smoke: Dashboard URL routing", () => {
  test("loads and refreshes a direct admin-screen URL", async ({ page }) => {
    const pathname = adminPath("screens/sql-editor");
    await page.goto(pathname);
    await waitForDashboard(page);

    await expect(page.getByLabel("SQL query")).toBeVisible();
    await expect(page).toHaveURL(new URL(pathname, page.url()).toString());

    await page.reload();
    await waitForDashboard(page);
    await expect(page.getByLabel("SQL query")).toBeVisible();
    await expect(page).toHaveURL(new URL(pathname, page.url()).toString());
  });

  test("loads and refreshes an encoded selected-table URL", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const schemaName = `tenant/east_${runID}`;
    const tableName = `order history_${runID}`;
    const seededValue = `deep-link-row-${runID}`;
    const pathname = adminPath(`tables/${encodeURIComponent(schemaName)}/${encodeURIComponent(tableName)}`);

    try {
      await execSQL(
        request,
        adminToken,
        `CREATE SCHEMA "${schemaName}";
         CREATE TABLE "${schemaName}"."${tableName}" (id integer PRIMARY KEY, evidence text NOT NULL);
         INSERT INTO "${schemaName}"."${tableName}" VALUES (1, '${seededValue}')`,
      );

      await page.goto(pathname);
      await waitForDashboard(page);
      await expect(page.getByRole("button", { name: "Data", exact: true })).toBeVisible();
      await expect(page.getByRole("heading", { name: `${schemaName}.${tableName}`, exact: true })).toBeVisible();
      await expect(page.getByRole("cell", { name: seededValue })).toBeVisible();
      await expect(page).toHaveURL(new URL(pathname, page.url()).toString());

      await page.reload();
      await waitForDashboard(page);
      await expect(page.getByRole("heading", { name: `${schemaName}.${tableName}`, exact: true })).toBeVisible();
      await expect(page.getByRole("cell", { name: seededValue })).toBeVisible();
      await expect(page).toHaveURL(new URL(pathname, page.url()).toString());
    } finally {
      await execSQL(
        request,
        adminToken,
        `DROP TABLE IF EXISTS "${schemaName}"."${tableName}";
         DROP SCHEMA IF EXISTS "${schemaName}"`,
      ).catch(() => {});
    }
  });

  test("keeps the first Back and Forward traversal inside the console", async ({ page }) => {
    const basePath = adminPath();
    const sqlEditorPath = adminPath("screens/sql-editor");
    await page.goto(basePath);
    await waitForDashboard(page);

    await page.getByRole("complementary").getByRole("button", { name: "SQL Editor", exact: true }).click();
    await expect(page).toHaveURL(new URL(sqlEditorPath, page.url()).toString());
    await expect(page.getByLabel("SQL query")).toBeVisible();

    await page.goBack();
    await expect(page).toHaveURL(new URL(basePath, page.url()).toString());
    await expect(page.getByRole("complementary")).toBeVisible();

    await page.goForward();
    await expect(page).toHaveURL(new URL(sqlEditorPath, page.url()).toString());
    await expect(page.getByLabel("SQL query")).toBeVisible();
  });

  test("restores command-palette screen and table choices through history", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `routing_history_${runID}`;
    const basePath = adminPath();
    const sqlEditorPath = adminPath("screens/sql-editor");
    const tablePath = adminPath(`tables/public/${tableName}`);

    try {
      await execSQL(request, adminToken, `CREATE TABLE ${tableName} (id integer PRIMARY KEY)`);
      await page.goto(basePath);
      await waitForDashboard(page);
      await page.getByRole("button", { name: "Refresh schema" }).click();

      await page.getByRole("complementary").getByRole("button", { name: "Search... K" }).click();
      await page.getByRole("dialog", { name: "Command palette" }).getByRole("button", { name: "SQL Editor", exact: true }).click();
      await expect(page).toHaveURL(new URL(sqlEditorPath, page.url()).toString());

      await page.getByRole("complementary").getByRole("button", { name: "Search... K" }).click();
      await page.getByRole("dialog", { name: "Command palette" }).getByRole("button", { name: tableName, exact: true }).click();
      await expect(page).toHaveURL(new URL(tablePath, page.url()).toString());
      await expect(page.getByRole("heading", { name: tableName, exact: true })).toBeVisible();

      await page.goBack();
      await expect(page).toHaveURL(new URL(sqlEditorPath, page.url()).toString());
      await expect(page.getByLabel("SQL query")).toBeVisible();
      await page.goBack();
      await expect(page).toHaveURL(new URL(basePath, page.url()).toString());
      await page.goForward();
      await expect(page).toHaveURL(new URL(sqlEditorPath, page.url()).toString());
      await page.goForward();
      await expect(page).toHaveURL(new URL(tablePath, page.url()).toString());
      await expect(page.getByRole("heading", { name: tableName, exact: true })).toBeVisible();
    } finally {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName}`).catch(() => {});
    }
  });

  test("preserves screen-owned query and hash during shell navigation", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const tableName = `routing_query_${buildParallelSafeRunID(testInfo)}`;
    const sqlEditorPath = adminPath("screens/sql-editor");
    const cases = [
      {
        path: adminPath(`screens/schema-designer?schemaTable=public.${tableName}#diagram`),
        heading: "Schema Designer",
        query: { schemaTable: `public.${tableName}` },
      },
      {
        path: adminPath("screens/security-advisor?secSeverity=high&secCategory=rls&secStatus=open#finding"),
        heading: "Security Advisor",
        query: { secSeverity: "high", secCategory: "rls", secStatus: "open" },
      },
      {
        path: adminPath("screens/performance-advisor?perfRange=7d#slow-query"),
        heading: "Performance Advisor",
        query: { perfRange: "7d" },
      },
    ];

    try {
      await execSQL(request, adminToken, `CREATE TABLE ${tableName} (id integer PRIMARY KEY)`);
      for (const routeCase of cases) {
        await page.goto(routeCase.path);
        await waitForDashboard(page);
        await expect(page.getByRole("heading", { name: routeCase.heading, exact: true })).toBeVisible();
        const sourceURL = new URL(page.url());
        for (const [name, value] of Object.entries(routeCase.query)) {
          expect(sourceURL.searchParams.get(name)).toBe(value);
        }

        await page.getByRole("complementary").getByRole("button", { name: "SQL Editor", exact: true }).click();
        const expectedURL = new URL(sqlEditorPath, page.url());
        expectedURL.search = sourceURL.search;
        expectedURL.hash = sourceURL.hash;
        await expect(page).toHaveURL(expectedURL.toString());
        await expect(page.getByLabel("SQL query")).toBeVisible();
      }
    } finally {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName}`).catch(() => {});
    }
  });

  test("keeps OAuth authorization outside the console route namespace", async ({ page }) => {
    await page.goto("/oauth/authorize");

    await expect(page.getByRole("heading", { name: "Authorization Error" })).toBeVisible();
    await expect(page.getByText("Missing required parameters")).toBeVisible();
    await expect(page.getByRole("complementary")).toHaveCount(0);
    await expect(page).toHaveURL(new URL("/oauth/authorize", page.url()).toString());
  });

  test("hard-refreshes a non-root admin base without a trailing slash", async ({ page }) => {
    test.skip(adminPath() === "/", "The legal root admin base already consists of its trailing slash");
    const baseWithoutTrailingSlash = adminPath("", { trailingSlash: false });

    await page.goto(baseWithoutTrailingSlash);
    await waitForDashboard(page);
    await expect(page.getByRole("complementary")).toBeVisible();

    await page.reload();
    await waitForDashboard(page);
    await expect(page.getByRole("complementary")).toBeVisible();
  });
});
