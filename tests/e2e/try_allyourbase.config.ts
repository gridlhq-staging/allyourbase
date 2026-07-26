import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "try_allyourbase.spec.ts",
  timeout: 15_000,
  retries: 0,
  reporter: [["list"]],
  webServer: {
    command: "python3 -m http.server 4177 --directory ../../examples/apex_landing",
    port: 4177,
    reuseExistingServer: false,
  },
  use: {
    baseURL: "http://127.0.0.1:4177",
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
});
