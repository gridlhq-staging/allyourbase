import { describe, expect, it, vi } from "vitest";
import { AYBClient } from "./client";
import { createInstantSearchClient } from "./instantsearch";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";
import type {
  FacetValueSearchParams,
  FacetValueSearchResponse,
  ListParams,
  ListResponse,
  SearchHit,
} from "./types";

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

function createFacetSearchClient(response: FacetValueSearchResponse) {
  const list = vi.fn(
    async (_collection: string, _params?: ListParams) =>
      ({
        items: [],
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
      }) as ListResponse<SearchHit>,
  );
  const searchFacetValues = vi.fn(
    async (
      _collection: string,
      _column: string,
      _params?: FacetValueSearchParams,
    ) => response,
  );
  return {
    records: { list, searchFacetValues },
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
          facetStats: {
            price_cents: {
              min: 1299,
              max: 4599,
            },
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
    expect(result.facetStats).toEqual({
      price_cents: { min: 1299, max: 4599 },
    });
    const echoedParams = new URLSearchParams(result.params);
    expect(echoedParams.get("query")).toBe("postgres");
    expect(echoedParams.get("page")).toBe("2");
    expect(echoedParams.get("hitsPerPage")).toBe("5");
    expect(echoedParams.get("facets")).toBe(JSON.stringify(["status", "brand"]));
  });

  it("requests disjunctive facets for OR facet groups", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await searchClient.search([
      {
        indexName: "products",
        params: {
          facets: ["category", "brand"],
          facetFilters: [["category:Books", "category:Games"], "brand:Acme"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.searchParams.get("facets")).toBe("category,brand");
    expect(url.searchParams.get("disjunctiveFacets")).toBe("category");
    expect(url.searchParams.get("filter")).toBe(
      "(category='Books' OR category='Games') AND brand='Acme'",
    );
  });

  it("requests disjunctive facets for a single selected OR facet", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await searchClient.search([
      {
        indexName: "products",
        params: {
          facets: ["category", "brand"],
          facetFilters: [["category:Books"], "brand:Acme"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.searchParams.get("facets")).toBe("category,brand");
    expect(url.searchParams.get("disjunctiveFacets")).toBe("category");
    expect(url.searchParams.get("filter")).toBe("category='Books' AND brand='Acme'");
  });

  it("passes explicit disjunctive facets through for single selected facet refinements", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await searchClient.search([
      {
        indexName: "products",
        params: {
          facets: ["category", "brand"],
          disjunctiveFacets: ["category"],
          facetFilters: ["category:Books", "brand:Acme"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.searchParams.get("facets")).toBe("category,brand");
    expect(url.searchParams.get("disjunctiveFacets")).toBe("category");
    expect(url.searchParams.get("filter")).toBe("category='Books' AND brand='Acme'");
  });

  it("passes adapter-level disjunctive facets through for widget refinements", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      disjunctiveFacets: ["category"],
    });

    await searchClient.search([
      {
        indexName: "products",
        params: {
          facets: ["category", "brand"],
          facetFilters: ["category:Books", "brand:Acme"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.searchParams.get("facets")).toBe("category,brand");
    expect(url.searchParams.get("disjunctiveFacets")).toBe("category");
    expect(url.searchParams.get("filter")).toBe("category='Books' AND brand='Acme'");
  });

  it("groups repeated flat filters for adapter-level disjunctive facets", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [],
          page: 1,
          perPage: 20,
          totalItems: 0,
          totalPages: 0,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      disjunctiveFacets: ["category"],
    });

    await searchClient.search([
      {
        indexName: "products",
        params: {
          facets: ["category", "brand"],
          facetFilters: ["category:Books", "category:Games", "brand:Acme"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.searchParams.get("disjunctiveFacets")).toBe("category");
    expect(url.searchParams.get("filter")).toBe(
      "(category='Books' OR category='Games') AND brand='Acme'",
    );
  });

  it("preserves backend disjunctive facet counts in InstantSearch results", async () => {
    const client = createListOnlyClient({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      facets: {
        category: [
          { value: "Books", count: 12 },
          { value: "Games", count: 7 },
          { value: "Electronics", count: 3 },
        ],
      },
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.search([
      {
        params: {
          facets: ["category"],
          facetFilters: [["category:Books", "category:Games"]],
        },
      },
    ]);

    expect(response.results[0].facets?.category).toEqual({
      Books: 12,
      Games: 7,
      Electronics: 3,
    });
  });

  it("keeps conjunctive facet filters out of disjunctive facets", async () => {
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
      defaultIndexName: "products",
    });

    await searchClient.search([
      {
        params: {
          facets: ["category", "brand"],
          facetFilters: ["category:Books", "brand:Acme"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      facets: ["category", "brand"],
      filter: "category='Books' AND brand='Acme'",
      highlight: true,
    });
  });

  it("translates numeric range filters into AYB comparisons", async () => {
    const client = createListOnlyClient({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      facetStats: {
        price_cents: {
          min: "1299",
          max: "8999",
        },
      },
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.search([
      {
        params: {
          filters: "price_cents:1000 TO 5000",
          facets: ["price_cents"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      facets: ["price_cents"],
      filter: "(price_cents>=1000 AND price_cents<=5000)",
      highlight: true,
    });
    expect(response.results[0].facetStats).toEqual({
      price_cents: { min: 1299, max: 8999 },
    });
  });

  it("translates InstantSearch numericFilters into AYB comparisons", async () => {
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
      defaultIndexName: "products",
    });

    const response = await searchClient.search([
      {
        params: {
          facets: ["price_cents"],
          numericFilters: ["price_cents>=1000", "price_cents<=5000"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      facets: ["price_cents"],
      filter: "price_cents>=1000 AND price_cents<=5000",
      highlight: true,
    });
    const [, params] = client.records.list.mock.calls[0];
    expect(params).not.toHaveProperty("numericFilters");
    const echoedParams = new URLSearchParams(response.results[0].params);
    expect(echoedParams.get("numericFilters")).toBe(
      JSON.stringify(["price_cents>=1000", "price_cents<=5000"]),
    );
  });

  it("ignores Algolia analytics params while preserving numeric range filters", async () => {
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
      defaultIndexName: "products",
    });

    await searchClient.search([
      {
        params: {
          analytics: false,
          clickAnalytics: false,
          facets: ["price_cents"],
          numericFilters: ["price_cents>=4000", "price_cents<=5000"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      facets: ["price_cents"],
      filter: "price_cents>=4000 AND price_cents<=5000",
      highlight: true,
    });
  });

  it("supports InstantSearch facet-only requests with zero hits per page", async () => {
    const client = createListOnlyClient({
      items: [{ id: "ignored_hit" }],
      page: 1,
      perPage: 1,
      totalItems: 14,
      totalPages: 14,
      facets: {
        price_cents: [{ value: 4599, count: 1 }],
      },
      facetStats: {
        price_cents: { min: 4599, max: 4599 },
      },
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.search([
      {
        params: {
          hitsPerPage: 0,
          facets: "price_cents",
          disjunctiveFacets: ["price_cents"],
          numericFilters: ["price_cents>=4000", "price_cents<=5000"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      perPage: 1,
      facets: ["price_cents"],
      disjunctiveFacets: ["price_cents"],
      filter: "price_cents>=4000 AND price_cents<=5000",
      highlight: true,
    });
    expect(response.results[0].hits).toEqual([]);
    expect(response.results[0].hitsPerPage).toBe(0);
    expect(response.results[0].nbPages).toBe(0);
    expect(response.results[0].disjunctiveFacets).toEqual([
      {
        name: "price_cents",
        data: { "4599": 1 },
        stats: { min: 4599, max: 4599 },
      },
    ]);
  });

  it("combines numericFilters with facetFilters into one AYB filter", async () => {
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
      defaultIndexName: "products",
    });

    await searchClient.search([
      {
        params: {
          facetFilters: [["brand:Acme", "brand:Zen"], "status:active"],
          numericFilters: ["price_cents>=1000", "price_cents<=5000"],
        },
      },
    ]);

    expect(client.records.list).toHaveBeenCalledWith("products", {
      page: 1,
      filter:
        "(brand='Acme' OR brand='Zen') AND status='active' AND price_cents>=1000 AND price_cents<=5000",
      highlight: true,
    });
    const [, params] = client.records.list.mock.calls[0];
    expect(params).not.toHaveProperty("numericFilters");
  });

  it("maps backend facetStats into InstantSearch facets_stats", async () => {
    const client = createListOnlyClient({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      facetStats: {
        price_cents: {
          min: "799",
          max: "8999",
        },
      },
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.search([{ params: {} }]);

    const result = response.results[0];
    expect(result.facets_stats?.price_cents).toEqual({ min: 799, max: 8999 });
    expect(result.facetStats?.price_cents).toEqual({ min: 799, max: 8999 });
    expect(result.facets_stats).toBe(result.facetStats);
  });

  it("maps backend facetStats into disjunctive facet stats for RangeInput", async () => {
    const client = createListOnlyClient({
      items: [],
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      facetStats: {
        price_cents: {
          min: "799",
          max: "8999",
        },
      },
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.search([
      {
        params: {
          facets: ["price_cents"],
          disjunctiveFacets: ["price_cents"],
        },
      },
    ]);

    expect(response.results[0].disjunctiveFacets).toEqual([
      {
        name: "price_cents",
        data: {},
        stats: { min: 799, max: 8999 },
      },
    ]);
  });

  it("rejects malformed numericFilters before calling AYB", async () => {
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
      defaultIndexName: "products",
    });

    await expect(
      searchClient.search([{ params: { numericFilters: ["price_cents=1000"] } }]),
    ).rejects.toThrow("numericFilters must use range comparison operators");
    await expect(
      searchClient.search([{ params: { numericFilters: ["price_cents:1000"] } }]),
    ).rejects.toThrow("numericFilters must use attribute<op>number form");
    await expect(
      searchClient.search([{ params: { numericFilters: ["price_cents>=cheap"] } }]),
    ).rejects.toThrow("numericFilters must use attribute<op>number form");
    expect(client.records.list).not.toHaveBeenCalled();
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
      searchClient.search([{ indexName: "posts", params: { filters: "author.name:stuart" } }]),
    ).rejects.toThrow("nested attributes are not supported");
    await expect(
      searchClient.search([{ indexName: "posts", params: { filters: "price:cheap TO 20" } }]),
    ).rejects.toThrow("numeric range filters require numeric bounds");
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

  it("searchForFacetValues delegates to records.searchFacetValues with translated params", async () => {
    const client = createFacetSearchClient({
      facetHits: [
        { value: "Acme", highlighted: "<mark>Ac</mark>me", count: 12 },
      ],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await searchClient.searchForFacetValues([
      {
        indexName: "products",
        params: {
          facetName: "brand",
          facetQuery: "ac",
          query: "guide",
          maxFacetHits: 3,
          facetFilters: ["category:books"],
          filters: "published:true",
          highlightPreTag: "__ais-highlight__",
          highlightPostTag: "__/ais-highlight__",
        },
      },
    ]);

    expect(client.records.searchFacetValues).toHaveBeenCalledTimes(1);
    expect(client.records.searchFacetValues).toHaveBeenCalledWith("products", "brand", {
      q: "ac",
      maxFacetHits: 3,
      filter: "category='books' AND published=true",
      search: "guide",
    });
  });

  it("searchForFacetValues works with the public AYBClient records owner", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          facetHits: [
            { value: "Acme", highlighted: "<mark>Ac</mark>me", count: 12 },
          ],
          exhaustiveFacetsCount: true,
        },
      },
    ]);
    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.searchForFacetValues([
      {
        params: {
          facetName: "brand",
          facetQuery: "ac",
          query: "guide",
          maxFacetHits: 3,
          facetFilters: ["category:books"],
        },
      },
    ]);

    const url = requestedURL(fetchFn);
    expect(url.pathname).toBe("/api/collections/products/facets/brand/search");
    expect(url.searchParams.get("q")).toBe("ac");
    expect(url.searchParams.get("maxFacetHits")).toBe("3");
    expect(url.searchParams.get("filter")).toBe("category='books'");
    expect(url.searchParams.get("search")).toBe("guide");
    expect(response[0].facetHits).toEqual([
      {
        value: "Acme",
        highlighted: "__ais-highlight__Ac__/ais-highlight__me",
        count: 12,
      },
    ]);
    expect(response[0].exhaustiveFacetsCount).toBe(true);
    expect(response[0].processingTimeMS).toBeGreaterThanOrEqual(0);
  });

  it("searchForFacetValues remaps backend <mark> tags to caller highlight tags and preserves shape", async () => {
    const client = createFacetSearchClient({
      facetHits: [
        { value: "Acme", highlighted: "<mark>Ac</mark>me", count: 12 },
        { value: "Acorn", highlighted: "<mark>Ac</mark>orn", count: 3 },
      ],
      exhaustiveFacetsCount: false,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.searchForFacetValues([
      { params: { facetName: "brand", facetQuery: "ac" } },
    ]);

    expect(response).toHaveLength(1);
    const [result] = response;
    expect(result.exhaustiveFacetsCount).toBe(false);
    expect(result.processingTimeMS).toBeGreaterThanOrEqual(0);
    expect(result.facetHits).toEqual([
      {
        value: "Acme",
        highlighted: "__ais-highlight__Ac__/ais-highlight__me",
        count: 12,
      },
      {
        value: "Acorn",
        highlighted: "__ais-highlight__Ac__/ais-highlight__orn",
        count: 3,
      },
    ]);
  });

  it("searchForFacetValues remaps <mark> to explicit <mark> highlight tags when requested", async () => {
    const client = createFacetSearchClient({
      facetHits: [{ value: "Acme", highlighted: "<mark>Ac</mark>me", count: 1 }],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    const response = await searchClient.searchForFacetValues([
      {
        params: {
          facetName: "brand",
          facetQuery: "ac",
          highlightPreTag: "<mark>",
          highlightPostTag: "</mark>",
        },
      },
    ]);

    expect(response[0].facetHits[0].highlighted).toBe("<mark>Ac</mark>me");
  });

  it("searchForFacetValues throws when records owner lacks searchFacetValues", async () => {
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
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        { params: { facetName: "brand", facetQuery: "ac" } },
      ]),
    ).rejects.toThrow(
      "client.records.searchFacetValues is required for searchForFacetValues",
    );
    expect(client.records.list).not.toHaveBeenCalled();
  });

  it("searchForFacetValues requires facetName", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        { params: { facetQuery: "ac" } as unknown as { facetName: string } },
      ]),
    ).rejects.toThrow("facetName is required");
    expect(client.records.searchFacetValues).not.toHaveBeenCalled();
  });

  it("searchForFacetValues rejects non-string facetQuery", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: 42 as unknown as string,
          },
        },
      ]),
    ).rejects.toThrow("facetQuery must be a string");
  });

  it("searchForFacetValues rejects invalid maxFacetHits", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: "ac",
            maxFacetHits: 0,
          },
        },
      ]),
    ).rejects.toThrow("maxFacetHits must be a positive integer");

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: "ac",
            maxFacetHits: 1.5 as unknown as number,
          },
        },
      ]),
    ).rejects.toThrow("maxFacetHits must be a positive integer");

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: "ac",
            maxFacetHits: 101,
          },
        },
      ]),
    ).rejects.toThrow("maxFacetHits must be less than or equal to 100");
  });

  it("searchForFacetValues rejects mixed indexName requests", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await expect(
      searchClient.searchForFacetValues([
        { indexName: "products", params: { facetName: "brand", facetQuery: "ac" } },
        { indexName: "posts", params: { facetName: "tag", facetQuery: "go" } },
      ]),
    ).rejects.toThrow("mixed indexName requests are not supported");
    expect(client.records.searchFacetValues).not.toHaveBeenCalled();
  });

  it("searchForFacetValues rejects unsupported highlight tags", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: "ac",
            highlightPreTag: "<em>",
            highlightPostTag: "</em>",
          },
        },
      ]),
    ).rejects.toThrow(
      "highlight tags must use InstantSearch placeholders or <mark> wrappers",
    );
    expect(client.records.searchFacetValues).not.toHaveBeenCalled();
  });

  it("searchForFacetValues rejects unsupported params", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
      defaultIndexName: "products",
    });

    await expect(
      searchClient.searchForFacetValues([
        {
          params: {
            facetName: "brand",
            facetQuery: "ac",
            ...({ analytics: true } as { analytics: boolean }),
          },
        } as never,
      ]),
    ).rejects.toThrow(/unsupported searchForFacetValues parameter/);
  });

  it("searchForFacetValues requires indexName or defaultIndexName", async () => {
    const client = createFacetSearchClient({
      facetHits: [],
      exhaustiveFacetsCount: true,
    });
    const searchClient = createInstantSearchClient({
      client,
      objectIDField: "id",
    });

    await expect(
      searchClient.searchForFacetValues([
        { params: { facetName: "brand", facetQuery: "ac" } },
      ]),
    ).rejects.toThrow("indexName or defaultIndexName is required");
  });
});
