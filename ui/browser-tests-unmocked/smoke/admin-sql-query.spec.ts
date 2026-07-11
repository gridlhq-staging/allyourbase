import { test, expect, waitForDashboard } from "../fixtures";

/**
 * SMOKE TEST: Admin Dashboard - SQL Query Execution
 *
 * Critical Path: Admin logs in → Executes SQL query → Views results
 */

test.describe("Smoke: Admin SQL Query", () => {
  test("admin can execute SQL query and view results", async ({ page }) => {
    const seededValue = `sql-smoke-${Date.now()}`;
    const persistedQuery = `SELECT 'stored-${seededValue}' AS seeded_value;`;
    await page.addInitScript((query) => {
      window.localStorage.setItem("ayb_sql_query", query);
    }, persistedQuery);

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Step 2: Navigate to SQL Editor via sidebar
    await page.locator("aside").getByRole("button", { name: /^SQL Editor$/i }).click();

    // Step 3: Find SQL input
    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
    await expect(sqlInput).toContainText(persistedQuery);
    await expect(page.getByText("Run a query to see results")).toBeVisible();

    // Step 4: Execute a query with a distinctive seeded value
    await sqlInput.fill(`SELECT '${seededValue}' AS seeded_value;`);

    // Step 5: Click Execute button
    const runButton = page.getByRole("button", { name: /^Execute$/i });
    await expect(runButton).toBeVisible();
    await runButton.click();

    // Step 6: Verify results appear
    await expect(page.getByRole("columnheader", { name: /seeded_value/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("cell", { name: seededValue, exact: true })).toBeVisible();
    await expect(page.getByText(/1 row in \d+ms/i)).toBeVisible();
    await page.getByTitle("Copy as CSV").click();
    await expect(page.getByText("CSV copied!")).toBeVisible();
    await page.getByTitle("Copy as JSON").click();
    await expect(page.getByText("JSON copied!")).toBeVisible();
  });
});
