import { defineConfig, devices } from "@playwright/test";

const appPort = process.env.AYB_APP_PORT ?? "8096";

export default defineConfig({
  testDir: "./browser-tests-unmocked",
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: `http://127.0.0.1:${appPort}`,
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "instantsearch_demo",
      testIgnore: "**/*mobile.spec.ts",
      use: { browserName: "chromium" },
    },
    {
      name: "instantsearch_demo-mobile",
      // Phone emulation is Chromium-only and intentionally does not run the desktop suite.
      testMatch: "**/*mobile.spec.ts",
      use: { ...devices["Pixel 7"] },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
});
