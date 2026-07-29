import {
  test,
  expect,
  cleanupLogDrain,
  fetchAdminJSON,
  probeEndpoint,
  seedLogDrain,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Log Drains
 *
 * Critical Path: Seed a log drain → Navigate to Log Drains → Verify the seeded
 * drain row renders with name and stats columns (Sent, Failed, Dropped).
 */

test.describe("Smoke: Log Drains", () => {
  const drainIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (drainIDs.length > 0) {
      const id = drainIDs.pop();
      if (!id) continue;
      await cleanupLogDrain(request, adminToken, id).catch(() => {});
    }
  });

  test("seeded drain renders with stats columns in the table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/logging/drains");
    test.skip(
      status === 501 || status === 404,
      `Log drains service not configured (status ${status})`,
    );

    const runId = Date.now();
    const drainName = `smoke-drain-${runId}`;
    const seeded = await seedLogDrain(request, adminToken, { name: drainName });
    drainIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Log Drains/i }).click();
    await expect(page.getByRole("heading", { name: /Log Drains/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers including stats columns
    await expect(page.getByRole("columnheader", { name: /Name/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Sent/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Failed/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Dropped/i })).toBeVisible();

    // Verify seeded drain row renders with name visible
    const drainRow = page.locator("tr").filter({ hasText: drainName }).first();
    await expect(drainRow).toBeVisible({ timeout: 5000 });
    await expect(drainRow.getByRole("cell")).toHaveText([drainName, "0", "0", "0", "Delete"]);

    // Verify create button
    await expect(page.getByRole("button", { name: /Create Drain/i })).toBeVisible();
  });

  test("empty state recovers after the admin API becomes reachable", async ({
    page,
    request,
    adminToken,
    context,
  }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/logging/drains");
    test.skip(status === 501 || status === 404, `Log drains service not configured (status ${status})`);
    expect(await fetchAdminJSON(request, adminToken, "/api/admin/logging/drains")).toEqual([]);

    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /API Explorer/i }).click();
    await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /Log Drains/i }).click();
    await expect(page.getByText("No log drains configured", { exact: true })).toBeVisible();

    try {
      // Closest-real proxy: the in-memory list handler has no fallible backing
      // capability, so browser offline mode makes the live API unreachable.
      // Bias: broader than one endpoint. Tolerance: only the exact fetch error
      // and recovery of this screen's empty state are asserted.
      await context.setOffline(true);
      await page.locator("aside").getByRole("button", { name: /API Explorer/i }).click();
      await page.locator("aside").getByRole("button", { name: /Log Drains/i }).click();
      await expect(page.getByText("Failed to fetch", { exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "Retry", exact: true })).toBeVisible();

      await context.setOffline(false);
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(page.getByText("No log drains configured", { exact: true })).toBeVisible();
    } finally {
      await context.setOffline(false);
    }
  });
});
