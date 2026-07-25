import {
  adminPath,
  buildParallelSafeRunID,
  cleanupAuthUser,
  deleteFile,
  ensureLinkedEmailAuthUser,
  execSQL,
  expect,
  openTableFromSidebar,
  probeEndpoint,
  seedFile,
  sqlLiteral,
  test,
  waitForDashboard,
} from "../fixtures";
import type {
  APIRequestContext,
  ConsoleMessage,
  Page,
  Response,
  TestInfo,
} from "@playwright/test";

const EXPECTED_CSP_POLICY =
  "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'";
const GRAPHQL_PATH = "/api/graphql";
const REALTIME_STATS_PATH = "/api/admin/realtime/stats";
const REALTIME_WS_PATH = "/api/realtime/ws";
const CSP_SENTINEL_HOST = "ayb-csp-enforcing-sentinel.invalid";
const CSP_SENTINEL_URL = `https://${CSP_SENTINEL_HOST}/finalize.png`;
const STORAGE_BUCKET = "default";
const ONE_PIXEL_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
  "base64",
);

interface CspViolationRecord {
  effectiveDirective: string;
  blockedURI: string;
  sourceFile: string;
  lineNumber: number;
  columnNumber: number;
  violatedDirective: string;
  disposition: string;
}

interface RealtimeWebSocketMessage {
  type?: string;
  ref?: string;
  status?: string;
  message?: string;
}

interface BrowserRealtimeWebSocketResult {
  closeCode: number | null;
  closeReason: string;
  connected: boolean;
  error: string;
  messages: RealtimeWebSocketMessage[];
  opened: boolean;
  subscribed: boolean;
  urlPath: string;
}

declare global {
  interface Window {
    __recordBrowserRealtimeWebSocketResult: (
      result: BrowserRealtimeWebSocketResult,
    ) => Promise<void>;
    __recordCspSentinelViolation: (record: CspViolationRecord) => Promise<void>;
    __recordCspViolation: (record: CspViolationRecord) => Promise<void>;
  }
}

const isSentinelViolation = (record: CspViolationRecord) =>
  record.blockedURI.includes(CSP_SENTINEL_HOST);

const isSentinelConsoleMessage = (message: string) =>
  message.includes(CSP_SENTINEL_HOST);

