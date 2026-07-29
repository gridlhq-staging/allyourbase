/** @module Browser-test core helpers for SQL execution, response validation, and endpoint probing. */
import {
  expect,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from "@playwright/test";

const SAFE_SQL_IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function assertSafeSQLIdentifier(identifier: string, label: string): string {
  if (!SAFE_SQL_IDENTIFIER.test(identifier)) {
    throw new Error(`Unsafe SQL identifier for ${label}: ${identifier}`);
  }
  return identifier;
}

/** Builds a console URL path for the admin base used by the current Playwright run. */
export function adminPath(
  relativePath = "",
  { trailingSlash = true }: { trailingSlash?: boolean } = {},
): string {
  const configuredBase = process.env.PLAYWRIGHT_ADMIN_BASE ?? "/admin";
  if (!configuredBase.startsWith("/") || configuredBase.includes("?") || configuredBase.includes("#")) {
    throw new Error(`PLAYWRIGHT_ADMIN_BASE must be an absolute URL path: ${configuredBase}`);
  }

  const normalizedBase = configuredBase === "/" ? "" : configuredBase.replace(/\/+$/, "");
  const normalizedRelativePath = relativePath.replace(/^\/+/, "");
  if (normalizedRelativePath) {
    return `${normalizedBase}/${normalizedRelativePath}`;
  }
  if (!normalizedBase) {
    return "/";
  }
  return trailingSlash ? `${normalizedBase}/` : normalizedBase;
}

export function sqlLiteral(value: string): string {
  return value.replaceAll("'", "''");
}

/**
 * Escapes a value for literal matching inside a single-quoted SQL `LIKE` pattern
 * declared with `ESCAPE '\'`. Neutralizes the LIKE wildcards and the escape
 * character itself, then quote-escapes the result so callers can interpolate it
 * straight into the quoted pattern.
 */
export function escapeLikePattern(value: string): string {
  const wildcardEscaped = value
    .replaceAll("\\", "\\\\")
    .replaceAll("%", "\\%")
    .replaceAll("_", "\\_");
  return sqlLiteral(wildcardEscaped);
}

/** Throws a descriptive error with status, message, and code if the response is not ok. */
export async function validateResponse(
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
      const text = await res.text();
      if (text) {
        errorMsg += `: ${text}`;
      }
    }
    throw new Error(errorMsg);
  }
}

