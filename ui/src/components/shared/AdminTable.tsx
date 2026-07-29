import { ChevronLeft, ChevronRight } from "lucide-react";
import { ErrorNotice } from "../ErrorNotice";
import type { GuidePath } from "../../lib/docs_url";

export interface Column<T> {
  key: string;
  header: string;
  render?: (row: T) => React.ReactNode;
  className?: string;
}

interface AdminTableBaseProps<T extends object> {
  columns: Column<T>[];
  rows: T[];
  rowKey: keyof T & string;
  page?: number;
  totalPages?: number;
  onPageChange?: (page: number) => void;
  emptyMessage?: string;
  loading?: boolean;
  loadingMessage?: string;
}

/**
 * The error branch delegates to ErrorNotice, whose docsPath is mandatory and is
 * not produced by useAdminResource. Pairing them here makes the omission a type
 * error at the call site instead of a silently unrendered error state.
 */
type AdminTableErrorProps =
  | { error?: undefined; docsPath?: undefined; onRetry?: undefined }
  | { error: string | null; docsPath: GuidePath; onRetry?: () => void };

type AdminTableProps<T extends object> = AdminTableBaseProps<T> & AdminTableErrorProps;

/**
 * Renders rows, the empty state, and pagination, and — when a caller passes the
 * optional degraded-state props — the loading and error states produced by
 * useAdminResource. The hook stays the async lifecycle owner: nothing here
 * fetches, retries on its own, or holds request state.
 *
 * Branch precedence is loading > error > empty > rows. Callers that pass none of
 * the degraded-state props keep the pre-existing empty-or-rows render exactly.
 */
export function AdminTable<T extends object>({
  columns,
  rows,
  rowKey,
  page,
  totalPages,
  onPageChange,
  emptyMessage = "No results found",
  loading,
  loadingMessage = "Loading...",
  error,
  docsPath,
  onRetry,
}: AdminTableProps<T>) {
  if (loading) {
    return (
      <div
        role="status"
        className="text-center py-12 border rounded-lg bg-gray-50 dark:bg-gray-800 text-gray-500 dark:text-gray-300 text-sm"
      >
        {loadingMessage}
      </div>
    );
  }

  if (error && docsPath) {
    return (
      <ErrorNotice message={error} docsPath={docsPath} actionLabel="Retry" onAction={onRetry} />
    );
  }

  if (rows.length === 0) {
    return (
      <div className="text-center py-12 border rounded-lg bg-gray-50 dark:bg-gray-800 text-gray-500 dark:text-gray-300 text-sm">
        {emptyMessage}
      </div>
    );
  }

  return (
    <>
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-800 border-b">
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300"
                >
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={String(row[rowKey])}
                className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={col.className ?? "px-4 py-2.5"}
                  >
                    {col.render ? col.render(row) : String((row as Record<string, unknown>)[col.key] ?? "")}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {page != null && totalPages != null && onPageChange && (
        <div className="mt-3 flex items-center justify-end text-sm text-gray-500 dark:text-gray-400">
          <div className="flex items-center gap-2">
            <button
              onClick={() => onPageChange(Math.max(1, page - 1))}
              disabled={page <= 1}
              aria-label="Previous page"
              className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-30"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span>
              {page} / {totalPages || 1}
            </span>
            <button
              onClick={() => onPageChange(Math.min(totalPages, page + 1))}
              disabled={page >= totalPages}
              aria-label="Next page"
              className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-30"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </>
  );
}
