import { test, expect, execSQL } from "../fixtures";

/**
 * FULL E2E TEST: API Keys Lifecycle
 *
 * Tests complete API key management:
 * - Setup: Create a test user via SQL (required for key creation)
 * - Create API key with name, user, scope
 * - Verify key displayed in creation modal
 * - Verify key appears in list
 * - Revoke API key
 */

test.describe("API Keys Lifecycle (Full E2E)", () => {
  const pendingCleanup: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of pendingCleanup) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    pendingCleanup.length = 0;
  });

  test("seeded API key renders in list view", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const keyName = `seed-key-${runId}`;
    const testEmail = `apikey-seed-${runId}@test.com`;

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(
      `DELETE FROM _ayb_api_keys WHERE name = '${keyName}';`,
      `DELETE FROM _ayb_users WHERE email = '${testEmail}';`,
    );

    // Arrange: create user and API key via SQL
    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_users (email, password_hash) VALUES ('${testEmail}', '$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g') ON CONFLICT DO NOTHING;`,
    );
    const userResult = await execSQL(
      request,
      adminToken,
      `SELECT id FROM _ayb_users WHERE email = '${testEmail}';`,
    );
    const userId = userResult.rows[0][0];
    await execSQL(
      request,
      adminToken,
      `INSERT INTO _ayb_api_keys (name, key_hash, key_prefix, user_id) VALUES ('${keyName}', 'seedhash${runId}', 'ayb_seed', '${userId}');`,
    );

    // Act: navigate to API Keys page
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();
    const sidebar = page.locator("aside");
    const apiKeysButton = sidebar.getByRole("button", { name: /^API Keys$/i });
    await apiKeysButton.click();
    await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible({ timeout: 5000 });

    // Assert: seeded key name appears in the list
    await expect(page.getByText(keyName).first()).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });

  test("create, view, and revoke API key", async ({ page, request, adminToken }) => {
    const runId = Date.now();
    const testEmail = `apikey-test-${runId}@test.com`;
    const keyName = `test-key-${runId}`;

    // Register cleanup early so afterEach runs it even on failure
    pendingCleanup.push(
      `DELETE FROM _ayb_api_keys WHERE name = '${keyName}';`,
      `DELETE FROM _ayb_users WHERE email = '${testEmail}';`,
    );

    // ============================================================
    // Setup: Create a test user via SQL
    // ============================================================
    await page.goto("/admin/");
    await expect(page.getByText("Allyourbase").first()).toBeVisible();

    const sidebar = page.locator("aside");
    await sidebar.getByRole("button", { name: /^SQL Editor$/i }).click();

    const sqlInput = page.locator('.cm-content[contenteditable="true"]');
    await expect(sqlInput).toBeVisible({ timeout: 5000 });

    await sqlInput.fill(
      `INSERT INTO _ayb_users (email, password_hash) VALUES ('${testEmail}', '$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$dGVzdGhhc2g') ON CONFLICT DO NOTHING RETURNING id;`
    );
    await page.getByRole("button", { name: /run|execute/i }).click();

    // ============================================================
    // Navigate to API Keys
    // ============================================================
    const apiKeysButton = sidebar.getByRole("button", { name: /^API Keys$/i });
    await expect(apiKeysButton).toBeVisible({ timeout: 5000 });
    await apiKeysButton.click();

    // Verify API Keys view loaded
    await expect(page.getByRole("heading", { name: /API Keys/i })).toBeVisible({ timeout: 5000 });

    // ============================================================
    // CREATE: Add new API key
    // ============================================================
    const createButton = page.getByRole("button", { name: /create key|new key|add/i });
    await expect(createButton.first()).toBeVisible({ timeout: 3000 });
    await createButton.first().click();

    // Fill creation form
    await page.getByLabel("Key name").fill(keyName);

    // User selector
    const userSelect = page.getByLabel("User");
    await expect(userSelect).toBeVisible({ timeout: 2000 });
    const options = userSelect.locator("option");
    const optCount = await options.count();
    expect(optCount).toBeGreaterThan(1);
    await userSelect.selectOption({ index: 1 });

    // Submit
    const submitBtn = page.getByRole("button", { name: /^create$|^save$/i });
    await expect(submitBtn).toBeVisible();
    await submitBtn.click();

    // ============================================================
    // VERIFY: Key created modal shows the key
    // ============================================================
    await expect(page.getByText("API Key Created")).toBeVisible({ timeout: 5000 });

    // Close the created modal by clicking Done
    const doneBtn = page.getByRole("button", { name: /^done$/i });
    await expect(doneBtn).toBeVisible({ timeout: 3000 });
    await doneBtn.click();

    // Wait for modal to fully dismiss
    await expect(page.getByText("API Key Created")).not.toBeVisible({ timeout: 3000 });

    // ============================================================
    // VERIFY: Key appears in list
    // ============================================================
    await expect(page.getByText(keyName).first()).toBeVisible({ timeout: 5000 });

    // Verify Active badge on the key's row
    const activeKeyRow = page.locator("tr").filter({ hasText: keyName });
    await expect(activeKeyRow.getByText("Active")).toBeVisible({ timeout: 2000 });

    // ============================================================
    // REVOKE: Revoke the API key
    // ============================================================
    const keyRow = page.locator("tr").filter({ hasText: keyName }).first();
    const revokeButton = keyRow.getByRole("button", { name: "Revoke key" });

    await expect(revokeButton).toBeVisible({ timeout: 3000 });
    await revokeButton.click();

    // Confirm revocation
    const confirmBtn = page.getByRole("button", { name: /^revoke$|^delete$|^confirm$/i });
    await expect(confirmBtn).toBeVisible({ timeout: 2000 });
    await confirmBtn.click();

    // Verify revoked
    const revokedKeyRow = page.locator("tr").filter({ hasText: keyName });
    await expect(revokedKeyRow.getByText("Revoked")).toBeVisible({ timeout: 5000 });

    // Cleanup handled by afterEach
  });
});
