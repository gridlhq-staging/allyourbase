import { useState, useCallback, useEffect, useMemo } from "react";
import type { SchemaCache, Table } from "../types";
import { CommandPalette } from "./CommandPalette";
import type { CommandAction } from "./CommandPalette";
import { useTheme } from "./ThemeProvider";
import { Sidebar } from "./Sidebar";
import { ContentRouter } from "./ContentRouter";
import {
  type View,
  type AdminView,
} from "./layout-types";
import { filterScreenRegistry, SCREEN_REGISTRY } from "../screens/registry";
import { useCapability } from "../capabilities";
import {
  formatDashboardRoute,
  parseDashboardRoute,
  readDashboardAdminBase,
  type DashboardRoute,
  type DashboardRouteTarget,
} from "./dashboard_url_routing";

interface LayoutProps {
  schema: SchemaCache;
  onLogout: () => void;
  onRefresh: () => void | Promise<void>;
}

export function Layout({ schema, onLogout, onRefresh }: LayoutProps) {
  const tables = useMemo(() => Object.values(schema.tables).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  ), [schema.tables]);
  const adminBase = useMemo(() => readDashboardAdminBase(), []);
  const [tableView, setTableView] = useState<View>("data");
  const [cmdOpen, setCmdOpen] = useState(false);
  const { theme, toggleTheme } = useTheme();
  const { canUse } = useCapability();
  const screenRegistry = useMemo(
    () => filterScreenRegistry(SCREEN_REGISTRY, canUse),
    [canUse],
  );
  const [activeRoute, setActiveRoute] = useState<DashboardRoute>(() => (
    parseDashboardRoute(window.location, { adminBase, tables, screenRegistry }).route
  ));

  const applyCurrentRoute = useCallback(() => {
    const parsed = parseDashboardRoute(window.location, {
      adminBase,
      tables,
      screenRegistry,
    });
    if (parsed.historyAction === "replace") {
      window.history.replaceState(
        window.history.state,
        "",
        `${parsed.location.pathname}${parsed.location.search}${parsed.location.hash}`,
      );
    }
    if (parsed.route.kind === "table") {
      setTableView("data");
    }
    setActiveRoute(parsed.route);
  }, [adminBase, screenRegistry, tables]);

  const navigate = useCallback((target: DashboardRouteTarget) => {
    const next = formatDashboardRoute(target, {
      adminBase,
      search: window.location.search,
      hash: window.location.hash,
    });
    window.history.pushState(
      null,
      "",
      `${next.pathname}${next.search}${next.hash}`,
    );
    applyCurrentRoute();
  }, [adminBase, applyCurrentRoute]);

  useEffect(() => {
    applyCurrentRoute();
    window.addEventListener("popstate", applyCurrentRoute);
    return () => window.removeEventListener("popstate", applyCurrentRoute);
  }, [applyCurrentRoute]);

  const selected = activeRoute.kind === "table"
    ? tables.find((table) => table.schema === activeRoute.schema && table.name === activeRoute.table) ?? null
    : activeRoute.kind === "base" ? tables[0] ?? null : null;
  const view: View = activeRoute.kind === "screen" ? activeRoute.screen.id : tableView;
  const adminViewActive = activeRoute.kind === "screen";
  const routeFailure = "label" in activeRoute ? activeRoute.label : null;

  const handleSelect = useCallback((table: Table) => {
    navigate({ kind: "table", schema: table.schema, table: table.name });
  }, [navigate]);

  const handleAdminView = useCallback((nextView: AdminView) => {
    navigate({ kind: "screen", screenId: nextView });
  }, [navigate]);

  const handleCommand = useCallback(
    (action: CommandAction) => {
      if (action.kind === "table") {
        handleSelect(action.table);
      } else {
        handleAdminView(action.view as AdminView);
      }
    },
    [handleSelect, handleAdminView],
  );

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCmdOpen((current) => !current);
      }
    };

    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const themeToggleLabel =
    theme === "dark" ? "Switch to light mode" : "Switch to dark mode";

  return (
    <div className="flex h-screen bg-gray-50 text-gray-900 dark:bg-gray-950 dark:text-gray-100">
      <CommandPalette
        open={cmdOpen}
        onClose={() => setCmdOpen(false)}
        onSelect={handleCommand}
        tables={tables}
        screenRegistry={screenRegistry}
      />

      <Sidebar
        tables={tables}
        selected={selected}
        view={view}
        isAdminView={adminViewActive}
        onSelectTable={handleSelect}
        onSelectAdminView={handleAdminView}
        onOpenCommandPalette={() => setCmdOpen(true)}
        onRefresh={onRefresh}
        onToggleTheme={toggleTheme}
        onLogout={onLogout}
        theme={theme as "dark" | "light"}
        themeToggleLabel={themeToggleLabel}
        screenRegistry={screenRegistry}
      />

      <ContentRouter
        schema={schema}
        view={view}
        isAdminView={adminViewActive}
        selected={selected}
        onRefresh={onRefresh}
        onSetView={setTableView}
        onSelectAdminView={handleAdminView}
        screenRegistry={screenRegistry}
        routeFailure={routeFailure}
        onReturnToBase={() => navigate({ kind: "base" })}
      />
    </div>
  );
}
