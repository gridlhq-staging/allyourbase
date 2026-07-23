import { test as base, expect, type Page, type Route } from "@playwright/test";
import {
  BROWSER_RUNTIME_SETUP_TIMEOUT_MS,
  startInstantSearchRuntime,
} from "../live_runtime.mjs";

type InstantSearchFixtures = {
  appURL: string;
};

const SEARCH_API_PATTERN = "**/api/collections/instantsearch_products**";
export const ZERO_HIT_QUERY = "zzzz-no-products";

type PendingSearchArrangement = {
  waitForIntercepted: Promise<void>;
  release: () => Promise<void>;
};

export const test = base.extend<InstantSearchFixtures>({
  appURL: [
    async ({}, use) => {
      const runtime = await startInstantSearchRuntime({ includeApp: true });
      try {
        await use(runtime.appURL);
      } finally {
        await runtime.stop();
      }
    },
    { scope: "worker", timeout: BROWSER_RUNTIME_SETUP_TIMEOUT_MS },
  ],
});

// Wait for the seeded catalog to render using the same signal the passing
// search/facets specs rely on: the first seeded hit plus the "20 results"
// summary copy. Gating on this rendered state, rather than InstantSearch's
// internal status, keeps the wait deterministic even though React StrictMode
// can leave a superseded search request pending during development.
export async function loadSeededCatalog(page: Page, appURL: string): Promise<void> {
  await page.goto(appURL);
  await expect(page.getByTestId("hit-red-notebook")).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByTestId("results-summary")).toContainText("20 results");
}

export async function arrangeSearchApiFailure(page: Page): Promise<void> {
  await page.route(SEARCH_API_PATTERN, (route) => route.abort("failed"));
}

export async function arrangePendingFirstSearch(page: Page): Promise<PendingSearchArrangement> {
  const heldRoutes: Route[] = [];
  let released = false;
  let signalIntercepted: () => void = () => {};
  const waitForIntercepted = new Promise<void>((resolve) => {
    signalIntercepted = resolve;
  });

  await page.route(SEARCH_API_PATTERN, async (route) => {
    if (released) {
      await route.continue();
      return;
    }
    heldRoutes.push(route);
    signalIntercepted();
  });

  return {
    waitForIntercepted,
    release: async () => {
      released = true;
      for (const route of heldRoutes) {
        await route.continue();
      }
    },
  };
}

export { expect };
