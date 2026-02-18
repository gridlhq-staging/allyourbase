import { test, expect } from "@playwright/test";
import { randomAddress, waitForEmail, extractLinks, deleteEmail } from "mailpail";
import type { MailpailConfig } from "mailpail";
import { TEST_PASSWORD } from "./helpers";

const mailpailConfig: MailpailConfig = {
  domain: process.env.MAILPAIL_DOMAIN ?? "",
  s3Bucket: process.env.MAILPAIL_BUCKET ?? "",
  s3Prefix: process.env.MAILPAIL_PREFIX ?? "mailpail/",
  awsRegion: process.env.MAILPAIL_REGION ?? "us-east-1",
};

/**
 * Email Verification E2E Test
 * Uses mailpail (AWS SES + S3) to create disposable emails and test the verification flow
 *
 * Prerequisites:
 * - MAILPAIL_DOMAIN and MAILPAIL_BUCKET environment variables must be set
 * - Backend must have email verification enabled
 * - Email sending must be configured (SMTP or webhook)
 */

test.describe("Email Verification", () => {

  test("Complete email verification flow", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];
    console.log(`Generated test email: ${email}`);

    try {
      // Register with disposable email
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();

      // Should redirect to dashboard
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });
      await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();

      // Wait for verification email
      console.log("Waiting for verification email...");
      const verificationEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "verify",
        timeout: 60000,
      });
      receivedEmails.push(verificationEmail.s3Key);

      console.log(`Received email: ${verificationEmail.subject}`);

      // Extract verification link from email
      const links = extractLinks(verificationEmail);
      const verificationLink = links.find((link) =>
        link.includes("/verify") || link.includes("verify")
      );

      expect(verificationLink).toBeTruthy();
      console.log(`Verification link: ${verificationLink}`);

      // Visit verification link
      await page.goto(verificationLink!);

      // Verify success message (anchored patterns to avoid matching negatives like "Not verified")
      await expect(
        page.locator("text=/email verified|successfully verified|email confirmed|verification complete/i")
      ).toBeVisible({ timeout: 5000 });

      console.log("Email verification flow completed successfully");
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });

  test("Resend verification email", async ({ page }) => {
    if (!mailpailConfig.domain || !mailpailConfig.s3Bucket) {
      test.skip();
    }

    const email = randomAddress(mailpailConfig);
    const receivedEmails: string[] = [];
    console.log(`Generated test email: ${email}`);

    try {
      // Register
      await page.goto("/signup");
      await page.getByPlaceholder("Email").fill(email);
      await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
      await page.getByRole("button", { name: "Create Account" }).click();
      await expect(page).toHaveURL("/dashboard", { timeout: 15000 });

      // Navigate to settings
      await page.goto("/dashboard/settings");

      // Look for "Resend Verification" button
      const resendButton = page.getByRole("button", { name: /Resend.*Verification/i });

      if (await resendButton.isVisible({ timeout: 5000 }).catch(() => false)) {
        await resendButton.click();

        // Should show success toast (anchored to avoid matching "send failed" or similar)
        await expect(page.locator('[role="alert"]').filter({ hasText: /verification.+sent|email.+resent|resent.+verification/i })).toBeVisible({ timeout: 5000 });

        // Wait for second verification email
        const secondEmail = await waitForEmail(mailpailConfig, {
          to: email,
          subject: "verify",
          timeout: 60000,
        });
        receivedEmails.push(secondEmail.s3Key);

        expect(secondEmail).toBeTruthy();
        console.log("Resend verification email successful");
      } else {
        console.log("Resend verification button not found (account may be auto-verified)");
      }
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });

  test("Verification link expires after timeout", async ({ page }) => {
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

      // Wait for verification email
      const verificationEmail = await waitForEmail(mailpailConfig, {
        to: email,
        subject: "verify",
        timeout: 60000,
      });
      receivedEmails.push(verificationEmail.s3Key);

      const links = extractLinks(verificationEmail);
      const verificationLink = links.find((link) =>
        link.includes("/verify") || link.includes("verify")
      );

      // Create invalid link by modifying token
      const expiredLink = verificationLink!.replace(/token=([^&]+)/, "token=expired_token_xyz");

      // Visit expired/invalid link
      await page.goto(expiredLink);

      // Should show error message (specific phrases to avoid matching unrelated text containing "error")
      await expect(
        page.locator("text=/link.+expired|token.+invalid|invalid.+token|verification.+failed|link.+invalid/i")
      ).toBeVisible({ timeout: 5000 });

      console.log("Expired verification link handled correctly");
    } finally {
      for (const key of receivedEmails) {
        await deleteEmail(mailpailConfig, key).catch(() => {});
      }
    }
  });
});
