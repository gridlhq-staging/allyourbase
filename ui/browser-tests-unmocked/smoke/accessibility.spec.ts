import {
  test,
  expect,
  waitForDashboard,
  execSQL,
  getAdminCapabilities,
  buildParallelSafeRunID,
  dropTableIfExists,
  type AdminCapabilities,
  type AdminCapabilityName,
} from "../fixtures";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

/**
 * SMOKE TEST: Accessibility (axe-core)
 *
 * Runs automated WCAG 2.1 AA accessibility scans on dashboard pages.
 * Critical/serious violations fail; moderate/minor are logged for follow-up.
 */

test.describe("Smoke: Accessibility", () => {
  // Run each a11y scan test in parallel — they are independent page loads.
  test.describe.configure({ mode: "parallel" });

  const pendingCleanupTables: string[] = [];
  type RegistryScreenScan = {
    readonly buttonName: RegExp;
    readonly pageName: string;
    readonly requires?: AdminCapabilityName;
  };

  const registryScreenScans = {
    "sql-editor": { buttonName: /^SQL Editor$/i, pageName: "SQL Editor" },
    graphql: { buttonName: /^GraphQL$/i, pageName: "GraphQL" },
    functions: { buttonName: /^Functions$/i, pageName: "Functions" },
    rls: { buttonName: /^RLS Policies$/i, pageName: "RLS Policies" },
    search: { buttonName: /^Search$/i, pageName: "Search" },
    matviews: { buttonName: /^Materialized Views$/i, pageName: "Materialized Views" },
    "schema-designer": { buttonName: /^Schema Designer$/i, pageName: "Schema Designer" },
    fdw: { buttonName: /^FDW Management$/i, pageName: "FDW Management" },
    storage: { buttonName: /^Storage$/i, pageName: "Storage" },
    sites: { buttonName: /^Sites$/i, pageName: "Sites" },
    "edge-functions": { buttonName: /^Edge Functions$/i, pageName: "Edge Functions" },
    webhooks: { buttonName: /^Webhooks$/i, pageName: "Webhooks" },
    "sms-health": { buttonName: /^SMS Health$/i, pageName: "SMS Health" },
    "sms-messages": { buttonName: /^SMS Messages$/i, pageName: "SMS Messages" },
    "email-templates": { buttonName: /^Email Templates$/i, pageName: "Email Templates" },
    push: { buttonName: /^Push Notifications$/i, pageName: "Push Notifications" },
    users: { buttonName: /^Users$/i, pageName: "Users" },
    apps: { buttonName: /^Applications$/i, pageName: "Applications" },
    "api-keys": { buttonName: /^API Keys$/i, pageName: "API Keys" },
    "oauth-clients": { buttonName: /^OAuth Clients$/i, pageName: "OAuth Clients" },
    "api-explorer": { buttonName: /^API Explorer$/i, pageName: "API Explorer" },
    jobs: { buttonName: /^Jobs$/i, pageName: "Jobs" },
    schedules: { buttonName: /^Schedules$/i, pageName: "Schedules" },
    "realtime-inspector": { buttonName: /^Realtime Inspector$/i, pageName: "Realtime Inspector" },
    "security-advisor": { buttonName: /^Security Advisor$/i, pageName: "Security Advisor" },
    "performance-advisor": { buttonName: /^Performance Advisor$/i, pageName: "Performance Advisor" },
    backups: { buttonName: /^Backups & PITR$/i, pageName: "Backups & PITR" },
    analytics: { buttonName: /^Analytics$/i, pageName: "Analytics" },
    usage: { buttonName: /^Usage Metering$/i, pageName: "Usage Metering" },
    replicas: { buttonName: /^Replicas$/i, pageName: "Replicas" },
    branches: { buttonName: /^Branches$/i, pageName: "Branches" },
    "audit-logs": { buttonName: /^Audit Logs$/i, pageName: "Audit Logs" },
    "admin-logs": { buttonName: /^Admin Logs$/i, pageName: "Admin Logs" },
    secrets: { buttonName: /^Secrets$/i, pageName: "Secrets" },
    "custom-domains": { buttonName: /^Custom Domains$/i, pageName: "Custom Domains" },
    extensions: { buttonName: /^Extensions$/i, pageName: "Extensions" },
    "vector-indexes": { buttonName: /^Vector Indexes$/i, pageName: "Vector Indexes" },
    "log-drains": { buttonName: /^Log Drains$/i, pageName: "Log Drains" },
    stats: { buttonName: /^Stats$/i, pageName: "Stats" },
    notifications: { buttonName: /^Notifications$/i, pageName: "Notifications" },
    incidents: { buttonName: /^Incidents$/i, pageName: "Incidents", requires: "status" },
    "support-tickets": {
      buttonName: /^Support Tickets$/i,
      pageName: "Support Tickets",
      requires: "support",
    },
    tenants: { buttonName: /^Tenants$/i, pageName: "Tenants" },
    organizations: { buttonName: /^Organizations$/i, pageName: "Organizations" },
    "ai-assistant": { buttonName: /^AI Assistant$/i, pageName: "AI Assistant" },
    "auth-settings": { buttonName: /^Auth Settings$/i, pageName: "Auth Settings" },
    "mfa-management": {
      buttonName: /^Multi-Factor Authentication$/i,
      pageName: "Multi-Factor Authentication",
    },
    "account-linking": { buttonName: /^Link Your Account$/i, pageName: "Link Your Account" },
    saml: { buttonName: /^SAML Configuration$/i, pageName: "SAML Configuration" },
    "auth-hooks": { buttonName: /^Auth Hooks$/i, pageName: "Auth Hooks" },
  } as const satisfies Record<string, RegistryScreenScan>;

  const uiRoot = resolve(fileURLToPath(import.meta.url), "..", "..", "..");

  function adminViewIdsFromRegistrySource(): string[] {
    const registrySource = readFileSync(
      resolve(uiRoot, "src", "screens", "registry.ts"),
      "utf8",
    );
    const adminViewsMatch = registrySource.match(/export const ADMIN_VIEWS = \[([\s\S]*?)\] as const;/);
    if (!adminViewsMatch) {
      throw new Error("Unable to read ADMIN_VIEWS from ui/src/screens/registry.ts");
    }
    return Array.from(adminViewsMatch[1].matchAll(/"([^"]+)"/g), (match) => match[1]);
  }

  test("registry accessibility scan metadata covers every admin view", () => {
    const adminViewIds = adminViewIdsFromRegistrySource();
    const scannedIds = Object.keys(registryScreenScans);
    const missing = adminViewIds.filter((id) => !scannedIds.includes(id));

    console.log(`A11Y_REGISTRY_METADATA:${scannedIds.length}/${adminViewIds.length}`);
    expect(
      missing,
      `missing: ${missing.join(", ")}; covered/total: ${scannedIds.length}/${adminViewIds.length}`,
    ).toEqual([]);
  });

  function isCapabilityEnabled(
    capabilities: AdminCapabilities,
    requiredCapability: AdminCapabilityName | undefined,
  ): boolean {
    return requiredCapability ? capabilities[requiredCapability] : true;
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const tableName of pendingCleanupTables) {
      await dropTableIfExists(request, adminToken, tableName, "a11y table-browser cleanup").catch(
        () => {},
      );
    }
    pendingCleanupTables.length = 0;
  });

  /**
   * Run axe-core on the current page and assert zero critical/serious
   * violations. Logs moderate/minor issues as warnings without failing.
   */
  async function assertAccessible(page: Page, pageName: string) {
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
      // CodeMirror owns its internal editor DOM; the surrounding dashboard chrome remains scanned.
      .exclude(".cm-editor")
      .analyze();

    const critical = results.violations.filter(
      (violation) => violation.impact === "critical" || violation.impact === "serious",
    );
    const minor = results.violations.filter(
      (violation) => violation.impact === "moderate" || violation.impact === "minor",
    );

    if (minor.length > 0) {
      console.log(
        `[a11y] ${pageName}: ${minor.length} moderate/minor issue(s):`,
        minor.map((violation) => `${violation.id}: ${violation.help} (${violation.nodes.length} node(s))`),
      );
    }

    expect(
      critical,
      `${pageName}: ${critical.length} critical/serious a11y violation(s): ${critical
        .map((violation) => `${violation.id}: ${violation.help}`)
        .join("; ")}`,
    ).toHaveLength(0);
  }

  /**
   * Navigate to a dashboard page via a sidebar button and run an axe scan.
   */
  async function navigateAndScan(page: Page, buttonName: RegExp, pageName: string) {
    const sidebar = page.locator("aside");
    const button = sidebar.getByRole("button", { name: buttonName });

    await expect(button, `Expected sidebar target ${buttonName} for ${pageName}`).toBeVisible({
      timeout: 5000,
    });
    await button.click();
    await expect(button).toHaveClass(/font-medium/, { timeout: 5000 });
    await page.locator("main").waitFor({ state: "visible", timeout: 5000 });
    await assertAccessible(page, pageName);
  }

  test("dashboard home page is accessible", async ({ page }) => {
    await page.goto("/admin/");
    await waitForDashboard(page);
    await assertAccessible(page, "Dashboard Home");
  });

  test("table browser page is accessible", async ({ page, request, adminToken }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `a11y_table_browser_${runID}`;
    pendingCleanupTables.push(tableName);

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );

      INSERT INTO ${tableName} (title) VALUES ('a11y seeded row ${runID}');`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    const sidebar = page.locator("aside");
    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    const tableButton = sidebar.getByRole("button", { name: tableName, exact: true });

    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await expect
      .poll(
        async () => {
          await refreshButton.click();
          return tableButton.isVisible();
        },
        { timeout: 15_000 },
      )
      .toBe(true);

    await tableButton.click();
    await page.locator("main").waitFor({ state: "visible", timeout: 5000 });
    await assertAccessible(page, "Table Browser");
  });

  for (const [registryId, scan] of Object.entries(registryScreenScans)) {
    test(`registry: ${scan.pageName} page is accessible`, async ({ page, request, adminToken }) => {
      if (scan.requires) {
        const capabilities = await getAdminCapabilities(request, adminToken);
        if (!isCapabilityEnabled(capabilities, scan.requires)) {
          console.log(`CAPABILITY_DISABLED:${registryId}`);
          return;
        }
      }

      await page.goto("/admin/");
      await waitForDashboard(page);
      await navigateAndScan(page, scan.buttonName, scan.pageName);
    });
  }
});
