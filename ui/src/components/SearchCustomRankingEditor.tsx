import { Plus, Trash2 } from "lucide-react";
import type { CollectionSearchSettingsCustomRanking, CollectionSearchRankingOrder } from "../api_admin";
import type { Column } from "../types";
import { cn } from "../lib/utils";

const MAX_CUSTOM_RANKING = 32;

const CARD_CLASS = "border border-gray-200 dark:border-gray-800 rounded bg-white dark:bg-gray-900";
const BUTTON_CLASS = "inline-flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium transition-colors disabled:opacity-60 disabled:cursor-not-allowed";
const SECONDARY_BUTTON_CLASS = "border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800";
const DANGER_BUTTON_CLASS = "text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-950/30";

const ORDER_OPTIONS: { value: CollectionSearchRankingOrder; label: string }[] = [
  { value: "asc", label: "Ascending" },
  { value: "desc", label: "Descending" },
];

interface SearchCustomRankingEditorProps {
  ranking: CollectionSearchSettingsCustomRanking[];
  eligibleColumns: Column[];
  disabled: boolean;
  onAdd: () => void;
  onRemove: (index: number) => void;
  onUpdateColumn: (index: number, column: string) => void;
  onUpdateOrder: (index: number, order: CollectionSearchRankingOrder) => void;
}

export function SearchCustomRankingEditor({
  ranking,
  eligibleColumns,
  disabled,
  onAdd,
  onRemove,
  onUpdateColumn,
  onUpdateOrder,
}: SearchCustomRankingEditorProps) {
  return (
    <div className="space-y-3">
      {ranking.length > 0 && (
        <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100">Custom ranking</h3>
      )}
      {ranking.map((item, rowIndex) => {
        const rowNumber = rowIndex + 1;
        return (
          <fieldset
            key={rowIndex}
            className={cn(CARD_CLASS, "p-4 grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem_auto]")}
            aria-label={`Custom ranking row ${rowNumber}`}
          >
            <label className="text-sm font-medium text-gray-800 dark:text-gray-200">
              <span className="sr-only">Ranking column for row {rowNumber}</span>
              <select
                value={item.column}
                onChange={(event) => onUpdateColumn(rowIndex, event.target.value)}
                disabled={disabled}
                aria-label={`Ranking column for row ${rowNumber}`}
                className="mt-1 w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-100"
              >
                {eligibleColumns.map((column) => (
                  <option key={column.name} value={column.name}>
                    {column.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm font-medium text-gray-800 dark:text-gray-200">
              <span className="sr-only">Ranking order for row {rowNumber}</span>
              <select
                value={item.order}
                onChange={(event) => onUpdateOrder(rowIndex, event.target.value as CollectionSearchRankingOrder)}
                disabled={disabled}
                aria-label={`Ranking order for row ${rowNumber}`}
                className="mt-1 w-full rounded border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-100"
              >
                {ORDER_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className={cn(BUTTON_CLASS, DANGER_BUTTON_CLASS, "self-end justify-center")}
              onClick={() => onRemove(rowIndex)}
              disabled={disabled}
              aria-label={`Remove ranking ${item.column}`}
            >
              <Trash2 className="h-4 w-4" />
              Remove
            </button>
          </fieldset>
        );
      })}
      <button
        type="button"
        className={cn(BUTTON_CLASS, SECONDARY_BUTTON_CLASS)}
        onClick={onAdd}
        disabled={disabled || ranking.length >= MAX_CUSTOM_RANKING || eligibleColumns.length === 0}
      >
        <Plus className="h-4 w-4" />
        Add ranking
      </button>
    </div>
  );
}
