import {
  test,
  expect,
  cleanupOrganizationDashboardSmokeOrg,
  replaceAdminRelationWithEmptyClone,
  seedOrganizationDashboardSmokeOrg,
  waitForDashboard,
} from "../fixtures";

test.describe("Smoke: Organizations Dashboard", () => {
  test("admin can open organizations and view a uniquely seeded organization", async ({
    page,
    request,
    adminToken,
  }) => {
    const seededOrg = await seedOrganizationDashboardSmokeOrg(
      request,
      adminToken,
      Date.now().toString(),
    );

    try {
      await page.goto("/admin/screens/organizations");
      await waitForDashboard(page);
      await expect(
        page.getByTestId("org-list-panel").getByText(seededOrg.orgName, { exact: true }),
      ).toBeVisible();
    } finally {
      await cleanupOrganizationDashboardSmokeOrg(request, adminToken, seededOrg.orgId);
    }
  });

  test("empty and unavailable organization storage recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const seededOrg = await seedOrganizationDashboardSmokeOrg(
      request,
      adminToken,
      Date.now().toString(),
    );
    const relationState = await replaceAdminRelationWithEmptyClone(
      request,
      adminToken,
      "_ayb_organizations",
    );

    try {
      await page.goto("/admin/screens/organizations");
      await waitForDashboard(page);
      await expect(page.getByText("No organizations found", { exact: true })).toBeVisible();

      await relationState.removeEmptyClone();
      await page.reload();
      await waitForDashboard(page);
      const errorNotice = page.getByRole("alert");
      await expect(
        errorNotice.getByText(
          "Failed to load organizations: failed to list orgs",
          { exact: true },
        ),
      ).toBeVisible();

      await relationState.restore();
      await errorNotice.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(
        page.getByTestId("org-list-panel").getByText(seededOrg.orgName, { exact: true }),
      ).toBeVisible();
    } finally {
      await relationState.restore();
      await cleanupOrganizationDashboardSmokeOrg(request, adminToken, seededOrg.orgId);
    }
  });
});
