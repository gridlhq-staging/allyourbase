import {
  test,
  expect,
  seedFile,
  deleteFile,
  deleteStorageBucket,
  ensureStorageBucket,
  expectOfflineRetryRecovery,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Storage - Upload, Download, Delete
 *
 * Critical Path: Navigate to Storage → Upload file → Verify in list → Delete
 */

function untrackSeededFile(seededFileNames: string[], fileName: string): void {
  const trackedIndex = seededFileNames.indexOf(fileName);
  if (trackedIndex >= 0) {
    seededFileNames.splice(trackedIndex, 1);
  }
}

function storageUnavailableMessage(err: unknown): string | null {
  const msg = err instanceof Error ? err.message : String(err);
  if (msg.includes("status 415")) {
    return msg;
  }
  return null;
}

test.describe("Smoke: Storage", () => {
  const seededFileNames: string[] = [];
  const seededBuckets: string[] = [];

  test("only unsupported media type skips the storage journey", () => {
    expect(storageUnavailableMessage(new Error("request failed with status 415"))).toBe(
      "request failed with status 415",
    );
    for (const status of [404, 501, 503]) {
      expect(storageUnavailableMessage(new Error(`request failed with status ${status}`))).toBeNull();
    }
  });

  test.afterEach(async ({ request, adminToken }) => {
    while (seededFileNames.length > 0) {
      const fileName = seededFileNames.pop();
      if (!fileName) continue;
      await deleteFile(request, adminToken, "default", fileName).catch(
        () => {},
      );
    }
    while (seededBuckets.length > 0) {
      const bucketName = seededBuckets.pop();
      if (!bucketName) continue;
      await deleteStorageBucket(request, adminToken, bucketName).catch(() => {});
    }
  });

  test("seeded file renders in storage list", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const fileName = `seed-verify-${runId}.txt`;
    seededFileNames.push(fileName);

    // Arrange: seed a file via API
    try {
      await ensureStorageBucket(request, adminToken, "default");
      await seedFile(
        request,
        adminToken,
        "default",
        fileName,
        "seed verify content",
      );
    } catch (err) {
      const msg = storageUnavailableMessage(err);
      if (msg) {
        test.skip(true, `Storage upload not available: ${msg}`);
        return;
      }
      throw err;
    }

    // Act: navigate to Storage page
    await page.goto("/admin/");
    await waitForDashboard(page);
    const storageButton = page
      .getByRole("complementary")
      .getByRole("button", { name: /^Storage$/i });
    await storageButton.click();
    await expect(
      page.getByRole("button", { name: "Upload", exact: true }),
    ).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel("Bucket name")).toHaveValue("default");
    await expect(page.getByText(/\d+ files?/).first()).toBeVisible();
    await expect(page.getByRole("button", { name: "CDN Purge" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Type" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Size" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Created" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Actions" })).toBeVisible();

    // Assert: seeded file name appears in the list
    const seededRow = page.locator("tr").filter({ hasText: fileName }).first();
    await expect(seededRow).toBeVisible({ timeout: 5000 });
    await expect(seededRow.getByText("text/plain")).toBeVisible();
    await expect(seededRow.getByRole("button", { name: "Copy name" })).toBeVisible();
    await expect(seededRow.getByRole("link", { name: "Download" })).toBeVisible();
    await expect(seededRow.getByRole("button", { name: "Copy signed URL" })).toBeVisible();
    await expect(seededRow.getByRole("button", { name: "Copy download URL" })).toBeVisible();
    await expect(seededRow.getByRole("button", { name: "Delete" })).toBeVisible();
  });

  test("empty bucket and list retry recover through the existing storage loader", async ({
    page,
    request,
    adminToken,
    context,
  }) => {
    const runId = Date.now();
    const bucketName = `smoke-empty-${runId}`;
    const retryBucketName = `smoke-retry-${runId}`;
    await ensureStorageBucket(request, adminToken, bucketName);
    await ensureStorageBucket(request, adminToken, retryBucketName);
    seededBuckets.push(bucketName, retryBucketName);

    await page.goto("/admin/");
    await waitForDashboard(page);
    const storageButton = page
      .getByRole("complementary")
      .getByRole("button", { name: /^Storage$/i });
    await storageButton.click();

    const bucketInput = page.getByLabel("Bucket name");
    await bucketInput.fill(bucketName);
    await expect(page.getByText(`No files in "${bucketName}"`, { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();

    // Closest-real proxy: storage list failures require the backing service to
    // be unreachable; browser offline mode exercises the same fetch rejection.
    await expectOfflineRetryRecovery(
      page,
      context,
      async () => {
        await bucketInput.fill(retryBucketName);
      },
      async () => {
        await expect(page.getByText(`No files in "${retryBucketName}"`, { exact: true })).toBeVisible();
        await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();
      },
    );
  });

  test("upload file and delete via storage UI", async ({ page, request, adminToken }) => {
    // Skip if storage uploads aren't available (415 Unsupported Media Type).
    try {
      await ensureStorageBucket(request, adminToken, "default");
      const probeFile = await seedFile(request, adminToken, "default", `probe-${Date.now()}.txt`, "probe");
      await deleteFile(request, adminToken, "default", probeFile.name).catch(() => {});
    } catch (err) {
      const msg = storageUnavailableMessage(err);
      if (msg) {
        test.skip(true, `Storage upload not available: ${msg}`);
        return;
      }
      throw err;
    }

    const runId = Date.now();

    // Step 1: Navigate to admin dashboard
    await page.goto("/admin/");
    await waitForDashboard(page);

    // Step 2: Navigate to Storage section
    const storageButton = page
      .getByRole("complementary")
      .getByRole("button", { name: /^Storage$/i });
    await expect(storageButton).toBeVisible({ timeout: 5000 });
    await storageButton.click();

    // Step 3: Wait for storage view to load
    const uploadButton = page.getByRole("button", {
      name: "Upload",
      exact: true,
    });
    await expect(uploadButton).toBeVisible({ timeout: 5000 });

    // Step 4: Upload a file through the visible upload control
    const fileName = `smoke-test-${runId}.txt`;
    seededFileNames.push(fileName);

    // Wait for any upload processing by listening for network response
    const uploadPromise = page.waitForResponse(
      (resp) =>
        resp.url().includes("/api/storage/") &&
        resp.request().method() === "POST",
      { timeout: 15000 },
    );

    const chooserPromise = page.waitForEvent("filechooser");
    await uploadButton.click();
    const fileChooser = await chooserPromise;
    await fileChooser.setFiles({
      name: fileName,
      mimeType: "text/plain",
      buffer: Buffer.from("This is a smoke test file for upload and delete"),
    });

    // Wait for upload to complete
    await uploadPromise;

    // Step 5: Verify file appears in the list
    const fileRow = page.locator("tr").filter({ hasText: fileName }).first();
    await expect(fileRow).toBeVisible({ timeout: 10000 });
    await expect(fileRow.getByText("text/plain")).toBeVisible();
    await expect(fileRow.getByRole("button", { name: "Copy name" })).toBeVisible();
    await expect(fileRow.getByRole("link", { name: "Download" })).toBeVisible();
    await expect(fileRow.getByRole("button", { name: "Copy signed URL" })).toBeVisible();
    await expect(fileRow.getByRole("button", { name: "Copy download URL" })).toBeVisible();

    // Step 7: Delete the file
    await fileRow.getByRole("button", { name: "Delete" }).click();

    // Step 8: Confirm deletion
    await expect(page.getByText("Are you sure")).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(fileName, { exact: true })).toHaveCount(2);
    await page
      .getByRole("button", { name: "Delete", exact: true })
      .last()
      .click();

    // Step 9: Verify file removed from table (scope to row to avoid toast/dialog text)
    await expect(
      page.locator("tr").filter({ hasText: fileName }),
    ).not.toBeVisible({ timeout: 5000 });
    untrackSeededFile(seededFileNames, fileName);
  });
});
