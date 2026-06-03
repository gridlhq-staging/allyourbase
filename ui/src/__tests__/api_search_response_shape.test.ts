import { beforeEach, describe, expect, it, vi } from "vitest";
import { listSearchPlaygroundRecords } from "../api_search";

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("api_search response normalization", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("accepts the backend list envelope and returns normalized response", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 2,
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: [{ id: "row-1" }, { id: "row-2" }],
      }),
    );

    await expect(listSearchPlaygroundRecords("posts", { perPage: 10 })).resolves.toEqual({
      page: 2,
      perPage: 10,
      totalItems: 42,
      totalPages: 5,
      items: [{ id: "row-1" }, { id: "row-2" }],
    });
  });

  it("accepts backend facet buckets without stringifying scalar values", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 10,
        totalItems: 4,
        totalPages: 1,
        items: [{ id: "row-1" }],
        facets: {
          status: [{ value: "published", count: 2 }],
          rating: [{ value: 5, count: 1 }],
          featured: [{ value: true, count: 3 }],
          archived_at: [{ value: null, count: 4 }],
        },
      }),
    );

    await expect(
      listSearchPlaygroundRecords("posts", { facets: ["status"] }),
    ).resolves.toEqual({
      page: 1,
      perPage: 10,
      totalItems: 4,
      totalPages: 1,
      items: [{ id: "row-1" }],
      facets: {
        status: [{ value: "published", count: 2 }],
        rating: [{ value: 5, count: 1 }],
        featured: [{ value: true, count: 3 }],
        archived_at: [{ value: null, count: 4 }],
      },
    });
  });

  it("throws when response payload is not an object", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(null));

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected list response object",
    );
  });

  it("throws when list envelope fields are malformed", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: "1",
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: [],
      }),
    );

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected integer list envelope fields: page, perPage, totalItems, totalPages",
    );
  });

  it("throws when list envelope fields are non-integer numbers", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1.5,
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: [],
      }),
    );

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected integer list envelope fields: page, perPage, totalItems, totalPages",
    );
  });

  it("throws when items is not an array", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: { id: "row-1" },
      }),
    );

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected list response items array",
    );
  });

  it("throws when items array contains non-object rows", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: [{ id: "row-1" }, null],
      }),
    );

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected list response items to contain only objects",
    );
  });

  it.each([
    [
      "facets is not an object",
      {
        facets: [],
      },
    ],
    [
      "column buckets are not arrays",
      {
        facets: {
          status: { value: "published", count: 2 },
        },
      },
    ],
    [
      "bucket row is not an object",
      {
        facets: {
          status: [null],
        },
      },
    ],
    [
      "bucket value is not a scalar",
      {
        facets: {
          status: [{ value: { nested: true }, count: 2 }],
        },
      },
    ],
    [
      "bucket count is not numeric",
      {
        facets: {
          status: [{ value: "published", count: "2" }],
        },
      },
    ],
    [
      "bucket count is a non-integer number",
      {
        facets: {
          status: [{ value: "published", count: 2.5 }],
        },
      },
    ],
  ])("throws when %s", async (_caseName, override) => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 10,
        totalItems: 42,
        totalPages: 5,
        items: [],
        ...override,
      }),
    );

    await expect(listSearchPlaygroundRecords("posts")).rejects.toThrow(
      "Expected list response facets object with array buckets containing value and numeric count",
    );
  });
});
