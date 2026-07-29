import {
  test,
  expect,
  probeEndpoint,
  ensureUserByEmail,
  cleanupUserByEmail,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Users Lifecycle
 *
 * Critical Path: Seed user → verify in list → search → delete via UI
 */

test.describe("Users Lifecycle (Full E2E)", () => {
  const userEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
  });

  test("load-and-verify seeded user, search, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/users");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Users service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const seededEmail = `user-full-seeded-${runId}@example.test`;
    const deletableEmail = `user-full-delete-${runId}@example.test`;

    const seededUser = await ensureUserByEmail(request, adminToken, seededEmail);
    userEmails.push(seededEmail);
    const deletableUser = await ensureUserByEmail(request, adminToken, deletableEmail);
    userEmails.push(deletableEmail);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page
      .getByRole("complementary")
      .getByRole("button", { name: /^Users$/i })
      .click();
    await expect(page.getByRole("heading", { name: /^Users$/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: "Email" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Verified" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Created" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Actions" })).toBeVisible();

    // Search for the seeded user
    const main = page.getByRole("main");
    const searchInput = main.getByPlaceholder("Search by email...");
    const searchButton = main.getByRole("button", { name: "Search", exact: true });
    await searchInput.fill(seededEmail);
    const clearSearchButton = main.getByRole("button", { name: "Clear search" });
    await expect(clearSearchButton).toBeVisible();
    await searchButton.click();

    const seededRow = page.getByRole("row", { name: new RegExp(seededEmail) }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow.getByText(seededUser.id)).toBeVisible();
    await expect(page.getByText(/^1 user$/)).toBeVisible();
    await expect(page.getByText("1 / 1")).toBeVisible();
    await expect(page.getByRole("button", { name: "Previous page" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Next page" })).toBeDisabled();

    // Clear search and find the deletable user
    await clearSearchButton.click();
    await expect(searchInput).toHaveValue("");
    await searchInput.fill(deletableEmail);
    await searchButton.click();

    const deletableRow = page.getByRole("row", { name: new RegExp(deletableEmail) }).first();
    await expect(deletableRow).toBeVisible({ timeout: 5000 });
    await expect(deletableRow.getByText(deletableUser.id)).toBeVisible();

    // Delete the user via UI
    await deletableRow.getByRole("button", { name: /Delete/i }).click();
    await expect(page.getByText(/Delete User/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(deletableEmail, { exact: true })).toHaveCount(2);
    await page.getByRole("button", { name: /^Delete$/i }).click();

    await expect(page.getByText(new RegExp(`User ${deletableEmail} deleted`, "i"))).toBeVisible({ timeout: 5000 });

    // Remove from cleanup list since it was deleted via UI
    const idx = userEmails.indexOf(deletableEmail);
    if (idx !== -1) userEmails.splice(idx, 1);
  });
});
