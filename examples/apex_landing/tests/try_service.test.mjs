import assert from "node:assert/strict";
import test from "node:test";

import { createTryLaunch, getTryLaunchStatus } from "../functions/_lib/try_service.js";

class MemoryKV {
  values = new Map();
  blockCooldown = false;
  async get(key, type) {
    if (this.blockCooldown && key.startsWith("cooldown:")) return "1";
    const value = this.values.get(key) ?? null;
    return type === "json" && value ? JSON.parse(value) : value;
  }
  async put(key, value) { this.values.set(key, value); }
  async delete(key) { this.values.delete(key); }
}

function response(status, body = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
}

function fixture() {
  const calls = [];
  const env = {
    TRY_STATE: new MemoryKV(),
    DAYTONA_API_KEY: "daytona-secret",
    DAYTONA_API_URL: "https://app.daytona.io/api",
    TURNSTILE_SECRET_KEY: "turnstile-secret",
    TRY_RATE_LIMIT_SECRET: "rate-secret",
    TRY_AYB_IMAGE: "ghcr.io/allyourbasehq/allyourbase@sha256:abc123",
    TRY_MAX_ACTIVE: "3",
    TRY_TTL_MINUTES: "30",
  };
  const fetchImpl = async (url, init = {}) => {
    calls.push({
      url: String(url),
      init,
      body: typeof init.body === "string" && init.headers?.["content-type"] === "application/json"
        ? JSON.parse(init.body)
        : null,
    });
    if (String(url).includes("siteverify")) return response(200, { success: true });
    if (String(url).endsWith("/sandbox") && (init.method ?? "GET") === "GET") return response(200, { items: [] });
    if (String(url).endsWith("/sandbox") && init.method === "POST") {
      return response(200, { id: "sandbox-1", state: "pending_build" });
    }
    if (String(url).endsWith("/ttl/30")) return response(201);
    throw new Error(`unexpected fetch ${init.method ?? "GET"} ${url}`);
  };
  return { calls, env, fetchImpl };
}

test("create uses a private, resource-bounded, expiring sandbox and stores only server-side state", async () => {
  const { calls, env, fetchImpl } = fixture();
  const result = await createTryLaunch({
    env, fetchImpl, clientIp: "203.0.113.7", turnstileToken: "human-token",
    now: () => 1_800_000_000_000,
    randomUUID: () => "launch-uuid",
    randomSecret: () => "generated-secret",
  });

  assert.deepEqual(result, { launchToken: "launch-uuid", expiresAt: "2027-01-15T08:30:00.000Z" });
  const create = calls.find((call) => call.url.endsWith("/sandbox") && call.init.method === "POST");
  assert.deepEqual(create.body, {
    name: "ayb-try-launch-uuid",
    buildInfo: { dockerfileContent: "FROM ghcr.io/allyourbasehq/allyourbase@sha256:abc123" },
    cpu: 1, memory: 1, disk: 5, public: false, ttlMinutes: 30,
    autoStopInterval: 0, autoPauseInterval: 0, autoArchiveInterval: 0, autoDeleteInterval: 0,
    env: {
      AYB_ADMIN_PASSWORD: "generated-secret",
      AYB_AUTH_ENABLED: "true",
      AYB_AUTH_JWT_SECRET: "generated-secret",
      AYB_SERVER_HOST: "0.0.0.0",
      AYB_STORAGE_ENABLED: "true",
      AYB_STORAGE_LOCAL_PATH: "/home/ayb/.ayb/storage",
    },
    labels: { "allyourbase-purpose": "try", "allyourbase-launch": "launch-uuid" },
  });
  assert.ok(calls.some((call) => call.url.endsWith("/sandbox/sandbox-1/ttl/30") && call.init.method === "POST"));
  const state = await env.TRY_STATE.get("launch:launch-uuid", "json");
  assert.equal(state.sandboxId, "sandbox-1");
  assert.equal(state.adminPassword, "generated-secret");
  assert.doesNotMatch(JSON.stringify(result), /daytona-secret|generated-secret/);
});

