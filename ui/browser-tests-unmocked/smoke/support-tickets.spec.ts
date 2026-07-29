import {
  test,
  expect,
  replaceAdminRelationWithEmptyClone,
  seedSupportTicket,
  cleanupSupportTicketByID,
  probeEndpoint,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Support Tickets
 *
 * Critical Path: Seed a ticket with priority/status → Navigate to Support Tickets →
 * Verify row metadata (subject, status, priority) and detail view (message thread).
 */

test.describe("Smoke: Support Tickets", () => {
  const ticketIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (ticketIDs.length > 0) {
      const ticketID = ticketIDs.pop();
      if (!ticketID) continue;
      await cleanupSupportTicketByID(request, adminToken, ticketID).catch(() => {});
    }
  });

  test("seeded ticket renders with metadata in list and message in detail", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/support/tickets");
    expect(probeStatus, "/api/admin/support/tickets must be registered by the embedded console server").not.toBe(404);
    test.skip(
      probeStatus === 501,
      `Support service not configured (status ${probeStatus})`,
    );

    const runId = Date.now();
    const subject = `Smoke Support Ticket ${runId}`;
    const message = `Support ticket body ${runId}`;
    const seeded = await seedSupportTicket(request, adminToken, {
      subject,
      priority: "high",
      status: "open",
      initialMessage: message,
    });
    ticketIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Support Tickets/i }).click();
    await expect(page.getByRole("heading", { name: /Support Tickets/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers including metadata columns
    await expect(page.getByRole("columnheader", { name: /Subject/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /Status/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Priority/i })).toBeVisible();

    // Verify seeded ticket row renders with metadata
    const row = page.locator("tr").filter({ hasText: subject }).first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row.getByText("open")).toBeVisible();
    await expect(row.getByText("high")).toBeVisible();

    // Open detail view and verify message thread
    await row.getByRole("button", { name: /Details/i }).click();
    await expect(page.getByText(subject).first()).toBeVisible();
    await expect(page.getByText(message).first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: /Apply/i })).toBeVisible();
  });

  test("empty and unavailable support storage recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const subject = `Retry Support Ticket ${runId}`;
    const seeded = await seedSupportTicket(request, adminToken, {
      subject,
      priority: "normal",
      status: "open",
      initialMessage: `Retry support body ${runId}`,
    });
    ticketIDs.push(seeded.id);
    const relationState = await replaceAdminRelationWithEmptyClone(
      request,
      adminToken,
      "_ayb_support_tickets",
    );

    try {
      await page.goto("/admin/screens/support-tickets");
      await waitForDashboard(page);
      await expect(page.getByText("No support tickets", { exact: true })).toBeVisible();

      await relationState.removeEmptyClone();
      await page.reload();
      await waitForDashboard(page);
      await expect(page.getByText("failed to list support tickets", { exact: true })).toBeVisible();
      await expect(page.getByRole("heading", { name: /Support Tickets/i })).toBeVisible();

      await relationState.restore();
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(page.getByText(subject, { exact: true }).first()).toBeVisible({
        timeout: 5000,
      });
    } finally {
      await relationState.restore();
    }
  });
});
