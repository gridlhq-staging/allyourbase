import { defineConfig, devices } from "@playwright/test";

// The demo app port. AYB_DEMO_APP_PORT lets the release-gate e2e harness serve
// the demo on an isolated port so it need not require the universal Vite default
// (5175) to be globally free; unset, it keeps the documented default.
const port = Number(process.env.AYB_DEMO_APP_PORT) || 5175;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: `http://localhost:${port}`,
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "live-polls",
      testIgnore: "**/*mobile.spec.ts",
      use: { browserName: "chromium" },
    },
    {
      name: "live-polls-mobile",
      // Phone emulation is Chromium-only and intentionally does not run the desktop suite.
      testMatch: "**/*mobile.spec.ts",
      use: { ...devices["Pixel 7"] },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : {
    command: "npm run dev",
    port,
    reuseExistingServer: false,
    timeout: 10000,
  },
});
