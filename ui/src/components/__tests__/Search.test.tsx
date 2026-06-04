import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Column, SchemaCache } from "../../types";
import { renderWithProviders } from "../../test-utils";
import { Search } from "../Search";
import { listSearchPlaygroundRecords } from "../../api_search";

vi.mock("../../api_search", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api_search")>();
  return {
    ...actual,
    listSearchPlaygroundRecords: vi.fn(),
  };
});

const mockListSearchPlaygroundRecords = vi.mocked(listSearchPlaygroundRecords);

function makeColumn(overrides: Partial<Column> & Pick<Column, "name">): Column {
  return {
    name: overrides.name,
    position: overrides.position ?? 1,
    type: overrides.type ?? "text",
    nullable: overrides.nullable ?? false,
    default: overrides.default,
    comment: overrides.comment,
    isPrimaryKey: overrides.isPrimaryKey ?? false,
    jsonType: overrides.jsonType ?? "string",
    enumValues: overrides.enumValues,
  };
}

function makeSchema(
  tables: Record<string, { schema: string; name: string; kind?: string; columns?: Column[] }> = {},
): SchemaCache {
  const schema: SchemaCache = {
    schemas: ["public"],
    builtAt: "2026-06-01T00:00:00Z",
    tables: {},
  };
  for (const [key, table] of Object.entries(tables)) {
    schema.tables[key] = {
      schema: table.schema,
      name: table.name,
      kind: table.kind ?? "table",
      columns: table.columns ?? [
        makeColumn({ name: "id", position: 1, type: "uuid", isPrimaryKey: true }),
        makeColumn({ name: "title", position: 2 }),
      ],
      primaryKey: ["id"],
      foreignKeys: [],
      indexes: [],
      relationships: [],
    };
  }
  return schema;
}

