import {
  test,
  expect,
  probeEndpoint,
  cleanupReplicaByName,
  resolveReplicaSeedTarget,
  waitForDashboard,
} from "../fixtures";
import type { Page } from "@playwright/test";

/**
 * FULL E2E TEST: Replicas Lifecycle
 *
 * Two focused console outcomes:
 *  1. Unreachable target — always runs. The console must stay recoverable and show
 *     actionable operator guidance. This never skips.
 *  2. Reachable standby — add succeeds and renders the created row. Skips only when no
 *     standby is configured, or when the server rejects the target for an explicit
 *     connectivity/standby reason before any 201.
 *
 * The API-seeded-row journey is owned by browser-tests-unmocked/smoke/replicas.spec.ts.
 */

/** Matches the screen-owned guidance in ui/src/components/Replicas.tsx for 502/503. */
const UNAVAILABLE_GUIDANCE = /Replica (target is not reachable|lifecycle support is not enabled)/i;

/** Port 1 on loopback refuses immediately, so the server's dial check fails fast. */
const UNREACHABLE_TARGET = { host: "127.0.0.1", port: 1, database: "postgres" };

async function openReplicasScreen(page: Page): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);
  await page.locator("aside").getByRole("button", { name: /^Replicas$/i }).click();
  await expect(page.getByRole("heading", { name: /Replicas/i })).toBeVisible({ timeout: 15_000 });
}

async function fillAddReplicaForm(
  page: Page,
  target: { name: string; host: string; port: number; database: string; sslMode?: string },
): Promise<void> {
  await page.getByRole("button", { name: /Add Replica/i }).click();
  await page.getByLabel("Name").fill(target.name);
  await page.getByLabel("Host").fill(target.host);
  await page.getByLabel("Port").fill(String(target.port));
  await page.getByLabel("Database").fill(target.database);
  if (target.sslMode) {
    await page.getByLabel("SSL Mode").selectOption(target.sslMode);
  }
}

/** Submits the add form and returns the status of the resulting add-replica POST. */
async function submitAddReplica(page: Page): Promise<number> {
  const responsePromise = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/admin/replicas" &&
      response.request().method() === "POST",
    { timeout: 30_000 },
  );
  const addButton = page.getByRole("button", { name: /^Add$/i });
  await expect(addButton).toBeEnabled();
  await addButton.click();
  const response = await responsePromise;
  return response.status();
}

test.describe("Replicas Lifecycle (Full E2E)", () => {
  const replicaNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (replicaNames.length > 0) {
      const name = replicaNames.pop();
      if (!name) continue;
      await cleanupReplicaByName(request, adminToken, name).catch(() => {});
    }
  });

  test("unreachable replica target leaves the add form recoverable with operator guidance", async ({
    page,
    request,
    adminToken,
  }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/replicas");
    expect(
      probeStatus,
      `Replica list probe must succeed; got ${probeStatus}`,
    ).toBeLessThan(400);

    await openReplicasScreen(page);

    const attemptedName = `replica-unreachable-${Date.now()}`;
    await fillAddReplicaForm(page, { name: attemptedName, ...UNREACHABLE_TARGET });
    const addStatus = await submitAddReplica(page);

    expect(
      [502, 503],
      `Unreachable replica target must report 502 or 503; got ${addStatus}`,
    ).toContain(addStatus);

    // The console must explain what the operator should do next.
    await expect(page.getByText(UNAVAILABLE_GUIDANCE)).toBeVisible({ timeout: 10_000 });

    // The add button is disabled exactly while the request is in flight, so an enabled
    // button proves the spinner stopped and the action is retryable.
    await expect(page.getByRole("button", { name: /^Add$/i })).toBeEnabled();

    // Entered values survive the failure so the operator can correct and resubmit.
    await expect(page.getByLabel("Name")).toHaveValue(attemptedName);
    await expect(page.getByLabel("Host")).toHaveValue(UNREACHABLE_TARGET.host);
    await expect(page.getByLabel("Database")).toHaveValue(UNREACHABLE_TARGET.database);

    // No success state leaked through.
    await expect(page.getByText(new RegExp(`Replica ${attemptedName} added`, "i"))).toHaveCount(0);
  });

  test("add replica via UI renders the created row when a standby is configured", async ({
    page,
    request,
    adminToken,
  }) => {
    const seedTarget = resolveReplicaSeedTarget();
    test.skip(
      !seedTarget,
      "Replica add success requires AYB_DATABASE_REPLICA_URLS to point at a reachable standby",
    );

    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/replicas");
    expect(
      probeStatus,
      `Replica list probe must succeed; got ${probeStatus}`,
    ).toBeLessThan(400);

    await openReplicasScreen(page);

    const createdName = `replica-created-${Date.now()}`;
    await fillAddReplicaForm(page, {
      name: createdName,
      host: seedTarget!.host,
      port: seedTarget!.port,
      database: seedTarget!.database,
      sslMode: seedTarget!.sslMode,
    });
    const addStatus = await submitAddReplica(page);

    // A pre-201 connectivity/standby rejection means the configured target is not usable
    // as a standby here. Every other non-201 outcome is a real defect.
    test.skip(
      addStatus === 502 || addStatus === 503,
      `Configured standby ${seedTarget!.host}:${seedTarget!.port} was rejected for connectivity/standby reasons (status ${addStatus})`,
    );
    expect(addStatus, `Replica add must return 201; got ${addStatus}`).toBe(201);
    replicaNames.push(createdName);

    await expect(page.getByText(`Replica ${createdName} added`)).toBeVisible({ timeout: 10_000 });

    // The created row must render; absence after a 201 is a defect, never a skip.
    const createdRow = page
      .locator("tr")
      .filter({ has: page.getByRole("button", { name: `Promote ${createdName}` }) })
      .first();
    await expect(createdRow, `Created replica row missing for ${createdName}`).toBeVisible({
      timeout: 10_000,
    });
    await expect(createdRow).toContainText(seedTarget!.host);

    // Clean up through the console so removal is exercised on the same row.
    await createdRow.getByRole("button", { name: `Remove ${createdName}` }).click();
    await expect(page.getByRole("heading", { name: /Remove Replica/i })).toBeVisible({
      timeout: 5000,
    });
    await page.getByRole("button", { name: /^Remove$/i }).click();
    await expect(page.getByText(/Replica removed/i)).toBeVisible({ timeout: 10_000 });
  });
});
