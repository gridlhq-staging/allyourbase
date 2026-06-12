import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CollectionSearchSettingsResponse } from "../../api_admin";
import {
  getCollectionSearchSettings,
  updateCollectionSearchSettings,
} from "../../api_admin";
import type { Column, SchemaCache, Table } from "../../types";
import { SearchSettingsEditor } from "../SearchSettingsEditor";

vi.mock("../../api_admin", () => ({
  getCollectionSearchSettings: vi.fn(),
  updateCollectionSearchSettings: vi.fn(),
}));

const mockGetSearchSettings = vi.mocked(getCollectionSearchSettings);
const mockUpdateSearchSettings = vi.mocked(updateCollectionSearchSettings);

function makeColumn(overrides: Partial<Column> = {}): Column {
  return {
    name: "title",
    position: 1,
    type: "text",
    nullable: false,
    isPrimaryKey: false,
    jsonType: "",
    ...overrides,
  };
}

function makeTable(overrides: Partial<Table> = {}): Table {
  return {
    schema: "public",
    name: "books",
    kind: "table",
    columns: [
      makeColumn({ name: "title", position: 1 }),
      makeColumn({ name: "summary", position: 2 }),
      makeColumn({ name: "description", position: 3 }),
      makeColumn({ name: "notes", position: 4 }),
      makeColumn({ name: "published_at", position: 5, type: "timestamptz", jsonType: "string" }),
      makeColumn({ name: "page_count", position: 6, type: "integer", jsonType: "integer" }),
      makeColumn({ name: "location", position: 7, type: "geometry(Point,4326)", jsonType: "object" }),
      makeColumn({ name: "metadata", position: 8, type: "jsonb", jsonType: "object" }),
      makeColumn({ name: "tags", position: 9, type: "text[]", jsonType: "array" }),
      makeColumn({ name: "status", position: 10, type: "book_status", jsonType: "string", enumValues: ["draft", "published"] }),
    ],
    primaryKey: [],
    ...overrides,
  };
}

function makeSchema(tables: Table[] = [makeTable()]): SchemaCache {
  return {
    schemas: [...new Set(tables.map((table) => table.schema))],
    builtAt: "2026-06-04T00:00:00Z",
    tables: Object.fromEntries(
      tables.map((table) => [`${table.schema}.${table.name}`, table]),
    ),
  };
}

function deferredResponse<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function renderEditor(
  response: CollectionSearchSettingsResponse | Promise<CollectionSearchSettingsResponse>,
  table = makeTable(),
  schema = makeSchema([table]),
) {
  mockGetSearchSettings.mockReturnValueOnce(Promise.resolve(response));
  return render(<SearchSettingsEditor selected={table} schema={schema} />);
}

async function renderLoadedEditor(
  response: CollectionSearchSettingsResponse = {
    attributes: [
      { column: "title", weight: "high" },
      { column: "summary", weight: "medium" },
    ],
    customRanking: [{ column: "published_at", order: "desc" }],
  },
  table = makeTable(),
  schema = makeSchema([table]),
) {
  renderEditor(response, table, schema);
  await screen.findByRole("heading", { name: "Search settings for books" });
}

function setFieldValue(field: HTMLElement, value: string) {
  fireEvent.change(field, { target: { value } });
}

