import {
  test,
  expect,
  probeEndpoint,
  seedRequestLogEntry,
  cleanupRequestLogsByPath,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Analytics
 *
 * Critical Path: Navigate to Analytics → Verify page loads with tab structure
 * and content-area elements (subtitle, table headers) visible in the page body.
 */

test.describe("Smoke: Analytics", () => {
  const seededPaths: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededPaths.length > 0) {
      const path = seededPaths.pop();
      if (!path) continue;
      await cleanupRequestLogsByPath(request, adminToken, path).catch(() => {});
    }
  });

  test("analytics page renders seeded request-log row and filter behavior", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/analytics/requests");
    test.skip(
      probeStatus === 503 || probeStatus === 501 || probeStatus === 404,
      `Analytics request-log endpoint unavailable (status ${probeStatus})`,
    );
    expect(probeStatus, "Analytics request-log endpoint returned HTTP 500").not.toBe(500);

    const runId = Date.now();
    const primaryPath = `/api/smoke-analytics/${runId}/primary`;
    const secondaryPath = `/api/smoke-analytics/${runId}/secondary`;
    const requestID = `smoke-analytics-request-${runId}`;
    const ipAddress = "198.51.100.42";
    const seedTimestamp = new Date(Date.now() + 5000).toISOString();

    const primaryEntry = await seedRequestLogEntry(request, adminToken, {
      method: "POST",
      path: primaryPath,
      statusCode: 418,
      durationMs: 321,
      timestampISO: seedTimestamp,
      requestSize: 1536,
      responseSize: 2048,
      requestID,
      ipAddress,
    });
    seededPaths.push(primaryPath);

    const secondaryEntry = await seedRequestLogEntry(request, adminToken, {
      method: "GET",
      path: secondaryPath,
      statusCode: 200,
      durationMs: 45,
      timestampISO: seedTimestamp,
    });
    seededPaths.push(secondaryPath);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.getByRole("complementary").getByRole("button", { name: /Analytics/i }).click();
    await expect(page.getByRole("heading", { name: /Analytics/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByText("Request logs and query performance insights")).toBeVisible();
    await expect(page.getByRole("button", { name: /Request Logs/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /Query Performance/i })).toBeVisible();

    await expect(page.getByRole("columnheader", { name: /Time/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Method/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Path/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Duration/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Size/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Identity/i })).toBeVisible();

    const primaryRow = page.getByTestId(`request-log-row-${primaryEntry.id}`);
    await expect(primaryRow).toContainText(primaryPath, { timeout: 5000 });
    await expect(primaryRow.getByRole("cell", { name: "POST" })).toBeVisible({ timeout: 5000 });
    await expect(primaryRow.getByRole("cell", { name: "418" })).toBeVisible();
    await expect(primaryRow.getByRole("cell", { name: "321ms" })).toBeVisible();
    await expect(primaryRow.getByRole("cell", { name: "1.5 KB / 2.0 KB" })).toBeVisible();

    await primaryRow.getByTestId(`request-log-view-details-${primaryEntry.id}`).click();
    const drawer = page.getByRole("dialog", { name: "Request details" });
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("request-log-detail-id")).toHaveText(primaryEntry.id);
    await expect(drawer.getByTestId("request-log-detail-path")).toHaveText(primaryPath);
    await expect(drawer.getByTestId("request-log-detail-status-code")).toHaveText("418");
    await expect(drawer.getByTestId("request-log-detail-duration-ms")).toHaveText("321ms");
    await expect(drawer.getByTestId("request-log-detail-ip-address")).toHaveText(ipAddress);
    await expect(drawer.getByTestId("request-log-detail-request-id")).toHaveText(requestID);
    await expect(drawer.getByTestId("request-log-detail-request-size")).toHaveText(
      "1.5 KB (1536 bytes)",
    );
    await expect(drawer.getByTestId("request-log-detail-response-size")).toHaveText(
      "2.0 KB (2048 bytes)",
    );
    await expect(drawer.getByTestId("request-log-detail-user-id")).toHaveText("-");
    await expect(drawer.getByTestId("request-log-copy-request-id")).toBeVisible();
    await drawer.getByRole("button", { name: "Close" }).click();

    await page.getByLabel("Method").selectOption("POST");
    await page.getByLabel("Path").fill(primaryPath);
    await page.getByLabel("Status Code").fill("418");
    await page.getByRole("button", { name: /Apply Filters/i }).click();

    await expect(page.getByTestId(`request-log-row-${primaryEntry.id}`)).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId(`request-log-row-${secondaryEntry.id}`)).toHaveCount(0);

    await page.getByRole("button", { name: /Query Performance/i }).click();
    const queryTableOrEmpty = page
      .getByRole("columnheader", { name: /Query/i })
      .or(page.getByText("No query statistics available"));
    await expect(queryTableOrEmpty).toBeVisible({ timeout: 5000 });
  });
});
