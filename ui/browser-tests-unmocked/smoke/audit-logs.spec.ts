import {
  test,
  expect,
  replaceAdminRelationWithEmptyClone,
  seedAuditLogEntry,
  seedUnreadableAuditLogTimestamp,
  cleanupAuditLogsByTable,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Audit Logs
 *
 * Critical Path: Seed a deterministic audit-log entry → Navigate to Audit Logs →
 * verify table row content, filter behavior, and expanded old/new payload details.
 */

test.describe("Smoke: Audit Logs", () => {
  const seededTables: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededTables.length > 0) {
      const tableName = seededTables.pop();
      if (!tableName) continue;
      await cleanupAuditLogsByTable(request, adminToken, tableName).catch(() => {});
    }
  });

  test("seeded audit row supports filter and expanded payload checks", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const tableName = `smoke_audit_logs_${runId}`;
    const seededEntry = await seedAuditLogEntry(request, adminToken, {
      tableName,
      operation: "UPDATE",
      recordID: { id: `record-${runId}` },
      oldValues: { status: "draft", runId },
      newValues: { status: "published", runId },
      timestampISO: new Date(Date.now() + 5000).toISOString(),
      ipAddress: "10.22.33.44",
    });
    seededTables.push(tableName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Audit Logs/i }).click();
    await expect(page.getByRole("heading", { name: /Audit Logs/i })).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole("columnheader", { name: /Operation/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Table/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Timestamp/i })).toBeVisible();

    await page.locator('main').getByLabel('Table').fill(tableName);
    await page.getByRole("button", { name: /Apply Filters/i }).click();

    const seededRow = page.locator("tr").filter({ hasText: tableName }).first();
    await expect(seededRow.getByRole("cell", { name: "UPDATE" })).toBeVisible({ timeout: 5000 });
    await expect(seededRow.getByRole("cell", { name: tableName })).toBeVisible();

    const shortID = seededEntry.id.slice(0, 8);
    await page.getByRole("button", { name: new RegExp(`Show changes for\\s+${shortID}`) }).click();
    await expect(page.getByRole("region", { name: "Audit change details" })).toContainText(
      `"status": "published"`,
      { timeout: 5000 },
    );
    await expect(page.getByRole("region", { name: "Audit change details" })).toContainText(
      `"status": "draft"`,
    );

    await expect(page.getByRole("button", { name: /Apply Filters/i })).toBeVisible();
  });

  test("empty audit filter and unavailable audit storage recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const tableName = `retry_audit_logs_${runId}`;
    await seedAuditLogEntry(request, adminToken, {
      tableName,
      operation: "INSERT",
      recordID: { id: `retry-record-${runId}` },
      newValues: { proof: `retry-proof-${runId}` },
    });
    seededTables.push(tableName);
    const relationState = await replaceAdminRelationWithEmptyClone(
      request,
      adminToken,
      "_ayb_audit_log",
    );

    try {
      await page.goto("/admin/screens/audit-logs");
      await waitForDashboard(page);
      await page.locator("main").getByLabel("Table").fill(tableName);
      await page.getByRole("button", { name: /Apply Filters/i }).click();
      await expect(page.getByText("No audit log entries found", { exact: true })).toBeVisible();

      await seedUnreadableAuditLogTimestamp(request, adminToken, tableName);
      await page.reload();
      await waitForDashboard(page);
      // The fixture retypes `timestamp` to text and inserts an unparseable value. Which
      // backend path reports it depends on the pooled connection that serves the reload:
      // one still holding a prepared statement that describes `timestamp` as timestamptz
      // fails in rows.Err() ("failed to iterate audit log rows"), while a freshly prepared
      // statement describes it as text and fails in rows.Scan ("failed to decode audit log
      // row"). Both are real degraded-state outcomes of admin_audit.go, so assert that
      // closed set rather than pinning one connection-cache outcome.
      const degradedMessage = page.getByText(
        /^failed to (decode audit log row|iterate audit log rows)$/,
      );
      await expect(degradedMessage).toBeVisible();
      console.log(`AUDIT_DEGRADED_MESSAGE:${(await degradedMessage.textContent())?.trim()}`);
      await expect(page.getByRole("heading", { name: /Audit Logs/i })).toBeVisible();

      await relationState.restore();
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(page.getByText(tableName, { exact: true })).toBeVisible({ timeout: 5000 });
    } finally {
      await relationState.restore();
    }
  });
});
