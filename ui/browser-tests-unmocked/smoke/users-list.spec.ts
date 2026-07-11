import {
  test,
  expect,
  cleanupUserByEmail,
  ensureUserByEmail,
  probeEndpoint,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Users - List View
 *
 * Critical Path: Navigate to Users → Verify list loads with user data
 */

test.describe("Smoke: Users List", () => {
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("seeded user renders in users list", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/users/");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Users service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const testEmail = `seed-verify-${runId}@test.com`;

    // Arrange: seed a user through the shared fixture seam
    const seededUser = await ensureUserByEmail(request, adminToken, testEmail);
    userEmails.push(testEmail);

    // Act: navigate to Users page
    await page.goto("/admin/");
    await waitForDashboard(page);
    const usersButton = page.locator("aside").getByRole("button", { name: /^Users$/i });
    await usersButton.click();
    await expect(page.getByRole("heading", { name: /Users/i })).toBeVisible({ timeout: 15_000 });

    // Assert: seeded user email appears in the list
    await expect(page.getByText(testEmail).first()).toBeVisible({ timeout: 5000 });

    // Assert: search input is present (page fully loaded)
    const searchInput = page.getByPlaceholder(/search/i);
    await expect(searchInput).toBeVisible({ timeout: 3000 });

    await expect(page.getByRole("columnheader", { name: "Email" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Verified" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Created" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Actions" })).toBeVisible();

    const main = page.getByRole("main");
    await searchInput.fill(testEmail);
    await main.getByRole("button", { name: "Search", exact: true }).click();
    const seededRow = page.getByRole("row", { name: new RegExp(testEmail) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow.getByText(seededUser.id)).toBeVisible();
    await expect(page.getByText(/^1 user$/)).toBeVisible();
    await expect(page.getByText("1 / 1")).toBeVisible();
    await expect(page.getByRole("button", { name: "Previous page" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Next page" })).toBeDisabled();

    userEmails.pop();
    await cleanupUserByEmail(request, adminToken, testEmail);
  });
});
