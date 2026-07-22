import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type SearchRequest = {
  indexName: string;
  params?: Record<string, unknown>;
};

type SearchResult = {
  hits: Array<Record<string, unknown>>;
  facets: Record<string, Record<string, number>>;
  facets_stats: Record<string, Record<string, number>>;
  page: number;
  nbHits: number;
  nbPages: number;
  hitsPerPage: number;
  processingTimeMS: number;
  query: string;
  params: string;
  exhaustiveNbHits: boolean;
};

type SearchResponse = {
  results: SearchResult[];
};

function serializeParams(params: Record<string, unknown> = {}) {
  return new URLSearchParams(
    Object.entries(params).map(([key, value]) => [
      key,
      Array.isArray(value) ? JSON.stringify(value) : String(value),
    ]),
  ).toString();
}

function populatedResult(
  request: SearchRequest = { indexName: "instantsearch_products" },
): SearchResult {
  const params = request.params ?? {};
  return {
    hits: [
      {
        slug: "red-notebook",
        objectID: "red-notebook",
        title: "Red Notebook",
        description: "Paper pages for research notes",
        category: "Stationery",
        brand: "Apex",
        price_cents: 1299,
        _highlightResult: {
          title: {
            value: "__ais-highlight__Red__/ais-highlight__ Notebook",
            matchLevel: "full",
          },
          description: {
            value: "Paper pages for __ais-highlight__research__/ais-highlight__ notes",
            matchLevel: "full",
          },
        },
      },
      {
        slug: "brass-desk-lamp",
        objectID: "brass-desk-lamp",
        title: "Brass Desk Lamp",
        description: "Focused light for workspaces",
        category: "Lighting",
        brand: "Beacon",
        price_cents: 4599,
        _highlightResult: {
          title: { value: "Brass Desk Lamp", matchLevel: "none" },
          description: { value: "Focused light for workspaces", matchLevel: "none" },
        },
      },
    ],
    facets: {
      category: {
        Stationery: 7,
        Lighting: 5,
      },
      brand: {
        Apex: 6,
        Beacon: 4,
      },
      price_cents: {
        799: 1,
        1299: 1,
        4599: 1,
        8999: 1,
      },
    },
    facets_stats: {
      price_cents: {
        min: 799,
        max: 8999,
      },
    },
    page: Number(params.page ?? 0),
    nbHits: 12,
    nbPages: 2,
    hitsPerPage: Number(params.hitsPerPage ?? 6),
    processingTimeMS: 4,
    query: String(params.query ?? ""),
    params: serializeParams(params),
    exhaustiveNbHits: true,
  };
}

function populatedSearchResult(requests: SearchRequest[] = []): SearchResponse {
  const resultRequests =
    requests.length > 0 ? requests : [{ indexName: "instantsearch_products" }];
  return {
    results: resultRequests.map((request) => populatedResult(request)),
  };
}

function emptyResult(
  request: SearchRequest = { indexName: "instantsearch_products" },
): SearchResult {
  const params = request.params ?? {};
  return {
    hits: [],
    facets: {
      category: {},
      brand: {},
      price_cents: {},
    },
    facets_stats: {},
    page: Number(params.page ?? 0),
    nbHits: 0,
    nbPages: 0,
    hitsPerPage: Number(params.hitsPerPage ?? 6),
    processingTimeMS: 3,
    query: String(params.query ?? "zzzz-no-products"),
    params: serializeParams(params),
    exhaustiveNbHits: true,
  };
}

function emptySearchResult(requests: SearchRequest[] = []): SearchResponse {
  const resultRequests =
    requests.length > 0 ? requests : [{ indexName: "instantsearch_products" }];
  return {
    results: resultRequests.map((request) => emptyResult(request)),
  };
}

const search = vi.fn(async (requests: SearchRequest[]): Promise<SearchResponse> =>
  populatedSearchResult(requests),
);

vi.mock("../src/lib/ayb", () => ({
  searchClient: {
    search,
    searchForFacetValues: vi.fn(),
  },
}));

function productRequests() {
  return search.mock.calls
    .flatMap(([requests]) => requests)
    .filter((request) => request.indexName === "instantsearch_products");
}

