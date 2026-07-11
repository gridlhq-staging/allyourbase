import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * SMOKE TEST: SQL View
 *
 * Critical Path: Open seeded table → switch to SQL tab → execute query → verify result row
 */

test.describe("Smoke: SQL View", () => {
  const cleanupSQL: string[] = [];

  async function openTableFromSidebar(page: Page, tableName: string): Promise<void> {
    const sidebar = page.locator("aside");
    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    const tableLink = sidebar.getByText(tableName, { exact: true });

    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await expect
      .poll(
        async () => {
          await refreshButton.click();
          return tableLink.isVisible();
        },
        { timeout: 15000 },
      )
      .toBe(true);

    await tableLink.click();
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of cleanupSQL) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    cleanupSQL.length = 0;
  });

  test("admin executes query in SQL view and sees seeded row", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `smoke_sql_view_${runId}`;
    const rowTitle = `SQL View Row ${runId}`;

    cleanupSQL.push(`DROP TABLE IF EXISTS ${tableName};`);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );

      INSERT INTO ${tableName} (title) VALUES ('${rowTitle}');`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await openTableFromSidebar(page, tableName);
    await page.getByRole("button", { name: /^SQL$/i }).click();

    const sqlEditor = page.getByLabel("SQL query");
    const executeButton = page.getByRole("button", { name: /^Execute$/i });
    await expect(sqlEditor).toBeVisible({ timeout: 5000 });
    await expect(sqlEditor).toContainText("SELECT 1 AS hello;");
    await expect(page.getByText("Run a query to see results")).toBeVisible();
    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.press("Backspace");
    await expect(executeButton).toBeDisabled();
    await page.keyboard.type(`SELECT pg_sleep(1), '${rowTitle}' AS title;`);
    await executeButton.click();
    await expect(page.getByRole("button", { name: /^Running\.\.\.$/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "title" })).toBeVisible({ timeout: 5000 });

    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type("SELECT * FROM definitely_missing_sql_view_table;");
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(page.getByText(/definitely_missing_sql_view_table/i)).toBeVisible({ timeout: 5000 });

    await sqlEditor.click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type(`SELECT title FROM ${tableName} WHERE title = '${rowTitle}';`);

    await page.getByRole("button", { name: /^Execute$/i }).click();

    await expect(page.getByRole("columnheader", { name: "title" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("table").getByText(rowTitle)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/1 row in \d+ms/i)).toBeVisible();
    await expect(page.getByTitle("Copy as CSV")).toBeVisible();
    await expect(page.getByTitle("Copy as JSON")).toBeVisible();
  });
});
