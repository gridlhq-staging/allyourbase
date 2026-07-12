import type { TestInfo } from "@playwright/test";
import {
  cleanupAuthUser,
  cleanupApiKeyByName,
  createApiKeyForUser,
  createLinkedEmailAuthSessionToken,
  ensureAuthSettings,
  execSQL,
  expect,
  fetchAuthSettings,
  resolveAuthUserIdByEmail,
  resolveDefaultTenantIdByUserId,
  seedRecord,
  startSSECapture,
  test,
} from "../fixtures";
import type { SSECaptureHandle } from "../fixtures";

interface CleanupState {
  capture?: SSECaptureHandle;
  apiKeyName?: string;
  email?: string;
  tableName?: string;
  tenantID?: string;
  anonymousAuthEnabled?: boolean;
}

function extractCreateEventShape(
  event: Record<string, unknown> | undefined,
): { action: string | null; table: string | null; name: string | null } | null {
  if (!event) {
    return null;
  }
  const record = event["record"];
  let name: string | null = null;
  if (record && typeof record === "object" && !Array.isArray(record)) {
    const candidate = (record as Record<string, unknown>)["name"];
    if (typeof candidate === "string") {
      name = candidate;
    }
  }
  return {
    action: typeof event["action"] === "string" ? event["action"] : null,
    table: typeof event["table"] === "string" ? event["table"] : null,
    name,
  };
}

test.describe("Dashboard Client Realtime Journey (Full E2E)", () => {
  const cleanupByTestID = new Map<string, CleanupState>();

  test.afterEach(async ({ request, adminToken }, testInfo: TestInfo) => {
    const cleanup = cleanupByTestID.get(testInfo.testId);
    if (!cleanup) {
      return;
    }

    // Keep teardown deterministic: close SSE subscription before dropping data.
    if (cleanup.capture) {
      await cleanup.capture.close().catch(() => {});
    }
    if (cleanup.tableName) {
      await execSQL(
        request,
        adminToken,
        `DROP TABLE IF EXISTS ${cleanup.tableName}`,
        { tenantID: cleanup.tenantID },
      ).catch(() => {});
    }
    if (cleanup.apiKeyName) {
      await cleanupApiKeyByName(request, adminToken, cleanup.apiKeyName).catch(() => {});
    }
    if (cleanup.email) {
      await cleanupAuthUser(request, adminToken, cleanup.email).catch(() => {});
    }
    if (typeof cleanup.anonymousAuthEnabled === "boolean") {
      await ensureAuthSettings(request, adminToken, {
        anonymous_auth_enabled: cleanup.anonymousAuthEnabled,
      }).catch(() => {});
    }
    cleanupByTestID.delete(testInfo.testId);
  });

  test("tenant SQL table + client insert emits realtime create event", async (
    { page, request, adminToken, baseURL },
    testInfo: TestInfo,
  ) => {
    if (!baseURL) {
      throw new Error("PLAYWRIGHT_BASE_URL is required for realtime browser assertions");
    }
    const runID = `${Date.now()}_${testInfo.parallelIndex}_${testInfo.repeatEachIndex}_${testInfo.retry}`;
    const tableName = `dashboard_rt_${runID}`;
    const email = `dashboard-rt-${runID}@example.com`;
    const password = `TestPass!${runID}`;
    const apiKeyName = `dashboard-rt-key-${runID}`;

    const originalAuthSettings = await fetchAuthSettings(request, adminToken);
    cleanupByTestID.set(testInfo.testId, {
      apiKeyName,
      email,
      tableName,
      anonymousAuthEnabled: originalAuthSettings.anonymous_auth_enabled,
    });

    // Linked email signup depends on the anonymous session bootstrap route.
    await ensureAuthSettings(request, adminToken, {
      anonymous_auth_enabled: true,
    });

    const clientToken = await createLinkedEmailAuthSessionToken(request, email, password);
    const userID = await resolveAuthUserIdByEmail(request, adminToken, email);
    const tenantID = await resolveDefaultTenantIdByUserId(request, adminToken, userID);
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (id serial PRIMARY KEY, name text NOT NULL, user_id uuid);
       GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE ${tableName} TO ayb_authenticated;
       GRANT USAGE, SELECT ON SEQUENCE ${tableName}_id_seq TO ayb_authenticated`,
      { tenantID },
    );
    cleanupByTestID.set(testInfo.testId, {
      ...cleanupByTestID.get(testInfo.testId),
      tenantID,
    });

    const { key: writeToken } = await createApiKeyForUser(
      request,
      adminToken,
      userID,
      apiKeyName,
      "readwrite",
      [tableName],
    );

    const capture = await startSSECapture(page, baseURL, clientToken, [tableName]);
    const existingCleanup = cleanupByTestID.get(testInfo.testId);
    cleanupByTestID.set(testInfo.testId, { ...existingCleanup, capture });

    await seedRecord(request, writeToken, tableName, { name: "hello", user_id: userID });

    await expect
      .poll(
        async () => {
          const events = await capture.getEvents();
          return extractCreateEventShape(events[0]);
        },
        { timeout: 10000 },
      )
      .toEqual({
        action: "create",
        table: tableName,
        name: "hello",
      });
  });
});
