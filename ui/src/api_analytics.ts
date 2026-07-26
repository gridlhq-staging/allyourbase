/**
 * @module ui/src/api_analytics.ts
 */
import { fetchAdmin, request, throwApiError } from "./api_client";
import type {
  RequestLogAggregateResponse,
  RequestLogEntry,
  RequestLogListResponse,
  QueryAnalyticsResponse,
} from "./types/analytics";

export interface ListRequestLogsParams {
  method?: string;
  path?: string;
  status?: string;
  statusClass?: string;
  minDurationMs?: string;
  maxDurationMs?: string;
  from?: string;
  to?: string;
  cursorTimestamp?: string;
  cursorId?: string;
  limit?: number;
  offset?: number;
}

export type RequestLogFilterParams = Omit<
  ListRequestLogsParams,
  "cursorId" | "cursorTimestamp" | "limit" | "offset"
>;

const REQUEST_LOG_EXPORT_PAGE_SIZE = 500;
const REQUEST_LOG_STREAM_MAX_BUFFER_BYTES = 1_000_000;

function requestLogFilterQuery(
  filters?: RequestLogFilterParams,
): URLSearchParams {
  const query = new URLSearchParams();
  if (filters?.method) query.set("method", filters.method);
  if (filters?.path) query.set("path", filters.path);
  if (filters?.status) query.set("status", filters.status);
  if (filters?.statusClass) query.set("status_class", filters.statusClass);
  if (filters?.minDurationMs)
    query.set("min_duration_ms", filters.minDurationMs);
  if (filters?.maxDurationMs)
    query.set("max_duration_ms", filters.maxDurationMs);
  if (filters?.from) query.set("from", filters.from);
  if (filters?.to) query.set("to", filters.to);
  return query;
}

export function listRequestLogs(
  params?: ListRequestLogsParams,
): Promise<RequestLogListResponse> {
  const qs = requestLogFilterQuery(params);
  if (params?.cursorTimestamp)
    qs.set("cursor_timestamp", params.cursorTimestamp);
  if (params?.cursorId) qs.set("cursor_id", params.cursorId);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  const query = qs.toString();
  return request<RequestLogListResponse>(
    `/api/admin/analytics/requests${query ? `?${query}` : ""}`,
  );
}

export function listRequestLogAggregates(
  filters?: RequestLogFilterParams,
): Promise<RequestLogAggregateResponse> {
  const query = requestLogFilterQuery(filters).toString();
  return request<RequestLogAggregateResponse>(
    `/api/admin/analytics/requests/aggregate${query ? `?${query}` : ""}`,
  );
}

export interface StreamRequestLogsHandlers {
  signal?: AbortSignal;
  onReady?: () => void;
  onRequestLog: (entry: RequestLogEntry) => void;
}

export async function streamRequestLogs(
  filters: RequestLogFilterParams = {},
  handlers: StreamRequestLogsHandlers,
): Promise<void> {
  const query = requestLogFilterQuery(filters).toString();
  const response = await fetchAdmin(
    `/api/admin/analytics/requests/stream${query ? `?${query}` : ""}`,
    { signal: handlers.signal, headers: { Accept: "text/event-stream" } },
  );
  if (!response.ok) {
    await throwApiError(response);
  }
  if (!response.body) {
    throw new Error("Request log stream response body is unavailable");
  }

  await consumeRequestLogStream(response.body, handlers);
}

async function consumeRequestLogStream(
  body: ReadableStream<Uint8Array>,
  handlers: StreamRequestLogsHandlers,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      const result = await readRequestLogStreamChunk(reader, handlers.signal);
      if (result.done) break;
      if (!result.value) continue;
      buffer += decoder.decode(result.value, { stream: true });
      if (buffer.length > REQUEST_LOG_STREAM_MAX_BUFFER_BYTES) {
        throw new Error("Request log stream frame exceeded maximum size");
      }
      buffer = drainRequestLogStreamBuffer(buffer, handlers);
    }
    buffer += decoder.decode();
    if (buffer.trim()) processRequestLogStreamEvent(buffer, handlers);
  } finally {
    reader.releaseLock();
  }
}

