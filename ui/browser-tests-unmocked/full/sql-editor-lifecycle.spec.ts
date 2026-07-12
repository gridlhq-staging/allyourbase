import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * FULL E2E TEST: SQL Editor Lifecycle
 *
 * Critical Path: Navigate to admin SQL Editor → execute DDL (CREATE TABLE) →
 * execute DML (INSERT) → execute SELECT and verify result table → execute DROP TABLE
 */

test.describe("SQL Editor Lifecycle (Full E2E)", () => {
  const tablesToDrop: string[] = [];

  function escapeRegExp(text: string): string {
    return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  async function getStoredSQLQuery(page: Page): Promise<string | null> {
    const state = await page.context().storageState();
    for (const origin of state.origins) {
      const storedQuery = origin.localStorage.find((item) => item.name === "ayb_sql_query");
      if (storedQuery) return storedQuery.value;
    }
    return null;
  }

  async function expectMissingRelationError(page: Page, tableName: string): Promise<void> {
    const escapedTableName = escapeRegExp(tableName);
    await expect(
      page.getByText(new RegExp(`^ERROR: relation "${escapedTableName}".*does not exist`, "i")),
    ).toBeVisible({ timeout: 5000 });
  }

  test.afterEach(async ({ request, adminToken }) => {
    while (tablesToDrop.length > 0) {
      const table = tablesToDrop.pop();
      if (!table) continue;
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${table}`).catch(() => {});
    }
  });

  test("execute DDL, DML, SELECT, and DROP via admin SQL Editor", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `_test_sql_editor_${runId}`;
    tablesToDrop.push(tableName);

    // Navigate to SQL Editor
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    // DDL: CREATE TABLE
    await sqlInput.fill(`CREATE TABLE ${tableName} (id serial PRIMARY KEY, name text NOT NULL, value int);`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify DDL success feedback
    await expect(page.getByText(/Statement executed successfully/i)).toBeVisible({ timeout: 5000 });
    await expect(page.locator("aside").getByText(tableName, { exact: true })).toBeVisible({ timeout: 10000 });

    // DML: INSERT rows
    await sqlInput.fill(`INSERT INTO ${tableName} (name, value) VALUES ('alpha', 10), ('beta', 20), ('gamma', 30);`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify DML row count feedback
    await expect(page.getByText(/3 row.*affected/i)).toBeVisible({ timeout: 5000 });

    // SELECT: query the rows back
    await sqlInput.fill(`SELECT name, value FROM ${tableName} ORDER BY value;`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Verify result table headers
    await expect(page.getByRole("columnheader", { name: "name" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: "value" })).toBeVisible();

    // Verify result table data
    await expect(page.getByRole("cell", { name: "alpha" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "beta" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "gamma" })).toBeVisible();
    await expect(page.getByText(/3 row/i)).toBeVisible();
    await page.getByTitle("Copy as CSV").click();
    await expect(page.getByText("CSV copied!")).toBeVisible();
    await page.getByTitle("Copy as JSON").click();
    await expect(page.getByText("JSON copied!")).toBeVisible();

    const storedSuccessfulQuery = await getStoredSQLQuery(page);
    expect(storedSuccessfulQuery).toBe(
      `SELECT name, value FROM ${tableName} ORDER BY value;`,
    );

    await sqlInput.fill("SELECT * FROM definitely_missing_admin_sql_table;");
    await page.getByRole("button", { name: /Execute/i }).click();
    await expectMissingRelationError(page, "definitely_missing_admin_sql_table");
    expect(await getStoredSQLQuery(page)).toBe(storedSuccessfulQuery);

    // DDL: DROP TABLE
    await sqlInput.fill(`DROP TABLE ${tableName};`);
    await page.getByRole("button", { name: /Execute/i }).click();
    const destructiveDialog = page.getByRole("dialog", {
      name: "Confirm destructive SQL",
    });
    await expect(destructiveDialog).toBeVisible();
    await destructiveDialog.getByRole("button", { name: /^Execute destructive SQL$/i }).click();

    await expect(page.getByText(/Statement executed successfully/i)).toBeVisible({ timeout: 5000 });
    await expect(page.locator("aside").getByText(tableName, { exact: true })).not.toBeVisible({ timeout: 10000 });

    // Verify table is gone by querying it
    await sqlInput.fill(`SELECT * FROM ${tableName};`);
    await page.getByRole("button", { name: /Execute/i }).click();

    // Expect an error since the table was dropped
    await expectMissingRelationError(page, tableName);

    // Table already dropped — remove from cleanup list
    const idx = tablesToDrop.indexOf(tableName);
    if (idx !== -1) tablesToDrop.splice(idx, 1);
  });
});
