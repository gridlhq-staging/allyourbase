import { Clock, Trash2 } from "lucide-react";
import { cn } from "../lib/utils";
import { statusColor } from "./api-explorer-helpers";
import type { GraphqlHistoryEntry } from "./graphql-explorer-helpers";

interface GraphqlExplorerHistoryProps {
  history: readonly GraphqlHistoryEntry[];
  onClear: () => void;
  onSelect: (entry: GraphqlHistoryEntry) => void;
}

export function GraphqlExplorerHistory({
  history,
  onClear,
  onSelect,
}: GraphqlExplorerHistoryProps) {
  if (history.length === 0) {
    return null;
  }

  return (
    <div className="max-h-48 overflow-y-auto border-b bg-gray-50 px-6 py-3 dark:bg-gray-800">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs font-medium text-gray-500 dark:text-gray-300">
          Recent Queries
        </span>
        <button
          className="flex items-center gap-1 text-xs text-gray-500 hover:text-red-500 dark:text-gray-300"
          onClick={onClear}
        >
          <Trash2 className="h-3 w-3" />
          Clear
        </button>
      </div>
      {history.map((entry) => (
        <button
          className="flex w-full items-center gap-2 rounded px-2 py-1 text-left font-mono text-xs hover:bg-gray-100 dark:bg-gray-700"
          key={`${entry.timestamp}:${entry.query}:${entry.variablesText}`}
          onClick={() => onSelect(entry)}
        >
          <span className="min-w-0 flex-1 truncate">{entry.query}</span>
          <span className={cn("shrink-0", statusColor(entry.status))}>
            {entry.status}
          </span>
          <span className="shrink-0 text-gray-500 dark:text-gray-300">
            <Clock className="mr-1 inline h-3 w-3" />
            {Math.round(entry.durationMs)}ms
          </span>
        </button>
      ))}
    </div>
  );
}