async function readRequestLogStreamChunk(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal?: AbortSignal,
): Promise<ReadableStreamReadResult<Uint8Array>> {
  if (!signal) return reader.read();
  if (signal.aborted) {
    await reader.cancel().catch(() => {});
    return { done: true, value: undefined };
  }
  let handleAbort: (() => void) | null = null;
  const abortPromise = new Promise<ReadableStreamReadResult<Uint8Array>>(
    (resolve) => {
      handleAbort = () => {
        void reader.cancel().finally(() => {
          resolve({ done: true, value: undefined });
        });
      };
      signal.addEventListener("abort", handleAbort, { once: true });
    },
  );
  try {
    return await Promise.race([reader.read(), abortPromise]);
  } finally {
    if (handleAbort) signal.removeEventListener("abort", handleAbort);
  }
}

function drainRequestLogStreamBuffer(
  buffer: string,
  handlers: StreamRequestLogsHandlers,
): string {
  let nextBuffer = buffer;
  let boundaryIndex = nextBuffer.indexOf("\n\n");
  while (boundaryIndex >= 0) {
    processRequestLogStreamEvent(nextBuffer.slice(0, boundaryIndex), handlers);
    nextBuffer = nextBuffer.slice(boundaryIndex + 2);
    boundaryIndex = nextBuffer.indexOf("\n\n");
  }
  return nextBuffer;
}

function processRequestLogStreamEvent(
  rawEvent: string,
  handlers: StreamRequestLogsHandlers,
): void {
  if (!rawEvent.trim()) return;
  let eventName = "message";
  const dataLines: string[] = [];
  for (const line of rawEvent.split("\n")) {
    if (line.startsWith("event:")) {
      eventName = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
    }
  }
  const payload = dataLines.length > 0
    ? JSON.parse(dataLines.join("\n")) as Record<string, unknown>
    : {};
  if (eventName === "ready") {
    handlers.onReady?.();
    return;
  }
  if (eventName === "request_log") {
    handlers.onRequestLog(payload as unknown as RequestLogEntry);
    return;
  }
  if (eventName === "error") {
    throw new Error(typeof payload.message === "string" ? payload.message : "Request log stream failed");
  }
}

export async function listAllRequestLogs(
  filters: RequestLogFilterParams = {},
): Promise<RequestLogListResponse> {
  const items: RequestLogListResponse["items"] = [];
  const seenIds = new Set<string>();
  let total = 0;
  let cursorTimestamp: string | undefined;
  let cursorId: string | undefined;
  let isFirstPage = true;

  while (true) {
    const page = await listRequestLogs({
      ...filters,
      ...(cursorTimestamp && cursorId ? { cursorTimestamp, cursorId } : {}),
      limit: REQUEST_LOG_EXPORT_PAGE_SIZE,
      offset: 0,
    });
    if (isFirstPage) {
      total = page.count;
      isFirstPage = false;
    }

    const previousItemCount = items.length;
    for (const item of page.items) {
      if (seenIds.has(item.id)) continue;
      seenIds.add(item.id);
      items.push(item);
      if (items.length >= total) break;
    }

    if (items.length >= total) break;
    const lastScannedItem = page.items[page.items.length - 1];
    const nextCursorTimestamp = lastScannedItem?.timestamp;
    const nextCursorId = lastScannedItem?.id;
    if (
      page.items.length === 0 ||
      !nextCursorTimestamp ||
      !nextCursorId ||
      (nextCursorTimestamp === cursorTimestamp && nextCursorId === cursorId) ||
      items.length === previousItemCount
    ) {
      throw new Error(
        `Request log export incomplete: received ${items.length} of ${total} matching rows`,
      );
    }
    cursorTimestamp = nextCursorTimestamp;
    cursorId = nextCursorId;
  }

  return {
    items: items.slice(0, total),
    count: total,
    limit: REQUEST_LOG_EXPORT_PAGE_SIZE,
    offset: 0,
  };
}

interface ListQueryStatsParams {
  sort?: string;
  limit?: number;
}

export function listQueryStats(
  params?: ListQueryStatsParams,
): Promise<QueryAnalyticsResponse> {
  const qs = new URLSearchParams();
  if (params?.sort) qs.set("sort", params.sort);
  if (params?.limit) qs.set("limit", String(params.limit));
  const query = qs.toString();
  return request<QueryAnalyticsResponse>(
    `/api/admin/analytics/queries${query ? `?${query}` : ""}`,
  );
}
