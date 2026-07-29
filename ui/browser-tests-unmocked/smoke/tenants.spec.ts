import {
  test,
  expect,
  cleanupTenantDashboardSmokeTenant,
  replaceAdminRelationWithEmptyClone,
  seedTenantDashboardSmokeTenant,
  waitForDashboard,
} from "../fixtures";

test.describe("Smoke: Tenants Dashboard", () => {
  test("admin can open tenants and view a uniquely seeded tenant", async ({
    page,
    request,
    adminToken,
  }) => {
    const seededTenant = await seedTenantDashboardSmokeTenant(
      request,
      adminToken,
      Date.now().toString(),
    );

    try {
      await page.goto("/admin/screens/tenants");
      await waitForDashboard(page);
      await expect(
        page.getByTestId("tenant-list-panel").getByText(seededTenant.tenantName, { exact: true }),
      ).toBeVisible();
    } finally {
      await cleanupTenantDashboardSmokeTenant(request, adminToken, seededTenant.tenantId);
    }
  });

  test("empty and unavailable tenant storage recover through Retry", async ({
    page,
    request,
    adminToken,
  }) => {
    const seededTenant = await seedTenantDashboardSmokeTenant(
      request,
      adminToken,
      Date.now().toString(),
    );
    const relationState = await replaceAdminRelationWithEmptyClone(
      request,
      adminToken,
      "_ayb_tenants",
    );

    try {
      await page.goto("/admin/screens/tenants");
      await waitForDashboard(page);
      await expect(page.getByText("No tenants found", { exact: true })).toBeVisible();

      await relationState.removeEmptyClone();
      await page.reload();
      await waitForDashboard(page);
      const errorNotice = page.getByRole("alert");
      await expect(
        errorNotice.getByText("Failed to load tenants: failed to list tenants", { exact: true }),
      ).toBeVisible();

      await relationState.restore();
      await errorNotice.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(
        page.getByTestId("tenant-list-panel").getByText(seededTenant.tenantName, { exact: true }),
      ).toBeVisible();
    } finally {
      await relationState.restore();
      await cleanupTenantDashboardSmokeTenant(request, adminToken, seededTenant.tenantId);
    }
  });
});
