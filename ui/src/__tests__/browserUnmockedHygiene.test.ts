import { describe, it, expect, vi } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import {
  failIfReadinessForced,
  readinessNotMet,
} from "../../browser-tests-unmocked/fixtures/conditional-readiness";

function readProjectFile(relativePath: string): string {
  return readFileSync(resolve(__dirname, "..", "..", relativePath), "utf8");
}

function readFullLifecycleSpecFiles(): Array<{ name: string; content: string }> {
  const fullSpecDirectory = resolve(__dirname, "..", "..", "browser-tests-unmocked/full");
  return readdirSync(fullSpecDirectory)
    .filter((name) => name.endsWith(".spec.ts"))
    .map((name) => ({
      name,
      content: readFileSync(resolve(fullSpecDirectory, name), "utf8"),
    }));
}

describe("browser-unmocked test hygiene", () => {
  it("records structured readiness evidence before failing the test", async () => {
    const attach = vi.fn().mockResolvedValue(undefined);

    await expect(
      readinessNotMet({ attach }, "usage", "usage aggregation service returned 503"),
    ).rejects.toThrow(/^READINESS_NOT_MET:/);
    expect(attach).toHaveBeenCalledWith("readiness-not-met", {
      body: JSON.stringify({
        kind: "READINESS_NOT_MET",
        screenID: "usage",
        reason: "usage aggregation service returned 503",
      }),
      contentType: "application/json",
    });
  });

  it("forces readiness failures through the shared evidence owner", async () => {
    const attach = vi.fn().mockResolvedValue(undefined);
    vi.stubEnv("AYB_BROWSER_FORCE_READINESS_NOT_MET", "usage, admin-logs");

    await expect(failIfReadinessForced({ attach }, "admin-logs")).rejects.toThrow(
      "READINESS_NOT_MET: admin-logs:",
    );
    expect(attach).toHaveBeenCalledWith("readiness-not-met", {
      body: JSON.stringify({
        kind: "READINESS_NOT_MET",
        screenID: "admin-logs",
        reason: "admin-logs backend forced unavailable for readiness proof",
      }),
      contentType: "application/json",
    });

    vi.unstubAllEnvs();
  });

  it("does not log admin credentials or token fragments in auth setup", () => {
    const authSetup = readProjectFile("browser-tests-unmocked/auth.setup.ts");

    expect(authSetup).not.toMatch(/Using admin password:/);
    expect(authSetup).not.toMatch(/Admin token in localStorage:/);
  });

  it("cleans up MFA lifecycle user in test.afterEach", () => {
    const spec = readProjectFile("browser-tests-unmocked/full/auth-mfa-lifecycle.spec.ts");
    expect(spec).toContain("test.afterEach(");
    expect(spec).toContain("await mfaHelpers.cleanupAuthUser(email)");
    expect(spec).not.toContain("finally {");
  });

  it("steps up to AAL2 before backup code and additional MFA enrollment actions", () => {
    const spec = readProjectFile("browser-tests-unmocked/full/auth-mfa-lifecycle.spec.ts");
    expect(spec).toContain("await mfaHelpers.promoteSessionToAAL2WithTOTP(");
  });

  it("uses token-based readiness and waits for dashboard navigation to mount", () => {
    const authSetup = readProjectFile("browser-tests-unmocked/auth.setup.ts");
    expect(authSetup).not.toContain("waitForURL(/\\/admin\\/.+/");
    expect(authSetup).toContain("localStorage.getItem(\"ayb_admin_token\")");
    expect(authSetup).toContain('getByRole("navigation")');
  });

  it("auth setup bootstraps with saved admin token before password fallback", () => {
    const authSetup = readProjectFile("browser-tests-unmocked/auth.setup.ts");
    const adminBootstrap = readProjectFile("browser-tests-unmocked/admin-bootstrap.ts");

    expect(authSetup).toContain("resolveAdminBootstrapCredential");
    expect(authSetup).toContain("bootstrapWithSavedToken");
    expect(adminBootstrap).toContain("resolveAdminPasswordForBrowserLogin");
  });

  it("admin login smoke does not treat saved bearer auth as a password", () => {
    const adminLoginSmoke = readProjectFile("browser-tests-unmocked/smoke/admin-login.spec.ts");

    expect(adminLoginSmoke).toContain("resolveAdminPasswordForBrowserLogin");
    expect(adminLoginSmoke).not.toContain('from "fs"');
    expect(adminLoginSmoke).not.toContain('from "path"');
    expect(adminLoginSmoke).not.toContain('from "os"');
    expect(adminLoginSmoke).toContain("positive admin password login requires");
  });

  it("Stage 4 smoke specs use waitForDashboard instead of inline brand-text check", () => {
    const stage4Specs = [
      "functions-list", "email-templates-list", "push-devices", "ai-assistant",
      "extensions", "vector-indexes", "auth-hooks", "api-explorer-view",
      "auth-settings-view", "mfa-management-view", "account-linking-view",
      "realtime-inspector-view", "security-advisor-view", "performance-advisor-view",
    ];
    for (const specName of stage4Specs) {
      const content = readProjectFile(`browser-tests-unmocked/smoke/${specName}.spec.ts`);
      expect(content).not.toContain('getByText("Allyourbase")');
      expect(content).toContain("waitForDashboard");
    }
  });

  it("waitForDashboard helper is exported from fixtures barrel", () => {
    const barrel = readProjectFile("browser-tests-unmocked/fixtures/index.ts");
    const core = readProjectFile("browser-tests-unmocked/fixtures/core.ts");
    expect(core).toContain("waitForDashboard");
    expect(barrel).toMatch(/export \*.*core/);
  });

  it("collections CRUD tests isolate table names across parallel workers", () => {
    const collectionsCRUD = readProjectFile("browser-tests-unmocked/full/collections-crud.spec.ts");

    expect(collectionsCRUD).toContain("buildParallelSafeRunID");
    expect(collectionsCRUD.match(/buildParallelSafeRunID\(testInfo\)/g)).toHaveLength(2);
    expect(collectionsCRUD).not.toContain("DROP TABLE IF EXISTS crud_test_products;");
  });

  it("virtual authenticator helper is exported from auth fixtures and top-level barrel", () => {
    const authFixtures = readProjectFile("browser-tests-unmocked/fixtures/auth.ts");
    const topLevelFixtures = readProjectFile("browser-tests-unmocked/fixtures.ts");
    expect(authFixtures).toContain("export async function createVirtualAuthenticator(");
    expect(topLevelFixtures).toContain('export * from "./fixtures/index"');
  });

  it("accessibility smoke waits for sidebar admin navigation to activate before scanning", () => {
    const accessibilitySmoke = readProjectFile("browser-tests-unmocked/smoke/accessibility.spec.ts");

    expect(accessibilitySmoke).toContain('getByRole("button", { name: buttonName })');
    expect(accessibilitySmoke).toContain("Expected sidebar target");
    expect(accessibilitySmoke).toContain('await expect(button).toHaveClass(/font-medium/');
  });

  it("Stage 5 accessibility smoke covers the expanded dashboard suite", () => {
    const accessibilitySmoke = readProjectFile("browser-tests-unmocked/smoke/accessibility.spec.ts");

    expect(accessibilitySmoke).toContain("registryScreenScans");
    expect(accessibilitySmoke).toContain("registry accessibility scan metadata covers every admin view");
    expect(accessibilitySmoke).toContain("buildParallelSafeRunID");
    expect(accessibilitySmoke).toContain("dropTableIfExists");
    expect(accessibilitySmoke).toContain('test("table browser page is accessible"');
    expect(accessibilitySmoke).toContain('graphql: { buttonName: /^GraphQL$/i, pageName: "GraphQL" }');
    expect(accessibilitySmoke).toContain('search: { buttonName: /^Search$/i, pageName: "Search" }');
    expect(accessibilitySmoke).toContain('requires: "status"');
    expect(accessibilitySmoke).toContain('requires: "support"');
    expect(accessibilitySmoke).toContain("CAPABILITY_DISABLED:");
    expect(accessibilitySmoke).not.toContain('/Table Editor/i');
    expect(accessibilitySmoke).not.toContain('test("settings page is accessible"');
    expect(accessibilitySmoke).not.toContain('navigateAndScan(page, /SQL Editor/i, "SQL Editor")');
  });

  it("run-with-ayb script sets high default API/auth limits for parallel browser suites", () => {
    const runner = readProjectFile("../scripts/run-with-ayb.sh");

    expect(runner).toContain('export AYB_AUTH_ENABLED="${AYB_AUTH_ENABLED:-true}"');
    expect(runner).toContain('export AYB_AUTH_RATE_LIMIT="${AYB_AUTH_RATE_LIMIT:-10000}"');
    expect(runner).toContain('export AYB_AUTH_ANONYMOUS_RATE_LIMIT="${AYB_AUTH_ANONYMOUS_RATE_LIMIT:-10000}"');
    expect(runner).toContain('export AYB_RATE_LIMIT_API="${AYB_RATE_LIMIT_API:-10000/min}"');
    expect(runner).toContain(
      'export AYB_RATE_LIMIT_API_ANONYMOUS="${AYB_RATE_LIMIT_API_ANONYMOUS:-10000/min}"',
    );
    expect(runner).toContain('export AYB_STATUS_ENABLED="${AYB_STATUS_ENABLED:-true}"');
    expect(runner).toContain('export AYB_SUPPORT_ENABLED="${AYB_SUPPORT_ENABLED:-true}"');
  });

  it("runs unmocked projects serially while smoke specs mutate shared backend relations", () => {
    const playwrightConfig = readProjectFile("playwright.config.ts");

    expect(playwrightConfig).toContain("fullyParallel: false");
    expect(playwrightConfig).toContain("workers: 1");
  });

  it("shared runner records the replica smoke capability as disabled without a standby", () => {
    const runner = readProjectFile("../scripts/run-with-ayb.sh");
    const replicas = readProjectFile("browser-tests-unmocked/smoke/replicas.spec.ts");

    expect(runner).toContain(
      'export AYB_BROWSER_DISABLED_CAPABILITIES="${AYB_BROWSER_DISABLED_CAPABILITIES-replicas}"',
    );
    expect(replicas).toContain('console.log("CAPABILITY_DISABLED:replicas")');
    expect(replicas.indexOf('failIfReadinessForced(testInfo, "replicas")')).toBeLessThan(
      replicas.indexOf('disabledCapabilities.has("replicas")'),
    );
  });

  it("CI browser job starts AYB with a WebAuthn-compatible localhost public origin", () => {
    const ciWorkflow = readProjectFile("../.github/workflows/ci.yml");

    expect(ciWorkflow).toContain("AYB_START_COMMAND: ./ayb start --foreground --host 0.0.0.0");
    expect(ciWorkflow).toContain("bash scripts/run-with-ayb.sh 'cd ui && npm run test:browser -- --project=smoke");
    expect(ciWorkflow).toContain("bash scripts/run-with-ayb.sh 'cd ui && npm run test:browser -- --project=full");
  });

  it("apps and api-keys smoke specs guard missing backend routes with endpoint probes", () => {
    const appsList = readProjectFile("browser-tests-unmocked/smoke/apps-list.spec.ts");
    const apiKeysList = readProjectFile("browser-tests-unmocked/smoke/api-keys-list.spec.ts");

    expect(appsList).toContain("probeEndpoint");
    expect(appsList).toContain('"/api/admin/apps"');
    expect(appsList).toContain("test.skip(");

    expect(apiKeysList).toContain("probeEndpoint");
    expect(apiKeysList).toContain('"/api/admin/api-keys"');
    expect(apiKeysList).toContain("test.skip(");
  });

  it("users smoke spec guards missing backend routes with endpoint probe", () => {
    const usersList = readProjectFile("browser-tests-unmocked/smoke/users-list.spec.ts");

    expect(usersList).toContain("probeEndpoint");
    expect(usersList).toContain('"/api/admin/users"');
    expect(usersList).not.toContain('"/api/admin/users/"');
    expect(usersList).toContain("test.skip(");
  });

  it("conditional-readiness smoke specs fail non-green instead of passing on absent backends", () => {
    const fixturesBarrel = readProjectFile("browser-tests-unmocked/fixtures/index.ts");
    const conditionalReadinessSpecs = [
      "backups",
      "replicas",
      "vector-indexes",
      "notifications",
      "usage-metering",
      "admin-logs",
    ];

    expect(fixturesBarrel).toContain('export * from "./conditional-readiness"');
    for (const specName of conditionalReadinessSpecs) {
      const spec = readProjectFile(`browser-tests-unmocked/smoke/${specName}.spec.ts`);
      expect(spec).toMatch(
        /import\s*\{\s*[^}]*\breadinessNotMet\b[^}]*\}\s*from\s*"\.\.\/fixtures"/,
      );
      expect(spec).toContain("failIfReadinessForced");
      expect(spec).not.toContain("test.skip(");
    }

    const usageMetering = readProjectFile("browser-tests-unmocked/smoke/usage-metering.spec.ts");
    expect(usageMetering).not.toContain("503 fallback");
    expect(usageMetering).not.toMatch(
      /if\s*\(\s*usageResponse\.status\(\)\s*===\s*503\s*\)\s*\{[^}]*\breturn\b/,
    );

    const adminLogs = readProjectFile("browser-tests-unmocked/smoke/admin-logs.spec.ts");
    expect(adminLogs).not.toMatch(
      /if\s*\(\s*isBufferingUnavailable\(initialPayload\)\s*\)\s*\{[^}]*\breturn\b/,
    );
  });

  it("full storage lifecycle does not preflight-skip before bucket setup", () => {
    const storageLifecycle = readProjectFile("browser-tests-unmocked/full/storage-lifecycle.spec.ts");

    expect(storageLifecycle).not.toContain("probeEndpoint");
    expect(storageLifecycle).not.toMatch(/probeStatus\s*===\s*(404|500|501|503)/);
    expect(storageLifecycle).toContain("ensureStorageBucket");
  });

  it("full and smoke users proofs probe the same mounted users list route", () => {
    const usersLifecycle = readProjectFile("browser-tests-unmocked/full/users-lifecycle.spec.ts");
    const usersList = readProjectFile("browser-tests-unmocked/smoke/users-list.spec.ts");

    expect(usersLifecycle).toContain('"/api/admin/users"');
    expect(usersLifecycle).not.toContain('"/api/admin/users/"');
    expect(usersList).toContain('"/api/admin/users"');
    expect(usersList).not.toContain('"/api/admin/users/"');
  });

  it("edge-function lookup fixture resolves functions via by-name admin endpoint", () => {
    const edgeFixtures = readProjectFile("browser-tests-unmocked/fixtures/edge-functions.ts");

    expect(edgeFixtures).toContain("/api/admin/functions/by-name/");
    expect(edgeFixtures).not.toContain('request.get("/api/admin/functions"');
  });

  it("edge-function fixture does not default seeded functions to public access", () => {
    const edgeFixtures = readProjectFile("browser-tests-unmocked/fixtures/edge-functions.ts");

    expect(edgeFixtures).toContain("public: overrides.public ?? false");
    expect(edgeFixtures).not.toContain("public: overrides.public ?? true");
  });

  it("changed lifecycle specs use waitForDashboard and avoid inline brand-text checks", () => {
    const changedLifecycleSpecs = [
      "browser-tests-unmocked/full/jobs-management.spec.ts",
      "browser-tests-unmocked/full/schedules-lifecycle.spec.ts",
      "browser-tests-unmocked/full/webhooks-lifecycle.spec.ts",
    ];

    for (const relativePath of changedLifecycleSpecs) {
      const content = readProjectFile(relativePath);
      expect(content).not.toContain('getByText("Allyourbase")');
      expect(content).toMatch(
        /import\s*\{\s*[^}]*\bwaitForDashboard\b[^}]*\}\s*from\s*"\.\.\/fixtures"/,
      );
    }
  });

  it("edge trigger lifecycle spec asserts the disabled cron manual-run rejection", () => {
    const spec = readProjectFile("browser-tests-unmocked/full/edge-function-triggers.spec.ts");

    expect(spec).toContain("assertDisabledAction");
    expect(spec).toContain("cron trigger is disabled");
    expect(spec).toContain('page.getByTestId("toast")');
  });

  it("Stage 4 auth smokes respect auth config and seed auth state deliberately", () => {
    const accountLinking = readProjectFile("browser-tests-unmocked/smoke/account-linking-view.spec.ts");
    const edgeFunctionsPrivateAuth = readProjectFile("browser-tests-unmocked/smoke/edge-functions-private-auth.spec.ts");
    const mfaManagement = readProjectFile("browser-tests-unmocked/smoke/mfa-management-view.spec.ts");

    expect(accountLinking).toContain("fetchAuthSettings");
    expect(accountLinking).toContain("anonymous_auth_enabled");

    expect(edgeFunctionsPrivateAuth).toContain("getAuthSettingsUnavailableSkipReason");
    expect(edgeFunctionsPrivateAuth).toContain("fetchAuthSettings");
    expect(edgeFunctionsPrivateAuth).toContain("anonymous_auth_enabled");

    expect(mfaManagement).toContain("fetchAuthSettings");
    expect(mfaManagement).toContain("createLinkedEmailAuthSessionToken");
    expect(mfaManagement).not.toContain("createAnonymousAuthSessionToken");
    expect(mfaManagement).toContain("totp_enabled");
    expect(mfaManagement).toContain("ayb_auth_token");
  });

  it("auth-dependent full browser proofs gate on the shared auth-settings availability helper", () => {
    const authFixtures = readProjectFile("browser-tests-unmocked/fixtures/auth.ts");
    const dashboardRLS = readProjectFile("browser-tests-unmocked/full/dashboard-rls-denial-journey.spec.ts");
    const oauthRestrictions = readProjectFile("browser-tests-unmocked/full/oauth-auth-restrictions.spec.ts");
    const secretsRotation = readProjectFile("browser-tests-unmocked/full/secrets-jwt-rotation.spec.ts");

    expect(authFixtures).toContain("getAuthSettingsUnavailableSkipReason");
    expect(authFixtures).toContain('"/api/admin/auth-settings"');

    expect(dashboardRLS).toContain("getAuthSettingsUnavailableSkipReason");
    expect(dashboardRLS).toContain("test.skip(Boolean(authSettingsSkipReason)");

    expect(oauthRestrictions).toContain("getAuthSettingsUnavailableSkipReason");
    expect(oauthRestrictions).toContain("test.skip(Boolean(authSettingsSkipReason)");

    expect(secretsRotation).toContain("getAuthSettingsUnavailableSkipReason");
    expect(secretsRotation).toContain("test.skip(Boolean(authSettingsSkipReason)");
  });

  it("email template smoke uses a valid dotted custom template key", () => {
    const emailTemplates = readProjectFile("browser-tests-unmocked/smoke/email-templates-list.spec.ts");
    expect(emailTemplates).toContain("smoke.template_");
    expect(emailTemplates).not.toContain("smoke_template_");
  });

  it("Stage 5 config-heavy full lifecycle spec files exist and use waitForDashboard", () => {
    const lifecycleSpecs = readFullLifecycleSpecFiles();
    const stage5SpecNames = [
      "analytics-lifecycle.spec.ts",
      "audit-logs-lifecycle.spec.ts",
      "schema-designer-lifecycle.spec.ts",
      "advisors-lifecycle.spec.ts",
      "realtime-inspector-lifecycle.spec.ts",
    ];

    for (const specName of stage5SpecNames) {
      const spec = lifecycleSpecs.find((candidate) => candidate.name === specName);
      expect(spec, `${specName} should exist in browser-tests-unmocked/full`).toBeDefined();
      expect(spec?.content).toContain("waitForDashboard");
    }
  });

  it("passkey lifecycle full spec exists and exercises shared auth fixture owners", () => {
    const passkeySpec = readProjectFile("browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts");
    expect(passkeySpec).toContain("test.step(");
    expect(passkeySpec).toContain("getAuthSettingsUnavailableSkipReason");
    expect(passkeySpec).toContain("mfaHelpers.ensureAuthSettings");
    expect(passkeySpec).toContain("waitForDashboard");
    expect(passkeySpec).toContain("createVirtualAuthenticator");
  });

  it("webhook browser specs avoid external network targets and keep explicit success assertions", () => {
    const lifecycle = readProjectFile("browser-tests-unmocked/full/webhooks-lifecycle.spec.ts");
    const smoke = readProjectFile("browser-tests-unmocked/smoke/webhooks-crud.spec.ts");

    expect(lifecycle).not.toContain("httpbin.org");
    expect(smoke).not.toContain("httpbin.org");
    expect(lifecycle).toContain("startWebhookTargetServer");
    expect(lifecycle).not.toContain("__webhook-target__");
    expect(smoke).not.toContain("__webhook-target__");
    expect(lifecycle).toContain('getByText(/test passed/i)');
    expect(lifecycle).toContain("webhookTarget.getRequestCount(), { timeout: 10000 }).toBe(2)");
    expect(lifecycle).toContain('expect(testDeliveryRequest.url).toBe(`/updated-${runId}`)');
    expect(lifecycle).toContain('expect(testDeliveryRequest.headers["x-ayb-signature"]).toBe(expectedTestSignature)');
    expect(lifecycle).not.toContain("test (passed|failed)|test request failed");
  });
});

describe("vitest config concurrency contract", () => {
  it("bounds the Vitest worker pool with a CI-gated top-level maxWorkers", () => {
    const viteConfig = readProjectFile("vite.config.ts");

    // The CI runner memory envelope requires a bounded worker pool: an
    // unbounded fork worker stayed alive and exhausted ~4 GiB on the public CI
    // run. The bound must be top-level `maxWorkers`, gated on CI so local
    // developer runs stay unbounded, with an optional VITEST_MAX_WORKERS
    // override parsed in this same config file.
    expect(viteConfig).toMatch(/maxWorkers/);
    expect(viteConfig).toContain("process.env.CI");
    expect(viteConfig).toContain("VITEST_MAX_WORKERS");
  });

  it("does not reintroduce removed Vitest 4 poolOptions wiring", () => {
    const viteConfig = readProjectFile("vite.config.ts");
    expect(viteConfig).not.toContain("poolOptions");
  });
});