test("failed TTL enforcement deletes the sandbox and leaves no launch", async () => {
  const { calls, env } = fixture();
  const fetchImpl = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url).includes("siteverify")) return response(200, { success: true });
    if (String(url).endsWith("/sandbox") && (init.method ?? "GET") === "GET") return response(200, { items: [] });
    if (String(url).endsWith("/sandbox") && init.method === "POST") return response(200, { id: "sandbox-1" });
    if (String(url).endsWith("/ttl/30")) return response(500, { message: "no ttl" });
    if (String(url).endsWith("/sandbox/sandbox-1") && init.method === "DELETE") return response(200);
    throw new Error(`unexpected fetch ${init.method ?? "GET"} ${url}`);
  };
  await assert.rejects(() => createTryLaunch({
    env, fetchImpl, clientIp: "203.0.113.8", turnstileToken: "human-token",
    randomUUID: () => "launch-uuid", randomSecret: () => "generated-secret",
  }), /safe expiration/i);
  assert.ok(calls.some((call) => call.url.endsWith("/sandbox/sandbox-1") && call.init.method === "DELETE"));
  assert.equal(await env.TRY_STATE.get("launch:launch-uuid"), null);
});

test("rate limit and capacity reject before sandbox creation", async () => {
  const limited = fixture();
  limited.env.TRY_STATE.blockCooldown = true;
  await assert.rejects(() => createTryLaunch({
    env: limited.env, fetchImpl: limited.fetchImpl, clientIp: "203.0.113.9", turnstileToken: "human-token",
  }), (error) => error.status === 429);
  assert.equal(limited.calls.length, 0);

  const full = fixture();
  full.env.TRY_MAX_ACTIVE = "1";
  full.fetchImpl = async (url, init = {}) => {
    full.calls.push({ url: String(url), init });
    if (String(url).includes("siteverify")) return response(200, { success: true });
    if (String(url).endsWith("/sandbox")) return response(200, { items: [{ labels: { "allyourbase-purpose": "try" } }] });
    throw new Error("sandbox create must not be called");
  };
  await assert.rejects(() => createTryLaunch({
    env: full.env, fetchImpl: full.fetchImpl, clientIp: "203.0.113.10", turnstileToken: "human-token",
  }), (error) => error.status === 503);
  assert.equal(full.calls.filter((call) => call.init.method === "POST" && call.url.endsWith("/sandbox")).length, 0);
});

test("missing human challenge rejects without any external request", async () => {
  const { calls, env, fetchImpl } = fixture();
  await assert.rejects(() => createTryLaunch({
    env, fetchImpl, clientIp: "203.0.113.11", turnstileToken: "",
  }), (error) => error.status === 400 && error.code === "challenge_required");
  assert.equal(calls.length, 0);
});

test("status starts AYB once, waits for exact health, then returns admin credentials", async () => {
  const { env } = fixture();
  await env.TRY_STATE.put("launch:launch-uuid", JSON.stringify({
    sandboxId: "sandbox-1", adminPassword: "admin-pass",
    expiresAt: "2027-01-15T08:30:00.000Z", processStarted: false,
  }));
  const calls = [];
  let healthy = false;
  const fetchImpl = async (url, init = {}) => {
    calls.push({ url: String(url), init, body: init.body ? JSON.parse(init.body) : null });
    if (String(url).endsWith("/sandbox/sandbox-1")) {
      return response(200, { id: "sandbox-1", state: "started", toolboxProxyUrl: "https://toolbox.example" });
    }
    if (String(url).endsWith("/process/session")) return response(201);
    if (String(url).endsWith("/process/session/ayb-server/exec")) return response(201, { cmdId: "cmd-1" });
    if (String(url).includes("signed-preview-url")) return response(200, { url: "https://private.example/?token=signed" });
    if (String(url).includes("private.example")) {
      return healthy ? response(200, { status: "ok", database: "ok" }) : response(502, { message: "starting" });
    }
    throw new Error(`unexpected fetch ${init.method ?? "GET"} ${url}`);
  };

  assert.deepEqual(await getTryLaunchStatus({ env, fetchImpl, launchToken: "launch-uuid" }), { status: "starting" });
  const exec = calls.find((call) => call.url.endsWith("/process/session/ayb-server/exec"));
  assert.deepEqual(exec.body, {
    command: "AYB_STORAGE_LOCAL_PATH=/home/ayb/.ayb/storage /usr/local/bin/docker-entrypoint.sh ayb start --foreground",
    runAsync: true,
    suppressInputEcho: true,
  });
  healthy = true;
  assert.deepEqual(await getTryLaunchStatus({ env, fetchImpl, launchToken: "launch-uuid" }), {
    status: "ready",
    adminUrl: "https://private.example/admin?token=signed",
    adminPassword: "admin-pass",
    expiresAt: "2027-01-15T08:30:00.000Z",
  });
  assert.equal(calls.filter((call) => call.url.endsWith("/process/session/ayb-server/exec")).length, 1);
});
