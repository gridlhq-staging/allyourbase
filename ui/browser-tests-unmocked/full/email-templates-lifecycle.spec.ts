import { cleanupEmailTemplate, seedEmailTemplate, test, expect, waitForDashboard } from "../fixtures";
import type { Page } from "@playwright/test";

async function openEmailTemplatesPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.getByRole("button", { name: /^Email Templates$/i }).click();
  await expect(page.getByRole("heading", { name: "Email Templates" })).toBeVisible({ timeout: 5000 });
}

test.describe("Email Templates Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const templateKey of pendingCleanup) {
      await cleanupEmailTemplate(request, adminToken, templateKey).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded custom template renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const templateKey = `app.seed_email_${runId}`;
    const subjectTemplate = `Seeded subject ${runId}`;
    const htmlTemplate = `<p>Seeded body ${runId} for {{.Invitee}}</p>`;

    pendingCleanup.push(templateKey);

    await seedEmailTemplate(request, adminToken, { key: templateKey, subjectTemplate, htmlTemplate });

    await openEmailTemplatesPage(page);

    const seededListItem = page.getByRole("listitem").filter({ hasText: templateKey }).first();
    await expect(seededListItem).toBeVisible({ timeout: 5000 });

    await seededListItem.getByRole("button").click();
    await expect(page.getByRole("heading", { name: templateKey })).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel("Subject Template")).toHaveValue(subjectTemplate);
  });

  test("customizes system template, previews render, then resets to default", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const customSubject = `Custom reset ${runId} for {{.AppName}}`;
    const customHTML = `<p>Lifecycle ${runId}: <a href=\"{{.ActionURL}}\">reset</a> for {{.AppName}}</p>`;

    pendingCleanup.push("auth.password_reset");

    await cleanupEmailTemplate(request, adminToken, "auth.password_reset");

    await openEmailTemplatesPage(page);

    const passwordResetItem = page
      .getByRole("listitem")
      .filter({ hasText: "auth.password_reset" })
      .first();
    await expect(passwordResetItem).toBeVisible({ timeout: 5000 });
    await passwordResetItem.getByRole("button").click();

    await expect(page.getByRole("heading", { name: "auth.password_reset" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel("Subject Template")).toHaveValue("Reset your password");

    await page.getByLabel("Subject Template").fill(customSubject);
    await page.getByLabel("HTML Template").fill(customHTML);
    await page.getByRole("button", { name: "Save Template" }).click();

    await expect(page.getByText("Saved auth.password_reset")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Reset to Default" })).toBeVisible({ timeout: 5000 });

    await page.getByLabel("Preview Variables (JSON)").fill(`{
  "AppName": "Sigil ${runId}",
  "ActionURL": "https://sigil.example/reset/${runId}"
}`);

    await expect(page.getByText(`Custom reset ${runId} for Sigil ${runId}`)).toBeVisible({ timeout: 10000 });
    await expect(page.getByTestId("email-template-preview-html")).toContainText(
      `https://sigil.example/reset/${runId}`,
      { timeout: 10000 },
    );

    await page.getByRole("button", { name: "Reset to Default" }).click();

    await expect(page.getByText("Reset auth.password_reset to default")).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel("Subject Template")).toHaveValue("Reset your password", {
      timeout: 5000,
    });
    await expect(page.getByRole("button", { name: "Reset to Default" })).not.toBeVisible({ timeout: 5000 });
  });
});
