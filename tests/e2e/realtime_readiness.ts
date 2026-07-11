/// <reference lib="dom" />

import type { Page } from "@playwright/test";

declare global {
  interface Window {
    __aybRealtimeReadinessInstalled?: boolean;
    __aybRealtimeTablesReady?: (tables: string[]) => boolean;
  }
}

export async function installRealtimeReadinessProbe(page: Page): Promise<void> {
  await page.addInitScript(() => {
    if (window.__aybRealtimeReadinessInstalled) {
      return;
    }
    window.__aybRealtimeReadinessInstalled = true;

    const readyTables = new Set<string>();
    window.__aybRealtimeTablesReady = (tables: string[]) =>
      tables.every((table) => readyTables.has(table));

    const markReady = (tables: unknown) => {
      if (!Array.isArray(tables)) {
        return;
      }
      for (const table of tables) {
        if (typeof table === "string" && table.length > 0) {
          readyTables.add(table);
        }
      }
    };

    const realtimeTablesFromURL = (rawURL: string | URL): string[] => {
      const url = new URL(String(rawURL), window.location.href);
      if (url.pathname !== "/api/realtime") {
        return [];
      }
      return (url.searchParams.get("tables") ?? "")
        .split(",")
        .map((table) => table.trim())
        .filter(Boolean);
    };

    const NativeEventSource = window.EventSource;
    window.EventSource = class AYBTrackingEventSource extends NativeEventSource {
      constructor(url: string | URL, eventSourceInitDict?: EventSourceInit) {
        super(url, eventSourceInitDict);
        const tables = realtimeTablesFromURL(url);
        this.addEventListener("open", () => markReady(tables));
      }
    };

    const NativeWebSocket = window.WebSocket;
    window.WebSocket = class AYBTrackingWebSocket extends NativeWebSocket {
      private readonly pendingSubscriptions: Map<string, unknown>;

      constructor(url: string | URL, protocols?: string | string[]) {
        if (protocols === undefined) {
          super(url);
        } else {
          super(url, protocols);
        }
        this.pendingSubscriptions = new Map<string, unknown>();
        this.addEventListener("message", (event) => {
          if (typeof event.data !== "string") {
            return;
          }
          try {
            const message = JSON.parse(event.data) as {
              ref?: unknown;
              status?: unknown;
              type?: unknown;
            };
            if (message.type !== "reply" || message.status !== "ok" || typeof message.ref !== "string") {
              return;
            }
            const tables = this.pendingSubscriptions.get(message.ref);
            if (tables !== undefined) {
              markReady(tables);
              this.pendingSubscriptions.delete(message.ref);
            }
          } catch {
            // Ignore non-JSON frames such as browser/runtime protocol noise.
          }
        });
      }

      send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
        if (typeof data === "string") {
          try {
            const message = JSON.parse(data) as {
              ref?: unknown;
              tables?: unknown;
              type?: unknown;
            };
            if (message.type === "subscribe" && typeof message.ref === "string") {
              this.pendingSubscriptions.set(message.ref, message.tables);
            }
          } catch {
            // Preserve native send behavior for non-JSON frames.
          }
        }
        super.send(data);
      }
    };
  });
}

export async function waitForRealtimeTables(page: Page, tables: string[]): Promise<void> {
  await page.waitForFunction(
    (expectedTables) => window.__aybRealtimeTablesReady?.(expectedTables) ?? false,
    tables,
    { timeout: 10_000 },
  );
}