describe("InstantSearch demo", () => {
  afterEach(async () => {
    cleanup();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  beforeEach(() => {
    search.mockReset();
    let searchCallCount = 0;
    search.mockImplementation(async (requests) => {
      searchCallCount += 1;
      if (searchCallCount > 25) {
        throw new Error("Search adapter was called too many times");
      }
      return populatedSearchResult(requests);
    });
  });

  it("renders canonical adapter results without local response normalization", async () => {
    const { default: App } = await import("../src/App");

    render(<App />);

    await waitFor(() =>
      expect(
        productRequests().some((request) =>
          expect.objectContaining({
            params: expect.objectContaining({
              facets: expect.arrayContaining(["category", "brand", "price_cents"]),
              highlightPostTag: "__/ais-highlight__",
              highlightPreTag: "__ais-highlight__",
              hitsPerPage: 6,
              query: "",
            }),
          }).asymmetricMatch(request),
        ),
      ).toBe(true),
    );
    expect(screen.getByRole("searchbox")).toHaveValue("");
    expect(screen.getByText(/12 results/)).toBeInTheDocument();
    const filters = within(screen.getByLabelText("Filters"));
    expect(filters.getByText("Stationery")).toBeInTheDocument();
    expect(filters.getByText("7")).toBeInTheDocument();
    expect(filters.getByText("Lighting")).toBeInTheDocument();
    expect(filters.getByText("5")).toBeInTheDocument();
    expect(filters.getByText("Brand")).toBeInTheDocument();
    expect(filters.getByText("Apex")).toBeInTheDocument();
    expect(filters.getByText("6")).toBeInTheDocument();
    expect(filters.getByText("Beacon")).toBeInTheDocument();
    expect(filters.getByText("4")).toBeInTheDocument();
    expect(filters.getByText("Price range")).toBeInTheDocument();
    const rangeInputs = within(screen.getByLabelText("Price range")).getAllByRole(
      "spinbutton",
    );
    expect(rangeInputs[0]).toHaveValue(null);
    expect(rangeInputs[0]).toHaveAttribute("placeholder", "799");
    expect(rangeInputs[1]).toHaveValue(null);
    expect(rangeInputs[1]).toHaveAttribute("placeholder", "8999");
    expect(screen.getByRole("button", { name: "Go" })).toBeInTheDocument();

    const firstHit = screen.getByTestId("hit-red-notebook");
    expect(firstHit).toHaveTextContent("red-notebook");
    expect(screen.getByTestId("hit-red-notebook-title-highlight")).toHaveTextContent("Red");
    expect(screen.getByTestId("hit-red-notebook-description-highlight")).toHaveTextContent("research");
    expect(screen.getByRole("link", { name: "Page 2" })).toBeInTheDocument();
  });

  it("submits a numeric range refinement through the adapter request shape", async () => {
    const { default: App } = await import("../src/App");

    render(<App />);

    await waitFor(() => expect(search).toHaveBeenCalled());

    const rangeInputs = within(screen.getByLabelText("Price range")).getAllByRole(
      "spinbutton",
    );

    fireEvent.input(rangeInputs[0], {
      target: { value: "1000" },
    });
    fireEvent.input(rangeInputs[1], {
      target: { value: "5000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));

    await waitFor(() =>
      expect(
        search.mock.calls.some(([requests]) =>
          JSON.stringify(requests[0]).includes('"numericFilters":["price_cents>=1000","price_cents<=5000"]'),
        ),
      ).toBe(true),
    );
  });

  it("renders a concrete empty state when a query has zero hits and no facets", async () => {
    search.mockImplementation(async (requests) => emptySearchResult(requests));
    const { default: App } = await import("../src/App");

    render(<App />);

    await waitFor(() => expect(search).toHaveBeenCalled());
    expect(await screen.findByRole("status")).toHaveTextContent(
      "No products match this search.",
    );
    expect(screen.queryByTestId("hit-red-notebook")).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: /pagination/i })).not.toBeInTheDocument();
    const filters = within(screen.getByLabelText("Filters"));
    expect(filters.getByText("No category filters available.")).toBeInTheDocument();
    expect(filters.getByText("No brand filters available.")).toBeInTheDocument();
  });

  it("renders an API error state when the search request is rejected", async () => {
    search.mockRejectedValue(new Error("network down"));
    const { default: App } = await import("../src/App");

    render(<App />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "We could not load products. Check the API connection and retry.",
    );
    expect(screen.queryByTestId("hit-red-notebook")).not.toBeInTheDocument();
  });

  it("renders a first-search loading state before the adapter resolves", async () => {
    search.mockImplementation(async (requests) => {
      await new Promise((resolve) => setTimeout(resolve, 25));
      return populatedSearchResult(requests);
    });
    const { default: App } = await import("../src/App");

    render(<App />);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Loading product search...",
    );
    await waitFor(() => expect(screen.getByText(/12 results/)).toBeInTheDocument());
  });
});
