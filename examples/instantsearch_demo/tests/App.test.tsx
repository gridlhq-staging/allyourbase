import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const search = vi.fn(async (_requests) => {
  return {
    results: [
      {
        hits: [
          {
            slug: "red-notebook",
            objectID: "red-notebook",
            title: "Red Notebook",
            description: "Paper pages for research notes",
            category: "Stationery",
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
        page: 0,
        nbHits: 12,
        nbPages: 2,
        hitsPerPage: 6,
        processingTimeMS: 4,
        query: "",
        params:
          "query=&page=0&hitsPerPage=6&facets=%5B%22category%22%2C%22price_cents%22%5D",
        exhaustiveNbHits: true,
      },
    ],
  };
});

vi.mock("../src/lib/ayb", () => ({
  searchClient: {
    search,
    searchForFacetValues: vi.fn(),
  },
}));

describe("InstantSearch demo", () => {
  it("renders canonical adapter results without local response normalization", async () => {
    const { default: App } = await import("../src/App");

    render(<App />);

    await waitFor(() => expect(search).toHaveBeenCalledTimes(1));
    expect(search.mock.calls[0][0][0]).toMatchObject({
      indexName: "instantsearch_products",
      params: {
        facets: expect.arrayContaining(["category", "price_cents"]),
        highlightPostTag: "__/ais-highlight__",
        highlightPreTag: "__ais-highlight__",
        hitsPerPage: 6,
        page: 0,
        query: "",
      },
    });
    expect(screen.getByRole("searchbox")).toHaveValue("");
    expect(screen.getByText(/12 results/)).toBeInTheDocument();
    const filters = within(screen.getByLabelText("Filters"));
    expect(filters.getByText("Stationery")).toBeInTheDocument();
    expect(filters.getByText("7")).toBeInTheDocument();
    expect(filters.getByText("Lighting")).toBeInTheDocument();
    expect(filters.getByText("5")).toBeInTheDocument();
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

    await waitFor(() => expect(search).toHaveBeenCalledTimes(1));

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
});
