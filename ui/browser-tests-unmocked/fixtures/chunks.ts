import { expect, type Page, type Request } from "@playwright/test";

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
