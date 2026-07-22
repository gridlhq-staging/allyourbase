import type { SchemaCache, Table } from "../types";
import { TableBrowser } from "./TableBrowser";
import { SchemaView } from "./SchemaView";
import { SearchSettingsEditor } from "./SearchSettingsEditor";
import { SynonymsEditor } from "./SynonymsEditor";
import type { AdminView, View } from "./layout-types";
import { Code, Columns3, SlidersHorizontal, Tags, Table as TableIcon, TableProperties } from "lucide-react";
import { cn } from "../lib/utils";
import { findAdminScreen, SCREEN_REGISTRY, type ScreenProps, type ScreenRegistry } from "../screens/registry";
import { ErrorNotice } from "./ErrorNotice";

const CONTENT_ROUTER_MAIN_CLASS = "flex-1 flex flex-col overflow-hidden bg-gray-50 dark:bg-gray-950";
const VIEW_TOGGLE_BUTTON_CLASS = "px-3 py-1 text-xs rounded font-medium transition-colors";
const VIEW_TOGGLE_ACTIVE_CLASS = "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100";
const VIEW_TOGGLE_INACTIVE_CLASS = "text-gray-600 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200";

interface ContentRouterProps {
  schema: SchemaCache;
  view: View;
  isAdminView: boolean;
  selected: Table | null;
  onRefresh: () => void | Promise<void>;
  onSetView: (view: View) => void;
  onSelectAdminView: (view: AdminView) => void;
  screenRegistry?: ScreenRegistry;
  routeFailure?: string | null;
  onReturnToBase?: () => void;
}

interface TableViewToggleButtonProps {
  active: boolean;
  icon: typeof TableIcon;
  label: string;
  onClick: () => void;
}

type BaseScreenProps = Omit<ScreenProps, "screenLabel">;

function TableViewToggleButton({
  active,
  icon: Icon,
  label,
  onClick,
}: TableViewToggleButtonProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        VIEW_TOGGLE_BUTTON_CLASS,
        active ? VIEW_TOGGLE_ACTIVE_CLASS : VIEW_TOGGLE_INACTIVE_CLASS,
      )}
    >
      <Icon className="w-3.5 h-3.5 inline mr-1" />
      {label}
    </button>
  );
}

function renderAdminContent(
  view: View,
  props: BaseScreenProps,
  screenRegistry: ScreenRegistry,
) {
  const screen = findAdminScreen(view, screenRegistry);
  return screen?.render({ ...props, screenLabel: screen.label });
}

function renderSelectedContent(
  view: View,
  selected: Table,
  schema: SchemaCache,
  onRefresh: () => void | Promise<void>,
  screenRegistry: ScreenRegistry,
) {
  switch (view) {
    case "schema":
      return <SchemaView table={selected} />;
    case "sql":
      {
        const sqlScreen = findAdminScreen("sql-editor", screenRegistry);
        return sqlScreen?.render({ schema, onRefresh, screenLabel: sqlScreen.label });
      }
    case "synonyms":
      return <SynonymsEditor selected={selected} schema={schema} />;
    case "search-settings":
      return <SearchSettingsEditor selected={selected} schema={schema} />;
    case "data":
    default:
      return <TableBrowser table={selected} />;
  }
}

export function ContentRouter({
  schema,
  view,
  isAdminView,
  selected,
  onRefresh,
  onSetView,
  screenRegistry = SCREEN_REGISTRY,
  routeFailure = null,
  onReturnToBase,
}: ContentRouterProps) {
  if (routeFailure) {
    return (
      <main className={CONTENT_ROUTER_MAIN_CLASS}>
        <div className="flex flex-1 items-center justify-center">
          <ErrorNotice
            message={routeFailure}
            docsPath="/guide/getting-started"
            actionLabel="Return to console"
            onAction={onReturnToBase}
            variant="page"
          />
        </div>
      </main>
    );
  }
  if (isAdminView) {
    return (
      <main className={CONTENT_ROUTER_MAIN_CLASS}>
        <div className="flex-1 overflow-auto">
          {renderAdminContent(view, { schema, onRefresh }, screenRegistry)}
        </div>
      </main>
    );
  }

  if (selected) {
    return (
      <main className={CONTENT_ROUTER_MAIN_CLASS}>
        <header className="border-b border-gray-200 dark:border-gray-700 px-6 py-3 flex items-center gap-4">
          <h1 className="font-semibold text-gray-900 dark:text-gray-100">
            {selected.schema !== "public" && (
              <span className="text-gray-600 dark:text-gray-400">{selected.schema}.</span>
            )}
            {selected.name}
          </h1>
          <span className="text-xs text-gray-600 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded px-2 py-0.5">
            {selected.kind}
          </span>

          <div className="ml-auto flex gap-1 bg-gray-100 dark:bg-gray-800 rounded p-0.5">
            <TableViewToggleButton
              active={view === "data"}
              icon={TableIcon}
              label="Data"
              onClick={() => onSetView("data")}
            />
            <TableViewToggleButton
              active={view === "schema"}
              icon={Columns3}
              label="Schema"
              onClick={() => onSetView("schema")}
            />
            <TableViewToggleButton
              active={view === "sql"}
              icon={Code}
              label="SQL"
              onClick={() => onSetView("sql")}
            />
            <TableViewToggleButton
              active={view === "synonyms"}
              icon={Tags}
              label="Synonyms"
              onClick={() => onSetView("synonyms")}
            />
            <TableViewToggleButton
              active={view === "search-settings"}
              icon={SlidersHorizontal}
              label="Search Settings"
              onClick={() => onSetView("search-settings")}
            />
          </div>
        </header>

        <div className="flex-1 overflow-auto">
          {renderSelectedContent(view, selected, schema, onRefresh, screenRegistry)}
        </div>
      </main>
    );
  }

  return (
    <main className={CONTENT_ROUTER_MAIN_CLASS}>
      <div className="flex-1 flex flex-col items-center justify-center text-gray-500 dark:text-gray-400">
        <TableProperties className="w-12 h-12 text-gray-300 dark:text-gray-700 mb-3" />
        <p className="text-sm mb-1">Select a table from the sidebar</p>
        <p className="text-xs text-gray-600 dark:text-gray-400">
          Use SQL Editor from the sidebar to create one.
        </p>
      </div>
    </main>
  );
}
