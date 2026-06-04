import { beforeEach, describe, expect, it, vi } from "vitest";
import { listSearchPlaygroundRecords } from "../api_search";

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("api_search request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("serializes only supported query keys for search-playground list calls", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 25,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1" }],
      }),
    );

    await listSearchPlaygroundRecords("posts", {
      search: "postgres",
      fuzzy: true,
      filter: "status=active",
      perPage: 25,
      facets: ["status"],
      typo_threshold: 0.2,
    } as unknown as Parameters<typeof listSearchPlaygroundRecords>[1]);

    const [calledPath] = fetchMock.mock.calls[0] as [string];
    expect(calledPath).toContain("/api/collections/posts?");

    const params = new URL(calledPath, "http://localhost").searchParams;
    expect(params.get("search")).toBe("postgres");
    expect(params.get("fuzzy")).toBe("true");
    expect(params.get("filter")).toBe("status=active");
    expect(params.get("perPage")).toBe("25");
    expect(params.get("facets")).toBe("status");
    expect(params.has("typo_threshold")).toBe(false);
    expect(Array.from(params.keys()).sort()).toEqual(
      ["facets", "filter", "fuzzy", "perPage", "search"].sort(),
    );
  });

  it("serializes highlight only for full-text search requests", async () => {
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          page: 1,
          perPage: 20,
          totalItems: 1,
          totalPages: 1,
          items: [{ id: "row-1", _highlight: "<b>Postgres</b>" }],
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
          items: [],
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
          items: [],
        }),
      );

    await listSearchPlaygroundRecords("posts", {
      search: "postgres",
      highlight: true,
    });
    await listSearchPlaygroundRecords("posts", {
      search: "   ",
      highlight: true,
    });
    await listSearchPlaygroundRecords("posts", {
      filter: "status='published'",
      highlight: true,
    });

    const [searchPath] = fetchMock.mock.calls[0] as [string];
    const [blankSearchPath] = fetchMock.mock.calls[1] as [string];
    const [filterOnlyPath] = fetchMock.mock.calls[2] as [string];

    expect(new URL(searchPath, "http://localhost").searchParams.get("highlight")).toBe("true");
    expect(new URL(blankSearchPath, "http://localhost").searchParams.has("highlight")).toBe(false);
    expect(new URL(filterOnlyPath, "http://localhost").searchParams.has("highlight")).toBe(false);
  });

  it("omits fuzzy when search is empty even if fuzzy=false is provided", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      }),
    );

    await listSearchPlaygroundRecords("posts", { fuzzy: false });

    const [calledPath] = fetchMock.mock.calls[0] as [string];
    const params = new URL(calledPath, "http://localhost").searchParams;
    expect(params.has("fuzzy")).toBe(false);
  });

  it("omits undefined and empty params from query string", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      }),
    );

    await listSearchPlaygroundRecords("posts", {
      search: "",
      filter: undefined,
      perPage: undefined,
      fuzzy: undefined,
      facets: [],
    });

    const [calledPath] = fetchMock.mock.calls[0] as [string];
    expect(calledPath).toBe("/api/collections/posts");
  });

  it("omits blank facet entries and unsupported cast-injected facet shapes", async () => {
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
          items: [],
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
          items: [],
        }),
      );

    await listSearchPlaygroundRecords("posts", { facets: ["", "   "] });
    await listSearchPlaygroundRecords("posts", {
      facets: "status",
    } as unknown as Parameters<typeof listSearchPlaygroundRecords>[1]);

    const [blankFacetPath] = fetchMock.mock.calls[0] as [string];
    const [unsupportedFacetPath] = fetchMock.mock.calls[1] as [string];
    expect(blankFacetPath).toBe("/api/collections/posts");
    expect(unsupportedFacetPath).toBe("/api/collections/posts");
  });

  it("omits facet names that would be split into unintended backend columns", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      }),
    );

    await listSearchPlaygroundRecords("posts", {
      facets: ["status,role", "status", "review-status"],
    });

    const [calledPath] = fetchMock.mock.calls[0] as [string];
    const params = new URL(calledPath, "http://localhost").searchParams;
    expect(params.get("facets")).toBe("status,review-status");
  });

  it("encodes perPage as a stable integer query value", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        page: 1,
        perPage: 17,
        totalItems: 0,
        totalPages: 0,
        items: [],
      }),
    );

    await listSearchPlaygroundRecords("posts", { perPage: 17.9 });

    const [calledPath] = fetchMock.mock.calls[0] as [string];
    const params = new URL(calledPath, "http://localhost").searchParams;
    expect(params.get("perPage")).toBe("17");
  });

  it("propagates backend 400 errors via shared request behavior", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(
        {
          code: 400,
          message: "fuzzy search is unavailable because pg_trgm is not installed",
        },
        { status: 400 },
      ),
    );

    await expect(
      listSearchPlaygroundRecords("posts", { search: "alpha", fuzzy: true }),
    ).rejects.toThrow("fuzzy search is unavailable because pg_trgm is not installed");
  });
});
