import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { accessSync, constants } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export type DemoName = "kanban" | "live-polls" | "movies";

export interface DemoTarget {
  name: DemoName;
  port: number;
  url: string;
  runtime?: DemoRuntimeConfig;
}

interface DemoRuntimeConfig {
  requiresAiSearchRuntime?: boolean;
  aybConfigPath?: string;
  managedPorts?: readonly number[];
}

export interface DemoRuntimePlan {
  startFakeOllama: boolean;
  aybConfigPath?: string;
}

export const STAGE5_NOT_IMPLEMENTED_ERROR =
  "Stage 5: demo orchestration is not yet implemented for this demo target.";

const API_HEALTH_URL = "http://127.0.0.1:8090/health";
const API_PORT = 8090;
const OLLAMA_FAKE_PORT = 11_434;
const OLLAMA_FAKE_HEALTH_URL = `http://127.0.0.1:${OLLAMA_FAKE_PORT}/health`;
const READINESS_TIMEOUT_MS = 60_000;
const POLL_INTERVAL_MS = 500;
const GRACEFUL_EXIT_TIMEOUT_MS = 10_000;
const STAGE3_JWT_SECRET = "stage3-cross-demo-jwt-secret-minimum-32-bytes";
const FIXTURE_DIR = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(FIXTURE_DIR, "..", "..");
const MOVIES_E2E_CONFIG_PATH = join(FIXTURE_DIR, "ayb_movies_e2e.toml");
const MOVIES_FAKE_OLLAMA_SCRIPT_PATH = join(REPO_ROOT, "examples", "movies", "e2e", "fake_ollama_server.cjs");

export const DEMO_TARGETS: Record<"kanban" | "livePolls" | "movies", DemoTarget> = {
  kanban: {
    name: "kanban",
    port: 5173,
    url: "http://127.0.0.1:5173",
  },
  livePolls: {
    name: "live-polls",
    port: 5175,
    url: "http://127.0.0.1:5175",
  },
  movies: {
    name: "movies",
    port: 5177,
    url: "http://127.0.0.1:5177",
    runtime: {
      requiresAiSearchRuntime: true,
      aybConfigPath: MOVIES_E2E_CONFIG_PATH,
      managedPorts: [OLLAMA_FAKE_PORT],
    },
  },
};

function uniquePorts(ports: readonly number[]): readonly number[] {
  return [...new Set(ports)];
}

function managedPortsFromTargets(targets: readonly DemoTarget[]): readonly number[] {
  return uniquePorts([
    API_PORT,
    ...targets.map((target) => target.port),
    ...targets.flatMap((target) => target.runtime?.managedPorts ?? []),
  ]);
}

function managedPortsForDemoTarget(demoTarget: DemoTarget): readonly number[] {
  return uniquePorts([
    API_PORT,
    demoTarget.port,
    ...(demoTarget.runtime?.managedPorts ?? []),
  ]);
}

// Single source of truth for fixture-owned ports — derived from DEMO_TARGETS, the
// shared API port, and any demo runtime ports required by shared orchestration.
const MANAGED_PORTS: readonly number[] = managedPortsFromTargets(Object.values(DEMO_TARGETS));

function runtimePlanForDemoTarget(demoTarget: DemoTarget): DemoRuntimePlan {
  return {
    startFakeOllama: demoTarget.runtime?.requiresAiSearchRuntime === true,
    aybConfigPath: demoTarget.runtime?.aybConfigPath,
  };
}

export function managedPortsForTest(): readonly number[] {
  return MANAGED_PORTS;
}

export function managedPortsForDemoTargetForTest(demoTarget: DemoTarget): readonly number[] {
  return managedPortsForDemoTarget(demoTarget);
}

export function runtimePlanForTest(demoTarget: DemoTarget): DemoRuntimePlan {
  return runtimePlanForDemoTarget(demoTarget);
}

// Demos for which fixture orchestration is implemented. Adding a demo here is the
// single switch that promotes it out of the not-yet-implemented rejection path;
// this set should include every DemoName with a green roundtrip contract.
const IMPLEMENTED_DEMOS: ReadonlySet<DemoName> = new Set<DemoName>([
  "kanban",
  "live-polls",
  "movies",
]);

