import { defineConfig, devices } from "@playwright/test";

const testPort = Number(process.env.AYB_DEMO_APP_PORT) || 4173;
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
  projects: [
    {
      name: "kanban",
      testIgnore: "**/*mobile.spec.ts",
      use: { browserName: "chromium" },
    },
    {
      name: "kanban-mobile",
      // Phone emulation is Chromium-only and intentionally does not run the desktop suite.
      testMatch: "**/*mobile.spec.ts",
      use: { ...devices["Pixel 7"] },
    },
  ],
  reporter: [["list"], ["json", { outputFile: "playwright-report/results.json" }]],
  webServer: process.env.AYB_DEMO_EXTERNAL_SERVER ? undefined : {
    command:
      `bash -lc 'set -euo pipefail; ` +
      `source ../../tests/port_helpers.sh; ` +
      `shared_ayb_dir="$HOME/.ayb"; ` +
      `runtime_home=""; AYB_DATABASE_EMBEDDED_DATA_DIR=""; ` +
      `cleanup() { if [ -n "\${demo_pid:-}" ]; then kill -INT "$demo_pid" 2>/dev/null || true; wait "$demo_pid" 2>/dev/null || true; fi; if [ -n "$AYB_DATABASE_EMBEDDED_DATA_DIR" ]; then rm -rf "$AYB_DATABASE_EMBEDDED_DATA_DIR"; fi; if [ -n "$runtime_home" ]; then rm -rf "$runtime_home"; fi; }; ` +
      `trap cleanup EXIT INT TERM; ` +
      `AYB_SERVER_PORT="$(pick_free_port 48090 49090 50090 51090 52090)" || { echo "No free isolated AYB API port available for Kanban demo" >&2; exit 1; }; export AYB_SERVER_PORT; ` +
      `AYB_DATABASE_EMBEDDED_PORT="$(pick_free_port 45432 46432 47432 48432 49432)" || { echo "No free isolated embedded Postgres port available for Kanban demo" >&2; exit 1; }; export AYB_DATABASE_EMBEDDED_PORT; ` +
      `AYB_DATABASE_EMBEDDED_DATA_DIR="$(mktemp -d /tmp/ayb-kanban-pg.XXXXXX)" || { echo "Cannot create isolated embedded Postgres data directory for Kanban demo" >&2; exit 1; }; export AYB_DATABASE_EMBEDDED_DATA_DIR; ` +
      `runtime_home="$(mktemp -d /tmp/ayb-kanban-home.XXXXXX)" || { echo "Cannot create isolated home directory for Kanban demo" >&2; exit 1; }; ` +
      `mkdir -p "$runtime_home/.ayb"; ` +
      `for cache_name in pg pgbin; do if [ -d "$shared_ayb_dir/$cache_name" ]; then ln -s "$shared_ayb_dir/$cache_name" "$runtime_home/.ayb/$cache_name"; fi; done; ` +
      `export HOME="$runtime_home"; ` +
      `AYB_AUTH_RATE_LIMIT=10000 AYB_AUTH_RATE_LIMIT_AUTH=10000/min AYB_AUTH_ANONYMOUS_RATE_LIMIT=10000 ` +
      `AYB_RATE_LIMIT_API_ANONYMOUS=10000/min AYB_RATE_LIMIT_API=10000/min ` +
      `AYB_DEMO_APP_PORT=${testPort} ../../ayb demo kanban & demo_pid="$!"; wait "$demo_pid"'`,
    gracefulShutdown: { signal: "SIGINT", timeout: 10000 },
    port: testPort,
    reuseExistingServer: false,
    timeout: 10000,
  },
});
