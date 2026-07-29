import {
  test,
  expect,
  expectOfflineRetryRecovery,
  failIfReadinessForced,
  readinessNotMet,
  triggerAdminStatsRequest,
  waitForDashboard,
} from "../fixtures";
import type { Page, Response } from "@playwright/test";

const ADMIN_LOGS_PATH = "/api/admin/logs";

interface AdminLogsPayload {
  entries?: unknown;
  message?: unknown;
}

function isBufferingUnavailable(payload: AdminLogsPayload): boolean {
  return typeof payload.message === "string" && payload.message.toLowerCase().includes("not enabled");
}

async function waitForAdminLogsResponse(page: Page): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === ADMIN_LOGS_PATH && response.request().method() === "GET";
  });
}

test.describe("Smoke: Admin Logs", () => {
  test("admin can verify a known in-run request log row", async ({
    page,
    request,
    adminToken,
    context,
  }, testInfo) => {
    await failIfReadinessForced(testInfo, "admin-logs");

    await page.goto("/admin/");
    await waitForDashboard(page);

    const initialLogsResponsePromise = waitForAdminLogsResponse(page);
    await page.locator("aside").getByRole("button", { name: /Admin Logs/i }).click();
    await expect(page.getByRole("heading", { name: /Admin Logs/i })).toBeVisible({ timeout: 15_000 });

    const initialLogsResponse = await initialLogsResponsePromise;
    expect(initialLogsResponse.ok()).toBeTruthy();

    const initialPayload = (await initialLogsResponse.json()) as AdminLogsPayload;
    if (isBufferingUnavailable(initialPayload)) {
      await readinessNotMet(testInfo, "admin-logs", "log buffering not enabled");
    }

    const requestId = await triggerAdminStatsRequest(request, adminToken);

    const refreshResponsePromise = waitForAdminLogsResponse(page);
    await page.getByTestId("admin-logs-panel").getByRole("button", { name: /^Refresh$/i }).click();
    const refreshResponse = await refreshResponsePromise;
    expect(refreshResponse.ok()).toBeTruthy();

    const refreshPayload = (await refreshResponse.json()) as AdminLogsPayload;
    expect(isBufferingUnavailable(refreshPayload)).toBe(false);

    await page.getByLabel("Search logs").fill(requestId);
    const triggeredRow = page
      .getByTestId("admin-logs-panel")
      .getByRole("row")
      .filter({ hasText: "request" })
      .first();

    await expect(triggeredRow).toBeVisible();
    await expect(triggeredRow).toContainText("request");
    await triggeredRow.getByRole("button", { name: /Inspect attrs/i }).click();
    await expect(triggeredRow).toContainText(requestId);
    await expect(triggeredRow).toContainText("/api/admin/stats");

    await page.getByLabel("Search logs").fill(`no-log-${Date.now()}`);
    await expect(page.getByText("No log entries found", { exact: true })).toBeVisible();

    await expectOfflineRetryRecovery(
      page,
      context,
      async () => {
        await page.getByTestId("admin-logs-panel").getByRole("button", { name: /^Refresh$/i }).click();
      },
      async () => {
        await page.getByLabel("Search logs").fill(requestId);
        await expect(triggeredRow).toBeVisible({ timeout: 5000 });
      },
    );
  });
});
