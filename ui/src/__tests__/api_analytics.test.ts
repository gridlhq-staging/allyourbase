import { beforeEach, describe, expect, it, vi } from "vitest";
import { listAllRequestLogs } from "../api_analytics";
import { request } from "../api_client";
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
    request: vi.fn(),
  };
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
