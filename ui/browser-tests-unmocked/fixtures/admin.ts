/** @module Browser-test fixtures for admin dashboard seeding and cleanup. */
import type { APIRequestContext } from "@playwright/test";
import { randomUUID } from "crypto";
import { execSQL, sqlLiteral, validateResponse } from "./core";
import {
  deleteAdminUserByID,
  ensureLinkedEmailAuthUser,
  resolveAuthUserIdByEmail,
} from "./auth";

const DEFAULT_FIXTURE_PASSWORD = "Password123!";

function assertSafeSQLInteger(value: number, label: string): number {
  if (!Number.isInteger(value) || value < 0) {
    throw new Error(`Unsafe SQL integer for ${label}: ${value}`);
  }
  return value;
}

/** Finds an existing user by email or creates one via the linked-email auth flow. */
export async function ensureUserByEmail(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<{ id: string; email: string }> {
  const existingID = await resolveAuthUserIdByEmail(request, token, email).catch(() => null);
  if (existingID) {
    return { id: existingID, email };
  }
  return ensureLinkedEmailAuthUser(request, email, DEFAULT_FIXTURE_PASSWORD);
}

export async function cleanupUserByEmail(
  request: APIRequestContext,
  token: string,
  email: string,
): Promise<void> {
  const userID = await resolveAuthUserIdByEmail(request, token, email).catch(() => null);
  if (userID) {
    await deleteAdminUserByID(request, token, userID);
  }
}

/** Creates an API key for a user via the admin API and returns its id and name. */
export async function seedApiKey(
  request: APIRequestContext,
  token: string,
  options: {
    userId: string;
    name: string;
    keyHash?: string;
    keyPrefix?: string;
    scope?: "*" | "readonly" | "readwrite";
  },
): Promise<{ id: string; name: string }> {
  const scope = options.scope || "*";
  const res = await request.post("/api/admin/api-keys", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { userId: options.userId, name: options.name, scope },
  });
  await validateResponse(res, `Create API key ${options.name}`);
  const body = await res.json();
  const id = body?.apiKey?.id;
  const name = body?.apiKey?.name;
  if (typeof id !== "string" || typeof name !== "string") {
    throw new Error(`Expected seeded API key id/name for key ${options.name}`);
  }
  return { id, name };
}

/** Lists API keys and deletes all that match the given name. */
export async function cleanupApiKeyByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.get("/api/admin/api-keys?perPage=200", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List API keys for cleanup ${name}`);
  const body = await res.json();
  const keys = Array.isArray(body?.items) ? body.items : [];
  for (const key of keys) {
    if (key?.name === name && typeof key?.id === "string") {
      const deleteRes = await request.delete(`/api/admin/api-keys/${encodeURIComponent(key.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      await validateResponse(deleteRes, `Revoke API key ${name}`);
    }
  }
}

/** Creates an admin app and optionally configures rate-limit settings via a follow-up PUT. */
export async function seedAdminApp(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    ownerUserId: string;
    description?: string;
    rateLimitRps?: number;
    rateLimitWindowSeconds?: number;
  },
): Promise<{ id: string; name: string }> {
  const createRes = await request.post("/api/admin/apps", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: options.name,
      description: options.description || "",
      ownerUserId: options.ownerUserId,
    },
  });
  await validateResponse(createRes, `Create admin app ${options.name}`);
  const created = await createRes.json();
  const id = created?.id;
  const name = created?.name;
  if (typeof id !== "string" || typeof name !== "string") {
    throw new Error(`Expected seeded app id/name for app ${options.name}`);
  }
  if (options.rateLimitRps !== undefined || options.rateLimitWindowSeconds !== undefined) {
    const updateRes = await request.put(`/api/admin/apps/${encodeURIComponent(id)}`, {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      data: {
        name,
        description: options.description || "",
        rateLimitRps: assertSafeSQLInteger(options.rateLimitRps ?? 0, "rateLimitRps"),
        rateLimitWindowSeconds: assertSafeSQLInteger(
          options.rateLimitWindowSeconds ?? 60,
          "rateLimitWindowSeconds",
        ),
      },
    });
    await validateResponse(updateRes, `Update admin app rate limits ${options.name}`);
  }
  return { id, name };
}

