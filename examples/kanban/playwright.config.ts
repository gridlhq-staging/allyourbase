import { defineConfig } from "@playwright/test";

const testPort = 4173;
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
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${testPort} --strictPort`,
    port: testPort,
    reuseExistingServer: false,
    timeout: 10000,
  },
});
