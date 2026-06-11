import { spawn, spawnSync } from "node:child_process";
import { accessSync, constants, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const INSTANTSEARCH_COLLECTION = "instantsearch_products";
export const INSTANTSEARCH_OBJECT_ID_FIELD = "slug";

const API_PORT = parseConfiguredPort("AYB_API_PORT", "8090");
const APP_PORT = parseConfiguredPort("AYB_APP_PORT", "8096");
const MANAGED_PG_PORT = parseConfiguredPort(
  "AYB_DATABASE_EMBEDDED_PORT",
  process.env.AYB_API_PORT || process.env.AYB_APP_PORT
    ? String(API_PORT + 2)
    : "15432",
);
const API_URL = `http://127.0.0.1:${API_PORT}`;
const APP_URL = `http://127.0.0.1:${APP_PORT}`;
const API_HEALTH_URL = `${API_URL}/health`;
export const READINESS_TIMEOUT_MS = 60_000;
export const BROWSER_RUNTIME_SETUP_TIMEOUT_MS = READINESS_TIMEOUT_MS * 2 + 30_000;
const POLL_INTERVAL_MS = 500;
const GRACEFUL_EXIT_TIMEOUT_MS = 10_000;
const DEMO_ROOT = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(DEMO_ROOT, "..", "..");
const RUNTIME_TMP_ROOT = process.env.AYB_RUNTIME_TMPDIR ?? "/tmp";

export async function startInstantSearchRuntime(options = {}) {
  const includeApp = options.includeApp === true;
  const requiredPorts = includeApp
    ? [API_PORT, APP_PORT, MANAGED_PG_PORT]
    : [API_PORT, MANAGED_PG_PORT];
  assertPortsAvailable(requiredPorts);

  const aybCommand = resolveAybCommand();
  const runtimeHome = mkdtempSync(join(RUNTIME_TMP_ROOT, "ayb-instantsearch-runtime-"));
  const runtimeEnv = createInstantSearchProcessEnv(runtimeHome);
  const managedProcesses = [];

  try {
    const apiProcess = spawnManagedProcess(
      "api",
      aybCommand.command,
      [...aybCommand.args, "start", "--foreground", "--port", String(API_PORT)],
      REPO_ROOT,
      runtimeEnv,
    );
    managedProcesses.push(apiProcess);
    await waitForURL(API_HEALTH_URL, READINESS_TIMEOUT_MS, apiProcess);
    seedInstantSearchDemo(aybCommand, runtimeEnv);

    if (includeApp) {
      const appProcess = spawnManagedProcess(
        "app",
        "npm",
        ["run", "dev", "--", "--host", "127.0.0.1", "--port", String(APP_PORT)],
        DEMO_ROOT,
        { ...runtimeEnv, VITE_AYB_URL: API_URL },
      );
      managedProcesses.push(appProcess);
      await waitForURL(APP_URL, READINESS_TIMEOUT_MS, appProcess);
    }

    return {
      apiURL: API_URL,
      appURL: APP_URL,
      async stop() {
        await stopInstantSearchRuntime(aybCommand, runtimeEnv, managedProcesses);
        removeRuntimeHome(runtimeHome);
      },
    };
  } catch (error) {
    await stopInstantSearchRuntime(aybCommand, runtimeEnv, managedProcesses);
    removeRuntimeHome(runtimeHome);
    throw error;
  }
}

export function createInstantSearchProcessEnv(runtimeHome) {
  const env = {
    ...process.env,
    ...resolveGoCacheEnv(),
    HOME: runtimeHome,
    AYB_DATABASE_EMBEDDED_PORT: String(MANAGED_PG_PORT),
  };
  delete env.AYB_ADMIN_TOKEN;
  delete env.AYB_DATABASE_URL;
  delete env.DATABASE_URL;
  return env;
}

function parseConfiguredPort(envName, fallbackValue) {
  const port = Number.parseInt(process.env[envName] ?? fallbackValue, 10);
  if (!Number.isFinite(port) || port <= 0) {
    throw new Error(`${envName} must be a positive integer port`);
  }
  return port;
}

function resolveGoCacheEnv() {
  const goEnv = spawnSync("go", ["env", "GOMODCACHE", "GOCACHE"], {
    encoding: "utf-8",
    env: process.env,
  });
  if (goEnv.status !== 0) return {};

  const [goModCache, goCache] = goEnv.stdout.trim().split(/\r?\n/);
  return {
    ...(goModCache ? { GOMODCACHE: goModCache } : {}),
    ...(goCache ? { GOCACHE: goCache } : {}),
  };
}

function resolveAybCommand() {
  if (process.env.AYB_BIN?.trim()) {
    return { command: process.env.AYB_BIN.trim(), args: [] };
  }

  const candidates = [join(REPO_ROOT, "ayb"), join(process.cwd(), "ayb")];
  for (const candidate of candidates) {
    try {
      accessSync(candidate, constants.X_OK);
      return { command: candidate, args: [] };
    } catch {
      // Try the next local binary candidate before falling back to go run.
    }
  }

  return { command: "go", args: ["run", "./cmd/ayb"] };
}

function spawnManagedProcess(role, command, args, cwd, env) {
  const proc = spawn(command, args, {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const managedProcess = {
    process: proc,
    role,
    label: [command, ...args].join(" "),
    output: "",
    spawnError: undefined,
  };

  proc.stdout?.on("data", (chunk) => appendOutput(managedProcess, chunk));
  proc.stderr?.on("data", (chunk) => appendOutput(managedProcess, chunk));
  proc.once("error", (error) => {
    managedProcess.spawnError = error;
  });

  return managedProcess;
}

function appendOutput(managedProcess, chunk) {
  managedProcess.output = `${managedProcess.output}${chunk.toString()}`.slice(-4_000);
}

async function waitForURL(url, timeoutMs, managedProcess) {
  const deadlineMs = Date.now() + timeoutMs;
  while (Date.now() < deadlineMs) {
    assertProcessStillRunning(managedProcess, `URL readiness at ${url}`);
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2_000) });
      if (response.ok) return;
    } catch {
      // Keep polling until the readiness deadline expires.
    }
    await sleep(POLL_INTERVAL_MS);
  }

  assertProcessStillRunning(managedProcess, `URL readiness at ${url}`);
  throw new Error(`Timed out waiting for URL readiness: ${url}`);
}

