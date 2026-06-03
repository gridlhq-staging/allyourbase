import {
  test,
  expect,
  getAuthSettingsUnavailableSkipReason,
  waitForDashboard,
  createVirtualAuthenticator,
} from "../fixtures";

test.describe("Auth Passkey Lifecycle (Full E2E)", () => {
  const userEmails: string[] = [];

  test.afterEach(async ({ mfaHelpers }) => {
    while (userEmails.length > 0) {
      const email = userEmails.pop();
      if (!email) continue;
      await mfaHelpers.cleanupAuthUser(email).catch(() => {});
    }
  });

  test("enroll passkey and use it for step-up", async ({
    page,
    request,
    adminToken,
    mfaHelpers,
  }) => {
    test.setTimeout(120_000);

    const authSettingsSkipReason = await getAuthSettingsUnavailableSkipReason(request, adminToken);
    test.skip(Boolean(authSettingsSkipReason), authSettingsSkipReason ?? "Auth settings unavailable");

    const runId = Date.now();
    const passkeyName = `e2e-passkey-${runId}`;

    await mfaHelpers.ensureAuthSettings({
      webauthn_enabled: true,
      anonymous_auth_enabled: true,
    });

    const virtualAuthenticator = await createVirtualAuthenticator(page);
    try {
      await test.step("Auth Settings: verify WebAuthn is enabled", async () => {
        await page.goto("/admin/");
        await waitForDashboard(page);
        await page.locator("aside").getByRole("button", { name: /Auth Settings/i }).click();
        await expect(page.getByRole("heading", { name: /Auth Settings/i })).toBeVisible({ timeout: 5000 });
        await expect(page.locator("aside").getByRole("button", { name: /MFA Management/i })).toBeVisible({
          timeout: 5000,
        });
      });

      await test.step("MFA Management: register a passkey with a distinctive name", async () => {
        await page.locator("aside").getByRole("button", { name: /MFA Management/i }).click();
        await expect(page.getByRole("heading", { name: /Multi-Factor Authentication/i })).toBeVisible({ timeout: 5000 });
        await expect(page.getByRole("button", { name: /Register Passkey/i })).toBeVisible({ timeout: 5000 });

        await page.getByTestId("passkey-display-name-input").fill(passkeyName);
        await page.getByTestId("passkey-register-button").click();
        await expect(page.getByTestId("passkey-name")).toContainText(passkeyName, { timeout: 10000 });
      });

      await test.step("MFA Management: require passkey-backed AAL2 success indicator", async () => {
        await expect(page.getByTestId("aal-level-indicator")).toContainText(/AAL2/i, { timeout: 5000 });
      });
    } finally {
      await virtualAuthenticator.remove();
    }
  });
});