/** Lists admin apps and deletes all that match the given name. */
export async function cleanupAdminAppByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.get("/api/admin/apps?perPage=200", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List admin apps for cleanup ${name}`);
  const body = await res.json();
  const apps = Array.isArray(body?.items) ? body.items : [];
  for (const app of apps) {
    if (app?.name === name && typeof app?.id === "string") {
      const deleteRes = await request.delete(`/api/admin/apps/${encodeURIComponent(app.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      await validateResponse(deleteRes, `Delete admin app ${name}`);
    }
  }
}

/** Creates a support ticket, optionally updates its status/priority, and returns the final state. */
export async function seedSupportTicket(
  request: APIRequestContext,
  token: string,
  options: {
    subject: string;
    priority?: "low" | "normal" | "high" | "urgent";
    status?: "open" | "in_progress" | "waiting_on_customer" | "resolved" | "closed";
    tenantId?: string;
    userId?: string;
    initialMessage?: string;
  },
): Promise<{ id: string; subject: string; priority: string; status: string }> {
  const status = options.status || "open";
  const priority = options.priority || "normal";
  const fixtureID = randomUUID();
  const tenantID = options.tenantId
    ? `'${sqlLiteral(options.tenantId)}'::uuid`
    : "NULL::uuid";
  const userID = options.userId
    ? `'${sqlLiteral(options.userId)}'::uuid`
    : `'${randomUUID()}'::uuid`;
  /* eslint-disable no-restricted-syntax -- Stage 1 product gap: support tickets have no deterministic admin seed API. */
  const result = await execSQL(
    request,
    token,
    `WITH fixture_tenant AS (
       INSERT INTO _ayb_tenants (id, name, slug, state)
       SELECT
         '${fixtureID}'::uuid,
         'E2E support ${fixtureID}',
         'e2e-support-${fixtureID}',
         'active'
       WHERE ${options.tenantId ? "FALSE" : "TRUE"}
       RETURNING id
     ), new_ticket AS (
       INSERT INTO _ayb_support_tickets (tenant_id, user_id, subject, status, priority)
       VALUES (
         COALESCE(${tenantID}, (SELECT id FROM fixture_tenant)),
         ${userID},
         '${sqlLiteral(options.subject)}',
         '${status}',
         '${priority}'
       )
       RETURNING id, subject, status, priority
     ), initial_message AS (
       INSERT INTO _ayb_support_messages (ticket_id, sender_type, body)
       SELECT id, 'customer', '${sqlLiteral(options.initialMessage || "Initial customer message")}'
       FROM new_ticket
     )
     SELECT id, subject, status, priority FROM new_ticket`,
  );
  /* eslint-enable no-restricted-syntax */
  const [id, subject, returnedStatus, returnedPriority] = result.rows[0] ?? [];
  if (
    typeof id !== "string" ||
    typeof subject !== "string" ||
    typeof returnedPriority !== "string" ||
    typeof returnedStatus !== "string"
  ) {
    throw new Error(`Expected seeded support ticket fields for subject ${options.subject}`);
  }

  return { id, subject, priority: returnedPriority, status: returnedStatus };
}

export async function cleanupSupportTicketByID(
  request: APIRequestContext,
  token: string,
  ticketID: string,
): Promise<void> {
  /* eslint-disable no-restricted-syntax -- Stage 1 product gap: support ticket cleanup has no domain delete/retention API. */
  await execSQL(
    request,
    token,
    `WITH deleted_ticket AS (
       DELETE FROM _ayb_support_tickets
       WHERE id = '${sqlLiteral(ticketID)}'
       RETURNING tenant_id
     )
     DELETE FROM _ayb_tenants
     WHERE id = (SELECT tenant_id FROM deleted_ticket)
       AND slug LIKE 'e2e-support-%'`,
  );
  /* eslint-enable no-restricted-syntax */
}

