import type { Locator, Page } from "@playwright/test";
import { test, expect, execSQL, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Functions Browser
 *
 * Tests PostgreSQL function browsing and execution:
 * - Setup: Create a test function via SQL
 * - Navigate to Functions section
 * - Verify function appears in list
 * - Expand function to see parameters
 * - Execute function with arguments
 * - Verify results
 * - Cleanup: Drop test function
 */

async function refreshSchemaIfPresent(page: Page): Promise<void> {
  await page
    .getByRole("button", { name: "Refresh schema" })
    .click({ timeout: 2000 })
    .catch(() => {});
}

async function openFunctionsBrowser(
  page: Page,
  sidebar: Locator,
): Promise<void> {
  const functionsButton = sidebar.getByRole("button", { name: /^Functions$/i });
  await expect(functionsButton).toBeVisible({ timeout: 5000 });
  await functionsButton.click();
  await expect(page.getByRole("heading", { name: /Functions/i })).toBeVisible({
    timeout: 5000,
  });
}

async function isFunctionVisible(
  page: Page,
  functionName: string,
): Promise<boolean> {
  return page
    .getByText(functionName)
    .first()
    .waitFor({ state: "visible", timeout: 3000 })
    .then(
      () => true,
      () => false,
    );
}

async function waitForFunctionToAppear(
  page: Page,
  sidebar: Locator,
  functionName: string,
): Promise<void> {
  for (let attempt = 0; attempt < 3; attempt++) {
    await page.reload();
    await waitForDashboard(page);
    await refreshSchemaIfPresent(page);
    await openFunctionsBrowser(page, sidebar);

    if (await isFunctionVisible(page, functionName)) {
      return;
    }
  }
}

test.describe("Functions Browser (Full E2E)", () => {
  const functionNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (functionNames.length > 0) {
      const functionName = functionNames.pop();
      if (!functionName) continue;
      await execSQL(
        request,
        adminToken,
        `DROP FUNCTION IF EXISTS ${functionName}(integer, integer)`,
      ).catch(() => {});
    }
  });

  test("browse, execute, and verify function results", async ({ page }) => {
    const runId = Date.now();
    const funcName = `test_add_${runId}`;
    functionNames.push(funcName);

    // ============================================================
    // Setup: Create test function via SQL
    // ============================================================
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(
      `CREATE OR REPLACE FUNCTION ${funcName}(a integer, b integer) RETURNS integer AS $$ SELECT a + b; $$ LANGUAGE SQL;`,
    );
    await page.getByRole("button", { name: /^Execute$/i }).click();
    await expect(
      page.getByText(/statement executed successfully/i),
    ).toBeVisible({ timeout: 10000 });

    // Reload page and refresh schema to pick up the new function.
    // Retry up to 3 times in case the schema cache is still rebuilding.
    await waitForFunctionToAppear(page, sidebar, funcName);

    // Final assertion
    await expect(page.getByText(funcName).first()).toBeVisible({
      timeout: 5000,
    });

    // ============================================================
    // EXPAND: Click function to see parameters
    // ============================================================
    const funcButton = page.getByRole("button", { name: new RegExp(funcName) });
    await funcButton.click();

    const paramInputs = page.getByPlaceholder("NULL");
    await expect(paramInputs.first()).toBeVisible({ timeout: 3000 });

    // ============================================================
    // EXECUTE: Fill params and run function
    // ============================================================
    await paramInputs.nth(0).fill("3");
    await paramInputs.nth(1).fill("5");

    const executeButton = page.getByRole("button", { name: /^Execute$/i }).last();
    await expect(executeButton).toBeVisible({ timeout: 2000 });
    await executeButton.click();

    // ============================================================
    // VERIFY: Check results show 8 (3 + 5)
    // ============================================================
    // Verify the Result label appeared (execution completed)
    await expect(page.getByText("Result").last()).toBeVisible({
      timeout: 10000,
    });
    // Verify the result value — exact match avoids matching durations like "8ms"
    await expect(page.getByText("8", { exact: true }).last()).toBeVisible();
  });

  test("unnamed-parameter function is shown as non-callable", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const funcName = `test_unnamed_${runId}`;

    // Arrange: create a function whose single parameter has no name, so it
    // cannot be called via the REST RPC endpoint. SQL is an Arrange shortcut.
    // The parameter is intentionally unused with a constant body: referencing
    // it as `$1` would be read as a bind placeholder, and `execSQL` splits on
    // `;`, so the body must contain neither a placeholder nor a semicolon.
    await execSQL(
      request,
      adminToken,
      `CREATE OR REPLACE FUNCTION public.${funcName}(integer) RETURNS integer AS $$ SELECT 42 $$ LANGUAGE SQL`,
    );

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);

      const sidebar = page.locator("aside");
      await waitForFunctionToAppear(page, sidebar, funcName);
      await expect(page.getByText(funcName).first()).toBeVisible({
        timeout: 5000,
      });

      // Expand the function row to reveal its callable-state panel.
      await page.getByRole("button", { name: new RegExp(funcName) }).click();

      // The non-callable notice appears and no Execute action is offered.
      await expect(
        page.getByText(/unnamed parameters and cannot be called/i),
      ).toBeVisible({ timeout: 5000 });
      await expect(
        page.getByRole("button", { name: /^Execute$/i }),
      ).toHaveCount(0);
    } finally {
      await execSQL(
        request,
        adminToken,
        `DROP FUNCTION IF EXISTS public.${funcName}(integer)`,
      ).catch(() => {});
    }
  });
});
