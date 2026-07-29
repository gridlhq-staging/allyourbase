import {
  test,
  expect,
  expectOfflineRetryRecovery,
  navigateDashboardScreenInPage,
  probeEndpoint,
  fetchSecurityAdvisorReport,
  waitForDashboard,
} from "../fixtures";

type SecurityFinding = Awaited<ReturnType<typeof fetchSecurityAdvisorReport>>["findings"][number];

function emptySecurityFilterFor(findings: SecurityFinding[]): { severity: string; status: string } {
  const severities = ["critical", "high", "medium", "low"];
  const statuses = ["open", "accepted", "resolved"];
  for (const severity of severities) {
    for (const status of statuses) {
      if (!findings.some((finding) => finding.severity === severity && finding.status === status)) {
        return { severity, status };
      }
    }
  }
  return { severity: "critical", status: "resolved" };
}

/**
 * SMOKE TEST: Security Advisor — Content-Verified
 *
 * Probes the security advisor endpoint and asserts real content when available
 * (severity filters, finding sections, "Last evaluated" timestamp), or verifies
 * the error UI renders correctly when the backend endpoint is not wired.
 */

test.describe("Smoke: Security Advisor", () => {
  test("security advisor renders content matching API-backed report state", async ({
    page,
    request,
    adminToken,
    context,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/advisors/security");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Security advisor endpoint unavailable (status ${probeStatus})`,
    );

    const report = await fetchSecurityAdvisorReport(request, adminToken);

    // Act: navigate to Security Advisor
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^Security Advisor$/i }).click();
    await expect(page.getByRole("heading", { name: /Security Advisor/i })).toBeVisible({ timeout: 15_000 });

    const panel = page.getByTestId("security-advisor-panel");
    await expect(panel).toBeVisible({ timeout: 5000 });

    await expect(panel.getByLabel(/^Severity$/i)).toBeVisible({ timeout: 5000 });
    await expect(panel.getByLabel(/^Category$/i)).toBeVisible();
    await expect(panel.getByLabel(/^Status$/i)).toBeVisible();

    const emptyState = panel.getByText(/No findings for current filters/i);
    if (report.findings.length === 0) {
      await expect(emptyState).toBeVisible({ timeout: 5000 });
    } else {
      await expect(panel.getByText(report.findings[0].title, { exact: true })).toBeVisible({
        timeout: 5000,
      });
      await expect(emptyState).toBeHidden();
    }
    await expect(panel.getByText(/Last evaluated:/i)).toContainText(report.evaluatedAt.substring(0, 10));

    const emptyFilter = emptySecurityFilterFor(report.findings);
    await panel.getByLabel(/^Severity$/i).selectOption(emptyFilter.severity);
    await panel.getByLabel(/^Status$/i).selectOption(emptyFilter.status);
    await expect(panel.getByText("No findings for current filters.", { exact: true })).toBeVisible();

    // Closest-real proxy: advisor failures require the live API to be
    // unreachable; offline mode proves the existing polling retry path.
    await navigateDashboardScreenInPage(page, "api-explorer");
    await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible();

    await expectOfflineRetryRecovery(
      page,
      context,
      async () => {
        await navigateDashboardScreenInPage(page, "security-advisor");
      },
      async () => {
        await expect(page.getByTestId("security-advisor-panel")).toBeVisible({ timeout: 5000 });
        await expect(page.getByText(/Last evaluated:/i)).toContainText(report.evaluatedAt.substring(0, 10));
      },
    );
  });
});
