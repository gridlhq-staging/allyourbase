import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  listAllRequestLogs,
  listRequestLogAggregates,
  streamRequestLogs,
} from "../api_analytics";
import { fetchAdmin, request } from "../api_client";
import type {
  RequestLogEntry,
  RequestLogListResponse,
} from "../types/analytics";

vi.mock("../api_client", async () => {
  const actual = await vi.importActual<typeof import("../api_client")>(
    "../api_client",
  );
  return {
    ...actual,
    fetchAdmin: vi.fn(),
    request: vi.fn(),
  };
});

describe("listRequestLogAggregates", () => {
  const requestMock = vi.mocked(request);

  beforeEach(() => {
    requestMock.mockReset();
  });

  it("serializes every request-log filter to the aggregate endpoint", async () => {
    requestMock.mockResolvedValue({ items: [], count: 0 });

    await listRequestLogAggregates({
      method: "POST",
      path: "/api/orders/*",
      status: "418",
      statusClass: "4xx",
      minDurationMs: "25",
      maxDurationMs: "750",
      from: "2026-07-26T12:00:00.000Z",
      to: "2026-07-26T13:00:00.000Z",
    });

    expect(requestMock).toHaveBeenCalledWith(
      "/api/admin/analytics/requests/aggregate?method=POST&path=%2Fapi%2Forders%2F*&status=418&status_class=4xx&min_duration_ms=25&max_duration_ms=750&from=2026-07-26T12%3A00%3A00.000Z&to=2026-07-26T13%3A00%3A00.000Z",
    );
  });
});

function streamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
    { status: 200 },
  );
}

describe("streamRequestLogs", () => {
  const fetchAdminMock = vi.mocked(fetchAdmin);

  beforeEach(() => {
    fetchAdminMock.mockReset();
  });

  it("uses fetchAdmin with serialized request-log filters and decodes fragmented SSE rows", async () => {
    const row = requestLog(42);
    fetchAdminMock.mockResolvedValue(
      streamResponse([
        'event: ready\ndata: {"delivery":"polling"}\n\n',
        "event: request_log\ndata: ",
        `${JSON.stringify(row)}\n\n`,
      ]),
    );
    const ready = vi.fn();
    const rows: RequestLogEntry[] = [];
    const signal = new AbortController().signal;

    await streamRequestLogs(
      {
        method: "GET",
        path: "/api/live/*",
        status: "404",
        statusClass: "4xx",
        minDurationMs: "10",
        maxDurationMs: "200",
        from: "2026-07-26",
        to: "2026-07-27",
      },
      { signal, onReady: ready, onRequestLog: (entry) => rows.push(entry) },
    );

    expect(fetchAdminMock).toHaveBeenCalledWith(
      "/api/admin/analytics/requests/stream?method=GET&path=%2Fapi%2Flive%2F*&status=404&status_class=4xx&min_duration_ms=10&max_duration_ms=200&from=2026-07-26&to=2026-07-27",
      expect.objectContaining({
        signal,
        headers: expect.objectContaining({ Accept: "text/event-stream" }),
      }),
    );
    expect(ready).toHaveBeenCalledTimes(1);
    expect(rows).toEqual([row]);
  });

  it("decodes multiple request_log frames and rejects server error events", async () => {
    const first = requestLog(1);
    const second = requestLog(2);
    fetchAdminMock.mockResolvedValueOnce(
      streamResponse([
        `event: request_log\ndata: ${JSON.stringify(first)}\n\n`,
        `event: request_log\ndata: ${JSON.stringify(second)}\n\n`,
      ]),
    );
    const rows: RequestLogEntry[] = [];

    await streamRequestLogs({}, { onRequestLog: (entry) => rows.push(entry) });

    expect(rows).toEqual([first, second]);

    fetchAdminMock.mockResolvedValueOnce(
      streamResponse([
        'event: error\ndata: {"message":"request log stream failed"}\n\n',
      ]),
    );
    await expect(streamRequestLogs({}, { onRequestLog: vi.fn() })).rejects.toThrow(
      "request log stream failed",
    );
  });

  it("returns cleanly when aborted while the reader is open", async () => {
    fetchAdminMock.mockResolvedValue(
      new Response(
        new ReadableStream({
          start(nextController) {
            nextController.enqueue(new TextEncoder().encode("event: ready\n"));
          },
        }),
        { status: 200 },
      ),
    );
    const abortController = new AbortController();
    const promise = streamRequestLogs({}, {
      signal: abortController.signal,
      onRequestLog: vi.fn(),
    });

    abortController.abort();
    await expect(promise).resolves.toBeUndefined();
  });

  it("removes abort listeners after each reader wait settles", async () => {
    fetchAdminMock.mockResolvedValue(
      streamResponse([
        'event: ready\ndata: {"delivery":"polling"}\n\n',
        `event: request_log\ndata: ${JSON.stringify(requestLog(7))}\n\n`,
      ]),
    );
    const abortController = new AbortController();
    const addEventListenerSpy = vi.spyOn(abortController.signal, "addEventListener");
    const removeEventListenerSpy = vi.spyOn(
      abortController.signal,
      "removeEventListener",
    );

    await streamRequestLogs({}, {
      signal: abortController.signal,
      onRequestLog: vi.fn(),
    });

    expect(addEventListenerSpy).toHaveBeenCalled();
    expect(removeEventListenerSpy).toHaveBeenCalledTimes(
      addEventListenerSpy.mock.calls.length,
    );
  });
});