function assertProcessStillRunning(managedProcess, readinessTarget) {
  if (managedProcess.spawnError) {
    throw new Error(
      `Process failed before ${readinessTarget}: ${managedProcess.label}: ${managedProcess.spawnError.message}`,
    );
  }

  if (managedProcess.process.exitCode !== null || managedProcess.process.signalCode !== null) {
    throw new Error(
      `Process exited before ${readinessTarget}: ${managedProcess.label} (exitCode=${managedProcess.process.exitCode ?? "null"}, signal=${managedProcess.process.signalCode ?? "null"})\n${managedProcess.output}`,
    );
  }
}

function assertPortsAvailable(ports) {
  const occupiedPorts = ports.filter((port) => pidsForPort(port).length > 0);
  if (occupiedPorts.length > 0) {
    throw new Error(
      `Required port(s) already occupied: ${occupiedPorts.join(", ")}. Stop the owning process and rerun the probe.`,
    );
  }
}

function pidsForPort(port) {
  const result = spawnSync("lsof", ["-ti", `:${port}`], { encoding: "utf-8" });
  if (result.status !== 0 || !result.stdout.trim()) return [];
  return result.stdout
    .trim()
    .split("\n")
    .map((value) => Number.parseInt(value, 10))
    .filter((value) => Number.isInteger(value) && value > 0);
}

function seedInstantSearchDemo(aybCommand, runtimeEnv) {
  const schemaSQL = readFileSync(join(DEMO_ROOT, "schema.sql"), "utf8");
  const seedSQL = readFileSync(join(DEMO_ROOT, "seed.sql"), "utf8");
  const statements = splitSQLStatements(
    `DROP TABLE IF EXISTS ${INSTANTSEARCH_COLLECTION};\n${schemaSQL}\n${seedSQL}`,
  );

  for (const statement of statements) {
    runAybSQL(aybCommand, runtimeEnv, statement);
  }
}

function splitSQLStatements(sqlText) {
  const withoutCommentLines = sqlText
    .split(/\r?\n/)
    .filter((line) => !line.trim().startsWith("--"))
    .join("\n");

  return withoutCommentLines
    .split(/;\s*(?:\r?\n|$)/)
    .map((statement) => statement.trim())
    .filter((statement) => statement.length > 0);
}

function runAybSQL(aybCommand, runtimeEnv, statement) {
  const result = spawnSync(aybCommand.command, [...aybCommand.args, "sql", "--json"], {
    cwd: REPO_ROOT,
    env: runtimeEnv,
    input: statement,
    encoding: "utf-8",
  });

  if (result.status !== 0) {
    throw new Error(
      `ayb sql failed for "${statement.slice(0, 80)}": ${result.stderr || result.stdout}`,
    );
  }
}

async function stopInstantSearchRuntime(aybCommand, runtimeEnv, managedProcesses) {
  await stopManagedProcesses(managedProcesses.filter((proc) => proc.role !== "api"));
  if (managedProcesses.some((proc) => proc.role === "api")) {
    stopAybServer(aybCommand, runtimeEnv);
  }
  await stopManagedProcesses(managedProcesses.filter((proc) => proc.role === "api"));
}

function stopAybServer(aybCommand, runtimeEnv) {
  spawnSync(aybCommand.command, [...aybCommand.args, "stop"], {
    cwd: REPO_ROOT,
    env: runtimeEnv,
    encoding: "utf-8",
  });
}

function removeRuntimeHome(runtimeHome) {
  try {
    rmSync(runtimeHome, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  } catch {
    // The runtime HOME lives under the OS temp directory; process ownership has already ended.
  }
}

async function stopManagedProcesses(managedProcesses) {
  for (const managedProcess of [...managedProcesses].reverse()) {
    await stopProcessWithFallback(managedProcess);
  }
}

async function stopProcessWithFallback(managedProcess) {
  const pid = managedProcess.process.pid;
  if (!pid || !isProcessAlive(pid)) return;

  managedProcess.process.kill("SIGINT");
  const exitedGracefully = await waitForProcessExit(
    managedProcess.process,
    GRACEFUL_EXIT_TIMEOUT_MS,
  );
  if (!exitedGracefully && isProcessAlive(pid)) {
    managedProcess.process.kill("SIGKILL");
    await waitForProcessExit(managedProcess.process, 2_000);
  }
}

function waitForProcessExit(proc, timeoutMs) {
  if (proc.exitCode !== null || proc.signalCode !== null) return Promise.resolve(true);

  return new Promise((resolve) => {
    const timeout = setTimeout(() => {
      cleanup();
      resolve(false);
    }, timeoutMs);
    const onExit = () => {
      cleanup();
      resolve(true);
    };
    const cleanup = () => {
      clearTimeout(timeout);
      proc.removeListener("exit", onExit);
      proc.removeListener("close", onExit);
    };

    proc.once("exit", onExit);
    proc.once("close", onExit);
  });
}

function isProcessAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function sleep(ms) {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}
