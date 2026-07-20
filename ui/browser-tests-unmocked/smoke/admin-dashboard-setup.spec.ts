import {
  buildParallelSafeRunID,
  execSQL,
  expect,
  test,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * SMOKE TEST: Admin Dashboard Setup
 *
 * Critical Path:
 * 1. Open dashboard (admin password already set via auth.setup.ts)
 * 2. Verify dashboard UI loads with sidebar sections
 * 3. Create a table via SQL Editor
 * 4. Verify table appears in sidebar
 */

test.describe("Smoke: Admin Dashboard Setup", () => {
  const pendingCleanup: string[] = [];
  const expectedNavigationLabels = [
    "SQL Editor",
    "GraphQL",
    "Functions",
    "RLS Policies",
    "Search",
    "Matviews",
    "Schema Designer",
    "FDW",
    "Storage",
    "Sites",
    "Edge Functions",
    "Webhooks",
    "SMS Health",
    "SMS Messages",
    "Email Templates",
    "Push Notifications",
    "Users",
    "Apps",
    "API Keys",
    "OAuth Clients",
    "API Explorer",
    "Jobs",
    "Schedules",
    "Realtime Inspector",
    "Security Advisor",
    "Performance Advisor",
    "Backups",
    "Analytics",
    "Usage",
    "Replicas",
    "Branches",
    "Audit Logs",
    "Admin Logs",
    "Secrets",
    "Custom Domains",
    "Extensions",
    "Vector Indexes",
    "Log Drains",
    "Stats",
    "Notifications",
    "Incidents",
    "Support Tickets",
    "Tenants",
    "Organizations",
    "AI Assistant",
    "Auth Settings",
    "MFA Management",
    "Account Linking",
    "SAML",
    "Auth Hooks",
  ];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  function normalizeLabels(labels: string[]): string[] {
    return labels.map((label) => label.trim().replace(/\s+/g, " ")).filter(Boolean);
  }

  function navigationSpan(labels: string[]): string[] {
    const normalized = normalizeLabels(labels);
    const firstIndex = normalized.indexOf(expectedNavigationLabels[0]);
    const lastIndex = normalized.indexOf(expectedNavigationLabels[expectedNavigationLabels.length - 1]);
    expect(firstIndex).toBeGreaterThanOrEqual(0);
    expect(lastIndex).toBeGreaterThan(firstIndex);
    return normalized.slice(firstIndex, lastIndex + 1);
  }

  async function sidebarNavigationLabels(page: Page): Promise<string[]> {
    const sidebarButtons = await page
      .getByRole("complementary")
      .getByRole("button")
      .allInnerTexts();
    return navigationSpan(sidebarButtons);
  }

  async function commandPaletteNavigationLabels(page: Page): Promise<string[]> {
    await page
      .getByRole("complementary")
      .getByRole("button", { name: "Search... K" })
      .click();
    const dialog = page.getByRole("dialog", { name: "Command palette" });
    await expect(dialog).toBeVisible();
    const paletteButtons = await dialog.getByRole("button").allInnerTexts();
    return navigationSpan(paletteButtons);
  }

  async function expectFullNavigation(page: Page): Promise<void> {
    expect(await sidebarNavigationLabels(page)).toEqual(expectedNavigationLabels);
    expect(await commandPaletteNavigationLabels(page)).toEqual(expectedNavigationLabels);
  }

  async function expectDashboardWithoutCapabilityError(page: Page): Promise<void> {
    await waitForDashboard(page);
    await expect(page.getByTestId("login").or(page.getByText("Connection Error"))).toHaveCount(0);
    await expectFullNavigation(page);
  }

  test("dashboard loads with shell chrome and command palette entry points", async ({ page }) => {
    const commandPaletteShortcut = process.platform === "darwin" ? "Meta+K" : "Control+K";

    // Act: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    const commandPaletteHint = sidebar.getByRole("button", { name: "Search... K" });

    await expect(sidebar.getByText("Allyourbase")).toBeVisible();
    await expect(commandPaletteHint).toBeVisible();

    for (const section of ["Tables", "Database", "Services", "Messaging", "Admin", "AI", "Auth"]) {
      await expect(sidebar.getByText(section, { exact: true })).toBeVisible();
    }

    await expect(sidebar.getByRole("button", { name: /^SQL Editor$/i })).toBeVisible();
    await expect(sidebar.getByRole("button", { name: /^Storage$/i })).toBeVisible();
    await expect(sidebar.getByRole("button", { name: /^Users$/i })).toBeVisible();
    await expect(sidebar.getByRole("button", { name: /^Refresh schema$/i })).toBeVisible();
    await expect(sidebar.getByRole("button", { name: /Switch to (dark|light) mode/i })).toBeVisible();
    await expect(sidebar.getByRole("button", { name: /^Log out$/i })).toBeVisible();

    await commandPaletteHint.click();
    await expect(page.getByRole("dialog", { name: "Command palette" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Command palette" })).toBeHidden();

    await page.keyboard.press(commandPaletteShortcut);
    await expect(page.getByRole("dialog", { name: "Command palette" })).toBeVisible();
    await page.keyboard.press("Escape");

    expect(await sidebarNavigationLabels(page)).toEqual(expectedNavigationLabels);
    expect(await commandPaletteNavigationLabels(page)).toEqual(expectedNavigationLabels);
  });

  test("capability 401 keeps passwordless-compatible navigation fail-open", async ({ page }) => {
    await page.route("**/api/admin/capabilities", async (route) => {
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ message: "unauthorized" }),
      });
    });

    await page.goto("/admin/");

    await expect(page.getByRole("complementary")).toBeVisible();
    await expectDashboardWithoutCapabilityError(page);
  });

  for (const failureCase of [
    {
      name: "network abort",
      install: async (page: Page) => {
        await page.route("**/api/admin/capabilities", async (route) => {
          await route.abort();
        });
      },
    },
    {
      name: "non-JSON 200",
      install: async (page: Page) => {
        await page.route("**/api/admin/capabilities", async (route) => {
          await route.fulfill({
            status: 200,
            contentType: "text/plain",
            body: "not json",
          });
        });
      },
    },
    {
      name: "representative non-200",
      install: async (page: Page) => {
        await page.route("**/api/admin/capabilities", async (route) => {
          await route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({ message: "unavailable" }),
          });
        });
      },
    },
  ]) {
    test(`capability ${failureCase.name} keeps navigation fail-open`, async ({ page }) => {
      await failureCase.install(page);

      await page.goto("/admin/");

      await expect(page.getByRole("complementary")).toBeVisible();
      await expectDashboardWithoutCapabilityError(page);
    });
  }

  test("dashboard selects the first sorted table on load", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runId = buildParallelSafeRunID(testInfo).replaceAll("_", "");
    const schemaName = `a0_shell_${runId}`;
    const firstTableName = "alpha_first";
    const secondTableName = "zeta_second";

    pendingCleanup.push(`DROP SCHEMA IF EXISTS ${schemaName} CASCADE;`);

    await execSQL(
      request,
      adminToken,
      `CREATE SCHEMA ${schemaName};
       CREATE TABLE ${schemaName}.${secondTableName} (id SERIAL PRIMARY KEY);
       CREATE TABLE ${schemaName}.${firstTableName} (id SERIAL PRIMARY KEY);`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    await expect(sidebar.getByRole("button", { name: `${schemaName}.${firstTableName}` })).toBeVisible();
    await expect(page.getByRole("heading", { name: `${schemaName}.${firstTableName}` })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Data$/i })).toBeVisible();

    await page.getByTestId("nav-schema-designer").click();
    await expect(page.getByRole("heading", { name: "Schema Designer" })).toBeVisible();

    await sidebar.getByRole("button", { name: `${schemaName}.${secondTableName}` }).click();
    await expect(page.getByRole("heading", { name: `${schemaName}.${secondTableName}` })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Data$/i })).toBeVisible();
  });

  test("first-run journey creates first table and verifies first row in table data view", async ({
    page,
  }) => {
    const runId = Date.now();
    const tableName = `posts_smoke_${runId}`;
    const rowTitle = `First Post ${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Step 2: Click SQL Editor in sidebar
    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Verify SQL Editor opened
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // Step 4: Create table (single statement per execution)
    const createTableSQL = `CREATE TABLE ${tableName} (
      id SERIAL PRIMARY KEY,
      title TEXT NOT NULL
    )`;

    await sqlInput.fill(createTableSQL);

    // Step 5: Execute CREATE TABLE
    let runButton = page.getByRole("button", { name: /^Execute$/i });
    await expect(runButton).toBeVisible();
    await runButton.click();
    await expect(page.getByText(/statement executed successfully/i)).toBeVisible({ timeout: 10000 });

    // Step 6: Sidebar should refresh automatically after CREATE TABLE
    const tableLink = sidebar.getByText(tableName, { exact: true });
    await expect(tableLink).toBeVisible({ timeout: 15000 });

    // Step 7: Insert first row
    const insertSQL = `INSERT INTO ${tableName} (title) VALUES ('${rowTitle}');`;

    await sqlInput.clear();
    await sqlInput.fill(insertSQL);

    runButton = page.getByRole("button", { name: /^Execute$/i });
    await runButton.click();
    await expect(page.getByText(/rows? affected/i).first()).toBeVisible({ timeout: 10000 });

    // Step 8: Click table and verify existing table data view renders the new row
    await tableLink.click();
    await expect(page.getByRole("button", { name: /^Data$/i })).toBeVisible();
    await expect(page.getByRole("cell", { name: rowTitle })).toBeVisible({ timeout: 10000 });

    // Cleanup handled by afterEach
  });

  test("SQL Editor shows query results and duration", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `test_query_${runId}`;

    pendingCleanup.push(`DROP TABLE IF EXISTS ${tableName};`);

    // Arrange: Create a simple table via API
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT);
       INSERT INTO ${tableName} (name) VALUES ('Test 1'), ('Test 2');`
    );

    // Act: Navigate to SQL Editor
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    // Execute a SELECT query
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
    await sqlInput.fill(`SELECT * FROM ${tableName};`);

    const runButton = page.getByRole("button", { name: /^Execute$/i });
    await runButton.click();

    // Assert: Results should appear
    await expect(page.getByText("Test 1")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Test 2")).toBeVisible();

    // Assert: Duration should be displayed (in ms or similar)
    await expect(page.getByText(/\d+\s*ms/i).or(page.getByText(/duration/i))).toBeVisible({
      timeout: 5000,
    });

    // Cleanup handled by afterEach
  });
});
