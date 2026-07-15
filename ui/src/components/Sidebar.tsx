import type { Table } from "../types";
import { CommandPaletteHint } from "./CommandPalette";
import type { AdminView, View } from "./layout-types";
import { SCREEN_REGISTRY, type AdminScreen, type ScreenRegistry } from "../screens/registry";
import {
  Table as TableIcon,
  LogOut,
  Moon,
  RefreshCw,
  Sun,
  Plus,
  TableProperties,
} from "lucide-react";
import { cn } from "../lib/utils";

const SIDEBAR_ICON_CLASS = "w-3.5 h-3.5 text-gray-400 dark:text-gray-500 shrink-0";
const SIDEBAR_ITEM_BASE_CLASS = "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 rounded text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800";
const SIDEBAR_ITEM_ACTIVE_CLASS = "bg-gray-100 dark:bg-gray-800 font-medium text-gray-900 dark:text-gray-100";
const SIDEBAR_SECTION_CLASS = "mt-3 pt-3 border-t border-gray-200 dark:border-gray-700 mx-3";
const SIDEBAR_SECTION_TITLE_CLASS = "px-1 pb-1 text-[10px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider";
const SIDEBAR_ACTION_BUTTON_CLASS = "p-2 text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60";

function sidebarItemClass(active: boolean) {
  return cn(SIDEBAR_ITEM_BASE_CLASS, active && SIDEBAR_ITEM_ACTIVE_CLASS);
}

interface SidebarProps {
  tables: Table[];
  selected: Table | null;
  view: View;
  isAdminView: boolean;
  onSelectTable: (table: Table) => void;
  onSelectAdminView: (view: AdminView) => void;
  onOpenCommandPalette: () => void;
  onRefresh: () => void | Promise<void>;
  onToggleTheme: () => void;
  onLogout: () => void;
  theme: "dark" | "light";
  themeToggleLabel: string;
  screenRegistry?: ScreenRegistry;
}

interface SidebarAdminNavButtonProps {
  item: AdminScreen;
  active: boolean;
  onSelectAdminView: (view: AdminView) => void;
}

function SidebarAdminNavButton({
  item,
  active,
  onSelectAdminView,
}: SidebarAdminNavButtonProps) {
  const Icon = item.icon;

  return (
    <button
      onClick={() => onSelectAdminView(item.id)}
      className={sidebarItemClass(active)}
      data-testid={item.testId}
    >
      <Icon className={SIDEBAR_ICON_CLASS} />
      {item.label}
    </button>
  );
}

export function Sidebar({
  tables,
  selected,
  view,
  isAdminView,
  onSelectTable,
  onSelectAdminView,
  onOpenCommandPalette,
  onRefresh,
  onToggleTheme,
  onLogout,
  theme,
  themeToggleLabel,
  screenRegistry = SCREEN_REGISTRY,
}: SidebarProps) {
  return (
    <aside className="w-60 border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900 flex flex-col">
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex items-center gap-2">
        <span className="text-base">👾</span>
        <span className="font-semibold text-sm text-gray-900 dark:text-gray-100">Allyourbase</span>
      </div>

      <CommandPaletteHint onClick={onOpenCommandPalette} />

      <nav className="flex-1 overflow-y-auto py-2">
        <div className="px-4 pb-1 flex items-center justify-between">
          <p className="text-[10px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
            Tables
          </p>
          <button
            onClick={() => onSelectAdminView("sql-editor")}
            className="text-[10px] text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 flex items-center gap-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 rounded"
            title="New Table (opens SQL Editor)"
            aria-label="New Table"
          >
            <Plus className="w-3 h-3" />
            New Table
          </button>
        </div>

        {tables.length === 0 ? (
          <div className="px-4 py-6 text-center">
            <TableProperties className="w-8 h-8 text-gray-300 dark:text-gray-600 mx-auto mb-2" />
            <p className="text-xs text-gray-500 dark:text-gray-400 mb-1">No tables yet</p>
            <p className="text-[11px] text-gray-500 dark:text-gray-400 mb-3">
              Create your first table to get started.
            </p>
            <button
              onClick={() => onSelectAdminView("sql-editor")}
              className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
            >
              Open SQL Editor
            </button>
          </div>
        ) : (
          tables.map((table) => {
            const key = `${table.schema}.${table.name}`;
            const isSelected =
              !isAdminView &&
              selected &&
              selected.schema === table.schema &&
              selected.name === table.name;

            return (
              <button
                key={key}
                onClick={() => onSelectTable(table)}
                className={cn(SIDEBAR_ITEM_BASE_CLASS, isSelected && SIDEBAR_ITEM_ACTIVE_CLASS)}
              >
                <TableIcon className={SIDEBAR_ICON_CLASS} />
                <span className="truncate">
                  {table.schema !== "public" && (
                    <span className="text-gray-600 dark:text-gray-400">{table.schema}.</span>
                  )}
                  {table.name}
                </span>
              </button>
            );
          })
        )}

        {screenRegistry.sections.map((section) => (
          <div key={section.title} className={SIDEBAR_SECTION_CLASS}>
            <p className={SIDEBAR_SECTION_TITLE_CLASS}>{section.title}</p>
            {section.screens.map((item) => (
              <SidebarAdminNavButton
                key={item.id}
                item={item}
                active={view === item.id}
                onSelectAdminView={onSelectAdminView}
              />
            ))}
          </div>
        ))}
      </nav>

      <div className="border-t border-gray-200 dark:border-gray-700 p-2 flex gap-1">
        <button
          onClick={onRefresh}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title="Refresh schema"
          aria-label="Refresh schema"
        >
          <RefreshCw className="w-4 h-4" />
        </button>
        <button
          onClick={onToggleTheme}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title={themeToggleLabel}
          aria-label={themeToggleLabel}
        >
          {theme === "dark" ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
        </button>
        <button
          onClick={onLogout}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title="Log out"
          aria-label="Log out"
        >
          <LogOut className="w-4 h-4" />
        </button>
      </div>
    </aside>
  );
}
