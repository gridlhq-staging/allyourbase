import { defineConfig } from "@playwright/test";

const testPort = Number(process.env.AYB_DEMO_APP_PORT) || 4173;
export const kanbanPlaywrightDefaults = {
  testDir: "./tests",
  timeout: 30000,
  retries: 0,
  use: {
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
};

export default defineConfig({
  ...kanbanPlaywrightDefaults,
  use: {
    ...kanbanPlaywrightDefaults.use,
    baseURL: `http://127.0.0.1:${testPort}`,
  },
  projects: [
    {
      name: "kanban",
      use: { browserName: "chromium" },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${testPort} --strictPort`,
    port: testPort,
    reuseExistingServer: true,
    timeout: 10000,
  },
});
