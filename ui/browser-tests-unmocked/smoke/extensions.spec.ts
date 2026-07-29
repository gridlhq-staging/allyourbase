import { randomUUID } from "crypto";
import {
  test,
  expect,
  assertSafeSQLIdentifier,
  execSQL,
  sqlLiteral,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Extensions
 *
 * Critical Path: Navigate to Extensions → Verify page heading, table structure,
 * and at least one extension row renders with name/status/version in the page body.
 */

test.describe("Smoke: Extensions", () => {
  test("built-in extension renders as installed in extensions table", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Extensions/i }).click();
    await expect(page.getByRole("heading", { name: /Extensions/i })).toBeVisible({ timeout: 15_000 });

    const extensionRow = page.locator("tr").filter({ hasText: "plpgsql" }).first();
    await expect(extensionRow).toBeVisible({ timeout: 5000 });
    await expect(extensionRow.getByText("installed")).toBeVisible();
    await expect(extensionRow.getByRole("button", { name: /^Disable$/i })).toBeVisible();
  });

  test("isolated empty and unavailable extension catalog recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const runID = randomUUID().replaceAll("-", "").slice(0, 10);
    const extensionName = `e2e_extension_${runID}`;
    const backupView = assertSafeSQLIdentifier(
      `pg_available_extensions_e2e_${runID}`,
      "extension catalog backup",
    );
    let liveViewExists = false;

    const createCatalogView = async (includeFixture: boolean) => {
      const fixtureRow = includeFixture
        ? `UNION ALL SELECT '${sqlLiteral(extensionName)}'::name, '1.0'::text, NULL::text, 'E2E isolated fixture'::text`
        : "";
      await execSQL(
        request,
        adminToken,
        `CREATE VIEW public.pg_available_extensions AS
         SELECT name, default_version, installed_version, comment
         FROM pg_catalog.${backupView}
         WHERE FALSE
         ${fixtureRow}`,
      );
      liveViewExists = true;
    };
    const dropCatalogView = async () => {
      await execSQL(request, adminToken, "DROP VIEW public.pg_available_extensions");
      liveViewExists = false;
    };

    await execSQL(
      request,
      adminToken,
      `ALTER VIEW pg_catalog.pg_available_extensions RENAME TO ${backupView}`,
    );
    try {
      await createCatalogView(true);
      await page.goto("/admin/");
      await waitForDashboard(page);
      await page.locator("aside").getByRole("button", { name: /^Extensions$/i }).click();
      await expect(page.getByRole("heading", { name: /^Extensions$/i })).toBeVisible({
        timeout: 15_000,
      });
      await expect(page.getByText(extensionName, { exact: true })).toBeVisible();

      await dropCatalogView();
      await createCatalogView(false);
      await page.reload();
      await waitForDashboard(page);
      await expect(page.getByText("No extensions available", { exact: true })).toBeVisible();

      await dropCatalogView();
      await page.reload();
      await waitForDashboard(page);
      await expect(
        page.getByText(
          'querying extensions: ERROR: relation "pg_available_extensions" does not exist (SQLSTATE 42P01)',
          { exact: true },
        ),
      ).toBeVisible();
      await expect(page.getByRole("heading", { name: /^Extensions$/i })).toBeVisible();

      await createCatalogView(true);
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(page.getByText(extensionName, { exact: true })).toBeVisible();
    } finally {
      if (liveViewExists) {
        await dropCatalogView();
      }
      await execSQL(
        request,
        adminToken,
        `ALTER VIEW pg_catalog.${backupView} RENAME TO pg_available_extensions`,
      );
    }
  });
});
