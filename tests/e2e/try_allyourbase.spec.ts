import { test, expect } from "./try_allyourbase_fixtures";

test("a visitor launches a private Allyourbase instance and receives access", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Try Allyourbase" })).toBeVisible();
  await expect(page.getByText(/private.*30 minutes/i)).toBeVisible();
  await page.getByRole("button", { name: "Launch my private instance" }).click();

  await expect(page.getByRole("status")).toContainText("Starting your private Allyourbase instance");
  await expect(page.getByRole("link", { name: "Open Allyourbase" })).toHaveAttribute(
    "href",
    "https://private.example/admin?token=signed",
  );
  await expect(page.getByLabel("Temporary admin password")).toHaveText("temporary-admin-password");
  await expect(page.getByText(/expires/i)).toBeVisible();
});
