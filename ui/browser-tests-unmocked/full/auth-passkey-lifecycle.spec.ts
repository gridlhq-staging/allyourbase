import {
  test,
  expect,
  getAuthSettingsUnavailableSkipReason,
  waitForDashboard,
  createVirtualAuthenticator,
  createLinkedEmailAuthSessionToken,
  blockMatchingRequest,
} from "../fixtures";
import type { Locator, Page } from "@playwright/test";

function passkeyRow(page: Page, passkeyName: string): Locator {
  return page.getByTestId(/^passkey-row-/).filter({ has: page.getByText(passkeyName, { exact: true }) });
}

async function enrollPasskey(page: Page, passkeyName: string): Promise<void> {
  const registerButton = page.getByTestId("passkey-register-button");
  await page.getByTestId("passkey-display-name-input").fill(passkeyName);
  await registerButton.click();
  await expect(page.getByText(`Passkey "${passkeyName}" registered`)).toBeVisible({ timeout: 10000 });
  await expect(passkeyRow(page, passkeyName)).toBeVisible({ timeout: 10000 });
  await expect(registerButton).toBeEnabled({ timeout: 10000 });
}

async function deletePasskeyThroughUI(passkey: Locator): Promise<void> {
  await passkey.getByTestId("passkey-delete-button").click();
  const dialog = passkey.page().getByRole("dialog", { name: "Delete passkey" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Delete" }).click();
}

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
    const testEmail = `passkey-lifecycle-${runId}@example.test`;
    const testPassword = `PasskeyLifecycle!${runId}`;
    const firstPasskeyName = `e2e-passkey-primary-${runId}`;
    const secondPasskeyName = `e2e-passkey-backup-${runId}`;
    const renamedPasskeyName = `e2e-passkey-renamed-${runId}`;
    userEmails.push(testEmail);

    await mfaHelpers.ensureAuthSettings({
      webauthn_enabled: true,
      anonymous_auth_enabled: true,
    });
    const authToken = await createLinkedEmailAuthSessionToken(request, testEmail, testPassword);
    await page.addInitScript((token: string) => {
      window.localStorage.setItem("ayb_auth_token", token);
    }, authToken);

    let virtualAuthenticator: Awaited<ReturnType<typeof createVirtualAuthenticator>> | null = null;
    try {
      await test.step("Auth Settings: verify WebAuthn is enabled", async () => {
        await page.goto("/admin/");
        await waitForDashboard(page);
        await page.locator("aside").getByRole("button", { name: /Auth Settings/i }).click();
        await expect(page.getByRole("heading", { name: /Auth Settings/i })).toBeVisible({ timeout: 5000 });
        await expect(page.locator("aside").getByRole("button", { name: /Multi-Factor Authentication/i })).toBeVisible({
          timeout: 5000,
        });
      });

      await test.step("MFA Management: register two passkeys with distinctive names", async () => {
        await page.locator("aside").getByRole("button", { name: /Multi-Factor Authentication/i }).click();
        await expect(page.getByRole("heading", { name: /Multi-Factor Authentication/i })).toBeVisible({ timeout: 5000 });
        await expect(page.getByRole("button", { name: /Register Passkey/i })).toBeVisible({ timeout: 5000 });
        virtualAuthenticator = await createVirtualAuthenticator(page);

        await enrollPasskey(page, firstPasskeyName);
        await mfaHelpers.promoteSessionToAAL2WithPasskey(page, testEmail, testPassword);
        await virtualAuthenticator.remove();
        virtualAuthenticator = await createVirtualAuthenticator(page);
        await enrollPasskey(page, secondPasskeyName);
        await expect(passkeyRow(page, firstPasskeyName)).toBeVisible();
        await expect(passkeyRow(page, secondPasskeyName)).toBeVisible();
      });

      await test.step("MFA Management: rename one passkey and delete the other", async () => {
        const firstPasskey = passkeyRow(page, firstPasskeyName);
        await firstPasskey.getByTestId("passkey-rename-input").fill(renamedPasskeyName);
        const renameButton = firstPasskey.getByTestId("passkey-rename-button");
        await blockMatchingRequest(
          page,
          {
            method: "PATCH",
            urlIncludes: "/api/auth/mfa/webauthn/credentials/",
          },
          async (renameGate) => {
            await renameGate.startAndWaitForInterception(() => renameButton.click());
            await expect(firstPasskey).toBeVisible();
            await expect(firstPasskey.getByTestId("passkey-name")).toHaveText(firstPasskeyName);
            await expect(renameButton).toBeDisabled();
            await expect(renameButton).toContainText("Saving...");
            expect(await renameGate.release()).toBeLessThan(300);
            await expect(page.getByText(`Passkey "${renamedPasskeyName}" renamed`)).toBeVisible({ timeout: 10000 });
            await expect(passkeyRow(page, renamedPasskeyName)).toBeVisible({ timeout: 10000 });
            await expect(firstPasskey).not.toBeVisible({ timeout: 10000 });
          },
        );

        const secondPasskey = passkeyRow(page, secondPasskeyName);
        await deletePasskeyThroughUI(secondPasskey);
        await expect(page.getByText("Passkey deleted")).toBeVisible({ timeout: 10000 });
        await expect(secondPasskey).not.toBeVisible({ timeout: 10000 });
        await expect(passkeyRow(page, renamedPasskeyName)).toBeVisible();
      });

      await test.step("MFA Management: reject deleting the final remaining passkey", async () => {
        const remainingPasskey = passkeyRow(page, renamedPasskeyName);
        await deletePasskeyThroughUI(remainingPasskey);
        await expect(page.getByText("cannot delete final WebAuthn credential")).toBeVisible({ timeout: 10000 });
        await expect(passkeyRow(page, renamedPasskeyName)).toBeVisible();
      });

      await test.step("MFA Management: require passkey-backed AAL2 success indicator", async () => {
        await expect(page.getByTestId("aal-level-indicator")).toContainText(/AAL2/i, { timeout: 5000 });
      });
    } finally {
      await virtualAuthenticator?.remove();
    }
  });
});
