import {
  test,
  expect,
  expectOfflineRetryRecovery,
  probeEndpoint,
  fetchAdminStatsSnapshot,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: API Explorer — Content-Verified
 *
 * Executes a real GET /api/admin/stats through the API Explorer UI, then
 * asserts the response panel shows the actual status code and body content.
 */

test.describe("Smoke: API Explorer", () => {
  test("api explorer executes a real request and shows response content", async ({
    page,
    request,
    adminToken,
    context,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/stats");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Admin stats API unavailable (status ${probeStatus})`,
    );

    const statsSnapshot = await fetchAdminStatsSnapshot(request, adminToken);

    // Act: navigate to API Explorer
    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /^API Explorer$/i }).click();
    await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible({ timeout: 15_000 });

    // Verify request controls render
    await expect(page.getByLabel(/HTTP method/i)).toBeVisible();
    await expect(page.getByLabel(/Request path/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /^Send$/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /Query Parameters/i })).toBeVisible();

    await page.getByRole("button", { name: /Query Parameters/i }).click();
    await expect(page.getByLabel("filter")).toBeVisible();
    await expect(page.getByLabel("sort")).toBeVisible();
    await expect(page.getByRole("textbox", { name: "page", exact: true })).toBeVisible();
    await expect(page.getByRole("textbox", { name: "perPage", exact: true })).toBeVisible();
    await expect(page.getByLabel("fields")).toBeVisible();
    await expect(page.getByLabel("expand")).toBeVisible();
    await expect(page.getByLabel("search")).toBeVisible();

    // Arrange: set path to a known admin endpoint
    const pathInput = page.getByLabel(/Request path/i);
    await pathInput.fill("/api/admin/stats");

    // Act: execute the request
    await page.getByRole("button", { name: /^Send$/i }).click();

    // Assert: response panel shows status and real body content
    await expect(page.getByText(/^200 OK$/)).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(/uptime_seconds/)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/go_version/)).toBeVisible();
    await expect(page.getByText(new RegExp(statsSnapshot.go_version.replaceAll(".", "\\.")))).toBeVisible();
    await expect(page.getByRole("button", { name: /^cURL$/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /^JS SDK$/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Copy$/i })).toBeVisible();

    await page.getByRole("button", { name: /^History \(1\)$/i }).click();
    await expect(page.getByText("Recent Requests")).toBeVisible();
    await expect(page.getByRole("button", { name: new RegExp("GET.*/api/admin/stats.*200") })).toBeVisible();

    await page.getByRole("button", { name: /^History \(1\)$/i }).click();
    // Closest-real proxy: API Explorer request failures are surfaced from the
    // browser fetch boundary; offline mode preserves request controls and retry.
    await expectOfflineRetryRecovery(
      page,
      context,
      async () => {
        await page.getByRole("button", { name: /^Send$/i }).click();
      },
      async () => {
        await expect(page.getByText(/^200 OK$/)).toBeVisible({ timeout: 10000 });
        await expect(page.getByText(new RegExp(statsSnapshot.go_version.replaceAll(".", "\\.")))).toBeVisible();
        await expect(page.getByLabel(/Request path/i)).toHaveValue("/api/admin/stats");
      },
    );
  });
});
