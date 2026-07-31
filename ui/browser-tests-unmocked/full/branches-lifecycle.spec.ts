import {
  test,
  expect,
  probeEndpoint,
  seedBranch,
  cleanupBranch,
  observeMatchingRequests,
  blockMatchingRequest,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Branches Lifecycle
 *
 * Critical Path: Seed branch via API → verify in list → create via UI → delete via UI
 *
 * Skips are only permitted before the UI create, and only for a proven branch-service
 * precondition: a 503 service probe, or a seed error that specifically identifies a
 * missing pg_dump dependency. A missing row, a rendered `Failed` status, a 404/501/500,
 * or any failure after the UI create response is a real defect and must fail the test.
 */

/** Matches only a seed failure caused by the absent pg_dump binary, not generic 5xx noise. */
const MISSING_PG_DUMP = /pg_dump\b[^\n]*\b(not found|no such file|missing)/i;

test.describe("Branches Lifecycle (Full E2E)", () => {
  const branchNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (branchNames.length > 0) {
      const name = branchNames.pop();
      if (!name) continue;
      await cleanupBranch(request, adminToken, name).catch(() => {});
    }
  });

  test("seed branch, verify in list, create via UI, and delete via UI", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/branches/");
    test.skip(probeStatus === 503, `Branch service unavailable (status ${probeStatus})`);
    expect(
      probeStatus,
      `Branch list probe must succeed; got ${probeStatus}`,
    ).toBeLessThan(400);

    const runId = Date.now();
    const seededName = `branch-full-seeded-${runId}`;
    const createdName = `branch-full-created-${runId}`;

    try {
      await seedBranch(request, adminToken, seededName);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      test.skip(
        MISSING_PG_DUMP.test(message),
        `Branch seed requires pg_dump, which is absent in this environment (${message})`,
      );
      throw err;
    }
    branchNames.push(seededName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Branches$/i }).click();
    await expect(page.getByRole("heading", { name: /Branches/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded branch. A missing row or a Failed status is a defect, not an environment gap.
    const seededRow = page.getByRole("row", { name: new RegExp(seededName) }).first();
    await expect(seededRow, `Seeded branch row missing for ${seededName}`).toBeVisible({ timeout: 15000 });
    await expect(
      seededRow.getByText(/^Failed$/i),
      `Seeded branch ${seededName} reported Failed status`,
    ).toHaveCount(0);

    // Create new branch via UI. Past this point every outcome is a hard assertion.
    await page.getByRole("button", { name: /Add Branch/i }).click();
    await expect(page.getByRole("heading", { name: /Create Branch/i })).toBeVisible({ timeout: 5000 });

    await page.getByPlaceholder(/Branch name/i).fill(createdName);
    branchNames.push(createdName);
    const createButton = page.getByRole("button", { name: /^Creat(?:e|ing…)$/i });
    await blockMatchingRequest(
      page,
      {
        method: "POST",
        urlIncludes: "/api/admin/branches",
      },
      async (createGate) => {
        await createGate.startAndWaitForInterception(() => createButton.click());
        await expect(page.getByRole("heading", { name: /Create Branch/i })).toBeVisible();
        await expect(page.getByPlaceholder(/Branch name/i)).toHaveValue(createdName);
        await expect(createButton).toBeDisabled();
        await expect(createButton).toHaveText("Creating…");
        expect(await createGate.release()).toBeLessThan(300);
        await expect(
          page.getByText(new RegExp(`Branch "${createdName}" created`, "i")),
          `UI create did not report success for ${createdName}`,
        ).toBeVisible({ timeout: 15000 });
      },
    );

    const createdRow = page.getByRole("row", { name: new RegExp(createdName) }).first();
    await expect(createdRow, `Created branch row missing for ${createdName}`).toBeVisible({ timeout: 15000 });
    await expect(
      createdRow.getByText(/^Failed$/i),
      `Created branch ${createdName} reported Failed status`,
    ).toHaveCount(0);

    const deleteRequests = observeMatchingRequests(page, {
      method: "DELETE",
      urlIncludes: `/api/admin/branches/${createdName}`,
    });

    // Cancel the delete confirmation first; this must be a true no-op.
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    const deleteHeading = page.getByRole("heading", { name: /Delete Branch/i });
    await expect(deleteHeading).toBeVisible({ timeout: 5000 });
    await expect(
      page.getByText(`Are you sure you want to delete ${createdName}? This action cannot be undone.`),
    ).toBeVisible();
    await page.getByRole("button", { name: /^Cancel$/i }).click();
    await expect(deleteHeading).toHaveCount(0, { timeout: 5000 });
    await expect(createdRow, `Cancel removed branch row for ${createdName}`).toBeVisible();
    expect(deleteRequests.count()).toBe(0);

    // Delete the created branch via UI after the no-op cancel contract is proven.
    await createdRow.getByRole("button", { name: /Delete/i }).click();
    await expect(deleteHeading).toBeVisible({ timeout: 5000 });
    const deleteResponsePromise = page.waitForResponse(
      (response) =>
        response.url().includes(`/api/admin/branches/${createdName}`) &&
        response.request().method() === "DELETE",
    );
    await page.getByRole("button", { name: /^Confirm$/i }).click();

    const deleteResponse = await deleteResponsePromise;
    expect(deleteResponse.status()).toBeLessThan(300);
    expect(deleteRequests.count()).toBe(1);
    deleteRequests.dispose();
    await expect(page.getByText(new RegExp(`Branch "${createdName}" deleted`, "i"))).toBeVisible({ timeout: 10000 });
  });
});
