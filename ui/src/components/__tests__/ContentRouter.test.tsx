import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SchemaCache, Table } from "../../types";
import type { View } from "../layout-types";
import {
  ALGOLIA_MIGRATION_GUIDE_PATH,
  docsUrl,
  MIGRATIONS_GUIDE_PATH,
  SUPABASE_MIGRATION_GUIDE_PATH,
} from "../../lib/docs_url";
import { ContentRouter } from "../ContentRouter";
import {
  filterScreenRegistry,
  type AdminScreen,
  type ScreenRegistry,
} from "../../screens/registry";

vi.mock("../../api_admin", () => ({
  getCollectionSearchSettings: vi.fn().mockResolvedValue({ attributes: [], customRanking: [] }),
  getCollectionSearchSynonyms: vi.fn().mockResolvedValue({ groups: [] }),
  updateCollectionSearchSettings: vi.fn(),
  updateCollectionSearchSynonyms: vi.fn(),
}));

vi.mock("../TableBrowser", () => ({
  TableBrowser: ({
    table,
    onOpenSQLEditor,
  }: {
    table: Table;
    onOpenSQLEditor?: () => void;
  }) => (
    <div data-testid="table-browser">
      <span>{table.name}</span>
      <button type="button" onClick={onOpenSQLEditor}>
        Mock open SQL editor
      </button>
    </div>
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

function renderSelectedRouter(
  view: View = "data",
  screenRegistry?: ScreenRegistry,
) {
  const selected = makeTable();
  const onSetView = vi.fn();
  const onRefresh = vi.fn();
  const schema = makeSchema(selected);
  render(
    <ContentRouter
      schema={schema}
      view={view}
      isAdminView={false}
      selected={selected}
      onRefresh={onRefresh}
      onSetView={onSetView}
      onSelectAdminView={vi.fn()}
      screenRegistry={screenRegistry}
    />,
  );
  return { onSetView, onRefresh, schema, selected };
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

function registryWithSqlScreen(sqlScreen: AdminScreen): ScreenRegistry {
  return {
    sections: [
      {
        title: "Database",
        screens: [sqlScreen],
      },
    ],
  };
}

describe("ContentRouter admin views", () => {
  it("passes the resolved registry label to normal admin screen renders", () => {
    const renderScreen = vi.fn(({ screenLabel }) => (
      <div data-testid="registry-label-probe">{screenLabel}</div>
    ));
    const registry = {
      sections: [
        {
          title: "Admin",
          screens: [
            {
              id: "usage",
              label: "Distinct Registry Usage Label",
              icon: {} as never,
              render: renderScreen,
            },
          ],
        },
      ],
    } satisfies ScreenRegistry;

    render(
      <ContentRouter
        schema={makeSchema()}
        view="usage"
        isAdminView={true}
        selected={null}
        onRefresh={vi.fn()}
        onSetView={vi.fn()}
        onSelectAdminView={vi.fn()}
        screenRegistry={registry}
      />,
    );

    expect(screen.getByTestId("registry-label-probe")).toHaveTextContent(
      "Distinct Registry Usage Label",
    );
    expect(renderScreen).toHaveBeenCalledWith(
      expect.objectContaining({ screenLabel: "Distinct Registry Usage Label" }),
    );
  });

  it("routes known owners without a silent fallback", async () => {
    const { rerender } = renderAdminRouter("sql-editor");
    expect(await screen.findByTestId("sql-editor")).toBeInTheDocument();

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
      expect(await screen.findByTestId(ownerTestId)).toBeInTheDocument();
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
    expect(screen.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      docsUrl("/guide/getting-started"),
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Return to console" }));
    expect(onReturnToBase).toHaveBeenCalledOnce();
  });
});

describe("ContentRouter empty data state", () => {
  it("keeps table selection guidance and offers CLI migration guides", () => {
    render(
      <ContentRouter
        schema={makeSchema()}
        view="data"
        isAdminView={false}
        selected={null}
        onRefresh={vi.fn()}
        onSetView={vi.fn()}
        onSelectAdminView={vi.fn()}
      />,
    );

    expect(screen.getByText("Select a table from the sidebar")).toBeInTheDocument();
    expect(
      screen.getByText("Use SQL Editor from the sidebar to create one."),
    ).toBeInTheDocument();
    expect(screen.getByText("Migrating from another source?")).toBeInTheDocument();
    expect(screen.getByText("ayb migrate <source> --help")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Migration guide" })).toHaveAttribute(
      "href",
      docsUrl(MIGRATIONS_GUIDE_PATH),
    );
    expect(screen.getByRole("link", { name: "Supabase migration guide" })).toHaveAttribute(
      "href",
      docsUrl(SUPABASE_MIGRATION_GUIDE_PATH),
    );
    expect(screen.getByRole("link", { name: "Algolia migration guide" })).toHaveAttribute(
      "href",
      docsUrl(ALGOLIA_MIGRATION_GUIDE_PATH),
    );
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

  it("keeps Data, Schema, and SQL routed to their existing owners", async () => {
    renderSelectedRouter("data");
    expect(screen.getByTestId("table-browser")).toHaveTextContent("books");

    renderSelectedRouter("schema");
    expect(screen.getByTestId("schema-view")).toHaveTextContent("books");

    renderSelectedRouter("sql");
    expect(await screen.findByTestId("sql-editor")).toBeInTheDocument();
  });

  it("passes the SQL editor callback through the selected table browser seam", async () => {
    const user = userEvent.setup();
    const { onSetView } = renderSelectedRouter("data");

    await user.click(screen.getByRole("button", { name: "Mock open SQL editor" }));

    expect(onSetView).toHaveBeenCalledWith("sql");
  });

  it("routes the selected-table SQL view through the registry sql-editor screen", async () => {
    const sqlRender = vi.fn(({ onRefresh, screenLabel }) => (
      <button data-testid="registry-sql-editor" onClick={onRefresh}>
        {screenLabel}
      </button>
    ));
    const sqlScreen = {
      id: "sql-editor",
      label: "Distinct SQL Label",
      icon: {} as never,
      render: sqlRender,
    } satisfies AdminScreen;
    const user = userEvent.setup();

    const { onRefresh, schema } = renderSelectedRouter(
      "sql",
      registryWithSqlScreen(sqlScreen),
    );

    expect(await screen.findByTestId("registry-sql-editor")).toBeInTheDocument();
    expect(screen.getByTestId("registry-sql-editor")).toHaveTextContent("Distinct SQL Label");
    expect(screen.queryByTestId("sql-editor")).not.toBeInTheDocument();
    expect(sqlRender).toHaveBeenCalledWith(
      expect.objectContaining({ schema, onRefresh, screenLabel: "Distinct SQL Label" }),
    );

    await user.click(screen.getByTestId("registry-sql-editor"));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