const redactTokenQuery = (value: string) =>
  value.replace(/([?&]token=)[^&\s")]+/g, "$1redacted");

interface BrowserCollectors {
  browserRealtimeWebSocketPromise: Promise<BrowserRealtimeWebSocketResult>;
  cspConsoleMessages: string[];
  triggerCspSentinelViolation: () => Promise<void>;
  violations: CspViolationRecord[];
}

interface DashboardScreenFixtures {
  functionName: string | null;
  graphql: {
    expectedEnvelope: unknown;
    query: string;
    tableName: string;
    variables: Record<string, string>;
  } | null;
  runID: string;
  storageImageFileName: string | null;
  tableName: string;
  tableTitle: string;
}

// The smoke project configures a global one-retry (ui/playwright.config.ts).
// A first-attempt CSP violation captured on attempt 1 must not be masked by a
// clean retry, so this oracle opts out of retries entirely.
test.describe.configure({ retries: 0 });

async function createBrowserRealtimeToken(
  request: APIRequestContext,
  email: string,
): Promise<string> {
  const session = await ensureLinkedEmailAuthUser(
    request,
    email,
    `CspRealtime!${Date.now()}`,
  );
  return session.token;
}

async function installBrowserCollectors(
  page: Page,
  browserRealtimeToken: string,
): Promise<BrowserCollectors> {
  const violations: CspViolationRecord[] = [];
  const cspConsoleMessages: string[] = [];
  let resolveBrowserRealtimeWebSocket: (
    result: BrowserRealtimeWebSocketResult,
  ) => void;
  const browserRealtimeWebSocketPromise = new Promise<BrowserRealtimeWebSocketResult>((resolve) => {
    resolveBrowserRealtimeWebSocket = resolve;
  });
  let resolveSentinelViolation: (record: CspViolationRecord) => void;
  const sentinelViolationPromise = new Promise<CspViolationRecord>((resolve) => {
    resolveSentinelViolation = resolve;
  });

  await exposeCollectorBindings(page, {
    onBrowserRealtimeWebSocket: (result) => resolveBrowserRealtimeWebSocket(result),
    onSentinelViolation: (record) => resolveSentinelViolation(record),
    onViolation: (record) => violations.push(record),
  });
  await installBrowserInitScript(page, browserRealtimeToken);
  collectEnforcingCspConsoleMessages(page, cspConsoleMessages);

  return {
    browserRealtimeWebSocketPromise,
    cspConsoleMessages,
    triggerCspSentinelViolation: () =>
      triggerCspSentinelViolation(page, sentinelViolationPromise),
    violations,
  };
}

async function exposeCollectorBindings(
  page: Page,
  callbacks: {
    onBrowserRealtimeWebSocket: (result: BrowserRealtimeWebSocketResult) => void;
    onSentinelViolation: (record: CspViolationRecord) => void;
    onViolation: (record: CspViolationRecord) => void;
  },
): Promise<void> {
  await page.exposeFunction("__recordCspSentinelViolation", callbacks.onSentinelViolation);
  await page.exposeFunction("__recordBrowserRealtimeWebSocketResult", callbacks.onBrowserRealtimeWebSocket);
  await page.exposeFunction("__recordCspViolation", callbacks.onViolation);
}

async function installBrowserInitScript(
  page: Page,
  browserRealtimeToken: string,
): Promise<void> {
  await page.addInitScript(
    ({ realtimeWsPath, realtimeWsToken, sentinelHost, sentinelURL }) => {
      const redactTokenQuery = (value: string) =>
        value.replace(/([?&]token=)[^&\s")]+/g, "$1redacted");
      const websocketResult = {
        closeCode: null as number | null,
        closeReason: "",
        connected: false,
        error: "",
        messages: [] as RealtimeWebSocketMessage[],
        opened: false,
        subscribed: false,
        urlPath: realtimeWsPath,
      };
      const finishWebSocketProbe = () => {
        void window.__recordBrowserRealtimeWebSocketResult({
          ...websocketResult,
        });
      };
      const buildRealtimeWebSocketURL = () => {
        const wsURL = new URL(realtimeWsPath, window.location.href);
        wsURL.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        wsURL.searchParams.set("token", realtimeWsToken);
        return wsURL;
      };
      const openBrowserRealtimeWebSocket = () => {
        const wsURL = buildRealtimeWebSocketURL();
        websocketResult.urlPath = wsURL.pathname;
        let socket: WebSocket;
        try {
          socket = new WebSocket(wsURL);
        } catch (error) {
          websocketResult.error = error instanceof Error ? error.message : String(error);
          finishWebSocketProbe();
          return;
        }
        const timeout = window.setTimeout(() => {
          websocketResult.error = "Timed out waiting for browser WebSocket subscribe acknowledgement";
          socket.close();
          finishWebSocketProbe();
        }, 10_000);
        socket.addEventListener("open", () => {
          websocketResult.opened = true;
        });
        socket.addEventListener("message", (event) => {
          let message: RealtimeWebSocketMessage;
          try {
            message = JSON.parse(String(event.data)) as RealtimeWebSocketMessage;
          } catch {
            return;
          }
          websocketResult.messages.push(message);
          if (message.type === "connected") {
            websocketResult.connected = true;
            socket.send(
              JSON.stringify({
                type: "subscribe",
                ref: "csp-browser-ws",
                tables: ["users"],
              }),
            );
            return;
          }
          if (
            message.type === "reply" &&
            message.ref === "csp-browser-ws" &&
            message.status === "ok"
          ) {
            websocketResult.subscribed = true;
            window.clearTimeout(timeout);
            socket.close();
            finishWebSocketProbe();
          }
        });
        socket.addEventListener("close", (event) => {
          websocketResult.closeCode = event.code;
          websocketResult.closeReason = event.reason;
          if (!websocketResult.subscribed && websocketResult.error.length === 0) {
            websocketResult.error = `Browser WebSocket closed before subscribe acknowledgement (${event.code} ${event.reason})`;
            window.clearTimeout(timeout);
            finishWebSocketProbe();
          }
        });
        socket.addEventListener("error", () => {
          if (websocketResult.error.length === 0) {
            websocketResult.error = "Browser WebSocket emitted an error event";
          }
        });
      };
      const installFinalizer = () => {
        document.addEventListener(
          "click",
          (event) => {
            if (!(event instanceof MouseEvent) || !event.altKey) return;
            if (event.shiftKey) {
              openBrowserRealtimeWebSocket();
              return;
            }
            const image = new Image();
            image.alt = "";
            image.src = `${sentinelURL}?nonce=${crypto.randomUUID()}`;
            document.body.appendChild(image);
          },
          { capture: true },
        );
      };
      if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", installFinalizer, { once: true });
      } else {
        installFinalizer();
      }
      const pendingDeliveries = new Set<Promise<void>>();
      const isSentinelRecord = (record: CspViolationRecord) =>
        record.blockedURI.includes(sentinelHost);
      document.addEventListener("securitypolicyviolation", (event) => {
        const record = {
          effectiveDirective: event.effectiveDirective,
          blockedURI: redactTokenQuery(event.blockedURI),
          sourceFile: event.sourceFile,
          lineNumber: event.lineNumber,
          columnNumber: event.columnNumber,
          violatedDirective: event.violatedDirective,
          disposition: event.disposition,
        };
        const delivery = window.__recordCspViolation(record).finally(() => {
          pendingDeliveries.delete(delivery);
        });
        pendingDeliveries.add(delivery);
        if (isSentinelRecord(record)) {
          const pendingAtSentinel = Array.from(pendingDeliveries);
          void Promise.all(pendingAtSentinel).then(() => void window.__recordCspSentinelViolation(record));
        }
      });
    },
    {
      realtimeWsPath: REALTIME_WS_PATH,
      realtimeWsToken: browserRealtimeToken,
      sentinelHost: CSP_SENTINEL_HOST,
      sentinelURL: CSP_SENTINEL_URL,
    },
  );
}

function collectEnforcingCspConsoleMessages(
  page: Page,
  messages: string[],
): void {
  page.on("console", (message: ConsoleMessage) => {
    const text = message.text();
    if (
      text.includes("Content Security Policy") &&
      text.includes("Refused to") &&
      !isSentinelConsoleMessage(text)
    ) {
      messages.push(redactTokenQuery(text));
    }
  });
}

async function triggerCspSentinelViolation(
  page: Page,
  sentinelViolationPromise: Promise<CspViolationRecord>,
): Promise<void> {
  await page.getByRole("button", { name: "Search... K" }).click({
    modifiers: ["Alt"],
  });
  const sentinelViolation = await sentinelViolationPromise;
  expect(sentinelViolation.disposition).toBe("enforce");
  expect(isSentinelViolation(sentinelViolation)).toBeTruthy();
}

function waitForAdminDocumentCspHeader(
  page: Page,
  adminDocumentPath: string,
): Promise<string> {
  return new Promise<string>((resolve) => {
    const onResponse = (response: Response) => {
      if (response.request().resourceType() !== "document") {
        return;
      }
      const pathname = new URL(response.url()).pathname;
      if (pathname !== adminDocumentPath) {
        return;
      }
      page.off("response", onResponse);
      resolve(response.headers()["content-security-policy"] ?? "");
    };
    page.on("response", onResponse);
  });
}

function createDashboardScreenFixtures(testInfo: TestInfo): DashboardScreenFixtures {
  const runID = buildParallelSafeRunID(testInfo);
  return {
    functionName: null,
    graphql: null,
    runID,
    storageImageFileName: null,
    tableName: `csp_table_${runID}`,
    tableTitle: `CSP table row ${runID}`,
  };
}

async function prepareDashboardScreenFixtures(
  request: APIRequestContext,
  adminToken: string,
  fixtures: DashboardScreenFixtures,
): Promise<void> {
  await execSQL(
    request,
    adminToken,
    `CREATE TABLE ${fixtures.tableName} (
      id integer PRIMARY KEY,
      title text NOT NULL,
      status text NOT NULL
    );
    INSERT INTO ${fixtures.tableName} (id, title, status)
    VALUES (1, '${sqlLiteral(fixtures.tableTitle)}', 'visible');`,
  );

  fixtures.functionName = await prepareFunctionFixture(request, adminToken, fixtures.runID);
  fixtures.graphql = await prepareGraphqlFixture(request, adminToken, fixtures.runID);
  await prepareStorageFixture(request, adminToken, fixtures);
}

async function prepareFunctionFixture(
  request: APIRequestContext,
  adminToken: string,
  runID: string,
): Promise<string | null> {
  const functionsStatus = await probeEndpoint(
    request,
    adminToken,
    "/api/admin/functions",
  );
  if ([404, 501, 503].includes(functionsStatus)) {
    return null;
  }
  expect(
    functionsStatus,
    `Functions precondition must be 200, received ${functionsStatus}`,
  ).toBe(200);

  const functionName = `csp_fn_${runID}`;
  await execSQL(
    request,
    adminToken,
    `CREATE OR REPLACE FUNCTION public.${functionName}(input_text text DEFAULT 'ok')
     RETURNS text
     LANGUAGE sql
     AS 'SELECT input_text'`,
  );
  return functionName;
}

async function prepareGraphqlFixture(
  request: APIRequestContext,
  adminToken: string,
  runID: string,
): Promise<DashboardScreenFixtures["graphql"]> {
  const graphqlStatus = await probeEndpoint(request, adminToken, GRAPHQL_PATH, {
    method: "POST",
    data: { query: "{ __typename }" },
  });
  if (
    graphqlStatus === 404 &&
    process.env.AYB_EXPECT_GRAPHQL_ENABLED === undefined
  ) {
    return null;
  }
  expect(
    graphqlStatus,
    `GraphQL precondition must be 200, received ${graphqlStatus}`,
  ).toBe(200);

  const tableName = `csp_graphql_${runID}`;
  const seed = {
    id: 7,
    score: 29,
    title: `CSP GraphQL row ${runID}`,
  };
  await execSQL(
    request,
    adminToken,
    `CREATE TABLE ${tableName} (
      id integer PRIMARY KEY,
      title text NOT NULL,
      score integer NOT NULL
    );
    INSERT INTO ${tableName} (id, title, score)
    VALUES (${seed.id}, '${sqlLiteral(seed.title)}', ${seed.score});`,
  );

  return {
    expectedEnvelope: { data: { [tableName]: [seed] } },
    query: `query CspGraphqlRow($title: String!) {
  ${tableName}(where: { title: { _eq: $title } }, limit: 1) {
    id
    title
    score
  }
}`,
    tableName,
    variables: { title: seed.title },
  };
}

async function prepareStorageFixture(
  request: APIRequestContext,
  adminToken: string,
  fixtures: DashboardScreenFixtures,
): Promise<void> {
  const fileName = `csp-preview-${fixtures.runID}.png`;
  fixtures.storageImageFileName = fileName;
  try {
    await seedFile(request, adminToken, STORAGE_BUCKET, fileName, ONE_PIXEL_PNG, {
      mimeType: "image/png",
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (
      message.includes("status 404") ||
      message.includes("status 415") ||
      message.includes("status 503")
    ) {
      fixtures.storageImageFileName = null;
      return;
    }
    throw error;
  }
}

async function cleanupDashboardScreenFixtures(
  request: APIRequestContext,
  adminToken: string,
  fixtures: DashboardScreenFixtures,
): Promise<void> {
  if (fixtures.storageImageFileName) {
    await deleteFile(
      request,
      adminToken,
      STORAGE_BUCKET,
      fixtures.storageImageFileName,
    ).catch(() => {});
  }
  if (fixtures.functionName) {
    await execSQL(
      request,
      adminToken,
      `DROP FUNCTION IF EXISTS public.${fixtures.functionName}(text)`,
    ).catch(() => {});
  }
  if (fixtures.graphql) {
    await execSQL(
      request,
      adminToken,
      `DROP TABLE IF EXISTS ${fixtures.graphql.tableName}`,
    ).catch(() => {});
  }
  await execSQL(
    request,
    adminToken,
    `DROP TABLE IF EXISTS ${fixtures.tableName}`,
  ).catch(() => {});
}

async function exerciseDashboardScreens(
  page: Page,
  fixtures: DashboardScreenFixtures,
  browserRealtimeWebSocketPromise: Promise<BrowserRealtimeWebSocketResult>,
): Promise<void> {
  await expect(page.getByRole("button", { name: "Search... K" })).toBeVisible();
  await exerciseSqlEditor(page);
  await exerciseTableBrowser(page, fixtures);
  await exerciseRealtimeInspector(page);
  await assertBrowserRealtimeWebSocket(page, browserRealtimeWebSocketPromise);
  if (fixtures.graphql) {
    await exerciseGraphqlExplorer(page, fixtures.graphql);
  }
  if (fixtures.functionName) {
    await exerciseFunctions(page, fixtures.functionName);
  }
  if (fixtures.storageImageFileName) {
    await exerciseStoragePreview(page, fixtures.storageImageFileName);
  }
}

async function exerciseSqlEditor(page: Page): Promise<void> {
  await page
    .locator("aside")
    .getByRole("button", { name: /^SQL Editor$/i })
    .click();
  const sqlQuery = page.getByLabel("SQL query");
  await expect(sqlQuery).toBeVisible({ timeout: 5000 });
  await sqlQuery.fill("SELECT 7 AS csp_alive;");
  await page.getByRole("button", { name: /^Execute$/i }).click();
  await expect(page.getByRole("columnheader", { name: "csp_alive" })).toBeVisible({
    timeout: 5000,
  });
  await expect(page.getByRole("cell", { name: "7", exact: true })).toBeVisible();
}

async function exerciseTableBrowser(
  page: Page,
  fixtures: DashboardScreenFixtures,
): Promise<void> {
  await openTableFromSidebar(page, fixtures.tableName);
  await expect(page.getByText(fixtures.tableTitle, { exact: true })).toBeVisible({
    timeout: 5000,
  });
  await expect(page.getByText("visible", { exact: true })).toBeVisible();
}

async function exerciseGraphqlExplorer(
  page: Page,
  graphql: NonNullable<DashboardScreenFixtures["graphql"]>,
): Promise<void> {
  const responsePromise = page.waitForResponse((response) => {
    const observedRequest = response.request();
    return (
      observedRequest.method() === "POST" &&
      new URL(response.url()).pathname === GRAPHQL_PATH
    );
  });
  await page
    .locator("aside")
    .getByRole("button", { name: "GraphQL", exact: true })
    .click();
  await page.getByLabel("GraphQL query").fill(graphql.query);
  await page
    .getByLabel("GraphQL variables")
    .fill(JSON.stringify(graphql.variables));
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const graphqlResponse = await responsePromise;
  expect(graphqlResponse.status()).toBe(200);
  expect(await graphqlResponse.json()).toEqual(graphql.expectedEnvelope);
  const renderedResponse = page.getByTestId("graphql-response-body");
  await expect(renderedResponse).toBeVisible();
  expect(JSON.parse((await renderedResponse.textContent()) ?? "")).toEqual(
    graphql.expectedEnvelope,
  );
}

async function exerciseFunctions(
  page: Page,
  functionName: string,
): Promise<void> {
  await page
    .locator("aside")
    .getByRole("button", { name: /^Functions$/i })
    .click();
  await expect(page.getByRole("heading", { name: /^Functions \(\d+\)$/ })).toBeVisible({
    timeout: 15_000,
  });
  await expect(
    page.getByRole("button", { name: new RegExp(`\\b${functionName}\\b`) }).first(),
  ).toBeVisible({ timeout: 5000 });
}

async function exerciseStoragePreview(
  page: Page,
  imageFileName: string,
): Promise<void> {
  await page
    .locator("aside")
    .getByRole("button", { name: /^Storage$/i })
    .click();
  await expect(
    page.getByRole("button", { name: "Upload", exact: true }),
  ).toBeVisible({ timeout: 5000 });
  await expect(page.getByLabel("Bucket name")).toHaveValue(STORAGE_BUCKET);
  const seededRow = page.locator("tr").filter({ hasText: imageFileName }).first();
  await expect(seededRow).toBeVisible({ timeout: 5000 });
  await expect(seededRow.getByText("image/png")).toBeVisible();
  await seededRow.getByRole("button", { name: "Preview" }).click();
  const previewImage = page.getByRole("img", { name: imageFileName });
  await expect(previewImage).toBeVisible();
  await expect(previewImage).toHaveJSProperty("complete", true);
  await expect(previewImage).toHaveJSProperty("naturalWidth", 1);
  const previewSource = await previewImage.getAttribute("src");
  expect(
    previewSource?.startsWith("/api/storage/default/") ||
      previewSource?.startsWith("data:"),
  ).toBeTruthy();
  await page.getByRole("button", { name: "Close" }).click();
}

async function exerciseRealtimeInspector(page: Page): Promise<void> {
  const dashboardOrigin = new URL(page.url()).origin;
  const statsResponsePromise = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.origin === dashboardOrigin &&
      url.pathname === REALTIME_STATS_PATH &&
      response.request().method() === "GET"
    );
  });
  await page
    .locator("aside")
    .getByRole("button", { name: /^Realtime Inspector$/i })
    .click();
  await expect(
    page.getByRole("heading", { name: /Realtime Inspector/i }),
  ).toBeVisible({
    timeout: 15_000,
  });
  const statsResponse = await statsResponsePromise;
  expect(statsResponse.ok()).toBeTruthy();
  const snapshot = await statsResponse.json();
  const panel = page.getByTestId("realtime-inspector-panel");
  await expect(panel).toBeVisible({ timeout: 5000 });
  await expect(panel.getByTestId("realtime-total-metric")).toBeVisible();
  await expect(panel.getByTestId("realtime-total-metric-value")).toHaveText(
    String(snapshot.connections.total),
  );
  await expect(
    panel.getByRole("heading", { name: /^Subscriptions$/i }),
  ).toBeVisible();
  await expect(
    panel
      .getByRole("columnheader", { name: /^Name$/i })
      .or(panel.getByText(/No active subscriptions/i)),
  ).toBeVisible({ timeout: 5000 });
}

async function assertBrowserRealtimeWebSocket(
  page: Page,
  resultPromise: Promise<BrowserRealtimeWebSocketResult>,
): Promise<void> {
  await page.getByRole("heading", { name: /Realtime Inspector/i }).click({
    modifiers: ["Alt", "Shift"],
  });
  const result = await resultPromise;
  const diagnostic = JSON.stringify(result, null, 2);
  expect(result.urlPath).toBe(REALTIME_WS_PATH);
  expect(
    result.opened,
    `Browser WebSocket must open from Chromium. Result: ${diagnostic}`,
  ).toBeTruthy();
  expect(
    result.connected,
    `Browser WebSocket must receive connected. Result: ${diagnostic}`,
  ).toBeTruthy();
  expect(
    result.subscribed,
    `Browser WebSocket must subscribe. Result: ${diagnostic}`,
  ).toBeTruthy();
  expect(result.error).toBe("");
}

async function assertNoDashboardCspViolations(
  collectors: BrowserCollectors,
): Promise<void> {
  await collectors.triggerCspSentinelViolation();
  const dashboardViolations = collectors.violations.filter(
    (record) => !isSentinelViolation(record),
  );
  const diagnostics = JSON.stringify(
    {
      violations: dashboardViolations,
      cspConsoleMessages: collectors.cspConsoleMessages,
    },
    null,
    2,
  );
  expect(
    dashboardViolations,
    `Expected zero enforcing CSP violations from the real dashboard. Captured:\n${diagnostics}`,
  ).toEqual([]);
  expect(
    collectors.cspConsoleMessages,
    `Expected zero enforcing CSP console messages. Captured:\n${diagnostics}`,
  ).toEqual([]);
}

test.describe("Smoke: CSP Enforcement", () => {
  test("real dashboard emits enforcing CSP header and zero violations", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const realtimeEmail = `csp-browser-ws-${runID}@example.test`;
    const browserRealtimeToken = await createBrowserRealtimeToken(
      request,
      realtimeEmail,
    );
    const screenFixtures = createDashboardScreenFixtures(testInfo);
    try {
      await prepareDashboardScreenFixtures(
        request,
        adminToken,
        screenFixtures,
      );
      const collectors = await installBrowserCollectors(
        page,
        browserRealtimeToken,
      );
      const adminDocumentPath = adminPath();
      const cspHeaderPromise = waitForAdminDocumentCspHeader(
        page,
        adminDocumentPath,
      );

      await page.goto(adminDocumentPath);
      await waitForDashboard(page);
      await expect(
        cspHeaderPromise,
        "admin document must carry the enforcing Content-Security-Policy header",
      ).resolves.toBe(EXPECTED_CSP_POLICY);
      await exerciseDashboardScreens(
        page,
        screenFixtures,
        collectors.browserRealtimeWebSocketPromise,
      );
      await assertNoDashboardCspViolations(collectors);
    } finally {
      await cleanupAuthUser(request, adminToken, realtimeEmail).catch(() => {});
      await cleanupDashboardScreenFixtures(
        request,
        adminToken,
        screenFixtures,
      );
    }
  });
});
