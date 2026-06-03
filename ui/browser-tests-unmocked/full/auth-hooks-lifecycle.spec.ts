import { test, expect, probeEndpoint, fetchAuthHooksConfig, waitForDashboard } from "../fixtures";

/**
 * FULL E2E TEST: Auth Hooks Lifecycle
 *
 * Critical Path: Navigate to Auth Hooks → verify all 6 hook labels render →
 * verify each hook shows either a function reference or "Not configured" →
 * verify hook config reflects server state via API cross-check
 *
 * Note: Auth Hooks UI is read-only (config is server-side, no mutations in UI).
 * This lifecycle test verifies that the rendered state matches the API response
 * and that all hook slots are displayed correctly.
 */

test.describe("Auth Hooks Lifecycle (Full E2E)", () => {
  const HOOK_LABELS = [
    "Before Sign Up",
    "After Sign Up",
    "Custom Access Token",
    "Before Password Reset",
    "Send Email",
    "Send SMS",
  ];

  const HOOK_KEY_MAP: Record<string, string> = {
    "Before Sign Up": "before_sign_up",
    "After Sign Up": "after_sign_up",
    "Custom Access Token": "custom_access_token",
    "Before Password Reset": "before_password_reset",
    "Send Email": "send_email",
    "Send SMS": "send_sms",
  };

  test("verify all auth hook labels render with correct config state", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth/hooks");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Auth hooks service unavailable (status ${probeStatus})`,
    );

    // Fetch hook config via fixture for cross-check
    const hookConfig = await fetchAuthHooksConfig(request, adminToken);

    // Navigate to Auth Hooks
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Auth Hooks$/i }).click();
    await expect(page.getByRole("heading", { name: /Auth Hooks/i })).toBeVisible({ timeout: 5000 });

    // Verify all 6 hook labels render
    for (const label of HOOK_LABELS) {
      await expect(page.getByText(label, { exact: true })).toBeVisible({ timeout: 3000 });
    }

    // Cross-check each hook slot by its stable key to prevent false positives where
    // aggregate "Not configured" counts pass while values are rendered on the wrong row.
    for (const label of HOOK_LABELS) {
      const hookKey = HOOK_KEY_MAP[label];
      const expectedValue = hookConfig[hookKey] || "Not configured";
      await expect(page.getByTestId(`auth-hook-value-${hookKey}`)).toHaveText(expectedValue, {
        timeout: 3000,
      });
    }
  });
});
