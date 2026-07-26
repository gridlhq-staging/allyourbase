import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "try_allyourbase_live.spec.ts",
  timeout: 240_000,
  retries: 0,
  reporter: [["list"]],
  use: {
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
});
