import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  BROWSER_RUNTIME_SETUP_TIMEOUT_MS,
  READINESS_TIMEOUT_MS,
  createInstantSearchProcessEnv,
  resolveViteCommand,
} from "../live_runtime.mjs";

describe("instantsearch live runtime helper", () => {
  it("isolates AYB child commands from the operator home directory", () => {
    const operatorHome = process.env.HOME;
    const runtimeHome = mkdtempSync(join(tmpdir(), "ayb-instantsearch-test-"));

    const env = createInstantSearchProcessEnv(runtimeHome, {
      goCacheEnv: {
        GOMODCACHE: "/tmp/operator-go/pkg/mod",
        GOCACHE: "/tmp/operator-go/build",
      },
    });

    expect(env.HOME).toBe(runtimeHome);
    expect(env.HOME).not.toBe(operatorHome);
    expect(env.GOMODCACHE).toBe("/tmp/operator-go/pkg/mod");
    expect(env.GOCACHE).toBe("/tmp/operator-go/build");
    expect(env.AYB_ADMIN_TOKEN).toBeUndefined();
    expect(env.DATABASE_URL).toBeUndefined();
    rmSync(runtimeHome, { recursive: true, force: true });
  });

  it("scrubs operator-provided database owners from AYB child commands", () => {
    const originalAYBDatabaseURL = process.env.AYB_DATABASE_URL;
    const originalDatabaseURL = process.env.DATABASE_URL;
    const runtimeHome = mkdtempSync(join(tmpdir(), "ayb-instantsearch-test-"));

    process.env.AYB_DATABASE_URL = "postgres://operator.example/ayb";
    process.env.DATABASE_URL = "postgres://operator.example/postgres";
    try {
      const env = createInstantSearchProcessEnv(runtimeHome, { goCacheEnv: {} });

      expect(env.AYB_DATABASE_URL).toBeUndefined();
      expect(env.DATABASE_URL).toBeUndefined();
    } finally {
      if (originalAYBDatabaseURL === undefined) {
        delete process.env.AYB_DATABASE_URL;
      } else {
        process.env.AYB_DATABASE_URL = originalAYBDatabaseURL;
      }
      if (originalDatabaseURL === undefined) {
        delete process.env.DATABASE_URL;
      } else {
        process.env.DATABASE_URL = originalDatabaseURL;
      }
      rmSync(runtimeHome, { recursive: true, force: true });
    }
  });

  it("keeps the browser fixture startup budget above both readiness windows", () => {
    expect(BROWSER_RUNTIME_SETUP_TIMEOUT_MS).toBeGreaterThan(READINESS_TIMEOUT_MS * 2);
    const fixtureSource = readFileSync(
      join(process.cwd(), "browser-tests-unmocked", "fixtures.ts"),
      "utf8",
    );
    expect(fixtureSource).toContain("timeout: BROWSER_RUNTIME_SETUP_TIMEOUT_MS");
    expect(fixtureSource).not.toContain("timeout: 90_000");
  });

  it("launches Vite directly so teardown owns the listening process", () => {
    const viteCommand = resolveViteCommand();

    expect(viteCommand.command).toBe(process.execPath);
    expect(viteCommand.args[0]).toMatch(/node_modules[/\\]vite[/\\]bin[/\\]vite\.js$/);
    expect(viteCommand.args.slice(1)).toEqual([
      "--host",
      "127.0.0.1",
      "--port",
      process.env.AYB_APP_PORT ?? "8096",
    ]);
  });

  it("binds the checked app port before slower API startup and seeding", () => {
    const runtimeSource = readFileSync(
      join(process.cwd(), "live_runtime.mjs"),
      "utf8",
    );
    const appStart = runtimeSource.indexOf('spawnManagedProcess(\n        "app"');
    const apiReady = runtimeSource.indexOf("await waitForURL(API_HEALTH_URL");

    expect(appStart).toBeGreaterThan(-1);
    expect(apiReady).toBeGreaterThan(-1);
    expect(appStart).toBeLessThan(apiReady);
  });
});
