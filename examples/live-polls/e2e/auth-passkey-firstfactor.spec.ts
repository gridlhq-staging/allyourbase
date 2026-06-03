import { test, expect } from "@playwright/test";
import {
  attachVirtualAuthenticator,
  enrollPasskeyForCurrentSession,
  observePasskeyFirstFactorRequests,
  openExplicitAuth,
  registerUser,
} from "./helpers";

test("first-factor passkey sign-in through login CTA", async ({ page }) => {
  const authenticator = await attachVirtualAuthenticator(page);
  try {
    const email = await registerUser(page);
    await enrollPasskeyForCurrentSession(page, `passkey-${Date.now()}`);

    await page.getByRole("button", { name: "Sign out" }).click();
    await openExplicitAuth(page);

    const emailInput = page.getByPlaceholder("Email");
    await emailInput.fill(email);
    await expect(emailInput).toHaveValue(email);
    const passwordSignIn = page.getByRole("button", { name: "Sign In", exact: true });
    const passkeySignIn = page.getByRole("button", { name: "Sign in with a passkey" });
    await expect(passkeySignIn).toBeVisible();
    await expect(passkeySignIn).toBeEnabled();

    const authRequests = observePasskeyFirstFactorRequests(page);
    await passkeySignIn.click();

    await expect(passwordSignIn).toBeHidden({ timeout: 15000 });
    await expect(page.getByTestId("user-email")).toHaveText(email);
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

    authRequests.stop();
    authRequests.expectPasskeyOnlySignIn(email);
  } finally {
    await authenticator.remove();
  }
});
