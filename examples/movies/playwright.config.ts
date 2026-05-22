import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: "http://127.0.0.1:5177",
    headless: true,
    locale: "en-US",
    screenshot: "only-on-failure",
  },
  webServer: {
    command:
      "AYB_AUTH_RATE_LIMIT=10000 AYB_AUTH_RATE_LIMIT_AUTH=10000/min AYB_AUTH_ANONYMOUS_RATE_LIMIT=10000 AYB_RATE_LIMIT_API_ANONYMOUS=10000/min AYB_RATE_LIMIT_API=10000/min bash ./e2e/run_demo_with_fake_ollama.sh",
    port: 5177,
    reuseExistingServer: true,
    timeout: 60000,
  },
});
