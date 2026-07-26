import { useEffect, useRef, useState } from "react";
import type { RequestLogEntry, RequestLogListResponse } from "../types/analytics";
import { StatusBadge } from "./shared/StatusBadge";
import { formatBytes, formatDate } from "./shared/format";

const STATUS_VARIANT_MAP: Record<string, "success" | "error" | "warning" | "info"> = {
  "2": "success",
  "3": "info",
  "4": "warning",
  "5": "error",
};

function statusVariant(code: number): "success" | "error" | "warning" | "info" | "default" {
  return STATUS_VARIANT_MAP[String(code)[0]] ?? "default";
}

function formatDuration(durationMs: number): string {
  return `${durationMs}ms`;
}

function formatOptional(value: string | undefined): string {
  return value?.trim() || "-";
}

function formatIdentity(row: RequestLogEntry): string {
  if (row.user_id) return `User ${row.user_id}`;
  if (row.api_key_id) return `API key ${row.api_key_id}`;
  return "-";
}

function requestDetails(row: RequestLogEntry) {
  return [
    { label: "ID", testId: "request-log-detail-id", value: row.id },
    { label: "Timestamp", testId: "request-log-detail-timestamp", value: row.timestamp },
    { label: "Method", testId: "request-log-detail-method", value: row.method },
    { label: "Path", testId: "request-log-detail-path", value: row.path },
    { label: "Status code", testId: "request-log-detail-status-code", value: String(row.status_code) },
    { label: "Duration", testId: "request-log-detail-duration-ms", value: formatDuration(row.duration_ms) },
    { label: "User ID", testId: "request-log-detail-user-id", value: formatOptional(row.user_id) },
    { label: "API key ID", testId: "request-log-detail-api-key-id", value: formatOptional(row.api_key_id) },
    { label: "IP address", testId: "request-log-detail-ip-address", value: formatOptional(row.ip_address) },
    { label: "Request ID", testId: "request-log-detail-request-id", value: formatOptional(row.request_id) },
    {
      label: "Request size",
      testId: "request-log-detail-request-size",
      value: `${formatBytes(row.request_size)} (${row.request_size} bytes)`,
    },
    {
      label: "Response size",
      testId: "request-log-detail-response-size",
      value: `${formatBytes(row.response_size)} (${row.response_size} bytes)`,
    },
  ];
}

interface RequestLogsTableProps {
  rows: RequestLogEntry[];
  captionRef: React.RefObject<HTMLTableCaptionElement>;
  onOpenDetails: (row: RequestLogEntry, source: HTMLElement) => void;
}

