import {
  test,
  expect,
  cleanupFDWServerListEntry,
  replaceAdminRelationWithEmptyClone,
  seedFDWServerListEntry,
  seedUnreadableFDWServerListEntry,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: FDW Management
 *
 * Critical Path: Seed a file_fdw server → Navigate to FDW → Verify the seeded
 * server renders in the Foreign Servers table with name and type.
 */

test.describe("Smoke: FDW Management", () => {
  const seededServerNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededServerNames.length > 0) {
      const name = seededServerNames.pop();
      if (!name) continue;
      await cleanupFDWServerListEntry(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded FDW server renders in the foreign servers table", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const serverName = `smoke_fdw_${runId}`;
    await seedFDWServerListEntry(request, adminToken, serverName);
    seededServerNames.push(serverName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^FDW Management$/i }).click();
    await expect(page.getByRole("heading", { name: /FDW Management/i })).toBeVisible({ timeout: 15_000 });

    // Verify dual-section headings
    await expect(page.getByRole("heading", { name: "Foreign Servers", exact: true })).toBeVisible({
      timeout: 5000,
    });
    await expect(page.getByRole("heading", { name: "Foreign Tables", exact: true })).toBeVisible();

    // Verify seeded server renders in the Foreign Servers table
    const serverRow = page.locator("tr").filter({ hasText: serverName }).first();
    await expect(serverRow).toBeVisible({ timeout: 5000 });
    await expect(serverRow.getByText("fixture_fdw")).toBeVisible();

    // Verify action controls
    await expect(page.getByRole("button", { name: /Add Server/i })).toBeVisible();
  });

  test("empty and unavailable FDW server storage recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const serverName = `retry_fdw_${runId}`;
    await seedFDWServerListEntry(request, adminToken, serverName);
    seededServerNames.push(serverName);
    const relationState = await replaceAdminRelationWithEmptyClone(
      request,
      adminToken,
      "_ayb_fdw_servers",
    );

    try {
      await page.goto("/admin/");
      await waitForDashboard(page);
      await page.locator("aside").getByRole("button", { name: /^FDW Management$/i }).click();
      await expect(page.getByText("No foreign servers", { exact: true })).toBeVisible({
        timeout: 15_000,
      });
      await expect(page.getByText("Foreign Tables", { exact: true })).toBeVisible();

      await seedUnreadableFDWServerListEntry(request, adminToken, serverName);
      await page.reload();
      await waitForDashboard(page);
      await expect(
        page.getByText(
          `decode options for server "${serverName}": json: cannot unmarshal number into Go value of type string`,
          { exact: true },
        ),
      ).toBeVisible();
      await expect(page.getByText("Foreign Tables", { exact: true })).toBeVisible();

      await relationState.restore();
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      const recoveredRow = page.locator("tr").filter({ hasText: serverName }).first();
      await expect(recoveredRow).toBeVisible({ timeout: 5000 });
      await expect(recoveredRow.getByText("fixture_fdw", { exact: true })).toBeVisible();
    } finally {
      await relationState.restore();
    }
  });
});