/** Creates an incident and optionally posts an initial status update. */
export async function seedIncident(
  request: APIRequestContext,
  token: string,
  options: {
    title: string;
    status?: "investigating" | "identified" | "monitoring" | "resolved";
    affectedServices?: string[];
    initialUpdateMessage?: string;
    initialUpdateStatus?: "investigating" | "identified" | "monitoring" | "resolved";
  },
): Promise<{ id: string; title: string; status: string }> {
  const status = options.status || "investigating";
  const createRes = await request.post("/api/admin/incidents", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      title: options.title,
      status,
      affectedServices: options.affectedServices || [],
    },
  });
  await validateResponse(createRes, `Create incident ${options.title}`);
  const created = await createRes.json();
  const id = created?.id;
  const title = created?.title;
  const returnedStatus = created?.status;
  if (
    typeof id !== "string" ||
    typeof title !== "string" ||
    typeof returnedStatus !== "string"
  ) {
    throw new Error(`Expected seeded incident fields for title ${options.title}`);
  }

  if (options.initialUpdateMessage) {
    const updateStatus = options.initialUpdateStatus || returnedStatus;
    const updateRes = await request.post(`/api/admin/incidents/${encodeURIComponent(id)}/updates`, {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      data: { message: options.initialUpdateMessage, status: updateStatus },
    });
    await validateResponse(updateRes, `Add incident update ${options.title}`);
  }

  return { id, title, status: returnedStatus };
}

export async function cleanupIncidentByID(
  request: APIRequestContext,
  token: string,
  incidentID: string,
): Promise<void> {
  // Stage 1 product gap: incident cleanup has no domain delete/retention API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: incident cleanup has no domain delete/retention API.
    `DELETE FROM _ayb_incidents WHERE id = '${sqlLiteral(incidentID)}'`,
  );
}

export async function cleanupNotificationsByTitle(
  request: APIRequestContext,
  token: string,
  title: string,
): Promise<void> {
  // Stage 1 product gap: notifications have no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: notifications have no domain delete/cleanup API.
    `DELETE FROM _ayb_notifications WHERE title = '${sqlLiteral(title)}'`,
  );
}

interface SeedRequestLogEntryOptions {
  method: string;
  path: string;
  statusCode: number;
  durationMs: number;
  timestampISO?: string;
  requestSize?: number;
  responseSize?: number;
  requestID?: string;
  ipAddress?: string;
}

/** Inserts a request log row via SQL and returns the seeded entry's fields. */
export async function seedRequestLogEntry(
  request: APIRequestContext,
  token: string,
  options: SeedRequestLogEntryOptions,
): Promise<{ id: string; path: string; method: string; statusCode: number; durationMs: number }> {
  const statusCodeValue = assertSafeSQLInteger(options.statusCode, "statusCode");
  const durationMsValue = assertSafeSQLInteger(options.durationMs, "durationMs");
  const requestSizeValue = assertSafeSQLInteger(options.requestSize ?? 0, "requestSize");
  const responseSizeValue = assertSafeSQLInteger(options.responseSize ?? 0, "responseSize");
  const timestampSQL = options.timestampISO
    ? `'${sqlLiteral(options.timestampISO)}'::timestamptz`
    : "NOW()";
  const requestIDSQL = options.requestID
    ? `'${sqlLiteral(options.requestID)}'`
    : "NULL";
  const ipAddressSQL = options.ipAddress
    ? `'${sqlLiteral(options.ipAddress)}'::inet`
    : "NULL";

  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: request logs are generated internally and have no deterministic seed API.
    `INSERT INTO _ayb_request_logs (
       timestamp, method, path, status_code, duration_ms, request_size, response_size, request_id, ip_address
     )
     VALUES (
       ${timestampSQL},
       '${sqlLiteral(options.method)}',
       '${sqlLiteral(options.path)}',
       ${statusCodeValue},
       ${durationMsValue},
       ${requestSizeValue},
       ${responseSizeValue},
       ${requestIDSQL},
       ${ipAddressSQL}
     )
     RETURNING id::text, path, method, status_code, duration_ms`,
  );

  const id = result.rows[0]?.[0];
  const path = result.rows[0]?.[1];
  const method = result.rows[0]?.[2];
  const statusCode = result.rows[0]?.[3];
  const durationMs = result.rows[0]?.[4];
  if (
    typeof id !== "string" ||
    typeof path !== "string" ||
    typeof method !== "string" ||
    typeof statusCode !== "number" ||
    typeof durationMs !== "number"
  ) {
    throw new Error(`Expected seeded request log fields for path ${options.path}`);
  }

  return { id, path, method, statusCode, durationMs };
}