function demoTargetByName(demoName: DemoName): DemoTarget {
  const target = Object.values(DEMO_TARGETS).find((candidate) => candidate.name === demoName);
  if (!target) {
    throw new Error(`Unknown demo target: ${demoName}`);
  }
  return target;
}

export interface OrchestrationContext {
  demoTarget: DemoTarget;
}

interface SpawnedProcess {
  process: ChildProcess;
}

function resolveAybBin(): string {
  if (process.env.AYB_BIN && process.env.AYB_BIN.trim() !== "") {
    return process.env.AYB_BIN;
  }

  const candidateBinaries = [
    join(process.cwd(), "ayb"),
    join(REPO_ROOT, "ayb"),
  ];

  for (const candidate of candidateBinaries) {
    try {
      accessSync(candidate, constants.X_OK);
      return candidate;
    } catch {
      // continue searching candidate paths
    }
  }

  return "ayb";
}

function spawnManagedProcess(
  command: string,
  args: string[],
  envOverrides?: NodeJS.ProcessEnv,
): SpawnedProcess {
  const proc = spawn(command, args, {
    cwd: REPO_ROOT,
    env: {
      ...process.env,
      ...envOverrides,
    },
    // Keep Playwright output deterministic by suppressing child-process logs.
    stdio: "ignore",
  });

  return {
    process: proc,
  };
}

async function waitForUrl(url: string, timeoutMs: number): Promise<void> {
  const deadlineMs = Date.now() + timeoutMs;
  while (Date.now() < deadlineMs) {
    try {
      const response = await fetch(url, {
        signal: AbortSignal.timeout(2_000),
      });
      if (response.ok) {
        return;
      }
    } catch {
      // keep polling until timeout
    }
    await sleep(POLL_INTERVAL_MS);
  }

  throw new Error(`Timed out waiting for URL readiness: ${url}`);
}

