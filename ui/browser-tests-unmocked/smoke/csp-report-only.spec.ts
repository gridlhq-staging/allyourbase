import {
  test,
  expect,
  adminPath,
  cleanupAuthUser,
  ensureLinkedEmailAuthUser,
  waitForDashboard,
} from "../fixtures";
import type {
  APIRequestContext,
  ConsoleMessage,
  Page,
  Response,
} from "@playwright/test";

/**
 * SMOKE TEST: CSP Report-Only compatibility
 *
 * Proves the report-only Content-Security-Policy emitted by the canonical
 * `securityHeaders` middleware (`cspReportOnlyPolicy` in
 * internal/server/middleware.go) is compatible with the real embedded admin
 * dashboard in Chromium. The primary oracle is the browser-native
 * `securitypolicyviolation` event: a report-only policy that would block a real
 * dashboard resource still fires this event under `disposition === "report"`
 * even though nothing is enforced, so a zero-event run is direct proof the
 * shadow policy is clean.
 *
 * Header-content correctness (the exact directive list) stays owned by the
 * focused Go test `TestSecurityHeaders`; this spec only asserts the report-only
 * header is present on the live admin document and that exercising
 * representative CodeMirror (SQL Editor) and realtime (Realtime Inspector)
 * screens produces no violations.
 */

const CSP_REPORT_ONLY_HEADER = "content-security-policy-report-only";
const REALTIME_STATS_PATH = "/api/admin/realtime/stats";
const REALTIME_WS_PATH = "/api/realtime/ws";
const CSP_SENTINEL_HOST = "ayb-csp-report-only-sentinel.invalid";
const CSP_SENTINEL_URL = `https://${CSP_SENTINEL_HOST}/finalize.png`;

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
  reportOnlyConsoleMessages: string[];
  triggerCspSentinelViolation: () => Promise<void>;
  violations: CspViolationRecord[];
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
  const reportOnlyConsoleMessages: string[] = [];
  let resolveBrowserRealtimeWebSocket: (
    result: BrowserRealtimeWebSocketResult,
  ) => void;
  const browserRealtimeWebSocketPromise =
    new Promise<BrowserRealtimeWebSocketResult>((resolve) => {
      resolveBrowserRealtimeWebSocket = resolve;
    });
  let resolveSentinelViolation: (record: CspViolationRecord) => void;
  const sentinelViolationPromise = new Promise<CspViolationRecord>(
    (resolve) => {
      resolveSentinelViolation = resolve;
    },
  );

  await exposeCollectorBindings(page, {
    onBrowserRealtimeWebSocket: (result) =>
      resolveBrowserRealtimeWebSocket(result),
    onSentinelViolation: (record) => resolveSentinelViolation(record),
    onViolation: (record) => violations.push(record),
  });
  await installBrowserInitScript(page, browserRealtimeToken);
  collectReportOnlyConsoleMessages(page, reportOnlyConsoleMessages);

  return {
    browserRealtimeWebSocketPromise,
    reportOnlyConsoleMessages,
    triggerCspSentinelViolation: () =>
      triggerCspSentinelViolation(page, sentinelViolationPromise),
    violations,
  };
}

