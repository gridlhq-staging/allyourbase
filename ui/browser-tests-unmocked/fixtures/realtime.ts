import { createHash, randomBytes } from "node:crypto";
import * as http from "node:http";
import * as https from "node:https";
import type { Socket } from "node:net";
import type { TLSSocket } from "node:tls";
import type { Page } from "@playwright/test";

const REALTIME_WS_PATH = "/api/realtime/ws";

export interface SSECaptureHandle {
  getEvents: () => Promise<Array<Record<string, unknown>>>;
  close: () => Promise<void>;
}

interface SSEFrame {
  data: string;
  event: string;
}

interface RealtimeSSEOpenResult {
  body: ReadableStream<Uint8Array>;
}

type RealtimeWSSocket = Socket | TLSSocket;

interface RealtimeWsSubscriptionHandle {
  closed: boolean;
  closePromise: Promise<void>;
  socket: RealtimeWSSocket;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function openRealtimeSSE(
  endpoint: URL,
  token: string,
  signal: AbortSignal,
): Promise<RealtimeSSEOpenResult> {
  let lastError = "";
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const response = await fetch(endpoint, {
      headers: {
        Accept: "text/event-stream",
        Authorization: `Bearer ${token}`,
      },
      signal,
    });
    if (response.ok) {
      if (!response.body) {
        throw new Error("Realtime SSE response body was empty");
      }
      return { body: response.body };
    }

    lastError = await response.text().catch(() => "");
    if (response.status !== 400 || !lastError.includes("unknown table")) {
      throw new Error(`Realtime SSE request failed with status ${response.status}: ${lastError}`);
    }
    await delay(250);
  }

  throw new Error(`Realtime SSE request failed while waiting for schema cache: ${lastError}`);
}

function emitSSEFrame(
  pendingEvent: { dataLines: string[]; event: string | null },
  onFrame: (frame: SSEFrame) => void,
): void {
  if (!pendingEvent.event && pendingEvent.dataLines.length === 0) {
    return;
  }
  onFrame({
    data: pendingEvent.dataLines.join("\n"),
    event: pendingEvent.event ?? "message",
  });
  pendingEvent.event = null;
  pendingEvent.dataLines = [];
}

