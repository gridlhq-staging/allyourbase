import {
  test,
  expect,
  probeEndpoint,
  seedReplica,
  cleanupReplicaByName,
  failIfReadinessForced,
  fetchReplicaStatuses,
  readinessNotMet,
  resolveReplicaSeedTarget,
  waitForDashboard,
} from "../fixtures";

/**
 * SMOKE TEST: Replicas
 *
 * Critical Path: Seed a replica → Navigate to Replicas → Verify the seeded
 * replica renders in the table body with URL and state.
 */

test.describe("Smoke: Replicas", () => {
  const seededReplicaNames: string[] = [];
  const disabledCapabilities = new Set(
    (process.env.AYB_BROWSER_DISABLED_CAPABILITIES ?? "")
      .split(",")
      .map((capability) => capability.trim())
      .filter(Boolean),
  );

  test.afterEach(async ({ request, adminToken }) => {
    while (seededReplicaNames.length > 0) {
      const name = seededReplicaNames.pop();
      if (!name) continue;
      await cleanupReplicaByName(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded replica renders in the replicas table", async ({ page, request, adminToken }, testInfo) => {
    await failIfReadinessForced(testInfo, "replicas");

    if (disabledCapabilities.has("replicas")) {
      console.log("CAPABILITY_DISABLED:replicas");
      return;
    }

    const status = await probeEndpoint(request, adminToken, "/api/admin/replicas");
    if (status === 501 || status === 404) {
      await readinessNotMet(testInfo, "replicas", `replicas endpoint returned status ${status}`);
    }

    const seedTarget = resolveReplicaSeedTarget();
    if (!seedTarget) {
      await readinessNotMet(
        testInfo,
        "replicas",
        "AYB_DATABASE_REPLICA_URLS does not identify a reachable standby",
      );
    }

    const runId = Date.now();
    const replicaName = `smoke-replica-${runId}`;
    const baseline = await fetchReplicaStatuses(request, adminToken);
    try {
      await seedReplica(request, adminToken, {
        name: replicaName,
        host: seedTarget.host,
        database: seedTarget.database,
        port: seedTarget.port,
        ssl_mode: seedTarget.sslMode,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      if (
        /(status 503|replica lifecycle not available|dial connectivity pool|connectivity check failed|target is not a replica|target is not a standby replica)/i.test(
          message,
        )
      ) {
        await readinessNotMet(testInfo, "replicas", `replica seeding unavailable: ${message}`);
      }
      throw error;
    }
    seededReplicaNames.push(replicaName);
    const updated = await fetchReplicaStatuses(request, adminToken);
    expect(updated.replicas).toHaveLength(baseline.replicas.length + 1);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Replicas/i }).click();
    await expect(page.getByRole("heading", { name: /Replicas/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers
    await expect(page.getByRole("columnheader", { name: /URL/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /State/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Lag/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Connections/i })).toBeVisible();

    // Use the seeded replica name to identify the exact row when multiple replicas share a host.
    const replicaRow = page
      .locator("tr")
      .filter({
        has: page.getByRole("button", { name: `Promote ${replicaName}` }),
      })
      .first();
    await expect(replicaRow).toBeVisible({ timeout: 5000 });
    await expect(replicaRow).toContainText(seedTarget.host);
    await expect(replicaRow).toContainText("healthy");
  });
});
