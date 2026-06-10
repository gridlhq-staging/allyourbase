import { test as base, expect } from "@playwright/test";
import {
  BROWSER_RUNTIME_SETUP_TIMEOUT_MS,
  startInstantSearchRuntime,
} from "../live_runtime.mjs";

type InstantSearchFixtures = {
  appURL: string;
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

export { expect };