async function consumeSSEStream(
  stream: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void,
): Promise<void> {
  const decoder = new TextDecoder();
  const reader = stream.getReader();
  const pendingEvent = { dataLines: [] as string[], event: null as string | null };
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });
      if (done && buffer.length > 0 && !buffer.endsWith("\n")) {
        buffer += "\n";
      }

      let newlineIndex = buffer.indexOf("\n");
      while (newlineIndex !== -1) {
        let line = buffer.slice(0, newlineIndex);
        buffer = buffer.slice(newlineIndex + 1);
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }

        if (line.length === 0) {
          emitSSEFrame(pendingEvent, onFrame);
          newlineIndex = buffer.indexOf("\n");
          continue;
        }

        if (!line.startsWith(":")) {
          const separatorIndex = line.indexOf(":");
          const field = separatorIndex === -1 ? line : line.slice(0, separatorIndex);
          let valueText = separatorIndex === -1 ? "" : line.slice(separatorIndex + 1);
          if (valueText.startsWith(" ")) {
            valueText = valueText.slice(1);
          }
          if (field === "event") {
            pendingEvent.event = valueText;
          } else if (field === "data") {
            pendingEvent.dataLines.push(valueText);
          }
        }
        newlineIndex = buffer.indexOf("\n");
      }

      if (done) {
        emitSSEFrame(pendingEvent, onFrame);
        return;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

export async function startSSECapture(
  page: Page,
  baseURL: string,
  token: string,
  tables: string[],
): Promise<SSECaptureHandle> {
  void page;

  const endpoint = new URL("/api/realtime", baseURL);
  endpoint.searchParams.set("tables", tables.join(","));

  const abortController = new AbortController();
  const events: Array<Record<string, unknown>> = [];

  let connected = false;
  let resolveConnected: (() => void) | null = null;
  let rejectConnected: ((error: Error) => void) | null = null;
  const connectedPromise = new Promise<void>((resolve, reject) => {
    resolveConnected = resolve;
    rejectConnected = reject;
  });
  const timeoutHandle = setTimeout(() => {
    if (connected) {
      return;
    }
    abortController.abort();
    rejectConnected?.(new Error("Timed out waiting for SSE connected event"));
  }, 10_000);

  const streamTask = (async () => {
    const { body: responseBody } = await openRealtimeSSE(endpoint, token, abortController.signal);

    await consumeSSEStream(responseBody, ({ event, data }) => {
      if (event === "connected") {
        if (!connected) {
          connected = true;
          clearTimeout(timeoutHandle);
          resolveConnected?.();
        }
        return;
      }

      if (event !== "message" || data.length === 0) {
        return;
      }

      try {
        events.push(JSON.parse(data) as Record<string, unknown>);
      } catch {
        // Ignore malformed data events; tests consume successfully parsed records only.
      }
    });

    if (!connected) {
      throw new Error("Realtime SSE stream closed before connected event");
    }
  })().catch((error: unknown) => {
    clearTimeout(timeoutHandle);
    if (!connected && !abortController.signal.aborted) {
      rejectConnected?.(error instanceof Error ? error : new Error(String(error)));
    }
    if (abortController.signal.aborted) {
      return;
    }
    throw error;
  });

  await connectedPromise;

  return {
    getEvents: async () => events.slice(),
    close: async () => {
      clearTimeout(timeoutHandle);
      abortController.abort();
      await streamTask;
    },
  };
}

function buildRealtimeWsUrl(currentPageUrl: string): URL {
  const currentURL = new URL(currentPageUrl);
  const wsProtocol = currentURL.protocol === "https:" ? "wss:" : "ws:";
  const wsURL = new URL(REALTIME_WS_PATH, `${wsProtocol}//${currentURL.host}`);
  return wsURL;
}

function describeUpgradeFailure(res: http.IncomingMessage): string {
  const { statusCode, statusMessage } = res;
  if (statusCode === undefined) {
    return "WebSocket handshake failed before an HTTP status was returned";
  }
  const statusText = statusMessage ? ` ${statusMessage}` : "";
  return `WebSocket handshake failed with status ${statusCode}${statusText}`;
}

function encodeWebSocketFrame(opcode: number, payloadText: string): Buffer {
  const payload = Buffer.from(payloadText, "utf-8");
  const mask = randomBytes(4);

  let header = Buffer.from([0x80 | opcode, 0x80 | payload.length]);
  if (payload.length >= 126 && payload.length <= 0xffff) {
    header = Buffer.alloc(4);
    header[0] = 0x80 | opcode;
    header[1] = 0x80 | 126;
    header.writeUInt16BE(payload.length, 2);
  } else if (payload.length > 0xffff) {
    header = Buffer.alloc(10);
    header[0] = 0x80 | opcode;
    header[1] = 0x80 | 127;
    header.writeBigUInt64BE(BigInt(payload.length), 2);
  }

  const maskedPayload = Buffer.alloc(payload.length);
  for (let i = 0; i < payload.length; i += 1) {
    maskedPayload[i] = payload[i] ^ mask[i % mask.length];
  }

  return Buffer.concat([header, mask, maskedPayload]);
}

async function openRealtimeWsSocket(
  endpoint: URL,
  token: string,
): Promise<RealtimeWsSubscriptionHandle> {
  const upgradeKey = randomBytes(16).toString("base64");
  const expectedAccept = createHash("sha1")
    .update(`${upgradeKey}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");
  const transport = endpoint.protocol === "wss:" ? https : http;

  return await new Promise<RealtimeWsSubscriptionHandle>((resolve, reject) => {
    let settled = false;
    const req = transport.request({
      host: endpoint.hostname,
      method: "GET",
      path: `${endpoint.pathname}${endpoint.search}`,
      port:
        endpoint.port.length > 0 ? Number(endpoint.port) : endpoint.protocol === "wss:" ? 443 : 80,
      headers: {
        Authorization: `Bearer ${token}`,
        Connection: "Upgrade",
        Upgrade: "websocket",
        "Sec-WebSocket-Key": upgradeKey,
        "Sec-WebSocket-Version": "13",
      },
    });

    const fail = (error: Error): void => {
      if (settled) {
        return;
      }
      settled = true;
      reject(error);
    };

    req.once("response", (res) => {
      fail(new Error(describeUpgradeFailure(res)));
    });
    req.once("error", (error) => {
      fail(error instanceof Error ? error : new Error(String(error)));
    });
    req.once("upgrade", (res, socket, head) => {
      if (settled) {
        socket.destroy();
        return;
      }
      settled = true;
      if (res.headers["sec-websocket-accept"] !== expectedAccept) {
        socket.destroy();
        reject(new Error("WebSocket handshake returned an unexpected Sec-WebSocket-Accept header"));
        return;
      }
      if (head.length > 0) {
        socket.unshift(head);
      }
      socket.setNoDelay(true);
      const closePromise = new Promise<void>((resolveClose) => {
        socket.once("close", resolveClose);
        socket.once("end", resolveClose);
      });
      resolve({
        closed: false,
        closePromise,
        socket,
      });
    });
    req.end();
  });
}

function sendRealtimeWsSubscribeFrame(
  socket: RealtimeWSSocket,
  table: string,
): Promise<void> {
  const payload = JSON.stringify({ type: "subscribe", ref: "inspect-users", tables: [table] });
  const frame = encodeWebSocketFrame(0x1, payload);
  return new Promise<void>((resolve, reject) => {
    socket.write(frame, (error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}

async function openRealtimeWsSubscription(
  page: Page,
  currentPageUrl: string,
  token: string,
  table: string,
): Promise<RealtimeWsSubscriptionHandle> {
  void page;

  const wsURL = buildRealtimeWsUrl(currentPageUrl);
  const handle = await openRealtimeWsSocket(wsURL, token);
  await sendRealtimeWsSubscribeFrame(handle.socket, table);
  return handle;
}

async function closeRealtimeWsSubscription(
  page: Page,
  handle: RealtimeWsSubscriptionHandle,
): Promise<void> {
  void page;

  if (handle.closed) {
    return;
  }
  handle.closed = true;

  if (!handle.socket.destroyed) {
    handle.socket.write(encodeWebSocketFrame(0x8, ""));
    handle.socket.end();
  }

  let timeoutHandle: ReturnType<typeof setTimeout> | null = null;
  const timeoutPromise = new Promise<void>((_, reject) => {
    timeoutHandle = setTimeout(() => {
      handle.socket.destroy();
      reject(new Error("Timed out waiting for WebSocket to close"));
    }, 5000);
  });
  try {
    await Promise.race([handle.closePromise, timeoutPromise]);
  } finally {
    if (timeoutHandle) {
      clearTimeout(timeoutHandle);
    }
  }
}

export async function withRealtimeWsSubscription<T>(
  page: Page,
  currentPageUrl: string,
  token: string,
  table: string,
  run: () => Promise<T>,
): Promise<T> {
  const wsHandle = await openRealtimeWsSubscription(page, currentPageUrl, token, table);
  let runSucceeded = false;
  try {
    const result = await run();
    runSucceeded = true;
    return result;
  } finally {
    if (runSucceeded) {
      // Run body passed — await cleanup so a cleanup-only failure surfaces clearly.
      await closeRealtimeWsSubscription(page, wsHandle);
    } else {
      // Run body already failed — close fire-and-forget so the primary error
      // propagates immediately without risking a cleanup timeout that masks it.
      closeRealtimeWsSubscription(page, wsHandle).catch(() => {});
    }
  }
}

// Fixture helper: create an API key for a user via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule
// that bans request.* calls in spec files.
export async function createApiKeyForUser(
  request: import("@playwright/test").APIRequestContext,
  adminToken: string,
  userId: string,
  keyName: string,
  scope: string = "*",
  allowedTables: string[] = [],
): Promise<{ key: string }> {
  const requestBody = {
    headers: {
      Authorization: `Bearer ${adminToken}`,
      "Content-Type": "application/json",
    },
    data: {
      userId,
      name: keyName,
      scope,
      allowedTables,
    },
  };
  let res = await request.post("/api/admin/api-keys", requestBody);
  if (res.status() === 404) {
    // Some local route stacks expose this admin collection only on the
    // trailing-slash path; retry once so smoke coverage remains portable.
    res = await request.post("/api/admin/api-keys/", requestBody);
  }
  if (!res.ok()) {
    throw new Error(`Failed to create API key ${keyName}: ${res.status()} ${res.statusText()}`);
  }
  const body = (await res.json()) as { key?: unknown };
  if (typeof body.key !== "string" || body.key.length === 0) {
    throw new Error(`Expected API key plaintext in response for ${keyName}`);
  }
  return { key: body.key as string };
}