async function waitForFakeOllama(url: string, timeoutMs: number): Promise<void> {
  const deadlineMs = Date.now() + timeoutMs;
  while (Date.now() < deadlineMs) {
    try {
      const response = await fetch(url, {
        signal: AbortSignal.timeout(2_000),
      });
      if (response.ok) {
        const payload = (await response.json()) as { ok?: boolean };
        if (payload.ok === true) {
          return;
        }
      }
    } catch {
      // keep polling until timeout
    }
    await sleep(POLL_INTERVAL_MS);
  }

  throw new Error(`Timed out waiting for fake Ollama readiness: ${url}`);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

async function stopProcessWithFallback(managedProcess: SpawnedProcess): Promise<void> {
  const pid = managedProcess.process.pid;
  if (!pid || !isProcessAlive(pid)) {
    return;
  }

  managedProcess.process.kill("SIGINT");
  const exitedGracefully = await waitForProcessExit(managedProcess.process, GRACEFUL_EXIT_TIMEOUT_MS);
  if (!exitedGracefully && isProcessAlive(pid)) {
    managedProcess.process.kill("SIGKILL");
    await waitForProcessExit(managedProcess.process, 2_000);
  }
}

function waitForProcessExit(proc: ChildProcess, timeoutMs: number): Promise<boolean> {
  if (proc.exitCode !== null || proc.killed) {
    return Promise.resolve(true);
  }

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

function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function pidsForPort(port: number): number[] {
  const result = spawnSync("lsof", ["-ti", `:${port}`], {
    encoding: "utf-8",
  });

  if (result.status !== 0 || !result.stdout.trim()) {
    return [];
  }

  return result.stdout
    .trim()
    .split("\n")
    .map((value) => Number.parseInt(value, 10))
    .filter((value) => Number.isInteger(value) && value > 0);
}

function killPid(pid: number, signal: NodeJS.Signals): void {
  try {
    process.kill(pid, signal);
  } catch {
    // already exited or inaccessible
  }
}

/**
 * Best-effort SIGINT then SIGKILL sweep of any listeners on the given ports.
 * Used to guarantee a clean fixture start and to scrub leftovers on teardown.
 */
async function cleanupManagedPorts(ports: readonly number[]): Promise<void> {
  for (const port of ports) {
    const pids = pidsForPort(port);
    for (const pid of pids) {
      killPid(pid, "SIGINT");
    }
  }

  await sleep(1_000);

  for (const port of ports) {
    const pids = pidsForPort(port);
    for (const pid of pids) {
      killPid(pid, "SIGKILL");
    }
  }
}

export async function assertNoManagedPortListeners(ports: readonly number[]): Promise<void> {
  const leakingPorts = ports.filter((port) => pidsForPort(port).length > 0);
  if (leakingPorts.length > 0) {
    throw new Error(`Teardown leak: listeners still active on ports ${leakingPorts.join(", ")}`);
  }
}

/**
 * Boot the shared API server and the named demo SPA, then run the caller's
 * roundtrip callback against the demo. Tears down spawned PIDs and any
 * port-leakers deterministically. Demos not in IMPLEMENTED_DEMOS reject through
 * the explicit not-yet-implemented path so Stage N+1 only has to flip one set.
 */
export async function orchestrateDemoRoundtrip(
  demoName: DemoName,
  executeRoundtrip: (context: OrchestrationContext) => Promise<void>,
): Promise<void> {
  if (!IMPLEMENTED_DEMOS.has(demoName)) {
    throw new Error(`${STAGE5_NOT_IMPLEMENTED_ERROR} Demo: ${demoName}`);
  }

  const demoTarget = demoTargetByName(demoName);
  const runtimePlan = runtimePlanForDemoTarget(demoTarget);
  const orchestrationManagedPorts = managedPortsForDemoTarget(demoTarget);
  const aybBin = resolveAybBin();
  const managedProcesses: SpawnedProcess[] = [];

  // Kill any pre-existing listeners on test-owned ports to guarantee a clean fixture start.
  await cleanupManagedPorts(orchestrationManagedPorts);

  try {
    if (runtimePlan.startFakeOllama) {
      const fakeOllamaProcess = spawnManagedProcess("node", [MOVIES_FAKE_OLLAMA_SCRIPT_PATH]);
      managedProcesses.push(fakeOllamaProcess);

      await waitForFakeOllama(OLLAMA_FAKE_HEALTH_URL, READINESS_TIMEOUT_MS);
    }

    const apiStartArgs = runtimePlan.aybConfigPath
      ? ["start", "--config", runtimePlan.aybConfigPath]
      : ["start"];

    const apiProcess = spawnManagedProcess(aybBin, apiStartArgs, {
      AYB_AUTH_ENABLED: "true",
      AYB_AUTH_JWT_SECRET: STAGE3_JWT_SECRET,
      AYB_AUTH_RATE_LIMIT: "10000",
      AYB_AUTH_RATE_LIMIT_AUTH: "10000/min",
      AYB_AUTH_ANONYMOUS_RATE_LIMIT: "10000",
      AYB_RATE_LIMIT_API_ANONYMOUS: "10000/min",
      AYB_RATE_LIMIT_API: "10000/min",
    });
    managedProcesses.push(apiProcess);

    await waitForUrl(API_HEALTH_URL, READINESS_TIMEOUT_MS);

    const demoProcess = spawnManagedProcess(aybBin, ["demo", demoName]);
    managedProcesses.push(demoProcess);

    await waitForUrl(demoTarget.url, READINESS_TIMEOUT_MS);

    await executeRoundtrip({ demoTarget });
  } finally {
    for (const managedProcess of [...managedProcesses].reverse()) {
      await stopProcessWithFallback(managedProcess);
    }

    await cleanupManagedPorts(orchestrationManagedPorts);
    await assertNoManagedPortListeners(orchestrationManagedPorts);
    console.log(
      `[E2E-TEARDOWN-OK] demo=${demoName} ports=${orchestrationManagedPorts.join(",")}`,
    );
  }
}
