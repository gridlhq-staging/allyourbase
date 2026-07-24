import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const port = Number(process.env.AYB_DEMO_APP_PORT) || 5175;
const aybServerURL = process.env.AYB_SERVER_URL || "http://localhost:8090";

export default defineConfig({
  plugins: [react()],
  server: {
    port,
    proxy: {
      "/api": aybServerURL,
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.ts",
    // Only pick up unit tests in tests/. The e2e/ directory contains Playwright
    // specs that must be run via `npx playwright test`, not vitest.
    include: ["tests/**/*.{test,spec}.{ts,tsx}"],
  },
});
