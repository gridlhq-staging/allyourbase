import {
  test,
  expect,
  expectOfflineRetryRecovery,
  ensureAuthSettings,
  navigateDashboardScreenInPage,
  probeEndpoint,
  cleanupAuthUser,
  createLinkedEmailAuthSessionToken,
  fetchAuthSettings,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: MFA Management — Content-Verified
 *
 * Verifies the MFA enrollment view renders real content: enrollment buttons,
 * enrolled methods section, and enrollment flow transition on button click.
 *
 * NOTE: MFAEnrollment renders TOTP/Email buttons unconditionally (does not
 * read admin auth-settings), so assertions verify component behavior rather
 * than config-coupled conditional rendering.
 */

test.describe("Smoke: MFA Management", () => {
  const linkedEmails: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (linkedEmails.length > 0) {
      const email = linkedEmails.pop();
      if (!email) continue;
      await cleanupAuthUser(request, adminToken, email).catch(() => {});
    }
  });

  test("mfa management shows enrollment controls and enrolled methods section", async ({
    page,
    request,
    adminToken,
    context,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/auth-settings");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Auth service unavailable (status ${probeStatus})`,
    );
    const originalSettings = await fetchAuthSettings(request, adminToken);
    await ensureAuthSettings(request, adminToken, {
      anonymous_auth_enabled: true,
      totp_enabled: true,
    });

    const runId = Date.now();
    const email = `smoke-mfa-${runId}@example.test`;
    const password = `Sm0kePass!${runId}`;
    linkedEmails.push(email);

    try {
      const authToken = await createLinkedEmailAuthSessionToken(request, email, password);
      await page.addInitScript((token: string) => {
        window.localStorage.setItem("ayb_auth_token", token);
      }, authToken);

      // Act: navigate to MFA Management
      await page.goto("/admin/");
      await waitForDashboard(page);
      await page.locator("aside").getByRole("button", { name: /^Multi-Factor Authentication$/i }).click();
      await expect(page.getByRole("heading", { name: /Multi-Factor Authentication/i })).toBeVisible({ timeout: 15_000 });

      // Assert: enrolled methods section is always rendered
      const enrolledSection = page.getByTestId("mfa-enrolled-methods");
      await expect(enrolledSection).toBeVisible({ timeout: 5000 });

      // Assert: the uniquely created auth session starts with no enrolled factors.
      await expect(enrolledSection.getByText("No MFA methods enrolled", { exact: true })).toBeVisible({
        timeout: 3000,
      });

      // Assert: enrollment action buttons are unconditionally visible
      const totpButton = page.getByRole("button", { name: /Set Up Authenticator/i });
      const emailMfaButton = page.getByRole("button", { name: /Set Up Email MFA/i });
      await expect(totpButton).toBeVisible();
      await expect(emailMfaButton).toBeVisible();

      // Assert: clicking TOTP enrollment transitions to enrollment flow
      await totpButton.click();
      await expect(page.getByText(/Set Up Authenticator App/i)).toBeVisible({ timeout: 5000 });
      await expect(page.getByTestId("totp-confirm-code")).toBeVisible();

      // Cancel to restore idle state
      await page.getByRole("button", { name: /Cancel/i }).click();
      await expect(totpButton).toBeVisible({ timeout: 3000 });

      await navigateDashboardScreenInPage(page, "api-explorer");
      await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible();

      // Closest-real proxy: MFA factor load fails at the authenticated fetch
      // boundary; offline mode proves the load-error retry without action errors.
      await expectOfflineRetryRecovery(
        page,
        context,
        async () => {
          await navigateDashboardScreenInPage(page, "mfa-management");
        },
        async () => {
          await expect(page.getByTestId("mfa-enrolled-methods")).toBeVisible({ timeout: 5000 });
          await expect(page.getByText("No MFA methods enrolled", { exact: true })).toBeVisible();
        },
      );
    } finally {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: originalSettings.anonymous_auth_enabled,
        totp_enabled: originalSettings.totp_enabled,
      });
    }
  });
});
