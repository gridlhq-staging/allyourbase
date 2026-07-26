const DAYTONA_PURPOSE_LABEL = "allyourbase-purpose";
const DAYTONA_PURPOSE_VALUE = "try";
const SERVER_SESSION_ID = "ayb-server";
const SERVER_COMMAND = "AYB_STORAGE_LOCAL_PATH=/home/ayb/.ayb/storage /usr/local/bin/docker-entrypoint.sh ayb start --foreground";
const DEFAULT_TTL_MINUTES = 30;
const DEFAULT_MAX_ACTIVE = 3;

export class TryServiceError extends Error {
  constructor(message, status = 500, code = "launch_failed") {
    super(message);
    this.name = "TryServiceError";
    this.status = status;
    this.code = code;
  }
}

function required(env, name) {
  const value = env[name]?.trim?.() ?? "";
  if (!value) throw new TryServiceError(`Missing server configuration: ${name}`);
  return value;
}

function positiveInteger(value, fallback) {
  const parsed = Number.parseInt(value ?? "", 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function daytonaHeaders(env, hasBody = false) {
  return {
    Authorization: `Bearer ${required(env, "DAYTONA_API_KEY")}`,
    ...(hasBody ? { "content-type": "application/json" } : {}),
  };
}

async function responseJSON(response, operation) {
  if (!response.ok) {
    throw new TryServiceError(`${operation} failed safely`, 502);
  }
  return response.json();
}

async function cooldownKey(env, clientIp) {
  const material = new TextEncoder().encode(`${required(env, "TRY_RATE_LIMIT_SECRET")}:${clientIp}`);
  const digest = await crypto.subtle.digest("SHA-256", material);
  return `cooldown:${Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

async function verifyTurnstile(env, fetchImpl, token, clientIp) {
  if (!token) throw new TryServiceError("Please complete the human check.", 400, "challenge_required");
  const body = new URLSearchParams({
    secret: required(env, "TURNSTILE_SECRET_KEY"),
    response: token,
    remoteip: clientIp,
  });
  const response = await fetchImpl("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
    method: "POST",
    body,
  });
  const result = await responseJSON(response, "Human check");
  if (result.success !== true) {
    throw new TryServiceError("The human check expired. Please try again.", 403, "challenge_failed");
  }
}

function sandboxIsTryInstance(sandbox) {
  return sandbox?.labels?.[DAYTONA_PURPOSE_LABEL] === DAYTONA_PURPOSE_VALUE;
}

async function listActiveTrySandboxes(env, fetchImpl) {
  const baseURL = required(env, "DAYTONA_API_URL").replace(/\/$/, "");
  const response = await fetchImpl(`${baseURL}/sandbox`, { headers: daytonaHeaders(env) });
  const result = await responseJSON(response, "Capacity check");
  return (result.items ?? []).filter(sandboxIsTryInstance);
}

async function deleteSandbox(env, fetchImpl, sandboxId) {
  const baseURL = required(env, "DAYTONA_API_URL").replace(/\/$/, "");
  await fetchImpl(`${baseURL}/sandbox/${encodeURIComponent(sandboxId)}`, {
    method: "DELETE",
    headers: daytonaHeaders(env),
  }).catch(() => undefined);
}

function sandboxRequest(env, launchToken, adminPassword, jwtSecret, ttlMinutes) {
  const image = required(env, "TRY_AYB_IMAGE");
  if (!image.includes("@sha256:")) {
    throw new TryServiceError("The server image must be pinned to an immutable digest.");
  }
  return {
    name: `ayb-try-${launchToken}`.slice(0, 63),
    buildInfo: { dockerfileContent: `FROM ${image}` },
    cpu: 1,
    memory: 1,
    disk: 5,
    public: false,
    ttlMinutes,
    autoStopInterval: 0,
    autoPauseInterval: 0,
    autoArchiveInterval: 0,
    autoDeleteInterval: 0,
    env: {
      AYB_ADMIN_PASSWORD: adminPassword,
      AYB_AUTH_ENABLED: "true",
      AYB_AUTH_JWT_SECRET: jwtSecret,
      AYB_SERVER_HOST: "0.0.0.0",
      AYB_STORAGE_ENABLED: "true",
      AYB_STORAGE_LOCAL_PATH: "/home/ayb/.ayb/storage",
    },
    labels: {
      [DAYTONA_PURPOSE_LABEL]: DAYTONA_PURPOSE_VALUE,
      "allyourbase-launch": launchToken,
    },
  };
}

export async function createTryLaunch({
  env,
  clientIp,
  turnstileToken,
  fetchImpl = fetch,
  now = Date.now,
  randomUUID = () => crypto.randomUUID(),
  randomSecret = () => `${crypto.randomUUID()}${crypto.randomUUID()}`,
}) {
  const stateStore = env.TRY_STATE;
  if (!stateStore) throw new TryServiceError("Launch state is unavailable.");
  const rateKey = await cooldownKey(env, clientIp);
  if (await stateStore.get(rateKey)) {
    throw new TryServiceError("You recently launched an instance. Please try again later.", 429, "cooldown");
  }

  await verifyTurnstile(env, fetchImpl, turnstileToken, clientIp);
  const active = await listActiveTrySandboxes(env, fetchImpl);
  if (active.length >= positiveInteger(env.TRY_MAX_ACTIVE, DEFAULT_MAX_ACTIVE)) {
    throw new TryServiceError("All trial instances are busy. Please try again soon.", 503, "capacity");
  }

  const launchToken = randomUUID();
  const adminPassword = randomSecret();
  const jwtSecret = randomSecret();
  const ttlMinutes = positiveInteger(env.TRY_TTL_MINUTES, DEFAULT_TTL_MINUTES);
  const baseURL = required(env, "DAYTONA_API_URL").replace(/\/$/, "");
  let sandboxId = "";
  try {
    const createResponse = await fetchImpl(`${baseURL}/sandbox`, {
      method: "POST",
      headers: daytonaHeaders(env, true),
      body: JSON.stringify(sandboxRequest(env, launchToken, adminPassword, jwtSecret, ttlMinutes)),
    });
    const sandbox = await responseJSON(createResponse, "Sandbox creation");
    sandboxId = sandbox.id;
    if (!sandboxId) throw new TryServiceError("Sandbox creation returned no identifier.", 502);

    const ttlResponse = await fetchImpl(`${baseURL}/sandbox/${encodeURIComponent(sandboxId)}/ttl/${ttlMinutes}`, {
      method: "POST",
      headers: daytonaHeaders(env),
    });
    if (!ttlResponse.ok) throw new TryServiceError("Sandbox safe expiration could not be enforced.", 502);

    const expiresAt = new Date(now() + ttlMinutes * 60_000).toISOString();
    await stateStore.put(`launch:${launchToken}`, JSON.stringify({
      sandboxId,
      adminPassword,
      expiresAt,
      processStarted: false,
    }), { expirationTtl: ttlMinutes * 60 + 300 });
    await stateStore.put(rateKey, "1", { expirationTtl: 3600 });
    return { launchToken, expiresAt };
  } catch (error) {
    if (sandboxId) await deleteSandbox(env, fetchImpl, sandboxId);
    throw error;
  }
}

function isTerminalState(state) {
  return ["error", "destroyed", "destroying", "archived", "stopped"].includes(state);
}

async function startServer(env, fetchImpl, sandbox, state) {
  const toolboxBase = `${sandbox.toolboxProxyUrl.replace(/\/$/, "")}/${encodeURIComponent(sandbox.id)}`;
  const headers = daytonaHeaders(env, true);
  const sessionResponse = await fetchImpl(`${toolboxBase}/process/session`, {
    method: "POST",
    headers,
    body: JSON.stringify({ sessionId: SERVER_SESSION_ID }),
  });
  if (!sessionResponse.ok && sessionResponse.status !== 409) {
    throw new TryServiceError("The private server session could not start.", 502);
  }
  const execResponse = await fetchImpl(`${toolboxBase}/process/session/${SERVER_SESSION_ID}/exec`, {
    method: "POST",
    headers,
    body: JSON.stringify({ command: SERVER_COMMAND, runAsync: true, suppressInputEcho: true }),
  });
  if (!execResponse.ok) throw new TryServiceError("Allyourbase could not start.", 502);
  state.processStarted = true;
}

async function signedPreviewURL(env, fetchImpl, sandboxId) {
  const baseURL = required(env, "DAYTONA_API_URL").replace(/\/$/, "");
  const response = await fetchImpl(`${baseURL}/sandbox/${encodeURIComponent(sandboxId)}/ports/8090/signed-preview-url?expiresInSeconds=3600`, {
    headers: daytonaHeaders(env),
  });
  const result = await responseJSON(response, "Private preview creation");
  if (!result.url) throw new TryServiceError("Private preview creation returned no URL.", 502);
  return result.url;
}

function URLAtPath(source, path) {
  const url = new URL(source);
  url.pathname = path;
  return url.toString();
}

export async function getTryLaunchStatus({ env, launchToken, fetchImpl = fetch }) {
  const stateStore = env.TRY_STATE;
  const state = await stateStore?.get(`launch:${launchToken}`, "json");
  if (!state) throw new TryServiceError("This launch has expired or does not exist.", 404, "not_found");

  const baseURL = required(env, "DAYTONA_API_URL").replace(/\/$/, "");
  const sandboxResponse = await fetchImpl(`${baseURL}/sandbox/${encodeURIComponent(state.sandboxId)}`, {
    headers: daytonaHeaders(env),
  });
  if (!sandboxResponse.ok) throw new TryServiceError("The private instance is no longer available.", 410, "gone");
  const sandbox = await sandboxResponse.json();
  if (isTerminalState(sandbox.state)) {
    await deleteSandbox(env, fetchImpl, state.sandboxId);
    await stateStore.delete(`launch:${launchToken}`);
    throw new TryServiceError("The private instance could not start. Please try again.", 502);
  }
  if (sandbox.state !== "started") return { status: "starting" };

  if (!state.processStarted) await startServer(env, fetchImpl, sandbox, state);
  if (!state.previewUrl) state.previewUrl = await signedPreviewURL(env, fetchImpl, state.sandboxId);
  await stateStore.put(`launch:${launchToken}`, JSON.stringify(state), { expirationTtl: 2100 });

  const healthResponse = await fetchImpl(URLAtPath(state.previewUrl, "/health"), {
    headers: { "X-Daytona-Skip-Preview-Warning": "true" },
  }).catch(() => null);
  if (!healthResponse?.ok) return { status: "starting" };
  const health = await healthResponse.json().catch(() => null);
  if (health?.status !== "ok" || health?.database !== "ok") return { status: "starting" };
  return {
    status: "ready",
    adminUrl: URLAtPath(state.previewUrl, "/admin"),
    adminPassword: state.adminPassword,
    expiresAt: state.expiresAt,
  };
}
