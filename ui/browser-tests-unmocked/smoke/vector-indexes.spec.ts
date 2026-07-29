import {
  test,
  expect,
  probeEndpoint,
  execSQL,
  failIfReadinessForced,
  fetchAdminJSON,
  readinessNotMet,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Vector Indexes
 *
 * Critical Path: Navigate to Vector Indexes → Verify page heading, table structure
 * with column headers, and action controls render in the page body.
 */

test.describe("Smoke: Vector Indexes", () => {
  const seededVectorTables: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededVectorTables.length > 0) {
      const tableName = seededVectorTables.pop();
      if (!tableName) continue;
      await execSQL(
        request,
        adminToken,
        `DROP TABLE IF EXISTS public.${tableName} CASCADE`,
      ).catch(() => {});
    }
  });

  test("seeded vector index renders in vector indexes table", async ({ page, request, adminToken }, testInfo) => {
    await failIfReadinessForced(testInfo, "vector-indexes");

    const status = await probeEndpoint(request, adminToken, "/api/admin/vector/indexes");
    if (status === 501 || status === 404) {
      await readinessNotMet(
        testInfo,
        "vector-indexes",
        `vector indexes endpoint returned status ${status}`,
      );
    }

    const vectorExtensionAvailability = await execSQL(
      request,
      adminToken,
      `SELECT EXISTS (
         SELECT 1
         FROM pg_available_extensions
         WHERE name = 'vector'
       )`,
    );
    const isVectorExtensionAvailable = vectorExtensionAvailability.rows[0]?.[0] === true;
    if (!isVectorExtensionAvailable) {
      await readinessNotMet(
        testInfo,
        "vector-indexes",
        "vector extension is unavailable in Postgres",
      );
    }

    const runId = Date.now();
    const tableName = `smoke_vectors_${runId}`;
    const indexName = `smoke_vectors_idx_${runId}`;
    await execSQL(
      request,
      adminToken,
      `CREATE EXTENSION IF NOT EXISTS vector`,
    );
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE public.${tableName} (
         id bigserial PRIMARY KEY,
         embedding vector(3)
       )`,
    );
    await execSQL(
      request,
      adminToken,
      `CREATE INDEX ${indexName}
       ON public.${tableName}
       USING hnsw (embedding vector_l2_ops)`,
    );
    seededVectorTables.push(tableName);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Vector Indexes/i }).click();
    await expect(page.getByRole("heading", { name: /Vector Indexes/i })).toBeVisible({ timeout: 15_000 });

    const seededIndexRow = page.locator("tr").filter({ hasText: indexName }).first();
    await expect(seededIndexRow).toBeVisible({ timeout: 5000 });
    await expect(seededIndexRow.getByText("public")).toBeVisible();
    await expect(seededIndexRow.getByText(tableName)).toBeVisible();
    await expect(seededIndexRow.getByText(/hnsw/i)).toBeVisible();
  });

  test("empty state recovers after the admin API becomes reachable", async ({
    page,
    request,
    adminToken,
    context,
  }, testInfo) => {
    await failIfReadinessForced(testInfo, "vector-indexes");

    const status = await probeEndpoint(request, adminToken, "/api/admin/vector/indexes");
    if (status === 501 || status === 404) {
      await readinessNotMet(
        testInfo,
        "vector-indexes",
        `vector indexes endpoint returned status ${status}`,
      );
    }
    expect(await fetchAdminJSON(request, adminToken, "/api/admin/vector/indexes")).toEqual({
      indexes: [],
    });

    await page.goto("/admin/");
    await waitForDashboard(page);
    await page.locator("aside").getByRole("button", { name: /API Explorer/i }).click();
    await expect(page.getByRole("heading", { name: /API Explorer/i })).toBeVisible();
    await page.locator("aside").getByRole("button", { name: /Vector Indexes/i }).click();
    await expect(page.getByText("No vector indexes found", { exact: true })).toBeVisible();

    try {
      // Closest-real proxy: the schema-cache list handler intentionally returns
      // 200 with an empty collection when its cache is absent, so browser
      // offline mode makes the live API unreachable. Bias: broader than one
      // endpoint. Tolerance: only this screen's exact error and recovery count.
      await context.setOffline(true);
      await page.locator("aside").getByRole("button", { name: /API Explorer/i }).click();
      await page.locator("aside").getByRole("button", { name: /Vector Indexes/i }).click();
      await expect(page.getByText("Failed to fetch", { exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "Retry", exact: true })).toBeVisible();

      await context.setOffline(false);
      await page.getByRole("button", { name: "Retry", exact: true }).click();
      await expect(page.getByText("No vector indexes found", { exact: true })).toBeVisible();
    } finally {
      await context.setOffline(false);
    }
  });
});
