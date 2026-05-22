import { defineConfig } from "@playwright/test";
import { kanbanPlaywrightDefaults } from "./playwright.config";

const smokePort = 5173;

export default defineConfig({
  ...kanbanPlaywrightDefaults,
  use: {
    ...kanbanPlaywrightDefaults.use,
    baseURL: `http://127.0.0.1:${smokePort}`,
  },
});
