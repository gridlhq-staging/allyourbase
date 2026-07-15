import { useState, useEffect, useRef, useCallback } from "react";
import type { Table } from "../types";
import {
  Search,
  Table as TableIcon,
  Command,
} from "lucide-react";
import { cn } from "../lib/utils";
import type { AdminView } from "./layout-types";
import { SCREEN_REGISTRY, type ScreenRegistry } from "../screens/registry";

export type CommandAction =
  | { kind: "table"; table: Table }
  | { kind: "view"; view: AdminView };

interface CommandItem {
  id: string;
  label: string;
  sublabel?: string;
  section: string;
  icon: React.ReactNode;
  action: CommandAction;
}

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onSelect: (action: CommandAction) => void;
  tables: Table[];
  screenRegistry?: ScreenRegistry;
}

const ICON_CLS = "w-4 h-4 text-gray-400 dark:text-gray-500 shrink-0";

function buildItems(tables: Table[], screenRegistry: ScreenRegistry): CommandItem[] {
  const tableItems: CommandItem[] = tables.map((t) => ({
    id: `table:${t.schema}.${t.name}`,
    label: t.name,
    sublabel: t.schema !== "public" ? t.schema : undefined,
    section: "Tables",
    icon: <TableIcon className={ICON_CLS} />,
    action: { kind: "table" as const, table: t },
  }));

  const navItems: CommandItem[] = screenRegistry.sections.flatMap((section) =>
    section.screens.map((screen) => {
      const Icon = screen.icon;
      return {
        id: `nav:${screen.id}`,
        label: screen.label,
        section: "Navigation",
        icon: <Icon className={ICON_CLS} />,
        action: { kind: "view" as const, view: screen.id },
      };
    }),
  );

  return [...tableItems, ...navItems];
}

function fuzzyMatch(query: string, text: string): boolean {
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  let qi = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++;
  }
  return qi === q.length;
}

export function CommandPalette({
  open,
  onClose,
  onSelect,
  tables,
  screenRegistry = SCREEN_REGISTRY,
}: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const allItems = buildItems(tables, screenRegistry);
  const filtered = query
    ? allItems.filter((item) => {
        const text = item.sublabel ? `${item.sublabel}.${item.label}` : item.label;
        return fuzzyMatch(query, text);
      })
    : allItems;

  // Reset state when opened
  useEffect(() => {
    if (open) {
      setQuery("");
      setActiveIndex(0);
      // Focus after the modal renders
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Scroll active item into view
  useEffect(() => {
    if (!listRef.current) return;
    const active = listRef.current.querySelector("[data-active=true]");
    if (active && typeof active.scrollIntoView === "function") {
      active.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex]);

  const handleSelect = useCallback(
    (item: CommandItem) => {
      onSelect(item.action);
      onClose();
    },
    [onSelect, onClose],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActiveIndex((i) => (i + 1) % Math.max(filtered.length, 1));
          break;
        case "ArrowUp":
          e.preventDefault();
          setActiveIndex((i) => (i - 1 + filtered.length) % Math.max(filtered.length, 1));
          break;
        case "Enter":
          e.preventDefault();
          if (filtered[activeIndex]) handleSelect(filtered[activeIndex]);
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
      }
    },
    [filtered, activeIndex, handleSelect, onClose],
  );

  // Clamp activeIndex when filter results change
  useEffect(() => {
    setActiveIndex((i) => Math.min(i, Math.max(filtered.length - 1, 0)));
  }, [filtered.length]);

  if (!open) return null;

  // Group items by section for rendering
  const sections: { name: string; items: CommandItem[] }[] = [];
  for (const item of filtered) {
    let section = sections.find((s) => s.name === item.section);
    if (!section) {
      section = { name: item.section, items: [] };
      sections.push(section);
    }
    section.items.push(item);
  }

  let flatIndex = 0;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]" onClick={onClose}>
      <div className="absolute inset-0 bg-black/20" />
      <div
        className="relative w-full max-w-lg bg-white dark:bg-gray-800 rounded-xl shadow-2xl border overflow-hidden"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-label="Command palette"
      >
        {/* Search input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b">
          <Search className="w-4 h-4 text-gray-400 dark:text-gray-500 shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Search tables, pages..."
            className="flex-1 text-sm outline-none bg-transparent placeholder:text-gray-400 dark:text-gray-500"
            autoComplete="off"
            spellCheck={false}
          />
          <kbd className="hidden sm:inline-flex items-center gap-0.5 text-[10px] text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-gray-700 rounded px-1.5 py-0.5 font-mono">
            ESC
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-72 overflow-y-auto py-1">
          {filtered.length === 0 ? (
            <p className="px-4 py-8 text-sm text-gray-400 dark:text-gray-500 text-center">No results found</p>
          ) : (
            sections.map((section) => (
              <div key={section.name}>
                <p className="px-4 pt-2 pb-1 text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase tracking-wider">
                  {section.name}
                </p>
                {section.items.map((item) => {
                  const idx = flatIndex++;
                  const isActive = idx === activeIndex;
                  return (
                    <button
                      key={item.id}
                      data-active={isActive}
                      onClick={() => handleSelect(item)}
                      onMouseEnter={() => setActiveIndex(idx)}
                      className={cn(
                        "w-full flex items-center gap-3 px-4 py-2 text-sm text-left",
                        isActive ? "bg-gray-100 dark:bg-gray-700" : "hover:bg-gray-50 dark:bg-gray-800",
                      )}
                    >
                      {item.icon}
                      <span className="flex-1 truncate">
                        {item.sublabel && (
                          <span className="text-gray-400 dark:text-gray-500">{item.sublabel}.</span>
                        )}
                        {item.label}
                      </span>
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        {/* Footer hint */}
        <div className="border-t px-4 py-2 flex items-center gap-4 text-[10px] text-gray-400 dark:text-gray-500">
          <span className="flex items-center gap-1">
            <kbd className="bg-gray-100 dark:bg-gray-700 rounded px-1 py-0.5 font-mono">↑↓</kbd> navigate
          </span>
          <span className="flex items-center gap-1">
            <kbd className="bg-gray-100 dark:bg-gray-700 rounded px-1 py-0.5 font-mono">↵</kbd> select
          </span>
          <span className="flex items-center gap-1">
            <kbd className="bg-gray-100 dark:bg-gray-700 rounded px-1 py-0.5 font-mono">esc</kbd> close
          </span>
        </div>
      </div>
    </div>
  );
}

/** Keyboard shortcut hint for the sidebar. */
export function CommandPaletteHint({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="w-full flex items-center gap-2 px-4 py-2 text-xs text-gray-500 hover:text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:bg-gray-800 transition-colors"
    >
      <Search className="w-3.5 h-3.5" />
      <span className="flex-1 text-left">Search...</span>
      <kbd className="flex items-center gap-0.5 text-[10px] bg-gray-100 dark:bg-gray-700 rounded px-1.5 py-0.5 font-mono">
        <Command className="w-2.5 h-2.5" />K
      </kbd>
    </button>
  );
}
