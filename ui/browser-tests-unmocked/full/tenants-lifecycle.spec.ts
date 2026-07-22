import { randomUUID } from "crypto";
import {
  cleanupTenantsBySlug,
  cleanupUserByEmail,
  ensureUserByEmail,
  test,
  expect,
  execSQL,
  sqlLiteral,
  waitForDashboard,
} from "../fixtures";
import type { APIRequestContext, Page } from "@playwright/test";

async function createOwnerUser(
  request: APIRequestContext,
  adminToken: string,
  email: string,
): Promise<string> {
  const user = await ensureUserByEmail(request, adminToken, email);
  return user.id;
}

async function openTenantsPage(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Tenants$/i }).click();
  await expect(page.getByTestId("tenants-view")).toBeVisible({ timeout: 5000 });
  await expect(page.getByRole("button", { name: "Create Tenant" })).toBeVisible();
}

async function expectTenantRecord(options: {
  request: APIRequestContext;
  adminToken: string;
  slug: string;
  expectedName: string;
  expectedState?: string;
  expectedOwnerUserID?: string | null;
}): Promise<void> {
  const tenantResult = await execSQL(
    options.request,
    options.adminToken,
    `SELECT name, slug, state
     FROM _ayb_tenants
     WHERE slug = '${sqlLiteral(options.slug)}'`,
  );
  expect(tenantResult.rowCount).toBe(1);
  expect(tenantResult.rows[0]?.[0]).toBe(options.expectedName);
  expect(tenantResult.rows[0]?.[1]).toBe(options.slug);
  if (options.expectedState) {
    expect(tenantResult.rows[0]?.[2]).toBe(options.expectedState);
  }
  if (Object.prototype.hasOwnProperty.call(options, "expectedOwnerUserID")) {
    const ownerMembershipResult = await execSQL(
      options.request,
      options.adminToken,
      `SELECT m.user_id::text
       FROM _ayb_tenant_memberships m
       INNER JOIN _ayb_tenants t ON t.id = m.tenant_id
       WHERE t.slug = '${sqlLiteral(options.slug)}'
         AND m.role = 'owner'`,
    );
    expect(ownerMembershipResult.rowCount).toBe(1);
    expect(ownerMembershipResult.rows[0]?.[0]).toBe(options.expectedOwnerUserID);
  }
}

async function setTenantState(options: {
  request: APIRequestContext;
  adminToken: string;
  slug: string;
  state: string;
}): Promise<void> {
  // Stage 1 product gap: tenant activation has no admin API for provisioning -> active.
  await execSQL(
    options.request,
    options.adminToken,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: tenant activation has no admin API for provisioning to active.
    `UPDATE _ayb_tenants
     SET state = '${sqlLiteral(options.state)}'
     WHERE slug = '${sqlLiteral(options.slug)}'`,
  );
}

