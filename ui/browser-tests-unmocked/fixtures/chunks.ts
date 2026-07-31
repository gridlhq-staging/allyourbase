import { expect, type Page, type Request, type Route } from "@playwright/test";

const JAVASCRIPT_ASSET_PATTERN = /\/assets\/[^/?]+\.js(?:[?#]|$)/;

export function normalizedAdminPath(): string {
  const configured = process.env.PLAYWRIGHT_ADMIN_PATH?.trim() || "/admin";
  const withLeadingSlash = configured.startsWith("/") ? configured : `/${configured}`;
  return withLeadingSlash.replace(/\/+$/, "") || "/";
}

export function adminURL(path = "/"): string {
  const adminPath = normalizedAdminPath();
  const suffix = path.startsWith("/") ? path : `/${path}`;
  if (adminPath === "/") {
    return suffix;
  }
  return `${adminPath}${suffix}`;
}

export function observeJavaScriptChunks(page: Page) {
  const requestedChunkURLs: string[] = [];
  const onRequest = (request: Request) => {
    const url = request.url();
    if (JAVASCRIPT_ASSET_PATTERN.test(url)) {
      requestedChunkURLs.push(url);
    }
  };

  page.on("request", onRequest);
  return {
    requestedChunkURLs,
    dispose: () => page.off("request", onRequest),
  };
}

export function observeMatchingRequests(
  page: Page,
  match: { method: string; urlIncludes: string },
) {
  let requestCount = 0;
  const expectedMethod = match.method.toUpperCase();
  const onRequest = (request: Request) => {
    if (request.method() === expectedMethod && request.url().includes(match.urlIncludes)) {
      requestCount += 1;
    }
  };

  page.on("request", onRequest);
  return {
    count: () => requestCount,
    dispose: () => page.off("request", onRequest),
  };
}

export interface BlockedRequestGate {
  intercepted: () => Promise<void>;
  startAndWaitForInterception: (action: () => Promise<unknown>) => Promise<void>;
  release: () => Promise<number>;
  uninstall: () => Promise<void>;
}

async function installBlockedRequestGate(
  page: Page,
  match: { method: string; urlIncludes: string },
): Promise<BlockedRequestGate> {
  let resolveIntercepted = () => {};
  const intercepted = new Promise<void>((resolve) => {
    resolveIntercepted = resolve;
  });
  let resolveCompleted = (_status: number) => {};
  let rejectCompleted = (_error: unknown) => {};
  const completed = new Promise<number>((resolve, reject) => {
    resolveCompleted = resolve;
    rejectCompleted = reject;
  });
  let matched = false;
  let released = false;
  let actionCompletion: Promise<unknown> | undefined;
  let releaseHeldRequest = () => {};
  const heldUntilReleased = new Promise<void>((resolve) => {
    releaseHeldRequest = resolve;
  });
  const routePattern = `**${match.urlIncludes}**`;
  const expectedMethod = match.method.toUpperCase();
  const handler = async (route: Route) => {
    if (matched || route.request().method() !== expectedMethod) {
      await route.fallback();
      return;
    }
    matched = true;
    resolveIntercepted();
    await heldUntilReleased;
    try {
      const response = await route.fetch();
      await route.fulfill({ response });
      resolveCompleted(response.status());
    } catch (error) {
      rejectCompleted(error);
      throw error;
    }
  };
  await page.route(routePattern, handler);
  const startAndWaitForInterception = async (action: () => Promise<unknown>) => {
    if (actionCompletion) {
      throw new Error("A blocked request gate can start only one action");
    }
    const startedAction = Promise.resolve().then(action);
    actionCompletion = startedAction;
    const actionFailed = new Promise<never>((_, reject) => {
      void startedAction.then(undefined, reject);
    });
    await Promise.race([intercepted, actionFailed]);
  };
  const release = async () => {
    if (!matched) {
      throw new Error(`Expected ${expectedMethod} ${match.urlIncludes} to be intercepted before release`);
    }
    if (!released) {
      released = true;
      releaseHeldRequest();
    }
    const status = await completed;
    await actionCompletion;
    return status;
  };
  return {
    intercepted: () => intercepted,
    startAndWaitForInterception,
    release,
    uninstall: async () => {
      if (matched) {
        await release().catch(() => {});
      }
      await page.unroute(routePattern, handler).catch(() => {});
    },
  };
}

/** Hold one matching request while the callback asserts the in-flight UI state. */
export async function blockMatchingRequest<T>(
  page: Page,
  match: { method: string; urlIncludes: string },
  run: (gate: BlockedRequestGate) => Promise<T>,
): Promise<T> {
  const gate = await installBlockedRequestGate(page, match);
  try {
    return await run(gate);
  } finally {
    await gate.uninstall();
  }
}

export async function failNextMatchingChunk(page: Page, match: RegExp): Promise<void> {
  let failed = false;
  await page.route(JAVASCRIPT_ASSET_PATTERN, async (route) => {
    const url = route.request().url();
    if (!failed && match.test(url)) {
      failed = true;
      await route.abort("failed");
      return;
    }
    await route.continue();
  });
}

export async function expectChunkRequestCount(
  requestedChunkURLs: readonly string[],
  match: RegExp,
  expectedCount: number,
): Promise<void> {
  await expect
    .poll(() => requestedChunkURLs.filter((url) => match.test(url)).length)
    .toBe(expectedCount);
}