/** Fetches and validates an authenticated admin JSON response for spec-level state assertions. */
export async function fetchAdminJSON(
  request: APIRequestContext,
  token: string,
  path: string,
): Promise<unknown> {
  const res = await request.get(path, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Fetch admin JSON ${path}`);
  return res.json();
}

export async function checkAuthEnabled(
  request: APIRequestContext,
): Promise<{ auth: boolean }> {
  const res = await request.get("/api/admin/status");
  await validateResponse(res, "Check admin status");
  const body = await res.json();
  return { auth: !!body.auth };
}

/** Executes SQL via the admin API, splitting on semicolons and returning the last result's columns, rows, and rowCount. */
export async function execSQL(
  request: APIRequestContext,
  token: string,
  query: string,
  options: { tenantID?: string } = {},
): Promise<{ columns: string[]; rows: unknown[][]; rowCount: number }> {
  const statements = query
    .split(";")
    .map((statement) => statement.trim())
    .filter((statement) => statement.length > 0);

  let lastResult: { columns: string[]; rows: unknown[][]; rowCount: number } = {
    columns: [],
    rows: [],
    rowCount: 0,
  };

  for (const statement of statements) {
    const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
    if (options.tenantID) {
      headers["X-Tenant-ID"] = options.tenantID;
    }
    const res = await request.post("/api/admin/sql", {
      headers,
      data: { query: statement },
    });
    await validateResponse(res, `Execute SQL: ${statement.substring(0, 50)}...`);
    lastResult = await res.json();
  }

  return lastResult;
}

/** Sends an HTTP request to a path and returns only the status code, without throwing on non-2xx. */
export async function probeEndpoint(
  request: APIRequestContext,
  token: string,
  path: string,
  options: {
    method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
    data?: unknown;
  } = {},
): Promise<number> {
  const method = options.method || "GET";
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (options.data !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const requestOptions = {
    headers,
    ...(options.data !== undefined ? { data: options.data } : {}),
  };

  switch (method) {
    case "POST": {
      const res = await request.post(path, requestOptions);
      return res.status();
    }
    case "PUT": {
      const res = await request.put(path, requestOptions);
      return res.status();
    }
    case "PATCH": {
      const res = await request.patch(path, requestOptions);
      return res.status();
    }
    case "DELETE": {
      const res = await request.delete(path, requestOptions);
      return res.status();
    }
    default: {
      const res = await request.get(path, requestOptions);
      return res.status();
    }
  }
}

/**
 * Waits for the admin dashboard to finish booting (loading/login → ready).
 * Ready state renders the Layout with its unique shell controls. Uses a
 * generous timeout to tolerate slow schema fetches under parallel test load.
 */
export async function waitForDashboard(page: Page): Promise<void> {
  const shellSearchButton = page.getByRole("button", { name: "Search... K" });
  const retryButton = page.getByRole("button", { name: /^retry$/i });
  const timeoutAt = Date.now() + 30000;

  while (Date.now() < timeoutAt) {
    try {
      await shellSearchButton.waitFor({ state: "visible", timeout: 1000 });
      return;
    } catch {
      // Boot can transiently land on a "Connection Error" screen when admin
      // endpoints are rate limited. Clicking Retry keeps dashboard startup
      // resilient without requiring every spec to duplicate this recovery path.
      if (await retryButton.isVisible().catch(() => false)) {
        await retryButton.click().catch(() => {});
      }
    }
  }

  await shellSearchButton.waitFor({ state: "visible", timeout: 1000 });
}

export async function expectOfflineRetryRecovery(
  page: Page,
  context: BrowserContext,
  triggerFailure: () => Promise<void>,
  assertRecovered: () => Promise<void>,
  options: { errorText?: string } = {},
): Promise<void> {
  const errorText = options.errorText ?? "Failed to fetch";
  try {
    await context.setOffline(true);
    await triggerFailure();
    const errorNotice = page.getByRole("alert").filter({ hasText: errorText }).first();
    await expect(errorNotice).toBeVisible();
    const retry = errorNotice.getByRole("button", { name: "Retry", exact: true });
    await expect(retry).toBeVisible();

    await context.setOffline(false);
    await retry.click();
    await assertRecovered();
  } finally {
    await context.setOffline(false);
  }
}

export async function navigateDashboardScreenInPage(page: Page, screenId: string): Promise<void> {
  await page.evaluate((nextScreenId) => {
    window.history.pushState(null, "", `/admin/screens/${nextScreenId}`);
    window.dispatchEvent(new PopStateEvent("popstate"));
  }, screenId);
}

/** Asserts that an RLS policy card appears in the page's main aria snapshot with the expected name, command, and USING expression. */
export async function expectRlsPolicyCard(
  page: Page,
  params: {
    policyName: string;
    command: string;
    usingExpression: string;
  },
): Promise<void> {
  const { policyName, command, usingExpression } = params;
  const ariaSnapshot = await page.locator("main").ariaSnapshot();
  const lines = ariaSnapshot.split("\n");
  const policyNameLine = lines.findIndex((line) => line.includes(policyName));

  expect(policyNameLine, `Expected policy ${policyName} to appear in the RLS policy list`).toBeGreaterThanOrEqual(0);

  const deleteButtonOffset = lines
    .slice(policyNameLine)
    .findIndex((line) => line.includes('button "Delete policy"'));
  const blockEnd = deleteButtonOffset >= 0 ? policyNameLine + deleteButtonOffset + 6 : policyNameLine + 20;
  const policyBlock = lines.slice(policyNameLine, Math.min(lines.length, blockEnd)).join("\n");

  expect(policyBlock).toContain(command);
  expect(policyBlock).toContain("USING:");
  expect(policyBlock).toContain(usingExpression);
}

/** Reads the admin auth token from the .auth/admin.json storage state file's localStorage. */
export async function getStoredAdminToken(): Promise<string> {
  const fs = await import("fs/promises");
  const path = await import("path");
  const url = await import("url");

  const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
  const authFile = path.join(__dirname, "../.auth/admin.json");

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
      `Make sure auth.setup.ts has run successfully.`,
    );
  }
}
