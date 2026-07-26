/**
 * @module ui/src/api_analytics.ts
 */
import { request } from "./api_client";
import type {
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

export function listRequestLogs(
  params?: ListRequestLogsParams,
): Promise<RequestLogListResponse> {
  const qs = new URLSearchParams();
  if (params?.method) qs.set("method", params.method);
  if (params?.path) qs.set("path", params.path);
  if (params?.status) qs.set("status", params.status);
  if (params?.statusClass) qs.set("status_class", params.statusClass);
  if (params?.minDurationMs) qs.set("min_duration_ms", params.minDurationMs);
  if (params?.maxDurationMs) qs.set("max_duration_ms", params.maxDurationMs);
  if (params?.from) qs.set("from", params.from);
  if (params?.to) qs.set("to", params.to);
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
