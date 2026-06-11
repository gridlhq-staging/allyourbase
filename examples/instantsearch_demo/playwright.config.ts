import { defineConfig } from "@playwright/test";

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
});
