import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  BROWSER_RUNTIME_SETUP_TIMEOUT_MS,
  READINESS_TIMEOUT_MS,
  createInstantSearchProcessEnv,
} from "../live_runtime.mjs";

describe("instantsearch live runtime helper", () => {
  it("isolates AYB child commands from the operator home directory", () => {
    const operatorHome = process.env.HOME;
    const runtimeHome = mkdtempSync(join(tmpdir(), "ayb-instantsearch-test-"));

    const env = createInstantSearchProcessEnv(runtimeHome);

    expect(env.HOME).toBe(runtimeHome);
    expect(env.HOME).not.toBe(operatorHome);
    expect(env.GOMODCACHE ?? "").not.toContain(runtimeHome);
    expect(env.GOCACHE ?? "").not.toContain(runtimeHome);
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
      const env = createInstantSearchProcessEnv(runtimeHome);

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
});
