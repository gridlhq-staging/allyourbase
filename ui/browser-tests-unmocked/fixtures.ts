import { test as base, type APIRequestContext } from "@playwright/test";

/**
 * Shared fixtures and helpers for browser-unmocked tests.
 *
 * Spec files import `test` from this file instead of `@playwright/test`.
 * All API calls and data seeding go here — spec files only contain
 * Act + Assert code (human-like UI interactions).
 */

const ADMIN_PASSWORD = process.env.AYB_ADMIN_PASSWORD || "admin";

// ---------------------------------------------------------------------------
// Error handling helper
// ---------------------------------------------------------------------------

/**
 * Validate API response and throw detailed error if request failed.
 * This prevents silent failures in test fixtures.
 */
async function validateResponse(
  res: Awaited<ReturnType<APIRequestContext["post"]>>,
  context: string,
): Promise<void> {
  if (!res.ok()) {
    const status = res.status();
    let errorMsg = `${context} failed with status ${status}`;
    try {
      const body = await res.json();
      if (body.message) {
        errorMsg += `: ${body.message}`;
      }
      if (body.code) {
        errorMsg += ` (code: ${body.code})`;
      }
    } catch {
      // If response isn't JSON, try to get text
      const text = await res.text();
      if (text) {
        errorMsg += `: ${text}`;
      }
    }
    throw new Error(errorMsg);
  }
}

// ---------------------------------------------------------------------------
// Admin auth
// ---------------------------------------------------------------------------

/** Get an admin JWT token via the login endpoint. */
async function getAdminToken(request: APIRequestContext): Promise<string> {
  const res = await request.post("/api/admin/auth", {
    data: { password: ADMIN_PASSWORD },
  });
  await validateResponse(res, "Admin login");
  const body = await res.json();
  if (!body.token) {
    throw new Error("Admin login succeeded but no token in response");
  }
  return body.token;
}

/** Check whether admin auth is enabled on the running server. */
export async function checkAuthEnabled(
  request: APIRequestContext,
): Promise<{ auth: boolean }> {
  const res = await request.get("/api/admin/status");
  await validateResponse(res, "Check admin status");
  const body = await res.json();
  return { auth: !!body.auth };
}

// ---------------------------------------------------------------------------
// SQL helper
// ---------------------------------------------------------------------------

/**
 * Execute SQL via the admin API. Returns { columns, rows, rowCount }.
 *
 * Handles multi-statement SQL by splitting on semicolons and executing
 * each statement separately. Returns the result of the last statement.
 */
export async function execSQL(
  request: APIRequestContext,
  token: string,
  query: string,
): Promise<{ columns: string[]; rows: unknown[][]; rowCount: number }> {
  // Split on semicolons to handle multi-statement SQL
  // Postgres prepared statements don't support multiple commands in one call
  const statements = query
    .split(";")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);

  let lastResult: { columns: string[]; rows: unknown[][]; rowCount: number } = {
    columns: [],
    rows: [],
    rowCount: 0,
  };

  for (const statement of statements) {
    const res = await request.post("/api/admin/sql", {
      headers: { Authorization: `Bearer ${token}` },
      data: { query: statement },
    });
    await validateResponse(res, `Execute SQL: ${statement.substring(0, 50)}...`);
    lastResult = await res.json();
  }

  return lastResult;
}

// ---------------------------------------------------------------------------
// Webhook helpers
// ---------------------------------------------------------------------------

export async function seedWebhook(
  request: APIRequestContext,
  token: string,
  url: string,
): Promise<{ id: number; url: string }> {
  const res = await request.post("/api/webhooks", {
    headers: { Authorization: `Bearer ${token}` },
    data: { url, events: ["create"], enabled: true },
  });
  await validateResponse(res, `Create webhook for ${url}`);
  const body = await res.json();
  if (!body.id) {
    throw new Error("Webhook created but no ID in response");
  }
  return { id: body.id, url: body.url };
}

export async function deleteWebhook(
  request: APIRequestContext,
  token: string,
  id: number,
): Promise<void> {
  const res = await request.delete(`/api/webhooks/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete webhook ${id}`);
}

// ---------------------------------------------------------------------------
// Storage helpers
// ---------------------------------------------------------------------------

export async function seedFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
  content: string,
): Promise<{ name: string }> {
  const res = await request.post(`/api/storage/${bucket}`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      file: {
        name: fileName,
        mimeType: "text/plain",
        buffer: Buffer.from(content),
      },
    },
  });
  await validateResponse(res, `Upload file ${fileName} to bucket ${bucket}`);
  const body = await res.json();
  if (!body.name) {
    throw new Error(`File upload succeeded but no name in response`);
  }
  return { name: body.name };
}

export async function deleteFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
): Promise<void> {
  const res = await request.delete(`/api/storage/${bucket}/${fileName}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete file ${fileName} from bucket ${bucket}`);
}

// ---------------------------------------------------------------------------
// Collection (table records) helpers
// ---------------------------------------------------------------------------

export async function seedRecord(
  request: APIRequestContext,
  token: string,
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const res = await request.post(`/api/collections/${table}`, {
    headers: { Authorization: `Bearer ${token}` },
    data,
  });
  await validateResponse(res, `Create record in table ${table}`);
  const body = await res.json();
  return body;
}

// ---------------------------------------------------------------------------
// Custom test fixture
// ---------------------------------------------------------------------------

/**
 * Extract admin token from saved auth state file.
 * This avoids hitting the rate limiter by reusing the token from auth.setup.ts
 */
async function getStoredAdminToken(): Promise<string> {
  const fs = await import("fs/promises");
  const path = await import("path");
  const url = await import("url");

  const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
  const authFile = path.join(__dirname, ".auth/admin.json");

  try {
    const authState = JSON.parse(await fs.readFile(authFile, "utf-8"));
    const origins = authState.origins || [];
    for (const origin of origins) {
      const localStorage = origin.localStorage || [];
      for (const item of localStorage) {
        if (item.name === "ayb_admin_token") {
          return item.value;
        }
      }
    }
    throw new Error("Admin token not found in auth state file");
  } catch (err) {
    throw new Error(
      `Failed to read admin token from ${authFile}: ${err}. ` +
      `Make sure auth.setup.ts has run successfully.`
    );
  }
}

/**
 * Custom test fixture that extends Playwright's base test.
 *
 * Provides:
 *   - authStatus: pre-fetched { auth: boolean } from the server
 *   - adminToken: admin JWT token extracted from saved auth state (no auth request needed)
 */
export const test = base.extend<{
  authStatus: { auth: boolean };
  adminToken: string;
}>({
  authStatus: async ({ request }, use) => {
    const status = await checkAuthEnabled(request);
    await use(status);
  },
  adminToken: async ({}, use) => {
    // Get token from saved auth state instead of making a new auth request
    // This prevents hitting the rate limiter when tests run in parallel
    const token = await getStoredAdminToken();
    await use(token);
  },
});

export { expect } from "@playwright/test";