describe("Search", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders collection selector from schema tables", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.z_posts": { schema: "public", name: "z_posts" },
          "internal.accounts": { schema: "internal", name: "accounts" },
        })}
      />,
    );

    const selector = await screen.findByLabelText("Collection");
    const options = Array.from(selector.querySelectorAll("option")).map((option) => option.textContent);
    expect(options).toEqual(["internal.accounts", "z_posts"]);
  });

  it("shows loading then results on fetch", async () => {
    mockListSearchPlaygroundRecords.mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(() => {
            resolve({
              page: 1,
              perPage: 20,
              totalItems: 1,
              totalPages: 1,
              items: [{ id: "row-1", title: "Alpha" }],
            });
          }, 0);
        }),
    );

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    expect(screen.getByText("Loading...")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("Alpha")).toBeInTheDocument();
    });
  });

  it("hides stale rows while a newer request is loading", async () => {
    let resolveInitial: ((value: Awaited<ReturnType<typeof listSearchPlaygroundRecords>>) => void) | undefined;
    let resolveNext: ((value: Awaited<ReturnType<typeof listSearchPlaygroundRecords>>) => void) | undefined;
    mockListSearchPlaygroundRecords
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveInitial = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveNext = resolve;
          }),
      );

    renderWithProviders(<Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />);
    const user = userEvent.setup();

    resolveInitial?.({
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      items: [{ id: "row-1", title: "Alpha" }],
    });
    await waitFor(() => {
      expect(screen.getByText("Alpha")).toBeInTheDocument();
    });

    await user.type(screen.getByRole("textbox", { name: "Search query" }), "beta");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(screen.getByText("Loading...")).toBeInTheDocument();
    });
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();

    resolveNext?.({
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      items: [{ id: "row-2", title: "Beta" }],
    });
    await waitFor(() => {
      expect(screen.getByText("Beta")).toBeInTheDocument();
    });
  });

  it("ignores stale out-of-order responses and keeps newest results", async () => {
    let resolveFirst: ((value: Awaited<ReturnType<typeof listSearchPlaygroundRecords>>) => void) | undefined;
    let resolveSecond: ((value: Awaited<ReturnType<typeof listSearchPlaygroundRecords>>) => void) | undefined;
    mockListSearchPlaygroundRecords
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      );

    renderWithProviders(<Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />);
    const user = userEvent.setup();

    await user.type(screen.getByRole("textbox", { name: "Search query" }), "newest");
    await user.click(screen.getByRole("button", { name: "Search" }));

    resolveSecond?.({
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      items: [{ id: "new", title: "Newest result" }],
    });
    await waitFor(() => {
      expect(screen.getByText("Newest result")).toBeInTheDocument();
    });

    resolveFirst?.({
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      items: [{ id: "old", title: "Old result" }],
    });
    await waitFor(() => {
      expect(screen.getByText("Newest result")).toBeInTheDocument();
    });
    expect(screen.queryByText("Old result")).not.toBeInTheDocument();
  });

  it("keeps draft search input local until submit", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(1);
    });

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");

    expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "alpha" }),
      );
    });
  });

  it("sends fuzzy true when enabled and submitted", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "fuzzy term");
    await user.click(screen.getByRole("checkbox", { name: "Use fuzzy matching" }));
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "fuzzy term", fuzzy: true }),
      );
    });
  });

  it("requests and shows highlighted matches for applied full-text searches by default", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1", title: "Alpha story", _highlight: "<b>Alpha</b> story" }],
      });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "alpha", highlight: true }),
      );
    });

    expect(screen.getByRole("checkbox", { name: "Show highlighted matches" })).toBeChecked();
    const strip = await screen.findByTestId("search-highlight-results");
    const snippet = within(strip).getByTestId("search-highlight-snippet-0");
    expect(snippet.textContent).toBe("Result 1: Alpha story");
    expect(snippet.querySelector("mark")?.textContent).toBe("Alpha");
  });

  it("hides the Search-owned snippet surface when highlighted matches are toggled off", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1", title: "Alpha story", _highlight: "<b>Alpha</b> story" }],
      });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await screen.findByTestId("search-highlight-results");

    await user.click(screen.getByRole("checkbox", { name: "Show highlighted matches" }));

    expect(screen.queryByTestId("search-highlight-results")).not.toBeInTheDocument();
  });

  it("suppresses blank highlight snippets when the backend returns only empty markup", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1", title: "Alpha story", _highlight: "<b></b>" }],
      });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "alpha", highlight: true }),
      );
    });

    expect(screen.queryByTestId("search-highlight-results")).not.toBeInTheDocument();
    expect(screen.getByText("Alpha story")).toBeInTheDocument();
  });

  it("renders only literal b highlight markers as emphasis and leaves hostile markup inert", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [
          {
            id: "row-1",
            title: "Hostile",
            _highlight:
              "Safe <b>match</b> &lt;script&gt;alert(1)&lt;/script&gt; &amp; &lt;img src=x onerror=alert(2)&gt; &lt;b onclick=&#34;alert(3)&#34;&gt;bad&lt;/b&gt;",
          },
        ],
      });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "match");
    await user.click(screen.getByRole("button", { name: "Search" }));

    const snippet = await screen.findByTestId("search-highlight-snippet-0");
    expect(snippet.querySelectorAll("mark")).toHaveLength(1);
    expect(snippet.querySelector("mark")?.textContent).toBe("match");
    expect(snippet.querySelector("script")).toBeNull();
    expect(snippet.querySelector("img")).toBeNull();
    expect(snippet.textContent).toContain("<script>alert(1)</script>");
    expect(snippet.textContent).toContain("&");
    expect(snippet.textContent).toContain("<img src=x onerror=alert(2)>");
    expect(snippet.textContent).toContain('<b onclick="alert(3)">bad</b>');
  });

  it("does not request generated highlights or consume real _highlight columns", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 0,
        totalPages: 0,
        items: [],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1", title: "Alpha story", _highlight: "owner column value" }],
      });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.posts": {
            schema: "public",
            name: "posts",
            columns: [
              makeColumn({ name: "id", position: 1, type: "uuid", isPrimaryKey: true }),
              makeColumn({ name: "title", position: 2 }),
              makeColumn({ name: "_highlight", position: 3 }),
            ],
          },
        })}
      />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "alpha", highlight: undefined }),
      );
    });

    expect(screen.queryByTestId("search-highlight-results")).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "_highlight" })).toBeInTheDocument();
    expect(screen.getByText("owner column value")).toBeInTheDocument();
  });

  it("keeps draft filter local until submit", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(1);
    });

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Filter expression" }), "status='active'");

    expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ filter: "status='active'" }),
      );
    });
  });

  it("hides facet controls when the selected collection has no eligible scalar columns", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 1,
      totalPages: 1,
      items: [{ metadata: { status: "draft" } }],
      facets: {
        metadata: [{ value: "draft", count: 1 }],
      },
    });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.events": {
            schema: "public",
            name: "events",
            columns: [
              makeColumn({ name: "metadata", position: 1, type: "jsonb", jsonType: "object" }),
              makeColumn({ name: "tags", position: 2, type: "text[]", jsonType: "array" }),
              makeColumn({ name: "embedding", position: 3, type: "vector", jsonType: "array" }),
            ],
          },
        })}
      />,
    );

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "events",
        expect.objectContaining({ facets: undefined }),
      );
    });
    expect(screen.queryByTestId("search-facet-controls")).not.toBeInTheDocument();
    expect(screen.queryByTestId("search-facet-panel-metadata")).not.toBeInTheDocument();
  });

  it("requests selected facet columns and renders exact returned buckets", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 3,
        totalPages: 1,
        items: [{ id: "row-1", status: "published" }],
      })
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 3,
        totalPages: 1,
        items: [{ id: "row-1", status: "published" }],
        facets: {
          status: [
            { value: "draft", count: 7 },
            { value: "published", count: 3 },
            { value: null, count: 5 },
          ],
        },
      });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.posts": {
            schema: "public",
            name: "posts",
            columns: [
              makeColumn({ name: "id", position: 1, type: "uuid", isPrimaryKey: true }),
              makeColumn({ name: "status", position: 2, enumValues: ["published", "draft"] }),
              makeColumn({ name: "title", position: 3, type: "text", jsonType: "string" }),
              makeColumn({ name: "archived", position: 4, type: "boolean", jsonType: "boolean" }),
              makeColumn({ name: "score", position: 5, type: "numeric", jsonType: "number" }),
              makeColumn({ name: "published_on", position: 6, type: "date", jsonType: "string" }),
              makeColumn({
                name: "created_at",
                position: 7,
                type: "timestamp with time zone",
                jsonType: "string",
              }),
              makeColumn({ name: "metadata", position: 8, type: "jsonb", jsonType: "object" }),
            ],
          },
        })}
      />,
    );

    const user = userEvent.setup();
    expect(
      within(screen.getByTestId("search-facet-controls"))
        .getAllByRole("checkbox")
        .map((option) => option.getAttribute("aria-label")),
    ).toEqual(["id", "status", "title", "archived", "score", "published_on", "created_at"]);

    await user.click(screen.getByRole("checkbox", { name: "status" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ facets: ["status"] }),
      );
    });

    const panel = await screen.findByTestId("search-facet-panel-status");
    expect(
      within(panel)
        .getAllByRole("button")
        .map((bucket) => bucket.textContent),
    ).toEqual(["draft7", "published3", "null5"]);
    expect(screen.getByTestId("search-facet-bucket-status-null")).toBeDisabled();
    expect(screen.queryByTestId("search-facet-option-metadata")).not.toBeInTheDocument();
  });

  it("hides facet controls for column names that cannot be serialized safely", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 2,
      totalPages: 1,
      items: [{ id: "row-1", status: "published" }],
    });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.posts": {
            schema: "public",
            name: "posts",
            columns: [
              makeColumn({ name: "status", position: 1 }),
              makeColumn({ name: "status,role", position: 2 }),
              makeColumn({ name: "review-status", position: 3 }),
            ],
          },
        })}
      />,
    );

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenCalled();
    });
    expect(screen.getByTestId("search-facet-option-status")).toBeInTheDocument();
    expect(screen.getByTestId("search-facet-option-review-status")).toBeInTheDocument();
    expect(screen.queryByTestId("search-facet-option-status,role")).not.toBeInTheDocument();
  });

  it("clicking a facet bucket rewrites the filter and refetches narrowed results", async () => {
    mockListSearchPlaygroundRecords
      .mockResolvedValueOnce({
        page: 1,
        perPage: 20,
        totalItems: 3,
        totalPages: 1,
        items: [{ id: "row-1", title: "Alpha", status: "published" }],
      })
      .mockResolvedValue({
        page: 1,
        perPage: 20,
        totalItems: 2,
        totalPages: 1,
        items: [
          {
            id: "row-1",
            title: "Alpha",
            status: "owner's pick",
            score: 42,
            active: false,
          },
          {
            id: "row-2",
            title: "Bravo",
            status: "owner's pick",
            score: 42,
            active: false,
          },
        ],
        facets: {
          status: [
            { value: "owner's pick", count: 2 },
            { value: null, count: 1 },
          ],
          score: [{ value: 42, count: 2 }],
          active: [{ value: false, count: 2 }],
          "review-status": [{ value: "owner's pick", count: 2 }],
          and: [{ value: "owner's pick", count: 2 }],
          TRUE: [{ value: "owner's pick", count: 2 }],
        },
      });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.posts": {
            schema: "public",
            name: "posts",
            columns: [
              makeColumn({ name: "id", position: 1, type: "uuid", isPrimaryKey: true }),
              makeColumn({ name: "status", position: 2 }),
              makeColumn({ name: "title", position: 3 }),
              makeColumn({ name: "score", position: 4, type: "numeric", jsonType: "number" }),
              makeColumn({ name: "active", position: 5, type: "boolean", jsonType: "boolean" }),
              makeColumn({ name: "review-status", position: 6 }),
              makeColumn({ name: "and", position: 7 }),
              makeColumn({ name: "TRUE", position: 8 }),
            ],
          },
        })}
      />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Filter expression" }), "title='Legacy'");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await user.click(screen.getByRole("checkbox", { name: "status" }));
    await user.click(screen.getByRole("checkbox", { name: "score" }));
    await user.click(screen.getByRole("checkbox", { name: "active" }));
    await user.click(screen.getByRole("checkbox", { name: "review-status" }));
    await user.click(screen.getByRole("checkbox", { name: "and" }));
    await user.click(screen.getByRole("checkbox", { name: "TRUE" }));
    await screen.findByTestId("search-facet-panel-status");
    await screen.findByTestId("search-facet-panel-score");
    await screen.findByTestId("search-facet-panel-active");
    await screen.findByTestId("search-facet-panel-review-status");
    await screen.findByTestId("search-facet-panel-and");
    await screen.findByTestId("search-facet-panel-TRUE");

    await user.click(screen.getByTestId("search-facet-bucket-status-owner_s_pick"));

    await waitFor(() => {
      expect(screen.getByRole("textbox", { name: "Filter expression" })).toHaveValue("status='owner\\'s pick'");
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({
          facets: ["status", "score", "active", "review-status", "and", "TRUE"],
          filter: "status='owner\\'s pick'",
        }),
      );
    });

    expect(screen.getByTestId("search-facet-bucket-review-status-owner_s_pick")).toBeDisabled();
    expect(screen.getByTestId("search-facet-bucket-and-owner_s_pick")).toBeDisabled();
    expect(screen.getByTestId("search-facet-bucket-TRUE-owner_s_pick")).toBeDisabled();

    const callsBeforeKeywordClick = mockListSearchPlaygroundRecords.mock.calls.length;
    await user.click(screen.getByTestId("search-facet-bucket-and-owner_s_pick"));
    expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(callsBeforeKeywordClick);

    await user.click(screen.getByTestId("search-facet-bucket-score-42"));

    await waitFor(() => {
      expect(screen.getByRole("textbox", { name: "Filter expression" })).toHaveValue("score=42");
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ filter: "score=42" }),
      );
    });

    await user.click(screen.getByTestId("search-facet-bucket-active-false"));

    await waitFor(() => {
      expect(screen.getByRole("textbox", { name: "Filter expression" })).toHaveValue("active=false");
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ filter: "active=false" }),
      );
    });

    const callsBeforeNullClick = mockListSearchPlaygroundRecords.mock.calls.length;
    await user.click(screen.getByTestId("search-facet-bucket-status-null"));
    expect(mockListSearchPlaygroundRecords).toHaveBeenCalledTimes(callsBeforeNullClick);
  });

  it("renders backend error messages including pg_trgm guidance", async () => {
    mockListSearchPlaygroundRecords.mockRejectedValue(
      new Error("fuzzy search is unavailable because pg_trgm is not installed"),
    );

    renderWithProviders(
      <Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />,
    );

    await waitFor(() => {
      expect(
        screen.getByText("fuzzy search is unavailable because pg_trgm is not installed"),
      ).toBeInTheDocument();
    });
  });

  it("resets search and filter when collection changes and refetches", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(
      <Search
        schema={makeSchema({
          "public.posts": { schema: "public", name: "posts" },
          "public.users": { schema: "public", name: "users" },
        })}
      />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "alpha");
    await user.type(screen.getByRole("textbox", { name: "Filter expression" }), "status='active'");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "posts",
        expect.objectContaining({ search: "alpha", filter: "status='active'" }),
      );
    });

    await user.selectOptions(screen.getByLabelText("Collection"), "users");

    await waitFor(() => {
      expect(mockListSearchPlaygroundRecords).toHaveBeenLastCalledWith(
        "users",
        expect.objectContaining({ search: undefined, filter: undefined }),
      );
    });
    expect(screen.getByRole("textbox", { name: "Search query" })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "Filter expression" })).toHaveValue("");
  });

  it("shows explicit no-collections state and skips fetch when schema has no tables", () => {
    renderWithProviders(<Search schema={makeSchema()} />);

    expect(screen.getByText("No collections available for search")).toBeInTheDocument();
    expect(screen.getByText("Create a table first, then come back to run search queries.")).toBeInTheDocument();
    expect(mockListSearchPlaygroundRecords).not.toHaveBeenCalled();
  });

  it("shows search-specific empty-results copy when search has no matches", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 0,
      totalPages: 0,
      items: [],
    });

    renderWithProviders(<Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />);
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Search query" }), "no-match");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(screen.getByText("No results matched this search")).toBeInTheDocument();
    });
    expect(screen.queryByText("No rows in this table yet")).not.toBeInTheDocument();
  });

  it("disables unsupported grid interactions in search results", async () => {
    mockListSearchPlaygroundRecords.mockResolvedValue({
      page: 1,
      perPage: 20,
      totalItems: 2,
      totalPages: 2,
      items: [
        { id: "row-1", title: "Alpha" },
        { id: "row-2", title: "Beta" },
      ],
    });

    renderWithProviders(<Search schema={makeSchema({ "public.posts": { schema: "public", name: "posts" } })} />);

    await waitFor(() => {
      expect(screen.getByText("Alpha")).toBeInTheDocument();
    });

    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();

    const titleHeader = screen.getByRole("columnheader", { name: "title" });
    expect(titleHeader.className).toContain("cursor-default");
    expect(titleHeader.className).not.toContain("cursor-pointer");

    const firstRow = screen.getByText("Alpha").closest("tr");
    expect(firstRow).not.toBeNull();
    expect(firstRow?.className).toContain("cursor-default");
    expect(firstRow?.className).not.toContain("cursor-pointer");
  });
});
