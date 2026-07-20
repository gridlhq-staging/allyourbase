import { defineConfig } from "@playwright/test";

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
      use: { browserName: "chromium" },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
  webServer: {
    command: "npm run dev",
    port,
    reuseExistingServer: true,
    timeout: 10000,
  },
});
