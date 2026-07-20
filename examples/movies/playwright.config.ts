import { defineConfig } from "@playwright/test";

// The demo app port. AYB_DEMO_APP_PORT lets the release-gate e2e harness serve
// the demo on an isolated port so it need not require the universal Vite default
// (5177) to be globally free; unset, it keeps the documented default.
const port = Number(process.env.AYB_DEMO_APP_PORT) || 5177;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "movies",
      use: { browserName: "chromium" },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
  webServer: {
    command:
      "AYB_AUTH_RATE_LIMIT=10000 AYB_AUTH_RATE_LIMIT_AUTH=10000/min AYB_AUTH_ANONYMOUS_RATE_LIMIT=10000 AYB_RATE_LIMIT_API_ANONYMOUS=10000/min AYB_RATE_LIMIT_API=10000/min bash ./e2e/run_demo_with_fake_ollama.sh",
    port,
    reuseExistingServer: true,
    timeout: 60000,
  },
});