export async function cleanupRequestLogsByPath(
  request: APIRequestContext,
  token: string,
  path: string,
): Promise<void> {
  // Stage 1 product gap: request logs have no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: request logs have no domain delete/cleanup API.
    `DELETE FROM _ayb_request_logs WHERE path = '${sqlLiteral(path)}'`,
  );
}

interface SeedAuditLogEntryOptions {
  tableName: string;
  operation: "INSERT" | "UPDATE" | "DELETE";
  recordID?: unknown;
  oldValues?: unknown;
  newValues?: unknown;
  timestampISO?: string;
  ipAddress?: string;
}

/** Inserts an audit log row via SQL and returns the seeded entry's id, table, and operation. */
export async function seedAuditLogEntry(
  request: APIRequestContext,
  token: string,
  options: SeedAuditLogEntryOptions,
): Promise<{ id: string; tableName: string; operation: "INSERT" | "UPDATE" | "DELETE" }> {
  const timestampSQL = options.timestampISO
    ? `'${sqlLiteral(options.timestampISO)}'::timestamptz`
    : "NOW()";
  const recordIDSQL = options.recordID === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.recordID))}'::jsonb`;
  const oldValuesSQL = options.oldValues === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.oldValues))}'::jsonb`;
  const newValuesSQL = options.newValues === undefined
    ? "NULL"
    : `'${sqlLiteral(JSON.stringify(options.newValues))}'::jsonb`;
  const ipAddressSQL = options.ipAddress
    ? `'${sqlLiteral(options.ipAddress)}'::inet`
    : "NULL";

  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: audit history is read-only and has no deterministic seed API.
    `INSERT INTO _ayb_audit_log (
       timestamp, table_name, operation, record_id, old_values, new_values, ip_address
     )
     VALUES (
       ${timestampSQL},
       '${sqlLiteral(options.tableName)}',
       '${sqlLiteral(options.operation)}',
       ${recordIDSQL},
       ${oldValuesSQL},
       ${newValuesSQL},
       ${ipAddressSQL}
     )
     RETURNING id::text, table_name, operation`,
  );

  const id = result.rows[0]?.[0];
  const tableName = result.rows[0]?.[1];
  const operation = result.rows[0]?.[2];
  if (
    typeof id !== "string" ||
    typeof tableName !== "string" ||
    (operation !== "INSERT" && operation !== "UPDATE" && operation !== "DELETE")
  ) {
    throw new Error(`Expected seeded audit log fields for table ${options.tableName}`);
  }

  return { id, tableName, operation };
}

export async function cleanupAuditLogsByTable(
  request: APIRequestContext,
  token: string,
  tableName: string,
): Promise<void> {
  // Stage 1 product gap: audit history has no domain delete/cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: audit history has no domain delete/cleanup API.
    `DELETE FROM _ayb_audit_log WHERE table_name = '${sqlLiteral(tableName)}'`,
  );
}

/** Makes the isolated audit relation return a row that cannot decode as an audit timestamp. */
export async function seedUnreadableAuditLogTimestamp(
  request: APIRequestContext,
  token: string,
  tableName: string,
): Promise<void> {
  /* eslint-disable no-restricted-syntax -- Stage 1 product gap: audit history has no fault-injection API. */
  await execSQL(
    request,
    token,
    "ALTER TABLE _ayb_audit_log ALTER COLUMN timestamp TYPE text USING timestamp::text",
  );
  await execSQL(
    request,
    token,
    `INSERT INTO _ayb_audit_log (timestamp, table_name, operation)
     VALUES ('not-a-timestamp', '${sqlLiteral(tableName)}', 'INSERT')`,
  );
  /* eslint-enable no-restricted-syntax */
}

