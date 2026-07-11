import { test, expect, waitForDashboard } from "../fixtures";
import { resolveAdminPasswordForBrowserLogin } from "../admin-bootstrap";
import type { Page, Route } from "@playwright/test";

/**
 * SMOKE TEST: Admin Dashboard - Login
 *
 * Critical Path: Admin enters password → Dashboard loads
 *
 * Note: This test uses its OWN storage state (no pre-auth)
 * since it tests the login flow itself.
 *
 * IMPORTANT: These tests are marked @slow and run serially
 * because they test the login flow which is rate-limited.
 * The auth.setup.ts already validates basic login functionality,
 * so these tests are supplementary.
 */

// Override storageState — login test must start unauthenticated
test.use({ storageState: { cookies: [], origins: [] } });

const ADMIN_STATUS_PATH = "**/api/admin/status";
const ADMIN_AUTH_PATH = "**/api/admin/auth";

async function fulfillJSON(route: Route, status: number, body: unknown): Promise<void> {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function requireAdminLogin(page: Page): Promise<void> {
  await page.route(ADMIN_STATUS_PATH, (route) => fulfillJSON(route, 200, { auth: true }));
}

test.describe("Smoke: Admin Login", () => {
  // Run login tests serially to avoid rate limiting
  // Tag as @slow since they require pauses between tests
  test.describe.configure({ mode: "serial" });

  test("renders the initial admin login shell", async ({ page }) => {
    await requireAdminLogin(page);

    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: /Allyourbase/ })).toBeVisible();
    await expect(page.getByText("Enter the admin password to continue.")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("admin can log in with correct password", async ({ page, request, authStatus }) => {
    test.slow(); // Mark as slow test - needs extra time
    test.skip(!authStatus.auth, "admin.password not configured — no-auth mode");

    // Step 1: Navigate to admin dashboard with fresh page load
    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    // Step 2: Verify login form is shown
    await expect(page.getByText("Enter the admin password")).toBeVisible({ timeout: 15000 });

    // Step 3: Enter an actual admin password. The saved admin auth file is
    // usually already a bearer token, so only use it here when the server
    // still accepts it as a legacy password value.
    const adminPassword = await resolveAdminPasswordForBrowserLogin(request);
    test.skip(
      adminPassword === null,
      "positive admin password login requires AYB_ADMIN_PASSWORD or a legacy password in ~/.ayb/admin-token",
    );
    const passwordInput = page.getByLabel("Password");
    await expect(passwordInput).toBeVisible({ timeout: 5000 });
    await passwordInput.fill(adminPassword);

    // Step 4: Click Sign in
    const signInButton = page.getByRole("button", { name: "Sign in" });
    await expect(signInButton).toBeVisible({ timeout: 5000 });
    await signInButton.click();

    // Step 5: Verify dashboard loads
    await waitForDashboard(page);
  });

  test("admin login rejects wrong password inline", async ({ page }) => {
    await requireAdminLogin(page);
    await page.route(ADMIN_AUTH_PATH, (route) =>
      fulfillJSON(route, 401, { message: "invalid password" }),
    );

    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    const passwordInput = page.getByLabel("Password");
    await expect(passwordInput).toBeVisible({ timeout: 15000 });
    await passwordInput.fill("wrongpassword123");

    const signInButton = page.getByRole("button", { name: "Sign in" });
    await signInButton.click();

    await expect(page.getByText("invalid password")).toBeVisible();
    await expect(signInButton).toBeVisible();
    await expect(passwordInput).toBeVisible();
  });

  test("admin boot shows connection error messaging", async ({ page }) => {
    await page.route(ADMIN_STATUS_PATH, (route) =>
      fulfillJSON(route, 503, { message: "status unavailable" }),
    );

    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "Connection Error" })).toBeVisible();
    await expect(page.getByText("status unavailable")).toBeVisible();
    await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  });

  test("admin boot retry recovers to the login shell", async ({ page }) => {
    let statusRequests = 0;
    await page.route(ADMIN_STATUS_PATH, async (route) => {
      statusRequests += 1;
      if (statusRequests === 1) {
        await fulfillJSON(route, 503, { message: "status unavailable" });
        return;
      }
      await fulfillJSON(route, 200, { auth: true });
    });

    await page.goto("/admin/", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "Connection Error" })).toBeVisible();
    await page.getByRole("button", { name: "Retry" }).click();

    await expect(page.getByRole("heading", { name: /Allyourbase/ })).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
  });

  test("safe return_to redirects through login and reaches dashboard", async ({
    page,
    request,
    authStatus,
  }) => {
    test.slow(); // Mark as slow test - needs extra time
    test.skip(!authStatus.auth, "admin.password not configured — no-auth mode");

    const adminPassword = await resolveAdminPasswordForBrowserLogin(request);
    test.skip(
      adminPassword === null,
      "return_to login proof requires AYB_ADMIN_PASSWORD or a legacy password in ~/.ayb/admin-token",
    );

    await page.goto("/admin/?return_to=%2Fadmin%2F", { waitUntil: "domcontentloaded" });

    await page.getByLabel("Password").fill(adminPassword);
    await page.getByRole("button", { name: "Sign in" }).click();

    await expect(page).toHaveURL(/\/admin\/$/);
    await waitForDashboard(page);
  });
});