export function RequestLogsTable({
  rows,
  captionRef,
  onOpenDetails,
}: RequestLogsTableProps) {
  const handleRowKeyDown = (
    event: React.KeyboardEvent<HTMLTableRowElement>,
    row: RequestLogEntry,
  ) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    if (event.target !== event.currentTarget) return;
    event.preventDefault();
    onOpenDetails(row, event.currentTarget);
  };

  return (
    <>
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm" data-testid="request-logs-table">
          <caption ref={captionRef} tabIndex={-1} className="sr-only">
            Request logs
          </caption>
          <thead className="bg-gray-50 dark:bg-gray-800 border-b">
            <tr>
              {["Time", "Method", "Path", "Status", "Duration", "Size", "Identity"].map((header) => (
                <th
                  key={header}
                  className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300"
                >
                  {header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={row.id}
                tabIndex={0}
                aria-label={`${row.method} ${row.path} request details`}
                data-testid={`request-log-row-${row.id}`}
                onClick={(event) => onOpenDetails(row, event.currentTarget)}
                onKeyDown={(event) => handleRowKeyDown(event, row)}
                className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer focus:outline-none focus:ring-2 focus:ring-inset focus:ring-blue-500"
              >
                <td className="px-4 py-2.5">{formatDate(row.timestamp)}</td>
                <td className="px-4 py-2.5">{row.method}</td>
                <td className="px-4 py-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <span>{row.path}</span>
                    <button
                      type="button"
                      data-testid={`request-log-view-details-${row.id}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        onOpenDetails(row, event.currentTarget);
                      }}
                      className="shrink-0 text-xs text-blue-600 hover:underline"
                    >
                      View details
                    </button>
                  </div>
                </td>
                <td className="px-4 py-2.5">
                  <StatusBadge
                    status={String(row.status_code)}
                    variantMap={{
                      [String(row.status_code)]: statusVariant(row.status_code),
                    }}
                  />
                </td>
                <td className="px-4 py-2.5">{formatDuration(row.duration_ms)}</td>
                <td className="px-4 py-2.5">
                  {formatBytes(row.request_size)} / {formatBytes(row.response_size)}
                </td>
                <td className="px-4 py-2.5">{formatIdentity(row)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length === 0 && (
        <div className="text-center py-12 border rounded-lg bg-gray-50 dark:bg-gray-800 text-gray-500 dark:text-gray-400 text-sm">
          No request logs found
        </div>
      )}
    </>
  );
}

interface RequestLogsSummaryProps {
  data: RequestLogListResponse;
  appliedFilters: Record<string, string>;
}

export function RequestLogsSummary({
  data,
  appliedFilters,
}: RequestLogsSummaryProps) {
  const firstVisible = data.items.length === 0 ? 0 : data.offset + 1;
  const lastVisible = data.offset + data.items.length;
  const activeFilters = Object.entries(appliedFilters)
    .filter(([, value]) => value)
    .map(([name, value]) => `${name}: ${value}`);

  return (
    <div
      data-testid="request-logs-summary"
      tabIndex={-1}
      className="mb-3 text-sm text-gray-600 dark:text-gray-300"
    >
      <span>
        Showing {firstVisible}–{lastVisible} of {data.count} request logs
        {" · "}Page size {data.limit}
      </span>
      {activeFilters.length > 0 && <span>{" · "}{activeFilters.join(", ")}</span>}
    </div>
  );
}

interface RequestLogsPagerProps {
  data: RequestLogListResponse;
  loading: boolean;
  onPrevious: () => void;
  onNext: () => void;
}

export function RequestLogsPager({
  data,
  loading,
  onPrevious,
  onNext,
}: RequestLogsPagerProps) {
  const previousDisabled = loading || data.offset <= 0;
  const nextDisabled =
    loading || data.items.length === 0 || data.offset + data.items.length >= data.count;

  return (
    <div
      data-testid="request-logs-pager"
      className="mt-4 flex items-center justify-between gap-4"
    >
      <button
        type="button"
        aria-label="Previous request-log page"
        disabled={previousDisabled}
        onClick={onPrevious}
        className="rounded border px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      >
        Previous
      </button>
      <span className="text-sm text-gray-500 dark:text-gray-400">
        Offset {data.offset} · Limit {data.limit}
      </span>
      <button
        type="button"
        aria-label="Next request-log page"
        disabled={nextDisabled}
        onClick={onNext}
        className="rounded border px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      >
        Next
      </button>
    </div>
  );
}

interface RequestLogDrawerProps {
  row: RequestLogEntry;
  onClose: () => void;
}

export function RequestLogDrawer({ row, onClose }: RequestLogDrawerProps) {
  const panelRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const [copyStatus, setCopyStatus] = useState("");

  useEffect(() => {
    closeButtonRef.current?.focus();
  }, []);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab") return;

    const focusableElements = Array.from(
      panelRef.current?.querySelectorAll<HTMLElement>("button:not([disabled])") ?? [],
    );
    if (focusableElements.length === 0) return;
    const firstElement = focusableElements[0];
    const lastElement = focusableElements[focusableElements.length - 1];
    if (event.shiftKey && document.activeElement === firstElement) {
      event.preventDefault();
      lastElement.focus();
    } else if (!event.shiftKey && document.activeElement === lastElement) {
      event.preventDefault();
      firstElement.focus();
    }
  };

  const copyRequestId = async () => {
    if (!row.request_id) return;
    try {
      await navigator.clipboard.writeText(row.request_id);
      setCopyStatus("Request ID copied");
    } catch {
      setCopyStatus("Failed to copy request ID");
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 bg-black/40"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="request-log-drawer-title"
        data-testid="request-log-drawer"
        onKeyDown={handleKeyDown}
        className="absolute inset-y-0 right-0 w-full max-w-lg overflow-y-auto bg-white dark:bg-gray-900 shadow-xl p-6"
      >
        <div className="flex items-center justify-between gap-4 mb-6">
          <h2 id="request-log-drawer-title" className="text-lg font-semibold">
            Request details
          </h2>
          <button
            ref={closeButtonRef}
            type="button"
            onClick={onClose}
            className="rounded border px-3 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            Close
          </button>
        </div>

        <dl className="space-y-4">
          {requestDetails(row).map((detail) => (
            <div key={detail.testId}>
              <dt className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {detail.label}
              </dt>
              <dd
                data-testid={detail.testId}
                className="mt-1 break-all text-sm text-gray-900 dark:text-gray-100"
              >
                {detail.value}
              </dd>
            </div>
          ))}
        </dl>

        <div className="mt-6 flex items-center gap-3">
          <button
            type="button"
            data-testid="request-log-copy-request-id"
            disabled={!row.request_id}
            onClick={copyRequestId}
            className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Copy request ID
          </button>
          <span role="status" className="text-sm text-gray-500 dark:text-gray-400">
            {copyStatus}
          </span>
        </div>
      </section>
    </div>
  );
}
