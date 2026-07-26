import {
  buildParallelSafeRunID,
  execSQL,
  expect,
  openTableFromSidebar,
  test,
  waitForDashboard,
} from "../fixtures";
import {
  ALGOLIA_MIGRATION_GUIDE_PATH,
  docsUrl,
  MIGRATIONS_GUIDE_PATH,
  SUPABASE_MIGRATION_GUIDE_PATH,
} from "../../src/lib/docs_url";

/**
 * SMOKE TEST: New Table CTA navigation
 *
 * Critical Path: Admin clicks New Table in sidebar → SQL Editor opens
 */

test.describe("Smoke: Create Table Nav Update", () => {
  test("clicking New Table opens SQL Editor", async ({ page }) => {
    // Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    const main = page.locator("main");

    await expect(main.getByText("Migrating from another source?")).toBeVisible();
    await expect(main.getByText("ayb migrate <source> --help")).toBeVisible();
    await expect(main.getByRole("link", { name: "Migration guide", exact: true })).toHaveAttribute(
      "href",
      docsUrl(MIGRATIONS_GUIDE_PATH),
    );
    await expect(main.getByRole("link", { name: "Supabase migration guide" })).toHaveAttribute(
      "href",
      docsUrl(SUPABASE_MIGRATION_GUIDE_PATH),
    );
    await expect(main.getByRole("link", { name: "Algolia migration guide" })).toHaveAttribute(
      "href",
      docsUrl(ALGOLIA_MIGRATION_GUIDE_PATH),
    );

    // Navigate to SQL Editor via the first-run CTA in Tables section
    await sidebar.getByRole("button", { name: /^New Table$/i }).click();

    const sqlInput = page.getByLabel("SQL query");
    await expect(sqlInput).toBeVisible({ timeout: 5000 });
  });

  test("an empty writable table opens the create form from its empty state", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `empty_table_cta_${runID}`;

    try {
      await execSQL(
        request,
        adminToken,
        `CREATE TABLE ${tableName} (
          id SERIAL PRIMARY KEY,
          title TEXT NOT NULL
        )`,
      );

      await page.goto("/admin/");
      await waitForDashboard(page);
      await openTableFromSidebar(page, tableName);

      const emptyStateCell = page.getByRole("cell").filter({
        hasText: "No rows in this table yet",
      });
      await expect(emptyStateCell).toHaveCount(1);
      const newRowButton = emptyStateCell.getByRole("button", { name: "New Row" });
      await expect(newRowButton).toBeVisible();

      await newRowButton.click();

      await expect(page.getByRole("heading", { name: "New Record" })).toBeVisible();
    } finally {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName}`);
    }
  });
});