const EXPORT_STARTED_AT = new Date("2026-07-26T12:00:00.000Z");

function requestLog(id: number): RequestLogEntry {
  return {
    id: `original-${id}`,
    timestamp: new Date(EXPORT_STARTED_AT.getTime() - id - 1).toISOString(),
    method: "GET",
    path: "/api/export-race",
    status_code: 200,
    duration_ms: id,
    request_size: 10,
    response_size: 20,
  };
}

function rowsAfterCursor(
  rows: RequestLogEntry[],
  query: URLSearchParams,
): RequestLogEntry[] {
  const cursorTimestamp = query.get("cursor_timestamp");
  const cursorId = query.get("cursor_id");
  return rows.filter(
    (row) =>
      cursorTimestamp === null ||
      row.timestamp < cursorTimestamp ||
      (row.timestamp === cursorTimestamp &&
        cursorId !== null &&
        row.id < cursorId),
  );
}

describe("listAllRequestLogs", () => {
  const requestMock = vi.mocked(request);

  beforeEach(() => {
    requestMock.mockReset();
  });

  it("exports every original match exactly once when a newer match arrives between pages", async () => {
    const originalRows = Array.from({ length: 501 }, (_, index) =>
      requestLog(index),
    );
    const liveRows = [...originalRows];
    let requestCount = 0;

    requestMock.mockImplementation(async (path) => {
      const query = new URL(path, "http://localhost").searchParams;
      const matchingRows = rowsAfterCursor(liveRows, query);
      const offset = Number(query.get("offset") ?? 0);
      const limit = Number(query.get("limit") ?? 100);
      const response: RequestLogListResponse = {
        items: matchingRows.slice(offset, offset + limit),
        count: matchingRows.length,
        limit,
        offset,
      };

      requestCount += 1;
      if (requestCount === 1) {
        liveRows.unshift({
          ...requestLog(999),
          id: "inserted-between-pages",
          timestamp: new Date(EXPORT_STARTED_AT.getTime() + 1).toISOString(),
        });
      }
      return response;
    });

    const result = await listAllRequestLogs({ path: "/api/export-race" });

    expect(result.items.map((row) => row.id)).toEqual(
      originalRows.map((row) => row.id),
    );
    expect(new Set(result.items.map((row) => row.id)).size).toBe(501);
    expect(requestMock).toHaveBeenCalledTimes(2);
    expect(
      new URL(requestMock.mock.calls[0][0], "http://localhost").searchParams.get(
        "to",
      ),
    ).toBeNull();
    expect(
      new URL(requestMock.mock.calls[1][0], "http://localhost").searchParams.get(
        "cursor_timestamp",
      ),
    ).toBe(originalRows[499].timestamp);
    expect(
      new URL(requestMock.mock.calls[1][0], "http://localhost").searchParams.get(
        "cursor_id",
      ),
    ).toBe(originalRows[499].id);
  });

  it("exports every original match exactly once when a match arrives inside the first page window", async () => {
    const originalRows = Array.from({ length: 501 }, (_, index) =>
      requestLog(index),
    );
    const liveRows = [...originalRows];
    let requestCount = 0;

    requestMock.mockImplementation(async (path) => {
      const query = new URL(path, "http://localhost").searchParams;
      const matchingRows = rowsAfterCursor(liveRows, query);
      const offset = Number(query.get("offset") ?? 0);
      const limit = Number(query.get("limit") ?? 100);
      const response: RequestLogListResponse = {
        items: matchingRows.slice(offset, offset + limit),
        count: matchingRows.length,
        limit,
        offset,
      };

      requestCount += 1;
      if (requestCount === 1) {
        liveRows.splice(101, 0, {
          ...requestLog(999),
          id: "inserted-inside-first-page",
          timestamp: new Date(
            Date.parse(originalRows[100].timestamp) - 1,
          ).toISOString(),
        });
      }
      return response;
    });

    const result = await listAllRequestLogs({ path: "/api/export-race" });

    expect(result.items.map((row) => row.id)).toEqual(
      originalRows.map((row) => row.id),
    );
    expect(new Set(result.items.map((row) => row.id)).size).toBe(501);
    expect(requestMock).toHaveBeenCalledTimes(2);
  });

  it("exports every match when more than one page shares a timestamp", async () => {
    const sharedTimestamp = "2026-07-26T11:00:00.000Z";
    const originalRows = Array.from({ length: 501 }, (_, index) => ({
      ...requestLog(index),
      id: `tie-${String(500 - index).padStart(4, "0")}`,
      timestamp: sharedTimestamp,
    }));

    requestMock.mockImplementation(async (path) => {
      const query = new URL(path, "http://localhost").searchParams;
      const matchingRows = rowsAfterCursor(originalRows, query);
      const limit = Number(query.get("limit") ?? 100);
      return {
        items: matchingRows.slice(0, limit),
        count: originalRows.length,
        limit,
        offset: 0,
      };
    });

    const result = await listAllRequestLogs({ path: "/api/export-race" });

    expect(result.items.map((row) => row.id)).toEqual(
      originalRows.map((row) => row.id),
    );
    expect(new Set(result.items.map((row) => row.id)).size).toBe(501);
    expect(requestMock).toHaveBeenCalledTimes(2);
    const secondPageQuery = new URL(
      requestMock.mock.calls[1][0],
      "http://localhost",
    ).searchParams;
    expect(secondPageQuery.get("cursor_timestamp")).toBe(sharedTimestamp);
    expect(secondPageQuery.get("cursor_id")).toBe(originalRows[499].id);
  });

  it("rejects an incomplete export when a later cursor page is empty", async () => {
    const originalRows = Array.from({ length: 501 }, (_, index) =>
      requestLog(index),
    );
    requestMock
      .mockResolvedValueOnce({
        items: originalRows.slice(0, 500),
        count: originalRows.length,
        limit: 500,
        offset: 0,
      })
      .mockResolvedValueOnce({
        items: [],
        count: originalRows.length,
        limit: 500,
        offset: 0,
      });

    await expect(
      listAllRequestLogs({ path: "/api/export-race" }),
    ).rejects.toThrow(
      "Request log export incomplete: received 500 of 501 matching rows",
    );
  });

  it("rejects an incomplete export when a cursor page makes no progress", async () => {
    const originalRows = Array.from({ length: 501 }, (_, index) =>
      requestLog(index),
    );
    const repeatedPage = originalRows.slice(0, 500);
    requestMock.mockResolvedValue({
      items: repeatedPage,
      count: originalRows.length,
      limit: 500,
      offset: 0,
    });

    await expect(
      listAllRequestLogs({ path: "/api/export-race" }),
    ).rejects.toThrow(
      "Request log export incomplete: received 500 of 501 matching rows",
    );
    expect(requestMock).toHaveBeenCalledTimes(2);
  });
});
