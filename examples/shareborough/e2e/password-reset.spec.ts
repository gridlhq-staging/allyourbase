import { test, expect } from "@playwright/test";
import { randomAddress, waitForEmail, extractLinks, deleteEmail } from "mailpail";
import type { MailpailConfig } from "mailpail";
import { TEST_PASSWORD, registerUser } from "./helpers";

const mailpailConfig: MailpailConfig = {
  domain: process.env.MAILPAIL_DOMAIN ?? "",
  s3Bucket: process.env.MAILPAIL_BUCKET ?? "",
  s3Prefix: process.env.MAILPAIL_PREFIX ?? "mailpail/",
  awsRegion: process.env.MAILPAIL_REGION ?? "us-east-1",
};

/**
 * Password Reset E2E Test
 * Uses mailpail (AWS SES + S3) to test the complete password reset flow
 *
 * Prerequisites:
 * - MAILPAIL_DOMAIN and MAILPAIL_BUCKET environment variables must be set
 * - Backend password reset functionality must be enabled
 * - Email sending must be configured (SMTP or webhook)
 */

test.describe("Password Reset", () => {

  test("Complete password reset flow", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];
    console.log(`Generated test email: ${email}`);

    try {
      // Register account
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      // Sign out
      await page.getByRole("button", { name: /Account menu/i }).click();
      await page.getByText("Sign out").click();
      await page.reload();
      await expect(page).toHaveURL("/login");

      // Click "Forgot password?" link
      await page.goto("/login");
      const forgotLink = page.getByRole("link", { name: /Forgot.*password/i });
      await expect(forgotLink).toBeVisible({ timeout: 5000 });
      await forgotLink.click();

      // Enter email and submit reset request
      await page.getByPlaceholder(/Email/i).fill(email);
      await page.getByRole("button", { name: /Send.*Reset.*Link|Reset.*Password/i }).click();

      // Should show success message (anchored to avoid matching "send failed")
      await expect(
        page.locator("text=/check your email|reset.+link.+sent|instructions.+sent|email.+sent/i")
      ).toBeVisible({ timeout: 10000 });

      // Wait for password reset email
      console.log("Waiting for password reset email...");
      const resetEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "reset",
        timeout: 60000,
      });
      receivedEmails.push(resetEmail.s3Key);

      console.log(`Received reset email: ${resetEmail.subject}`);

      // Extract reset link from email
      const links = extractLinks(resetEmail);
      const resetLink = links.find((link) =>
        link.includes("/reset") || link.includes("password")
      );

      expect(resetLink).toBeTruthy();
      console.log(`Reset link: ${resetLink}`);

      // Visit reset link
      await page.goto(resetLink!);

      // Enter new password
      const newPassword = "NewSecurePassword456!";
      await page.getByPlaceholder(/New.*Password|Password/i).first().fill(newPassword);

      // Some forms have confirmation field
      const confirmField = page.getByPlaceholder(/Confirm.*Password/i);
      if (await confirmField.isVisible({ timeout: 1000 }).catch(() => false)) {
        await confirmField.fill(newPassword);
      }

      await page.getByRole("button", { name: /Reset.*Password|Change.*Password|Update/i }).click();

      // Should show success message and redirect to login (anchored to avoid "update failed" etc.)
      await expect(
        page.locator("text=/password.+updated|password.+changed|password.+reset successfully|reset.+complete/i")
      ).toBeVisible({ timeout: 5000 });

      // Try logging in with new password
      await page.goto("/login");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(newPassword);
      await page.getByRole("button", { name: "Sign In" }).click();

      // Should successfully log in
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });
      await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();

      console.log("Password reset flow completed successfully");
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });

  test("Password reset: old password no longer works", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];

    try {
      // Register
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      // Sign out
      await page.getByRole("button", { name: /Account menu/i }).click();
      await page.getByText("Sign out").click();
      await page.reload();

      // Request password reset
      await page.goto("/login");
      await page.getByRole("link", { name: /Forgot.*password/i }).click();
      await page.getByPlaceholder(/Email/i).fill(email);
      await page.getByRole("button", { name: /Send.*Reset.*Link|Reset.*Password/i }).click();

      // Wait for email and get reset link
      const resetEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "reset",
        timeout: 60000,
      });
      receivedEmails.push(resetEmail.s3Key);

      const links = extractLinks(resetEmail);
      const resetLink = links.find((link) =>
        link.includes("/reset") || link.includes("password")
      );

      // Complete reset with new password
      await page.goto(resetLink!);
      const newPassword = "NewSecurePassword789!";
      await page.getByPlaceholder(/New.*Password|Password/i).first().fill(newPassword);
      const confirmField = page.getByPlaceholder(/Confirm.*Password/i);
      if (await confirmField.isVisible({ timeout: 1000 }).catch(() => false)) {
        await confirmField.fill(newPassword);
      }
      await page.getByRole("button", { name: /Reset.*Password|Change.*Password|Update/i }).click();

      // Try logging in with OLD password (should fail)
      await page.goto("/login");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD); // Old password
      await page.getByRole("button", { name: "Sign In" }).click();

      // Should show error (use role-based selector instead of fragile CSS class)
      await expect(page.locator('[role="alert"], [aria-invalid="true"], [data-testid="login-error"]').first()).toBeVisible({ timeout: 5000 });

      // Now try with NEW password (should succeed)
      await page.getByPlaceholder("Password").fill(newPassword);
      await page.getByRole("button", { name: "Sign In" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      console.log("Old password correctly invalidated after reset");
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });

  test("Password reset: invalid/expired token shows error", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];

    try {
      // Register
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      // Sign out and request reset
      await page.getByRole("button", { name: /Account menu/i }).click();
      await page.getByText("Sign out").click();
      await page.reload();

      await page.goto("/login");
      await page.getByRole("link", { name: /Forgot.*password/i }).click();
      await page.getByPlaceholder(/Email/i).fill(email);
      await page.getByRole("button", { name: /Send.*Reset.*Link|Reset.*Password/i }).click();

      // Wait for email
      const resetEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "reset",
        timeout: 60000,
      });
      receivedEmails.push(resetEmail.s3Key);

      const links = extractLinks(resetEmail);
      const resetLink = links.find((link) =>
        link.includes("/reset") || link.includes("password")
      );

      // Create invalid link by modifying token
      const invalidLink = resetLink!.replace(/token=([^&]+)/, "token=invalid_token_xyz_123");

      // Visit invalid link
      await page.goto(invalidLink);

      // Enter new password
      const newPassword = "ShouldNotWork123!";
      await page.getByPlaceholder(/New.*Password|Password/i).first().fill(newPassword);
      const confirmField = page.getByPlaceholder(/Confirm.*Password/i);
      if (await confirmField.isVisible({ timeout: 1000 }).catch(() => false)) {
        await confirmField.fill(newPassword);
      }
      await page.getByRole("button", { name: /Reset.*Password|Change.*Password|Update/i }).click();

      // Should show error (specific phrases to avoid matching unrelated text)
      await expect(
        page.locator("text=/token.+invalid|token.+expired|invalid.+token|reset.+link.+expired|reset.+failed/i")
      ).toBeVisible({ timeout: 5000 });

      console.log("Invalid reset token correctly rejected");
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });

  test("Password reset: can request multiple times", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];

    try {
      // Register
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      // Sign out
      await page.getByRole("button", { name: /Account menu/i }).click();
      await page.getByText("Sign out").click();
      await page.reload();

      // Request reset #1
      await page.goto("/login");
      await page.getByRole("link", { name: /Forgot.*password/i }).click();
      await page.getByPlaceholder(/Email/i).fill(email);
      await page.getByRole("button", { name: /Send.*Reset.*Link|Reset.*Password/i }).click();
      await expect(page.locator("text=/check your email|reset.+link.+sent|email.+sent/i")).toBeVisible({ timeout: 10000 });

      // Wait for first email
      const firstEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "reset",
        timeout: 60000,
      });
      receivedEmails.push(firstEmail.s3Key);

      // Request reset #2 (immediately after)
      await page.goto("/login");
      await page.getByRole("link", { name: /Forgot.*password/i }).click();
      await page.getByPlaceholder(/Email/i).fill(email);
      await page.getByRole("button", { name: /Send.*Reset.*Link|Reset.*Password/i }).click();
      await expect(page.locator("text=/check your email|reset.+link.+sent|email.+sent/i")).toBeVisible({ timeout: 10000 });

      // Check for rate limit message first; if none, wait for the second email
      const rateLimited = await page.locator("text=/rate.+limit|too many.+requests|please wait/i")
        .isVisible({ timeout: 3000 })
        .catch(() => false);

      if (rateLimited) {
        console.log("Rate limiting applied correctly");
      } else {
        const secondEmail = await waitForEmail(mailpailConfig, {
          to: email,
          subject: "reset",
          timeout: 30000,
          after: firstEmail.receivedAt,
        });
        receivedEmails.push(secondEmail.s3Key);
        expect(secondEmail).toBeTruthy();
        console.log("Second reset email sent successfully");
      }
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });
});
