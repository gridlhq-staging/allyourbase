import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./browser-tests-unmocked",
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:8096",
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
});
