import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "cross_demo.spec.ts",
  timeout: 30_000,
  retries: 0,
  reporter: [["list"]],
  use: {
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
});
