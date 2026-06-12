import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SchemaCache, Table } from "../../types";
import type { View } from "../layout-types";
import { ContentRouter } from "../ContentRouter";

vi.mock("../../api_admin", () => ({
  getCollectionSearchSettings: vi.fn().mockResolvedValue({ attributes: [], customRanking: [] }),
  getCollectionSearchSynonyms: vi.fn().mockResolvedValue({ groups: [] }),
  updateCollectionSearchSettings: vi.fn(),
  updateCollectionSearchSynonyms: vi.fn(),
}));

vi.mock("../TableBrowser", () => ({
  TableBrowser: ({ table }: { table: Table }) => (
    <div data-testid="table-browser">{table.name}</div>
  ),
}));

vi.mock("../SchemaView", () => ({
  SchemaView: ({ table }: { table: Table }) => (
    <div data-testid="schema-view">{table.name}</div>
  ),
}));

vi.mock("../SqlEditor", () => ({
  SqlEditor: () => <div data-testid="sql-editor" />,
}));

function makeTable(overrides: Partial<Table> = {}): Table {
  return {
    schema: "public",
    name: "books",
    kind: "table",
    columns: [],
    primaryKey: [],
    ...overrides,
  };
}

function makeSchema(selected = makeTable()): SchemaCache {
  return {
    schemas: [selected.schema],
    builtAt: "2026-06-04T00:00:00Z",
    tables: {
      [`${selected.schema}.${selected.name}`]: selected,
    },
  };
}

function renderSelectedRouter(view: View = "data") {
  const selected = makeTable();
  const onSetView = vi.fn();
  const onRefresh = vi.fn();
  render(
    <ContentRouter
      schema={makeSchema(selected)}
      view={view}
      isAdminView={false}
      selected={selected}
      onRefresh={onRefresh}
      onSetView={onSetView}
      onSelectAdminView={vi.fn()}
    />,
  );
  return { onSetView, onRefresh };
}

describe("ContentRouter selected table views", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the Synonyms tab and uses the existing onSetView path", async () => {
    const user = userEvent.setup();
    const { onSetView } = renderSelectedRouter();

    expect(screen.getByRole("button", { name: "Data" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Schema" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "SQL" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Search Settings" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Synonyms" }));

    expect(onSetView).toHaveBeenCalledWith("synonyms");
  });

  it("renders the Search Settings tab and uses the existing onSetView path", async () => {
    const user = userEvent.setup();
    const { onSetView } = renderSelectedRouter();

    await user.click(screen.getByRole("button", { name: "Search Settings" }));

    expect(onSetView).toHaveBeenCalledWith("search-settings");
  });

  it("mounts the dedicated synonyms editor for the synonyms view", async () => {
    renderSelectedRouter("synonyms");

    expect(
      await screen.findByRole("heading", { name: "Search synonyms for books" }),
    ).toBeInTheDocument();
  });

  it("mounts the dedicated search settings editor for the search settings view", async () => {
    renderSelectedRouter("search-settings");

    expect(
      await screen.findByRole("heading", { name: "Search settings for books" }),
    ).toBeInTheDocument();
  });

  it("keeps Data, Schema, and SQL routed to their existing owners", () => {
    renderSelectedRouter("data");
    expect(screen.getByTestId("table-browser")).toHaveTextContent("books");

    renderSelectedRouter("schema");
    expect(screen.getByTestId("schema-view")).toHaveTextContent("books");

    renderSelectedRouter("sql");
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();
  });
});