test.describe("Tenants Lifecycle (Full E2E)", () => {
  const pendingTenantCleanup: string[] = [];
  const pendingUserCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    await cleanupTenantsBySlug(request, adminToken, pendingTenantCleanup).catch(() => {});
    for (const email of pendingUserCleanup) {
      await cleanupUserByEmail(request, adminToken, email).catch(() => {});
    }
    pendingTenantCleanup.length = 0;
    pendingUserCleanup.length = 0;
  });

  test("create, update, suspend/resume, and delete tenant", async ({
    page,
    request,
    adminToken,
  }) => {
    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const ownerEmail = `tenant-owner-${runID}@example.com`;
    const tenantName = `Lifecycle Tenant ${runID}`;
    const tenantSlug = `tenant-lifecycle-${runID}`;
    const updatedTenantName = `Lifecycle Tenant Updated ${runID}`;

    pendingTenantCleanup.push(tenantSlug);
    pendingUserCleanup.push(ownerEmail);

    const ownerUserID = await createOwnerUser(request, adminToken, ownerEmail);

    await openTenantsPage(page);

    await page.getByRole("button", { name: "Create Tenant" }).click();
    await page.getByLabel("Tenant Name").fill(tenantName);
    await page.getByLabel("Slug").fill(tenantSlug);
    const ownerInput = page.getByLabel("Owner User ID");
    await ownerInput.fill(ownerUserID);
    await expect(ownerInput).toHaveValue(ownerUserID);
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
      expectedOwnerUserID: ownerUserID,
    });
    await setTenantState({
      request,
      adminToken,
      slug: tenantSlug,
      state: "active",
    });
    await openTenantsPage(page);
    await page.getByRole("button", { name: new RegExp(tenantSlug, "i") }).click();
    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Suspend", exact: true })).toBeVisible({
      timeout: 5000,
    });

    await page.getByLabel("Tenant Name").fill(updatedTenantName);
    await page.getByRole("button", { name: "Save Info" }).click();
    await expect(page.getByRole("heading", { name: updatedTenantName })).toBeVisible({ timeout: 5000 });

    const updatedTenantResult = await execSQL(
      request,
      adminToken,
      `SELECT name, state FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`,
    );
    expect(updatedTenantResult.rows[0]?.[0]).toBe(updatedTenantName);
    expect(updatedTenantResult.rows[0]?.[1]).toBe("active");

    await page.getByRole("button", { name: "Suspend", exact: true }).click();
    await expect(page.getByRole("button", { name: "Resume", exact: true })).toBeVisible({
      timeout: 5000,
    });

    await page.getByRole("button", { name: "Resume", exact: true }).click();
    await expect(page.getByRole("button", { name: "Suspend", exact: true })).toBeVisible({
      timeout: 5000,
    });

    await page.getByRole("button", { name: "Delete", exact: true }).click();
    const activeDeleteDialog = page.getByRole("dialog", { name: "Delete Tenant" });
    await expect(activeDeleteDialog).toBeVisible();
    await activeDeleteDialog.getByLabel("Tenant slug").fill(tenantSlug);
    await activeDeleteDialog.getByRole("button", { name: "Delete", exact: true }).click();

    const deletedTenantResult = await execSQL(
      request,
      adminToken,
      `SELECT state FROM _ayb_tenants WHERE slug = '${sqlLiteral(tenantSlug)}'`,
    );
    expect(deletedTenantResult.rows[0]?.[0]).toBe("deleting");
  });

  test("create tenant with empty owner user id opens detail view", async ({
    page,
    request,
    adminToken,
  }) => {
    const runID = randomUUID().replace(/-/g, "").slice(0, 10);
    const tenantName = `Ownerless Tenant ${runID}`;
    const tenantSlug = `tenant-ownerless-${runID}`;

    pendingTenantCleanup.push(tenantSlug);

    await openTenantsPage(page);

    await page.getByRole("button", { name: "Create Tenant" }).click();
    await page.getByLabel("Tenant Name").fill(tenantName);
    await page.getByLabel("Slug").fill(tenantSlug);

    const ownerInput = page.getByLabel("Owner User ID");
    await expect(ownerInput).toHaveValue("");
    await page.getByRole("button", { name: "Create", exact: true }).click();

    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible({ timeout: 5000 });
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
      expectedState: "provisioning",
    });

    const tenantInfoSection = page.getByTestId("tenant-info-section");
    await expect(tenantInfoSection.getByText("provisioning")).toBeVisible();
    await expect(page.getByRole("button", { name: "Delete", exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Delete", exact: true }).click();
    let deleteDialog = page.getByRole("dialog", { name: "Delete Tenant" });
    await expect(deleteDialog).toBeVisible();
    await expect(deleteDialog.getByText(tenantSlug)).toBeVisible();
    await expect(deleteDialog.getByRole("button", { name: "Delete", exact: true })).toBeDisabled();
    await deleteDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(deleteDialog).toHaveCount(0);
    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible();
    await expect(tenantInfoSection.getByText("provisioning")).toBeVisible();
    await expect(page.getByRole("button", { name: "Delete", exact: true })).toBeVisible();
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
      expectedState: "provisioning",
    });

    await page.getByRole("button", { name: "Delete", exact: true }).click();
    deleteDialog = page.getByRole("dialog", { name: "Delete Tenant" });
    const deleteConfirmButton = deleteDialog.getByRole("button", { name: "Delete", exact: true });
    const slugInput = deleteDialog.getByLabel("Tenant slug");
    await slugInput.fill("wrong-slug");
    await expect(deleteConfirmButton).toBeDisabled();
    await slugInput.fill(tenantSlug);
    await expect(deleteConfirmButton).toBeEnabled();
    await deleteConfirmButton.click();

    await expect(page.getByRole("dialog", { name: "Delete Tenant" })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: tenantName })).toBeVisible();
    await expect(tenantInfoSection.getByText("deleting")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Delete", exact: true })).toHaveCount(0);
    await expectTenantRecord({
      request,
      adminToken,
      slug: tenantSlug,
      expectedName: tenantName,
      expectedState: "deleting",
    });
  });
});
