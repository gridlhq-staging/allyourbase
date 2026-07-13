/** @module Browser-test fixtures for push notification device tokens, deliveries, and cleanup. */
import type { APIRequestContext } from "@playwright/test";
import { escapeLikePattern, execSQL, sqlLiteral, validateResponse } from "./core";
import { ensureLinkedEmailAuthUser } from "./auth";
import { cleanupAdminAppByName, seedAdminApp } from "./admin";

const PUSH_TEST_USER_ID = "00000000-0000-0000-0000-000000000098";
const PUSH_TEST_APP_ID = "00000000-0000-0000-0000-000000000098";
const PUSH_TEST_USER_EMAIL = "push-fixture-test@example.com";
const PUSH_TEST_APP_NAME = "push-test-app";

/** Creates a dedicated test user and app for push notification tests. */
async function ensurePushTestUserAndApp(
  request: APIRequestContext,
  token: string,
): Promise<{ userId: string; appId: string }> {
  const user = await ensureLinkedEmailAuthUser(request, PUSH_TEST_USER_EMAIL);
  const app = await seedAdminApp(request, token, {
    name: PUSH_TEST_APP_NAME,
    ownerUserId: user.id,
    description: "Push test application",
  });
  return { userId: user.id, appId: app.id };
}

/** Probes the push/devices endpoint: returns true on 200, false on 503, throws otherwise. */
export async function isPushEnabled(
  request: APIRequestContext,
  token: string,
): Promise<boolean> {
  const res = await request.get("/api/admin/push/devices", {
    headers: { Authorization: `Bearer ${token}` },
  });
  const status = res.status();
  if (status === 200) {
    return true;
  }
  if (status === 503) {
    return false;
  }
  let body = "";
  try {
    body = (await res.text()).trim();
  } catch {
    // Ignore parse errors and still throw a status-based error.
  }
  const suffix = body ? `: ${body}` : "";
  throw new Error(`Push enablement check failed with status ${status}${suffix}`);
}

/** Registers a push device token via the admin API, optionally revoking it afterward for inactive-device testing. */
export async function seedPushDeviceToken(
  request: APIRequestContext,
  token: string,
  overrides: {
    tokenValue?: string;
    provider?: string;
    platform?: string;
    deviceName?: string;
    isActive?: boolean;
  } = {},
): Promise<{ id: string; token: string; provider: string; platform: string; appId: string; userId: string }> {
  const owner = await ensurePushTestUserAndApp(request, token);
  const tokenValue = overrides.tokenValue || `test-push-token-${Date.now()}`;
  const provider = overrides.provider || "fcm";
  const platform = overrides.platform || "android";
  const deviceName = overrides.deviceName || "Test Device";
  const res = await request.post("/api/admin/push/devices", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      app_id: owner.appId,
      user_id: owner.userId,
      provider,
      platform,
      token: tokenValue,
      device_name: deviceName,
    },
  });
  await validateResponse(res, `Register push device ${tokenValue}`);
  const result = await res.json();
  if (overrides.isActive === false && typeof result?.id === "string") {
    const revokeRes = await request.delete(`/api/admin/push/devices/${encodeURIComponent(result.id)}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    await validateResponse(revokeRes, `Revoke inactive push device ${tokenValue}`);
  }
  return {
    id: result.id as string,
    token: result.token as string,
    provider: result.provider as string,
    platform: result.platform as string,
    appId: owner.appId,
    userId: owner.userId,
  };
}

/** Seeds a device token and inserts a push delivery row via SQL with configurable title, body, and status. */
export async function seedPushDelivery(
  request: APIRequestContext,
  token: string,
  overrides: {
    tokenValue?: string;
    provider?: string;
    platform?: string;
    deviceName?: string;
    title?: string;
    body?: string;
    status?: "pending" | "sent" | "failed" | "invalid_token";
    dataPayload?: Record<string, string>;
  } = {},
): Promise<{ id: string; title: string; status: string; device_token_id: string }> {
  const seededToken = await seedPushDeviceToken(request, token, {
    tokenValue: overrides.tokenValue,
    provider: overrides.provider,
    platform: overrides.platform,
    deviceName: overrides.deviceName,
  });

  const title = sqlLiteral(overrides.title || `seeded-push-delivery-${Date.now()}`);
  const body = sqlLiteral(overrides.body || "Seeded push delivery body");
  const rawStatus = overrides.status || "sent";
  const status = sqlLiteral(rawStatus);
  const dataPayload = sqlLiteral(
    JSON.stringify(overrides.dataPayload || { source: "browser-unmocked" }),
  );
  const sentAtSQL = rawStatus === "sent" ? "NOW()" : "NULL";

  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: push delivery history has no provider-independent seed API.
    `INSERT INTO _ayb_push_deliveries (device_token_id, app_id, user_id, provider, title, body, data_payload, status, sent_at)
     VALUES ('${seededToken.id}', '${seededToken.appId}', '${seededToken.userId}', '${seededToken.provider}', '${title}', '${body}', '${dataPayload}'::jsonb, '${status}', ${sentAtSQL})
     RETURNING id, title, status, device_token_id`,
  );
  return {
    id: result.rows[0][0] as string,
    title: result.rows[0][1] as string,
    status: result.rows[0][2] as string,
    device_token_id: result.rows[0][3] as string,
  };
}

/** Deletes push deliveries by SQL token pattern, revokes matching device tokens via API, and cleans up the test app. */
export async function cleanupPushTestData(
  request: APIRequestContext,
  token: string,
  tokenPattern: string,
): Promise<void> {
  const escapedPattern = escapeLikePattern(tokenPattern);
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: push delivery history has no domain delete/cleanup API.
    `DELETE FROM _ayb_push_deliveries WHERE device_token_id IN (
       SELECT id FROM _ayb_device_tokens WHERE token LIKE '%${escapedPattern}%' ESCAPE '\\'
     )`,
  );
  const listRes = await request.get("/api/admin/push/devices?include_inactive=true", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(listRes, `List push devices for cleanup ${tokenPattern}`);
  const body = await listRes.json();
  const devices = Array.isArray(body?.items) ? body.items : [];
  for (const device of devices) {
    if (typeof device?.id === "string" && typeof device?.token === "string" && device.token.includes(tokenPattern)) {
      const deleteRes = await request.delete(`/api/admin/push/devices/${encodeURIComponent(device.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      await validateResponse(deleteRes, `Revoke push device ${device.id}`);
    }
  }
  await cleanupAdminAppByName(request, token, PUSH_TEST_APP_NAME).catch(() => {});
}

export { PUSH_TEST_APP_ID, PUSH_TEST_USER_ID };
