/// <reference types="vitest" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// Bound the Vitest worker pool so the full Admin UI suite fits inside the
// GitHub runner memory envelope. Under unbounded concurrency a stray fork
// worker stayed alive and Node exhausted ~4 GiB on the public CI run. The
// cap applies only in CI (VITEST_MAX_WORKERS overrides it); local developer
// runs stay unbounded for fast feedback.
const CI_DEFAULT_MAX_WORKERS = 2;

function resolveVitestMaxWorkers(): number | undefined {
  if (!process.env.CI) {
    return undefined;
  }
  const override = process.env.VITEST_MAX_WORKERS;
  if (override) {
    const parsed = Number.parseInt(override, 10);
    if (Number.isInteger(parsed) && parsed > 0) {
      return parsed;
    }
  }
  return CI_DEFAULT_MAX_WORKERS;
}

export default defineConfig({
  plugins: [react()],
  base: "/admin/",
  experimental: {
    renderBuiltUrl(filename, { hostType, type }) {
      // Keep index.html entry references absolute so rewriteAdminIndexHTML can
      // remap /admin/ to a custom admin path; make JS-emitted chunk URLs
      // module-relative so dynamic imports work from any SPA document depth.
      if (hostType === "js" && type === "asset") {
        return { relative: true };
      }
      return undefined;
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8090",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    exclude: ["browser-tests-unmocked/**", "browser-tests-mocked/**", "node_modules/**"],
    maxWorkers: resolveVitestMaxWorkers(),
  },
});
