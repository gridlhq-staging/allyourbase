import { readFile } from "node:fs/promises";
import {
  test,
  expect,
  probeEndpoint,
  seedRequestLogEntry,
  cleanupRequestLogsByPath,
  escapeLikePattern,
  execSQL,
  sqlLiteral,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

const MATCHING_EXPORT_ROW_COUNT = 501;

const REQUEST_LOG_CSV_HEADERS = [
  "id",
  "timestamp",
  "method",
  "path",
  "status_code",
  "duration_ms",
  "user_id",
  "api_key_id",
  "request_size",
  "response_size",
  "ip_address",
  "request_id",
];

function parseCSV(content: string): string[][] {
  const rows: string[][] = [[]];
  let value = "";
  let quoted = false;

  for (let index = 0; index < content.length; index += 1) {
    const character = content[index];
    if (character === '"') {
      if (quoted && content[index + 1] === '"') {
        value += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
    } else if (character === "," && !quoted) {
      rows.at(-1)?.push(value);
      value = "";
    } else if (character === "\n" && !quoted) {
      rows.at(-1)?.push(value);
      rows.push([]);
      value = "";
    } else if (character !== "\r" || quoted) {
      value += character;
    }
  }
  rows.at(-1)?.push(value);
  return rows;
}

function csvRecords(content: string): Record<string, string>[] {
  const [headers, ...rows] = parseCSV(content);
  return rows.map((row) =>
    Object.fromEntries(headers.map((header, index) => [header, row[index] ?? ""])),
  );
}

async function openAnalytics(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page
    .getByRole("complementary")
    .getByRole("button", { name: "Analytics", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "Analytics", exact: true })).toBeVisible({
    timeout: 15_000,
  });
}

async function cleanupRequestLogsByPrefix(
  request: Parameters<typeof execSQL>[0],
  token: string,
  pathPrefix: string,
): Promise<void> {
  // Stage 1 product gap: request logs have no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: request logs have no domain delete/cleanup API.
    `DELETE FROM _ayb_request_logs WHERE path LIKE '${escapeLikePattern(pathPrefix)}%' ESCAPE '\\'`,
  );
}

async function seedMatchingRequestLogsForExportPagination(
  request: Parameters<typeof execSQL>[0],
  token: string,
  pathPrefix: string,
  baseTimestamp: number,
): Promise<Awaited<ReturnType<typeof seedRequestLogEntry>>[]> {
  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: request logs are generated internally and have no deterministic seed API.
    `INSERT INTO _ayb_request_logs (
       timestamp, method, path, status_code, duration_ms, request_size, response_size, request_id, ip_address
     )
     SELECT
       ('${new Date(baseTimestamp).toISOString()}'::timestamptz + (series.index * interval '1 second')),
       CASE WHEN series.index = ${MATCHING_EXPORT_ROW_COUNT - 1} THEN 'POST' ELSE 'GET' END,
       CASE
         WHEN series.index = 0 THEN '${sqlLiteral(`${pathPrefix}/oldest,"quoted"\ncontinuation`)}'
         ELSE '${sqlLiteral(pathPrefix)}/matching-' || series.index::text
       END,
       CASE WHEN series.index = ${MATCHING_EXPORT_ROW_COUNT - 1} THEN 499 ELSE 418 END,
       CASE
         WHEN series.index = 0 THEN 300
         WHEN series.index = ${MATCHING_EXPORT_ROW_COUNT - 1} THEN 400
         ELSE 320 + (series.index % 60)
       END,
       CASE WHEN series.index = ${MATCHING_EXPORT_ROW_COUNT - 1} THEN 1536 ELSE series.index END,
       CASE WHEN series.index = ${MATCHING_EXPORT_ROW_COUNT - 1} THEN 2048 ELSE series.index + 1 END,
       'analytics-request-${sqlLiteral(pathPrefix)}-' || series.index::text,
       '198.51.100.42'::inet
     FROM generate_series(0, ${MATCHING_EXPORT_ROW_COUNT - 1}) AS series(index);
     SELECT id::text, path, method, status_code, duration_ms
     FROM _ayb_request_logs
     WHERE path LIKE '${escapeLikePattern(pathPrefix)}%' ESCAPE '\\'
     ORDER BY timestamp ASC`,
  );

  return result.rows.map((row) => {
    const [id, path, method, statusCode, durationMs] = row;
    if (
      typeof id !== "string" ||
      typeof path !== "string" ||
      typeof method !== "string" ||
      typeof statusCode !== "number" ||
      typeof durationMs !== "number"
    ) {
      throw new Error(`Expected seeded request log fields for prefix ${pathPrefix}`);
    }
    return { id, path, method, statusCode, durationMs };
  });
}