describe("SearchSettingsEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads and renders seeded search attributes for the selected public collection", async () => {
    const response = deferredResponse<CollectionSearchSettingsResponse>();
    mockGetSearchSettings.mockReturnValueOnce(response.promise);

    render(<SearchSettingsEditor selected={makeTable()} schema={makeSchema()} />);

    expect(mockGetSearchSettings).toHaveBeenCalledWith("books");
    expect(screen.getByText("Loading search settings...")).toBeInTheDocument();

    response.resolve({
      attributes: [
        { column: "title", weight: "high" },
        { column: "summary", weight: "medium" },
        { column: "description", weight: "low" },
        { column: "notes", weight: "lowest" },
      ],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    expect(
      await screen.findByRole("heading", { name: "Search settings for books" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Weight for title" })).toHaveValue("high");
    expect(screen.getByRole("combobox", { name: "Weight for summary" })).toHaveValue("medium");
    expect(screen.getByRole("combobox", { name: "Weight for description" })).toHaveValue("low");
    expect(screen.getByRole("combobox", { name: "Weight for notes" })).toHaveValue("lowest");
    expect(screen.getByRole("combobox", { name: "Ranking order for row 1" })).toHaveValue(
      "desc",
    );
  });

  it("supports add, edit, and remove flows with semantic weight options", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();

    await user.selectOptions(screen.getByRole("combobox", { name: "Weight for summary" }), "low");
    await user.click(screen.getByRole("button", { name: "Add attribute" }));

    const thirdAttribute = screen.getByRole("group", { name: "Searchable attribute 3" });
    await user.selectOptions(
      within(thirdAttribute).getByRole("combobox", { name: "Column for attribute 3" }),
      "description",
    );
    const thirdWeight = within(thirdAttribute).getByRole("combobox", {
      name: "Weight for description",
    });
    expect(within(thirdWeight).getByRole("option", { name: "High" })).toHaveValue("high");
    expect(within(thirdWeight).getByRole("option", { name: "Medium" })).toHaveValue("medium");
    expect(within(thirdWeight).getByRole("option", { name: "Low" })).toHaveValue("low");
    expect(within(thirdWeight).getByRole("option", { name: "Lowest" })).toHaveValue("lowest");
    await user.selectOptions(thirdWeight, "lowest");

    await user.click(screen.getByRole("button", { name: "Remove attribute title" }));

    expect(screen.queryByRole("combobox", { name: "Weight for title" })).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Weight for summary" })).toHaveValue("low");
    expect(screen.getByRole("combobox", { name: "Weight for description" })).toHaveValue(
      "lowest",
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("excludes non-text and non-searchable columns from attribute selectors", async () => {
    await renderLoadedEditor();

    const firstColumnSelect = screen.getByRole("combobox", { name: "Column for attribute 1" });
    expect(within(firstColumnSelect).getByRole("option", { name: "title" })).toHaveValue("title");
    expect(within(firstColumnSelect).getByRole("option", { name: "summary" })).toHaveValue(
      "summary",
    );
    expect(
      within(firstColumnSelect).queryByRole("option", { name: "published_at" }),
    ).not.toBeInTheDocument();
  });

  it("blocks duplicate searchable columns before save and preserves the invalid draft", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Column for attribute 2" }),
      "title",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Searchable attributes cannot repeat the same column.")).toBeInTheDocument();
    expect(screen.getAllByRole("combobox", { name: "Column for attribute 1" })[0]).toHaveValue(
      "title",
    );
    expect(mockUpdateSearchSettings).not.toHaveBeenCalled();
  });

  it("preserves the draft and shows backend save errors", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();
    mockUpdateSearchSettings.mockRejectedValueOnce(new Error("search settings rejected"));

    await user.selectOptions(screen.getByRole("combobox", { name: "Weight for summary" }), "low");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("search settings rejected")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Weight for summary" })).toHaveValue("low");
    expect(mockUpdateSearchSettings).toHaveBeenCalledTimes(1);
  });

  it("sends the trimmed full replacement payload and shows successful save status", async () => {
    const user = userEvent.setup();
    const customRanking = [{ column: "published_at", order: "desc" }] as const;
    await renderLoadedEditor({
      attributes: [
        { column: " title ", weight: "high" },
        { column: "summary", weight: "medium" },
      ],
      customRanking: [...customRanking],
    });
    mockUpdateSearchSettings.mockResolvedValueOnce({
      attributes: [
        { column: "title", weight: "high" },
        { column: "description", weight: "low" },
      ],
      customRanking: [...customRanking],
    });

    setFieldValue(screen.getByRole("combobox", { name: "Column for attribute 2" }), "description");
    await user.selectOptions(screen.getByRole("combobox", { name: "Weight for description" }), "low");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateSearchSettings).toHaveBeenCalledWith("books", {
        attributes: [
          { column: "title", weight: "high" },
          { column: "description", weight: "low" },
        ],
        customRanking: [{ column: "published_at", order: "desc" }],
      });
    });
    expect(await screen.findByRole("status")).toHaveTextContent("Saved search settings.");
    expect(screen.getByRole("combobox", { name: "Weight for description" })).toHaveValue("low");
  });

  it("renders non-public duplicate-name selections as read-only without GET or PUT calls", () => {
    const selected = makeTable({ schema: "private", name: "books" });
    const schema = makeSchema([
      makeTable({ schema: "public", name: "books" }),
      selected,
    ]);

    render(<SearchSettingsEditor selected={selected} schema={schema} />);

    expect(
      screen.getByText(/unqualified admin route cannot safely target private\.books/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add attribute" })).not.toBeInTheDocument();
    expect(mockGetSearchSettings).not.toHaveBeenCalled();
    expect(mockUpdateSearchSettings).not.toHaveBeenCalled();
  });

  it("loads custom-ranking rows in order with exact column and order combobox values", async () => {
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "page_count", order: "asc" },
      ],
    });

    const rankingRows = screen.getAllByRole("group", { name: /^Custom ranking row \d+$/ });
    expect(rankingRows).toHaveLength(2);

    const firstRow = rankingRows[0];
    expect(within(firstRow).getByRole("combobox", { name: "Ranking column for row 1" })).toHaveValue("published_at");
    expect(within(firstRow).getByRole("combobox", { name: "Ranking order for row 1" })).toHaveValue("desc");

    const secondRow = rankingRows[1];
    expect(within(secondRow).getByRole("combobox", { name: "Ranking column for row 2" })).toHaveValue("page_count");
    expect(within(secondRow).getByRole("combobox", { name: "Ranking order for row 2" })).toHaveValue("asc");
  });

  it("includes rankable non-text columns and excludes JSON, array, and enum columns from ranking selectors", async () => {
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    const rankingColumnSelect = screen.getByRole("combobox", { name: "Ranking column for row 1" });
    expect(within(rankingColumnSelect).getByRole("option", { name: "published_at" })).toBeInTheDocument();
    expect(within(rankingColumnSelect).getByRole("option", { name: "page_count" })).toBeInTheDocument();
    expect(within(rankingColumnSelect).getByRole("option", { name: "location" })).toBeInTheDocument();
    expect(within(rankingColumnSelect).getByRole("option", { name: "title" })).toBeInTheDocument();
    expect(within(rankingColumnSelect).queryByRole("option", { name: "metadata" })).not.toBeInTheDocument();
    expect(within(rankingColumnSelect).queryByRole("option", { name: "tags" })).not.toBeInTheDocument();
    expect(within(rankingColumnSelect).queryByRole("option", { name: "status" })).not.toBeInTheDocument();
  });

  it("supports adding, editing, and removing custom ranking rows", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    await user.click(screen.getByRole("button", { name: "Add ranking" }));
    const rankingRows = screen.getAllByRole("group", { name: /^Custom ranking row \d+$/ });
    expect(rankingRows).toHaveLength(2);

    const secondRow = rankingRows[1];
    await user.selectOptions(
      within(secondRow).getByRole("combobox", { name: "Ranking column for row 2" }),
      "page_count",
    );
    await user.selectOptions(
      within(secondRow).getByRole("combobox", { name: "Ranking order for row 2" }),
      "asc",
    );

    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();

    await user.click(within(rankingRows[0]).getByRole("button", { name: /Remove ranking/ }));
    const remainingRows = screen.getAllByRole("group", { name: /^Custom ranking row \d+$/ });
    expect(remainingRows).toHaveLength(1);
    expect(
      within(remainingRows[0]).getByRole("combobox", { name: "Ranking column for row 1" }),
    ).toHaveValue("page_count");
  });

  it("validates duplicate ranking columns before save", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    await user.click(screen.getByRole("button", { name: "Add ranking" }));
    const rankingRows = screen.getAllByRole("group", { name: /^Custom ranking row \d+$/ });
    await user.selectOptions(
      within(rankingRows[1]).getByRole("combobox", { name: "Ranking column for row 2" }),
      "published_at",
    );

    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByText("Custom ranking cannot repeat the same column.")).toBeInTheDocument();
    expect(mockUpdateSearchSettings).not.toHaveBeenCalled();
  });

  it("marks save dirty for ranking-only changes", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Ranking order for row 1" }),
      "asc",
    );

    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("sends full replacement payload with ordered customRanking entries on save", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });
    mockUpdateSearchSettings.mockResolvedValueOnce({
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "page_count", order: "asc" },
      ],
    });

    await user.click(screen.getByRole("button", { name: "Add ranking" }));
    const rankingRows = screen.getAllByRole("group", { name: /^Custom ranking row \d+$/ });
    await user.selectOptions(
      within(rankingRows[1]).getByRole("combobox", { name: "Ranking column for row 2" }),
      "page_count",
    );
    await user.selectOptions(
      within(rankingRows[1]).getByRole("combobox", { name: "Ranking order for row 2" }),
      "asc",
    );

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateSearchSettings).toHaveBeenCalledWith("books", {
        attributes: [{ column: "title", weight: "high" }],
        customRanking: [
          { column: "published_at", order: "desc" },
          { column: "page_count", order: "asc" },
        ],
      });
    });
    expect(await screen.findByRole("status")).toHaveTextContent("Saved search settings.");
  });
});
