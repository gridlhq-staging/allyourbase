import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const readFileSync = vi.fn();
const homedir = vi.fn(() => "/home/smoke");

vi.mock("fs", () => {
  const mocked = { readFileSync: (...args: unknown[]) => readFileSync(...args) };
  return { ...mocked, default: mocked };
});
vi.mock("os", () => {
  const mocked = { homedir: () => homedir() };
  return { ...mocked, default: mocked };
});

import { resolveAdminBootstrapCredential } from "../../browser-tests-unmocked/admin-bootstrap";

function enoent(): NodeJS.ErrnoException {
  const error: NodeJS.ErrnoException = new Error("ENOENT: no such file or directory");
  error.code = "ENOENT";
  return error;
}

const ENV_KEYS = ["AYB_ADMIN_PASSWORD", "AYB_ADMIN_TOKEN"] as const;
let savedEnv: Record<string, string | undefined>;

beforeEach(() => {
  savedEnv = Object.fromEntries(ENV_KEYS.map((key) => [key, process.env[key]]));
  ENV_KEYS.forEach((key) => delete process.env[key]);
  readFileSync.mockReset();
  readFileSync.mockReturnValue("token-from-file\n");
});

afterEach(() => {
  ENV_KEYS.forEach((key) => {
    const value = savedEnv[key];
    if (value === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = value;
    }
  });
});

describe("resolveAdminBootstrapCredential", () => {
  it("prefers an explicit admin password so the login smoke can exercise the form", () => {
    process.env.AYB_ADMIN_PASSWORD = "hunter2";
    process.env.AYB_ADMIN_TOKEN = "exported-token";

    expect(resolveAdminBootstrapCredential()).toEqual({
      source: "env-password",
      value: "hunter2",
    });
  });

  it("uses the exported admin token when no password is set", () => {
    process.env.AYB_ADMIN_TOKEN = "exported-token";

    expect(resolveAdminBootstrapCredential()).toEqual({
      source: "env-admin-token",
      value: "exported-token",
    });
  });

  it("does not read the token file when the token is exported", () => {
    // run_all_tests.sh exports AYB_ADMIN_TOKEN precisely because any `ayb`
    // shutdown removes the shared ~/.ayb/admin-token file mid-suite.
    process.env.AYB_ADMIN_TOKEN = "exported-token";

    resolveAdminBootstrapCredential();

    expect(readFileSync).not.toHaveBeenCalled();
  });

  it("survives the token file disappearing when the token is exported", () => {
    process.env.AYB_ADMIN_TOKEN = "exported-token";
    readFileSync.mockImplementation(() => {
      throw enoent();
    });

    expect(resolveAdminBootstrapCredential().value).toBe("exported-token");
  });

  it("trims surrounding whitespace from the exported token", () => {
    process.env.AYB_ADMIN_TOKEN = "  exported-token\n";

    expect(resolveAdminBootstrapCredential().value).toBe("exported-token");
  });

  it("ignores a blank exported token and falls back to the token file", () => {
    process.env.AYB_ADMIN_TOKEN = "   ";

    expect(resolveAdminBootstrapCredential()).toEqual({
      source: "saved-admin-auth",
      value: "token-from-file",
    });
  });

  it("falls back to the token file `ayb start` writes", () => {
    expect(resolveAdminBootstrapCredential()).toEqual({
      source: "saved-admin-auth",
      value: "token-from-file",
    });
    expect(readFileSync).toHaveBeenCalledWith("/home/smoke/.ayb/admin-token", "utf-8");
  });

  it("names every accepted credential source when nothing is available", () => {
    readFileSync.mockImplementation(() => {
      throw enoent();
    });

    expect(() => resolveAdminBootstrapCredential()).toThrow(/AYB_ADMIN_TOKEN/);
    expect(() => resolveAdminBootstrapCredential()).toThrow(/AYB_ADMIN_PASSWORD/);
  });

  it("reports an unreadable token file distinctly from a missing one", () => {
    readFileSync.mockImplementation(() => {
      throw new Error("EACCES: permission denied");
    });

    expect(() => resolveAdminBootstrapCredential()).toThrow(/EACCES/);
  });

  it("rejects an empty token file rather than bootstrapping with a blank value", () => {
    readFileSync.mockReturnValue("   \n");

    expect(() => resolveAdminBootstrapCredential()).toThrow(/empty/i);
  });
});