export interface AdminStatsSnapshot {
  uptime_seconds: number;
  go_version: string;
  goroutines: number;
  memory_alloc: number;
  memory_sys: number;
  gc_cycles: number;
  db_pool_total?: number;
  db_pool_idle?: number;
  db_pool_in_use?: number;
  db_pool_max?: number;
}

/** Fetches runtime stats (uptime, memory, goroutines, DB pool) from GET /api/admin/stats. */
export async function fetchAdminStatsSnapshot(
  request: APIRequestContext,
  token: string,
): Promise<AdminStatsSnapshot> {
  const res = await request.get("/api/admin/stats", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch admin stats snapshot");

  const body = await res.json();
  if (
    typeof body?.uptime_seconds !== "number" ||
    typeof body?.go_version !== "string" ||
    typeof body?.goroutines !== "number" ||
    typeof body?.memory_alloc !== "number" ||
    typeof body?.memory_sys !== "number" ||
    typeof body?.gc_cycles !== "number"
  ) {
    throw new Error("Expected numeric/string runtime metrics from /api/admin/stats");
  }

  return body as AdminStatsSnapshot;
}

/** Creates or updates an email template by key via PUT and returns the stored template fields. */
export async function seedEmailTemplate(
  request: APIRequestContext,
  token: string,
  options: {
    key: string;
    subjectTemplate: string;
    htmlTemplate: string;
  },
): Promise<{
  templateKey: string;
  subjectTemplate: string;
  htmlTemplate: string;
  enabled: boolean;
}> {
  const res = await request.put(`/api/admin/email/templates/${encodeURIComponent(options.key)}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: {
      subjectTemplate: options.subjectTemplate,
      htmlTemplate: options.htmlTemplate,
    },
  });
  await validateResponse(res, `Seed email template ${options.key}`);

  const body = await res.json();
  if (
    typeof body?.templateKey !== "string" ||
    typeof body?.subjectTemplate !== "string" ||
    typeof body?.htmlTemplate !== "string" ||
    typeof body?.enabled !== "boolean"
  ) {
    throw new Error(`Expected seeded email template fields for key ${options.key}`);
  }

  return {
    templateKey: body.templateKey,
    subjectTemplate: body.subjectTemplate,
    htmlTemplate: body.htmlTemplate,
    enabled: body.enabled,
  };
}

export async function cleanupEmailTemplate(
  request: APIRequestContext,
  token: string,
  key: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/email/templates/${encodeURIComponent(key)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() === 404 || res.status() === 400) {
    return;
  }
  await validateResponse(res, `Delete email template ${key}`);
}

export interface AdminMatviewRegistration {
  id: string;
  schemaName: string;
  viewName: string;
  refreshMode: string;
}

/** Validates and extracts id/schema/view/refreshMode fields from a matview registration response. */
function parseAdminMatviewRegistration(body: unknown, context: string): AdminMatviewRegistration {
  const item = body as Record<string, unknown>;
  if (
    typeof item?.id !== "string" ||
    typeof item?.schemaName !== "string" ||
    typeof item?.viewName !== "string" ||
    typeof item?.refreshMode !== "string"
  ) {
    throw new Error(`Expected matview registration fields for ${context}`);
  }
  return {
    id: item.id,
    schemaName: item.schemaName,
    viewName: item.viewName,
    refreshMode: item.refreshMode,
  };
}

export async function registerAdminMatview(
  request: APIRequestContext,
  token: string,
  options: { schema: string; viewName: string; refreshMode: string },
): Promise<AdminMatviewRegistration> {
  const res = await request.post("/api/admin/matviews", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: options,
  });
  await validateResponse(res, `Register matview ${options.schema}.${options.viewName}`);
  return parseAdminMatviewRegistration(await res.json(), options.viewName);
}

/** Lists matview registrations and deletes the one matching the given schema and view name. */
export async function cleanupAdminMatviewByName(
  request: APIRequestContext,
  token: string,
  schemaName: string,
  viewName: string,
): Promise<void> {
  const res = await request.get("/api/admin/matviews", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List matviews for cleanup");
  const body = await res.json();
  const items = Array.isArray(body?.items) ? body.items : [];
  for (const item of items) {
    const registration = parseAdminMatviewRegistration(item, viewName);
    if (registration.schemaName !== schemaName || registration.viewName !== viewName) {
      continue;
    }
    const deleteRes = await request.delete(`/api/admin/matviews/${encodeURIComponent(registration.id)}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (deleteRes.status() !== 404) {
      await validateResponse(deleteRes, `Delete matview registration ${schemaName}.${viewName}`);
    }
  }
}

export interface AuthSettingsSnapshot {
  totp_enabled: boolean;
  anonymous_auth_enabled: boolean;
  email_mfa_enabled: boolean;
  sms_enabled: boolean;
  magic_link_enabled: boolean;
  [key: string]: unknown;
}

export async function fetchAuthSettings(
  request: APIRequestContext,
  token: string,
): Promise<AuthSettingsSnapshot> {
  const res = await request.get("/api/admin/auth-settings", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch auth settings");
  return await res.json();
}

export interface RealtimeStatsSnapshot {
  version: string;
  timestamp: string;
  connections: { sse: number; ws: number; total: number };
  counters: { dropped_messages: number; heartbeat_failures: number };
}

export async function fetchRealtimeStats(
  request: APIRequestContext,
  token: string,
): Promise<RealtimeStatsSnapshot> {
  const res = await request.get("/api/admin/realtime/stats", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch realtime stats");
  return await res.json();
}

export interface SecurityAdvisorSnapshot {
  evaluatedAt: string;
  stale: boolean;
  findings: Array<{
    id: string;
    severity: "critical" | "high" | "medium" | "low";
    category: string;
    status: "open" | "accepted" | "resolved";
    title: string;
    description: string;
    remediation: string;
  }>;
}

/** Fetches the security advisor report and validates its findings-array shape. */
export async function fetchSecurityAdvisorReport(
  request: APIRequestContext,
  token: string,
): Promise<SecurityAdvisorSnapshot> {
  const res = await request.get("/api/admin/advisors/security", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch security advisor report");
  const body = await res.json();
  if (
    typeof body?.evaluatedAt !== "string" ||
    typeof body?.stale !== "boolean" ||
    !Array.isArray(body?.findings)
  ) {
    throw new Error("Expected security advisor report shape from /api/admin/advisors/security");
  }
  return body as SecurityAdvisorSnapshot;
}

export interface PerformanceAdvisorSnapshot {
  generatedAt: string;
  stale: boolean;
  range: "15m" | "1h" | "6h" | "24h" | "7d";
  queries: Array<{
    fingerprint: string;
    normalizedQuery: string;
    meanMs: number;
    totalMs: number;
    calls: number;
    rows: number;
    endpoints: string[];
    trend: "up" | "down" | "flat";
  }>;
}

/** Fetches the performance advisor report for a given time range and validates its query-array shape. */
export async function fetchPerformanceAdvisorReport(
  request: APIRequestContext,
  token: string,
  range: "15m" | "1h" | "6h" | "24h" | "7d" = "1h",
): Promise<PerformanceAdvisorSnapshot> {
  const res = await request.get(`/api/admin/advisors/performance?range=${encodeURIComponent(range)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Fetch performance advisor report");
  const body = await res.json();
  if (
    typeof body?.generatedAt !== "string" ||
    typeof body?.stale !== "boolean" ||
    typeof body?.range !== "string" ||
    !Array.isArray(body?.queries)
  ) {
    throw new Error("Expected performance advisor report shape from /api/admin/advisors/performance");
  }
  return body as PerformanceAdvisorSnapshot;
}

/** Trigger an admin stats request with a unique request ID for smoke-test log assertions. */
export async function triggerAdminStatsRequest(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  const requestId = `admin-logs-${randomUUID()}`;
  const res = await request.get("/api/admin/stats", {
    headers: {
      Authorization: `Bearer ${token}`,
      "X-Request-Id": requestId,
    },
  });
  await validateResponse(res, "Trigger admin stats request");
  return requestId;
}
