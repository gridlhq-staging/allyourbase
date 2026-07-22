import { expect, test, waitForDashboard } from "../fixtures";

test.describe("Smoke: Console DX closeout", () => {
  test("SQL failure guidance preserves a working recovery action", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.getByRole("complementary");
    await sidebar.getByRole("button", { name: "SQL Editor", exact: true }).click();

    const sqlInput = page.getByLabel("SQL query");
    const executeButton = page.getByRole("button", { name: "Execute", exact: true });
    await sqlInput.fill("SELECT * FROM definitely_missing_stage3_console_dx_table;");
    await executeButton.click();

    const errorNotice = page.getByRole("alert");
    await expect(errorNotice).toContainText(
      /definitely_missing_stage3_console_dx_table|relation .* does not exist/i,
    );
    await expect(errorNotice.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      "https://allyourbase.io/guide/patterns",
    );
    await expect(executeButton).toBeVisible();
    await expect(executeButton).toBeEnabled();

    await sqlInput.fill("SELECT 'stage3-recovered' AS value;");
    await executeButton.click();

    await expect(page.getByRole("columnheader", { name: "value", exact: true })).toBeVisible();
    await expect(page.getByRole("cell", { name: "stage3-recovered", exact: true })).toBeVisible();
  });

  test("command palette selection renders the registry-owned Functions heading", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.getByRole("complementary").getByRole("button", { name: "Search... K" }).click();
    const commandPalette = page.getByRole("dialog", { name: "Command palette" });
    await commandPalette.getByRole("button", { name: "Functions", exact: true }).click();

    await expect(
      page.locator("main").getByRole("heading", { name: /^Functions \(\d+\)$/ }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