test.describe("Analytics Lifecycle (Full E2E)", () => {
  const seededPaths: string[] = [];
  const seededPathPrefixes: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededPathPrefixes.length > 0) {
      const pathPrefix = seededPathPrefixes.pop();
      if (!pathPrefix) continue;
      await cleanupRequestLogsByPrefix(request, adminToken, pathPrefix).catch(() => {});
    }
    while (seededPaths.length > 0) {
      const path = seededPaths.pop();
      if (!path) continue;
      await cleanupRequestLogsByPath(request, adminToken, path).catch(() => {});
    }
  });

  test("filters, pages, and exports every matching request log", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(
      request,
      adminToken,
      "/api/admin/analytics/requests",
    );
    test.skip(
      probeStatus === 503 || probeStatus === 501 || probeStatus === 404,
      `Analytics request-log endpoint unavailable (status ${probeStatus})`,
    );
    expect(
      probeStatus,
      `Analytics request-log endpoint must load successfully, received HTTP ${probeStatus}`,
    ).toBe(200);

    const runId = Date.now();
    const pathPrefix = `/api/full-analytics/${runId}`;
    const baseTimestamp = Date.now() + 120_000;
    const matchingEntries = await seedMatchingRequestLogsForExportPagination(
      request,
      adminToken,
      pathPrefix,
      baseTimestamp,
    );
    seededPathPrefixes.push(pathPrefix);

    const statusMismatchPath = `${pathPrefix}/status-mismatch`;
    const statusMismatch = await seedRequestLogEntry(request, adminToken, {
      method: "GET",
      path: statusMismatchPath,
      statusCode: 200,
      durationMs: 350,
      timestampISO: new Date(baseTimestamp - 2000).toISOString(),
    });
    seededPaths.push(statusMismatchPath);

    const latencyMismatchPath = `${pathPrefix}/latency-mismatch`;
    const latencyMismatch = await seedRequestLogEntry(request, adminToken, {
      method: "GET",
      path: latencyMismatchPath,
      statusCode: 418,
      durationMs: 401,
      timestampISO: new Date(baseTimestamp - 1000).toISOString(),
    });
    seededPaths.push(latencyMismatchPath);

    await openAnalytics(page);

    const newestEntry = matchingEntries.at(-1);
    const oldestEntry = matchingEntries[0];
    const uiSecondPageEntry = matchingEntries.at(-26);
    expect(newestEntry).toBeDefined();
    expect(oldestEntry).toBeDefined();
    expect(uiSecondPageEntry).toBeDefined();
    if (!newestEntry || !oldestEntry || !uiSecondPageEntry) return;

    const newestRow = page.getByTestId(`request-log-row-${newestEntry.id}`);
    await expect(newestRow).toContainText(newestEntry.path, { timeout: 5000 });
    await expect(newestRow.getByRole("cell", { name: newestEntry.method })).toBeVisible();
    await expect(
      newestRow.getByRole("cell", { name: String(newestEntry.statusCode) }),
    ).toBeVisible();

    await newestRow
      .getByTestId(`request-log-view-details-${newestEntry.id}`)
      .click();
    const drawer = page.getByRole("dialog", { name: "Request details" });
    await expect(drawer.getByTestId("request-log-detail-id")).toHaveText(newestEntry.id);
    await expect(drawer.getByTestId("request-log-detail-duration-ms")).toHaveText("400ms");
    await expect(drawer.getByTestId("request-log-detail-ip-address")).toHaveText(
      "198.51.100.42",
    );
    await expect(drawer.getByTestId("request-log-detail-request-size")).toHaveText(
      "1.5 KB (1536 bytes)",
    );
    await expect(drawer.getByTestId("request-log-detail-response-size")).toHaveText(
      "2.0 KB (2048 bytes)",
    );
    await drawer.getByRole("button", { name: "Close" }).click();

    await page.getByLabel("Path").fill(`${pathPrefix}*`);
    await page.getByLabel("Status Class").selectOption("4xx");
    await page.getByLabel("Minimum Latency (ms)").fill("300");
    await page.getByLabel("Maximum Latency (ms)").fill("400");
    await page.getByRole("button", { name: "Apply Filters" }).click();

    await expect(page.getByTestId("request-logs-summary")).toContainText(
      `Showing 1–25 of ${MATCHING_EXPORT_ROW_COUNT} request logs`,
    );
    await expect(page.getByTestId(`request-log-row-${newestEntry.id}`)).toBeVisible();
    await expect(page.getByTestId(`request-log-row-${oldestEntry.id}`)).toHaveCount(0);
    await expect(page.getByTestId(`request-log-row-${statusMismatch.id}`)).toHaveCount(0);
    await expect(page.getByTestId(`request-log-row-${latencyMismatch.id}`)).toHaveCount(0);

    const previousButton = page.getByRole("button", {
      name: "Previous request-log page",
    });
    const nextButton = page.getByRole("button", { name: "Next request-log page" });
    await expect(previousButton).toBeDisabled();
    await expect(nextButton).toBeEnabled();
    await nextButton.click();

    await expect(page.getByTestId(`request-log-row-${uiSecondPageEntry.id}`)).toBeVisible();
    await expect(page.getByTestId(`request-log-row-${newestEntry.id}`)).toHaveCount(0);
    await expect(page.getByTestId("request-logs-summary")).toContainText(
      `Showing 26–50 of ${MATCHING_EXPORT_ROW_COUNT} request logs`,
    );
    await expect(previousButton).toBeEnabled();
    await expect(nextButton).toBeEnabled();
    await previousButton.click();
    await expect(page.getByTestId(`request-log-row-${newestEntry.id}`)).toBeVisible();
    await expect(previousButton).toBeDisabled();

    const jsonDownloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: "Export JSON" }).click();
    const jsonDownload = await jsonDownloadPromise;
    expect(jsonDownload.suggestedFilename()).toMatch(
      /^request_logs_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z\.json$/,
    );
    const jsonDownloadPath = await jsonDownload.path();
    expect(jsonDownloadPath).not.toBeNull();
    const jsonRows = JSON.parse(
      await readFile(jsonDownloadPath as string, "utf8"),
    ) as Record<string, unknown>[];
    const oldestJSON = jsonRows.find((row) => row.id === oldestEntry.id);
    expect(oldestJSON).toMatchObject({
      id: oldestEntry.id,
      path: oldestEntry.path,
      status_code: oldestEntry.statusCode,
      duration_ms: oldestEntry.durationMs,
    });
    expect(
      jsonRows.findIndex((row) => row.id === oldestEntry.id),
      "oldest seeded row should be fetched from the second 500-row export API page",
    ).toBe(500);
    expect(jsonRows.find((row) => row.id === newestEntry.id)).toMatchObject({
      path: newestEntry.path,
      status_code: newestEntry.statusCode,
      duration_ms: newestEntry.durationMs,
    });

    const csvDownloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: "Export CSV" }).click();
    const csvDownload = await csvDownloadPromise;
    expect(csvDownload.suggestedFilename()).toMatch(
      /^request_logs_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z\.csv$/,
    );
    const csvDownloadPath = await csvDownload.path();
    expect(csvDownloadPath).not.toBeNull();
    const csvContent = await readFile(csvDownloadPath as string, "utf8");
    expect(parseCSV(csvContent)[0]).toEqual(REQUEST_LOG_CSV_HEADERS);
    expect(csvContent).toContain(',"/api/full-analytics/');
    expect(csvContent).toContain('""quoted""');
    const csvRows = csvRecords(csvContent);
    expect(csvRows.find((row) => row.id === oldestEntry.id)).toMatchObject({
      path: oldestEntry.path,
      status_code: String(oldestEntry.statusCode),
      duration_ms: String(oldestEntry.durationMs),
    });
    expect(
      csvRows.findIndex((row) => row.id === oldestEntry.id),
      "oldest seeded row should be fetched from the second 500-row export API page",
    ).toBe(500);
    expect(csvRows.find((row) => row.id === newestEntry.id)).toMatchObject({
      path: newestEntry.path,
      status_code: String(newestEntry.statusCode),
      duration_ms: String(newestEntry.durationMs),
    });

    await page.getByRole("button", { name: "Query Performance" }).click();
    await page.getByLabel("Sort by").selectOption("mean_time");
    const queryTableOrEmpty = page
      .getByRole("columnheader", { name: "Query", exact: true })
      .or(page.getByText("No query statistics available"));
    await expect(queryTableOrEmpty).toBeVisible({ timeout: 5000 });
  });
});
