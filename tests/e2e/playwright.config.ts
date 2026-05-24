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
    // Stage 1 red-baseline capture: produce video.webm artifacts for the
    // deployed live-polls regression repro (Playwright has no --video CLI flag).
    video: "on",
  },
});
