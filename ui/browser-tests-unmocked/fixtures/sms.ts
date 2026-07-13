/** @module Browser-test fixtures for SMS message seeding, daily count stats, and batch insertion. */
import type { APIRequestContext, TestInfo } from "@playwright/test";
import { escapeLikePattern, execSQL, sqlLiteral } from "./core";
import { ensureLinkedEmailAuthUser } from "./auth";

const SMS_TEST_USER_EMAIL = "sms-fixture-test@example.com";
const DEFAULT_SMS_PROVIDER_SKIP_REASON =
  "SMS provider not configured — skipping SMS Health smoke";

async function ensureSMSTestUser(
  request: APIRequestContext,
  _token: string,
): Promise<string> {
  const user = await ensureLinkedEmailAuthUser(request, SMS_TEST_USER_EMAIL);
  return user.id;
}

/** Inserts an SMS message row via SQL with configurable phone, body, provider, and status. */
export async function seedSMSMessage(
  request: APIRequestContext,
  token: string,
  overrides: {
    to_phone?: string;
    body?: string;
    provider?: string;
    status?: string;
    error_message?: string;
  } = {},
): Promise<{ id: string; to_phone: string; body: string; status: string }> {
  const userID = await ensureSMSTestUser(request, token);
  const toPhone = overrides.to_phone || "+15551234567";
  const body = overrides.body || "Test SMS message";
  const provider = overrides.provider || "log";
  const status = overrides.status || "delivered";
  const errorMessage = overrides.error_message || "";
  const safeToPhone = sqlLiteral(toPhone);
  const safeBody = sqlLiteral(body);
  const safeProvider = sqlLiteral(provider);
  const safeStatus = sqlLiteral(status);
  const safeErrorMessage = sqlLiteral(errorMessage);
  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS message history has no deterministic seed API.
    `INSERT INTO _ayb_sms_messages (user_id, to_phone, body, provider, status, error_message)
     VALUES ('${sqlLiteral(userID)}', '${safeToPhone}', '${safeBody}', '${safeProvider}', '${safeStatus}', '${safeErrorMessage}')
     RETURNING id, to_phone, body, status`,
  );
  return {
    id: result.rows[0][0] as string,
    to_phone: result.rows[0][1] as string,
    body: result.rows[0][2] as string,
    status: result.rows[0][3] as string,
  };
}

export async function cleanupSMSMessages(
  request: APIRequestContext,
  token: string,
  bodyPattern: string,
): Promise<void> {
  const safeBodyPattern = escapeLikePattern(bodyPattern);
  // Stage 1 product gap: SMS message history has no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS message history has no domain delete/cleanup API.
    `DELETE FROM _ayb_sms_messages WHERE body LIKE '%${safeBodyPattern}%' ESCAPE '\\'`,
  );
}

/** Inserts or upserts today's SMS daily count stats with configurable count, confirm, and fail values. */
export async function seedSMSDailyCounts(
  request: APIRequestContext,
  token: string,
  overrides: {
    count?: number;
    confirm_count?: number;
    fail_count?: number;
  } = {},
): Promise<void> {
  const count = overrides.count ?? 10;
  const confirm = overrides.confirm_count ?? 5;
  const fail = overrides.fail_count ?? 2;
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS health stats have no deterministic seed API.
    `INSERT INTO _ayb_sms_daily_counts (date, count, confirm_count, fail_count)
     VALUES (CURRENT_DATE, ${count}, ${confirm}, ${fail})
     ON CONFLICT (date) DO UPDATE SET
       count = EXCLUDED.count,
       confirm_count = EXCLUDED.confirm_count,
       fail_count = EXCLUDED.fail_count`,
  );
}

export async function cleanupSMSDailyCounts(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  // Stage 1 product gap: SMS health stats have no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS health stats have no domain delete/cleanup API.
    "DELETE FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE",
  );
}

export async function cleanupSMSDailyCountsAll(
  request: APIRequestContext,
  token: string,
): Promise<void> {
  // Stage 1 product gap: SMS health stats have no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS health stats have no domain delete/cleanup API.
    "DELETE FROM _ayb_sms_daily_counts WHERE date >= CURRENT_DATE - INTERVAL '29 days'",
  );
}

/** Bulk-inserts SMS message rows via generate_series for pagination testing. */
export async function seedSMSMessageBatch(
  request: APIRequestContext,
  token: string,
  count: number,
  bodyPrefix: string,
): Promise<void> {
  const userID = await ensureSMSTestUser(request, token);
  const safeBodyPrefix = sqlLiteral(bodyPrefix);
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: SMS pagination needs deterministic batch message history seed.
    `INSERT INTO _ayb_sms_messages (user_id, to_phone, body, provider, status)
     SELECT '${sqlLiteral(userID)}',
            '+1555' || LPAD(g::text, 7, '0'),
            '${safeBodyPrefix}' || g,
            'log',
            'delivered'
     FROM generate_series(1, ${count}) g`,
  );
}

export async function isSMSProviderConfigured(
  request: APIRequestContext,
  token: string,
): Promise<boolean> {
  const res = await request.post("/api/admin/sms/send", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: { to: "", body: "" },
  });
  return res.status() !== 404;
}

export async function skipUnlessSMSProviderConfigured(
  request: APIRequestContext,
  token: string,
  testInfo: TestInfo,
  reason = DEFAULT_SMS_PROVIDER_SKIP_REASON,
): Promise<void> {
  if (!(await isSMSProviderConfigured(request, token))) {
    testInfo.skip(reason);
  }
}
