import { describe, expect, it, vi } from "vitest";
import { AYBClient } from "./client";
import { createInstantSearchClient } from "./instantsearch";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";
import type { ListParams, ListResponse, SearchHit } from "./types";

function requestedURL(fetchFn: typeof globalThis.fetch): URL {
  const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
  return new URL(call[0] as string);
}

function createListOnlyClient(response: ListResponse<SearchHit>) {
  const list = vi.fn(async (_collection: string, _params?: ListParams) => response);
  return {
    records: { list },
  };
}

describe("createInstantSearchClient", () => {
  it("translates supported requests through records.list and maps AYB responses", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [
            {
              id: "post_1",
              title: "Postgres search",
              status: "published",
              _highlightResult: {
                title: { value: "<b>Postgres</b> search", matchLevel: "full" },
              },
            },
          ],
          page: 3,
          perPage: 5,
          totalItems: 11,
          totalPages: 3,
          facets: {
            status: [
              { value: "published", count: 7 },
              { value: null, count: 2 },
            ],
            brand: [{ value: "Apple", count: 4 }],
          },
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      highlight: true,
    });

    const response = await searchClient.search([
      {
        indexName: "posts",
        params: {
          query: "postgres",
          page: 2,
          hitsPerPage: 5,
          facets: ["status", "brand"],
          facetFilters: [["brand:Apple", "brand:Samsung"], "status:published"],
          filters: "published:true AND views >= 10",
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.pathname).toBe("/api/collections/posts");
    expect(url.searchParams.get("page")).toBe("3");
    expect(url.searchParams.get("perPage")).toBe("5");
    expect(url.searchParams.get("search")).toBe("postgres");
    expect(url.searchParams.get("highlight")).toBe("true");
    expect(url.searchParams.get("facets")).toBe("status,brand");
    expect(url.searchParams.get("filter")).toBe(
      "(brand='Apple' OR brand='Samsung') AND status='published' AND (published=true AND views>=10)",
    );

    const result = response.results[0];
    expect(result.hits).toEqual([
      {
        id: "post_1",
        objectID: "post_1",
        title: "Postgres search",
        status: "published",
        _highlightResult: {
          title: {
            value: "__ais-highlight__Postgres__/ais-highlight__ search",
            matchLevel: "full",
          },
        },
      },
    ]);
    expect(result.page).toBe(2);
    expect(result.nbHits).toBe(11);
    expect(result.nbPages).toBe(3);
    expect(result.hitsPerPage).toBe(5);
    expect(result.query).toBe("postgres");
    expect(result.exhaustiveNbHits).toBe(true);
    expect(result.processingTimeMS).toBeGreaterThanOrEqual(0);
    expect(result.facets).toEqual({
      status: { published: 7, null: 2 },
      brand: { Apple: 4 },
    });
    const echoedParams = new URLSearchParams(result.params);
    expect(echoedParams.get("query")).toBe("postgres");
    expect(echoedParams.get("page")).toBe("2");
    expect(echoedParams.get("hitsPerPage")).toBe("5");
    expect(echoedParams.get("facets")).toBe(JSON.stringify(["status", "brand"]));
  });

  it("uses the configured default index and sends empty queries as browsable list calls", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [{ uuid: "row_1", title: "First row" }],
          page: 1,
          perPage: 20,
          totalItems: 1,
          totalPages: 1,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "uuid",
      defaultIndexName: "posts",
      highlight: false,
    });

    const response = await searchClient.search([{ params: { query: "", facets: ["status"] } }]);

    const url = requestedURL(fetchFn);
    expect(url.pathname).toBe("/api/collections/posts");
    expect(url.searchParams.has("search")).toBe(false);
    expect(url.searchParams.get("page")).toBe("1");
    expect(url.searchParams.get("facets")).toBe("status");
    expect(url.searchParams.has("highlight")).toBe(false);
    expect(response.results[0].query).toBe("");
    expect(response.results[0].hits[0].objectID).toBe("row_1");
  });

  it("uses only records.list when an equivalent client is supplied", async () => {
    const client = createListOnlyClient({
      items: [{ id: "row_1", title: "From records list" }],
      page: 1,
      perPage: 10,
      totalItems: 1,
      totalPages: 1,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "posts",
    });

    await searchClient.search([{ params: { query: "records", hitsPerPage: 10 } }]);

    expect(client.records.list).toHaveBeenCalledWith("posts", {
      page: 1,
      perPage: 10,
      search: "records",
      highlight: true,
    });
  });

  it("remaps AYB highlight markers to the requested InstantSearch highlight tags", async () => {
    const client = createListOnlyClient({
      items: [
        {
          slug: "red-notebook",
          title: "Red Notebook",
          _highlightResult: {
            title: { value: "<b>Red</b> Notebook", matchLevel: "full" },
          },
        },
      ],
      page: 1,
      perPage: 6,
      totalItems: 1,
      totalPages: 1,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "slug",
    });

    const response = await searchClient.search([
      {
        indexName: "instantsearch_products",
        params: {
          query: "",
          page: 0,
          hitsPerPage: 6,
          facets: ["category"],
          highlightPreTag: "<mark>",
          highlightPostTag: "</mark>",
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("instantsearch_products", {
      page: 1,
      perPage: 6,
      facets: ["category"],
      highlight: true,
    });
    expect(response.results[0].hits[0]._highlightResult?.title?.value).toBe(
      "<mark>Red</mark> Notebook",
    );
    const echoedParams = new URLSearchParams(response.results[0].params);
    expect(echoedParams.get("highlightPreTag")).toBe("<mark>");
    expect(echoedParams.get("highlightPostTag")).toBe("</mark>");
  });

  it("rejects unsupported highlight tags before calling AYB", async () => {
    const fetchFn = mockFetchSequence([]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({ client, objectIDField: "id" });

    await expect(
      searchClient.search([
        {
          indexName: "posts",
          params: {
            highlightPreTag: "<img src=x onerror=alert(1)>",
            highlightPostTag: "</img>",
          },
        },
      ]),
    ).rejects.toThrow(
      "highlight tags must use InstantSearch placeholders or <mark> wrappers",
    );
    await expect(
      searchClient.search([
        {
          indexName: "posts",
          params: {
            highlightPreTag: "__ais-highlight__",
          },
        },
      ]),
    ).rejects.toThrow("highlightPreTag and highlightPostTag must be provided together");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("rejects mixed index requests before calling AYB", async () => {
    const fetchFn = mockFetchSequence([]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({ client, objectIDField: "id" });

    await expect(
      searchClient.search([
        { indexName: "posts", params: { query: "postgres" } },
        { indexName: "products", params: { query: "postgres" } },
      ]),
    ).rejects.toThrow("mixed indexName requests are not supported");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("rejects unsupported facetFilters shapes before calling AYB", async () => {
    const fetchFn = mockFetchSequence([]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({ client, objectIDField: "id" });

    await expect(
      searchClient.search([
        { indexName: "posts", params: { facetFilters: ["status:-archived"] } },
      ]),
    ).rejects.toThrow("negative facetFilters are not supported");
    await expect(
      searchClient.search([
        { indexName: "posts", params: { facetFilters: [[["status:active"]]] } },
      ]),
    ).rejects.toThrow("nested facetFilters deeper than one level are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { facetFilters: [[]] } }]),
    ).rejects.toThrow("facetFilters OR groups must not be empty");
    await expect(
      searchClient.search([
        { indexName: "posts", params: { facetFilters: ["status:active", []] } },
      ]),
    ).rejects.toThrow("facetFilters OR groups must not be empty");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("rejects unsupported filters shapes before calling AYB", async () => {
    const fetchFn = mockFetchSequence([]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({ client, objectIDField: "id" });

    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "NOT status:active" } }]),
    ).rejects.toThrow("NOT filters are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "_tags:beta" } }]),
    ).rejects.toThrow("_tags filters are not supported");
    await expect(
      searchClient.search([
        { indexName: "posts", params: { filters: "(_tags:beta OR status:active)" } },
      ]),
    ).rejects.toThrow("_tags filters are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "price:10 TO 20" } }]),
    ).rejects.toThrow("numeric range filters are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "author.name:stuart" } }]),
    ).rejects.toThrow("nested attributes are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "status:active AND" } }]),
    ).rejects.toThrow("malformed boolean filters are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "(status:active" } }]),
    ).rejects.toThrow("malformed boolean filters are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "status:active OR )" } }]),
    ).rejects.toThrow("malformed boolean filters are not supported");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("rejects wildcard facets and unlisted request parameters before calling AYB", async () => {
    const fetchFn = mockFetchSequence([]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({ client, objectIDField: "id" });

    await expect(
      searchClient.search([{ indexName: "posts", params: { facets: ["*"] } }]),
    ).rejects.toThrow('wildcard facets ["*"] are not supported');
    await expect(
      searchClient.search([{ indexName: "posts", params: { skipTotal: true } }]),
    ).rejects.toThrow("unsupported InstantSearch parameter: skipTotal");
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("rejects missing and null objectIDField values", async () => {
    const missingClient = createListOnlyClient({
      items: [{ id: "row_1" }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });
    const nullClient = createListOnlyClient({
      items: [{ uuid: null }],
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
    });

    await expect(
      createInstantSearchClient({
        client: missingClient,
        objectIDField: "uuid",
        defaultIndexName: "posts",
      }).search([{ params: {} }]),
    ).rejects.toThrow("objectIDField uuid is missing from a returned row");
    await expect(
      createInstantSearchClient({
        client: nullClient,
        objectIDField: "uuid",
        defaultIndexName: "posts",
      }).search([{ params: {} }]),
    ).rejects.toThrow("objectIDField uuid is null on a returned row");
  });

  it("rejects searchForFacetValues", async () => {
    const client = createListOnlyClient({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "posts",
    });

    await expect(searchClient.searchForFacetValues([])).rejects.toThrow(
      "searchForFacetValues is not supported",
    );
    expect(client.records.list).not.toHaveBeenCalled();
  });
});
