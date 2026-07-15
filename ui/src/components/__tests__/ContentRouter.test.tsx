import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SchemaCache, Table } from "../../types";
import type { View } from "../layout-types";
import { ContentRouter } from "../ContentRouter";
import {
  filterScreenRegistry,
  type ScreenRegistry,
} from "../../screens/registry";

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

vi.mock("../Users", () => ({
  Users: () => <div data-testid="users" />,
}));

vi.mock("../SchemaDesigner", () => ({
  SchemaDesigner: () => <div data-testid="schema-designer" />,
}));

vi.mock("../MFAEnrollment", () => ({
  MFAEnrollment: () => <div data-testid="mfa-management" />,
}));

vi.mock("../Tenants", () => ({
  Tenants: () => <div data-testid="tenants" />,
}));

vi.mock("../Organizations", () => ({
  Organizations: () => <div data-testid="organizations" />,
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

function renderAdminRouter(view: View) {
  return render(
    <ContentRouter
      schema={makeSchema()}
      view={view}
      isAdminView={true}
      selected={null}
      onRefresh={vi.fn()}
      onSetView={vi.fn()}
      onSelectAdminView={vi.fn()}
    />,
  );
}

const gatedRegistry = {
  sections: [
    {
      title: "Admin",
      screens: [
        {
          id: "users",
          label: "Users",
          icon: {} as never,
          render: () => <div data-testid="fixture-users" />,
        },
        {
          id: "storage",
          label: "Storage",
          icon: {} as never,
          requires: "storage",
          render: () => <div data-testid="fixture-storage" />,
        },
      ],
    },
  ],
} satisfies ScreenRegistry;

describe("ContentRouter admin views", () => {
  it("routes known owners without a silent fallback", () => {
    const { rerender } = renderAdminRouter("sql-editor");
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();

    for (const [view, ownerTestId] of [
      ["schema-designer", "schema-designer"],
      ["mfa-management", "mfa-management"],
      ["tenants", "tenants"],
      ["organizations", "organizations"],
    ] as const) {
      rerender(
        <ContentRouter
          schema={makeSchema()}
          view={view as View}
          isAdminView={true}
          selected={null}
          onRefresh={vi.fn()}
          onSetView={vi.fn()}
          onSelectAdminView={vi.fn()}
        />,
      );
      expect(screen.getByTestId(ownerTestId)).toBeInTheDocument();
    }
  });

  it("does not render a capability-filtered admin screen and preserves visible fallback", () => {
    const filteredRegistry = filterScreenRegistry(
      gatedRegistry,
      (capability) => capability !== "storage",
    );

    render(
      <ContentRouter
        schema={makeSchema()}
        view={"storage"}
        isAdminView={true}
        selected={null}
        onRefresh={vi.fn()}
        onSetView={vi.fn()}
        onSelectAdminView={vi.fn()}
        screenRegistry={filteredRegistry}
      />,
    );

    expect(screen.queryByTestId("fixture-storage")).not.toBeInTheDocument();
    expect(screen.queryByTestId("fixture-users")).not.toBeInTheDocument();
  });

  it.each([
    "Invalid console link",
    "Screen not found",
    "Table not found",
    "Screen unavailable",
    "Page not found",
  ])("renders the closed route failure %s with a return action", async (label) => {
    const onReturnToBase = vi.fn();
    render(
      <ContentRouter
        schema={makeSchema()}
        view="data"
        isAdminView={false}
        selected={null}
        onRefresh={vi.fn()}
        onSetView={vi.fn()}
        onSelectAdminView={vi.fn()}
        routeFailure={label}
        onReturnToBase={onReturnToBase}
      />,
    );
    expect(screen.getByRole("heading", { name: label })).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Return to console" }));
    expect(onReturnToBase).toHaveBeenCalledOnce();
  });
});

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