async function exposeCollectorBindings(
  page: Page,
  callbacks: {
    onBrowserRealtimeWebSocket: (
      result: BrowserRealtimeWebSocketResult,
    ) => void;
    onSentinelViolation: (record: CspViolationRecord) => void;
    onViolation: (record: CspViolationRecord) => void;
  },
): Promise<void> {
  await page.exposeFunction(
    "__recordCspSentinelViolation",
    callbacks.onSentinelViolation,
  );
  await page.exposeFunction(
    "__recordBrowserRealtimeWebSocketResult",
    callbacks.onBrowserRealtimeWebSocket,
  );
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
          websocketResult.error =
            error instanceof Error ? error.message : String(error);
          finishWebSocketProbe();
          return;
        }
        const timeout = window.setTimeout(() => {
          websocketResult.error =
            "Timed out waiting for browser WebSocket subscribe acknowledgement";
          socket.close();
          finishWebSocketProbe();
        }, 10_000);
        socket.addEventListener("open", () => {
          websocketResult.opened = true;
        });
        socket.addEventListener("message", (event) => {
          let message: RealtimeWebSocketMessage;
          try {
            message = JSON.parse(
              String(event.data),
            ) as RealtimeWebSocketMessage;
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
          if (
            !websocketResult.subscribed &&
            websocketResult.error.length === 0
          ) {
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
            if (!(event instanceof MouseEvent) || !event.altKey) {
              return;
            }
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
        document.addEventListener("DOMContentLoaded", installFinalizer, {
          once: true,
        });
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
          void Promise.all(pendingAtSentinel).then(() => {
            void window.__recordCspSentinelViolation(record);
          });
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

function collectReportOnlyConsoleMessages(
  page: Page,
  messages: string[],
): void {
  page.on("console", (message: ConsoleMessage) => {
    const text = message.text();
    if (text.includes("[Report Only]") && !isSentinelConsoleMessage(text)) {
      messages.push(redactTokenQuery(text));
    }
  });
}

async function triggerCspSentinelViolation(
  page: Page,
  sentinelViolationPromise: Promise<CspViolationRecord>,
): Promise<void> {
  await page
    .getByRole("heading", { name: /Realtime Inspector/i })
    .click({ modifiers: ["Alt"] });
  const sentinelViolation = await sentinelViolationPromise;
  expect(sentinelViolation.disposition).toBe("report");
  expect(isSentinelViolation(sentinelViolation)).toBeTruthy();
}

function waitForAdminDocumentReportOnlyHeader(
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
      resolve(response.headers()[CSP_REPORT_ONLY_HEADER] ?? "");
    };
    page.on("response", onResponse);
  });
}

async function exerciseDashboardScreens(page: Page): Promise<void> {
  await expect(page.getByRole("button", { name: "Search... K" })).toBeVisible();
  await page
    .locator("aside")
    .getByRole("button", { name: /^SQL Editor$/i })
    .click();
  await expect(page.getByLabel("SQL query")).toBeVisible({ timeout: 5000 });
  await exerciseRealtimeInspector(page);
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
      reportOnlyConsoleMessages: collectors.reportOnlyConsoleMessages,
    },
    null,
    2,
  );
  expect(
    dashboardViolations,
    `Expected zero report-only CSP violations from the real dashboard. Captured:\n${diagnostics}`,
  ).toEqual([]);
  expect(
    collectors.reportOnlyConsoleMessages,
    `Expected zero [Report Only] CSP console messages. Captured:\n${diagnostics}`,
  ).toEqual([]);
}

test.describe("Smoke: CSP Report-Only", () => {
  test("real dashboard emits report-only CSP header and zero violations", async ({
    page,
    request,
    adminToken,
  }) => {
    const realtimeEmail = `csp-browser-ws-${Date.now()}@example.test`;
    const browserRealtimeToken = await createBrowserRealtimeToken(
      request,
      realtimeEmail,
    );
    try {
      const collectors = await installBrowserCollectors(
        page,
        browserRealtimeToken,
      );
      const adminDocumentPath = adminPath();
      const reportOnlyHeaderPromise = waitForAdminDocumentReportOnlyHeader(
        page,
        adminDocumentPath,
      );

      await page.goto(adminDocumentPath);
      await waitForDashboard(page);
      await expect(
        reportOnlyHeaderPromise,
        "admin document must carry a Content-Security-Policy-Report-Only header",
      ).resolves.not.toBe("");
      await exerciseDashboardScreens(page);
      await assertBrowserRealtimeWebSocket(
        page,
        collectors.browserRealtimeWebSocketPromise,
      );
      await assertNoDashboardCspViolations(collectors);
    } finally {
      await cleanupAuthUser(request, adminToken, realtimeEmail).catch(() => {});
    }
  });
});
