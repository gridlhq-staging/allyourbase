import { test, expect } from "@playwright/test";

function requiredTrialURL(): string {
  const trialURL = process.env.TRY_ALLYOURBASE_URL;
  if (!trialURL) throw new Error("TRY_ALLYOURBASE_URL is required for the live lifecycle check");
  return trialURL;
}

test("a verification visitor receives a working isolated Allyourbase dashboard", async ({ page }) => {
  await page.goto(requiredTrialURL());

  await expect(page.getByRole("heading", { name: "Try Allyourbase" })).toBeVisible();
  const launchButton = page.getByRole("button", { name: "Launch my private instance" });
  await expect(launchButton).toBeEnabled({ timeout: 45_000 });
  await launchButton.click();

  await expect(page.getByRole("status")).toContainText("Starting your private Allyourbase instance");
  const openLink = page.getByRole("link", { name: "Open Allyourbase" });
  await expect(openLink).toBeVisible({ timeout: 180_000 });
  await expect(page.getByLabel("Temporary admin password")).not.toBeEmpty();
  await expect(page.getByText(/expires/i)).toBeVisible();

  const dashboardPromise = page.waitForEvent("popup");
  await openLink.click();
  const dashboard = await dashboardPromise;
  await expect(dashboard).toHaveTitle(/Allyourbase Admin/, { timeout: 45_000 });
});
