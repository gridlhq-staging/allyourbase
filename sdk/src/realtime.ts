/**
 * @module Provides a RealtimeClient class for subscribing to server-sent realtime event updates from specified database tables.
 */
import type { RealtimeEvent } from "./types";
import { normalizeRealtimeEvent } from "./helpers";

interface RealtimeClientRuntime {
  token: string | null;
  getBaseURL(): string;
}

let wsRefCounter = 0;

function nextRef(): string {
  wsRefCounter += 1;
  return String(wsRefCounter);
}

function deriveWSURL(httpURL: string): string {
  return httpURL.replace(/^http/, "ws");
}

function isRealtimeEventPayload(msg: Record<string, unknown>): boolean {
  return (
    typeof msg.action === "string" &&
    typeof msg.table === "string" &&
    typeof msg.record === "object" &&
    msg.record !== null &&
    !Array.isArray(msg.record)
  );
}

/**
 * Client for subscribing to server-sent realtime events via SSE or WebSocket.
 */
export class RealtimeClient {
  constructor(private client: RealtimeClientRuntime) {}

  subscribe(
    tables: string[],
    callback: (event: RealtimeEvent) => void,
  ): () => void {
    const params = new URLSearchParams({ tables: tables.join(",") });
    if (this.client.token) {
      params.set("token", this.client.token);
    }
    const url = `${this.client.getBaseURL()}/api/realtime?${params}`;
    const es = new EventSource(url);

    es.onmessage = (e) => {
      try {
        const event = normalizeRealtimeEvent(JSON.parse(e.data) as RealtimeEvent);
        callback(event);
      } catch {
        // Ignore parse errors for heartbeat/ping messages.
      }
    };

    return () => es.close();
  }

  subscribeWS(
    tables: string[],
    callback: (event: RealtimeEvent) => void,
  ): Promise<() => void> {
    const wsURL = `${deriveWSURL(this.client.getBaseURL())}/api/realtime/ws`;
    const token = this.client.token;
    const ws = new WebSocket(wsURL);
    let active = true;
    let pendingAuthRef: string | null = null;
    let pendingSubscribeRef: string | null = null;
    let settled = false;
    let handshakeStarted = false;

    const unsubscribe = () => {
      active = false;
      try {
        ws.send(JSON.stringify({ type: "unsubscribe", tables, ref: nextRef() }));
      } catch {
        // Best-effort: socket may already be closed.
      }
      ws.close();
    };

    return new Promise<() => void>((resolve, reject) => {
      const rejectOnce = (err: Error) => {
        if (settled) return;
        settled = true;
        active = false;
        try {
          ws.close();
        } catch {
          // Best-effort: the browser may already consider this socket closed.
        }
        reject(err);
      };
      const resolveOnce = () => {
        if (settled) return;
        settled = true;
        resolve(unsubscribe);
      };
      const beginHandshake = () => {
        if (handshakeStarted || settled) return;
        handshakeStarted = true;
        if (token) {
          const ref = nextRef();
          pendingAuthRef = ref;
          ws.send(JSON.stringify({ type: "auth", token, ref }));
        } else {
          sendSubscribe();
        }
      };
      const sendSubscribe = () => {
        const ref = nextRef();
        pendingSubscribeRef = ref;
        ws.send(JSON.stringify({ type: "subscribe", tables, ref }));
      };

      ws.onerror = () => {
        rejectOnce(new Error("WebSocket connection error"));
      };
      ws.onclose = () => {
        if (!settled && active) {
          rejectOnce(new Error("WebSocket closed before subscription was ready"));
        }
      };
      ws.onopen = () => {
        beginHandshake();
      };

      ws.onmessage = (e) => {
        let msg: Record<string, unknown>;
        try {
          msg = JSON.parse(String(e.data)) as Record<string, unknown>;
        } catch {
          return;
        }

        if (msg.type === "event" || isRealtimeEventPayload(msg)) {
          if (active) {
            callback(normalizeRealtimeEvent(msg as unknown as RealtimeEvent));
          }
          return;
        }

        if (msg.type === "reply") {
          if (pendingAuthRef && msg.ref === pendingAuthRef) {
            pendingAuthRef = null;
            if (msg.status === "ok") {
              sendSubscribe();
            } else {
              rejectOnce(new Error(String(msg.message ?? "auth failed")));
            }
            return;
          }
          if (pendingSubscribeRef && msg.ref === pendingSubscribeRef) {
            pendingSubscribeRef = null;
            if (msg.status === "ok") {
              resolveOnce();
            } else {
              rejectOnce(new Error(String(msg.message ?? "subscribe failed")));
            }
          }
          return;
        }

        if (msg.type === "connected") {
          beginHandshake();
        }
      };

      // Defensive: if the socket reached OPEN before handlers were attached,
      // begin the handshake immediately instead of waiting for any frame.
      if (ws.readyState === WebSocket.OPEN) {
        beginHandshake();
      }
    });
  }
}
