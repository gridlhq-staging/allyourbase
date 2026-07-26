import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import type {
  QueryAnalyticsResponse,
  RequestLogEntry,
  RequestLogListResponse,
} from "../types/analytics";
import {
  listAllRequestLogs,
  listQueryStats,
  listRequestLogs,
  type ListRequestLogsParams,
  type RequestLogFilterParams,
} from "../api_analytics";
import { useDraftFilters } from "../hooks/useDraftFilters";
import { cn } from "../lib/utils";
import {
  RequestLogDrawer,
  RequestLogsPager,
  RequestLogsSummary,
  RequestLogsTable,
} from "./AnalyticsRequestLogs";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { formatCSV } from "./shared/format";
import { useAppToast } from "./ToastProvider";

type Tab = "requests" | "queries";
type RequestLogExportFormat = "json" | "csv";

const REQUEST_PAGE_SIZE = 25;
const REQUEST_LOG_EXPORT_FIELDS = [
  "id",
  "timestamp",
  "method",
  "path",
  "status_code",
  "duration_ms",
  "user_id",
  "api_key_id",
  "request_size",
  "response_size",
  "ip_address",
  "request_id",
] as const satisfies readonly (keyof RequestLogEntry)[];

const REQUEST_FILTER_FIELDS: FilterField[] = [
  {
    name: "method",
    label: "Method",
    type: "select",
    options: [
      { value: "", label: "All methods" },
      { value: "GET", label: "GET" },
      { value: "POST", label: "POST" },
      { value: "PUT", label: "PUT" },
      { value: "PATCH", label: "PATCH" },
      { value: "DELETE", label: "DELETE" },
    ],
  },
  { name: "path", label: "Path", type: "text", placeholder: "/api/*" },
  { name: "status", label: "Status Code", type: "text", placeholder: "200" },
  {
    name: "statusClass",
    label: "Status Class",
    type: "select",
    options: [
      { value: "", label: "All status classes" },
      { value: "2xx", label: "2xx" },
      { value: "3xx", label: "3xx" },
      { value: "4xx", label: "4xx" },
      { value: "5xx", label: "5xx" },
    ],
  },
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
  {
    name: "minDurationMs",
    label: "Minimum Latency (ms)",
    type: "number",
    min: 0,
    placeholder: "0",
  },
  {
    name: "maxDurationMs",
    label: "Maximum Latency (ms)",
    type: "number",
    min: 0,
    placeholder: "1000",
  },
];

const INITIAL_REQUEST_FILTER_VALUES = {
  method: "",
  path: "",
  status: "",
  statusClass: "",
  from: "",
  to: "",
  minDurationMs: "",
  maxDurationMs: "",
};

type RequestFilterValues = typeof INITIAL_REQUEST_FILTER_VALUES;
type QueryRow = QueryAnalyticsResponse["items"][number];

const QUERY_COLUMNS: Column<QueryRow>[] = [
  {
    key: "query",
    header: "Query",
    render: (row) => (
      <code className="text-xs break-all max-w-[400px] block truncate" title={row.query}>
        {row.query}
      </code>
    ),
  },
  { key: "calls", header: "Calls", render: (row) => String(row.calls) },
  {
    key: "mean_exec_time",
    header: "Avg (ms)",
    render: (row) => row.mean_exec_time.toFixed(2),
  },
  {
    key: "total_exec_time",
    header: "Total (ms)",
    render: (row) => row.total_exec_time.toFixed(1),
  },
  { key: "rows", header: "Rows", render: (row) => String(row.rows) },
  {
    key: "index_suggestions",
    header: "Index Suggestions",
    render: (row) =>
      row.index_suggestions?.map((suggestion, index) => (
        <div key={index} className="text-xs">
          <code className="bg-yellow-50 dark:bg-yellow-900/20 px-1 py-0.5 rounded text-yellow-700 dark:text-yellow-300">
            {suggestion.statement}
          </code>
          <span className="ml-1 text-gray-400">({suggestion.confidence})</span>
        </div>
      )) ?? null,
  },
];

const TAB_CLASS =
  "px-4 py-2 text-sm font-medium rounded-t border-b-2 transition-colors";
const TAB_ACTIVE =
  "border-blue-600 text-blue-600 dark:text-blue-400 dark:border-blue-400";
const TAB_INACTIVE =
  "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400";
const MAX_INT64_TEXT = "9223372036854775807";

function validateLatencyInteger(value: string, label: string): string | null {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  if (trimmed.startsWith("-")) {
    return `${label} latency must be zero or greater`;
  }
  if (!/^\d+$/.test(trimmed)) {
    return `${label} latency must be a non-negative whole number`;
  }
  if (BigInt(trimmed) > BigInt(MAX_INT64_TEXT)) {
    return `${label} latency must be no greater than ${MAX_INT64_TEXT}`;
  }
  return null;
}

function validateLatencyFilters(filters: RequestFilterValues): string | null {
  const minimumError = validateLatencyInteger(filters.minDurationMs, "Minimum");
  if (minimumError) return minimumError;
  const maximumError = validateLatencyInteger(filters.maxDurationMs, "Maximum");
  if (maximumError) return maximumError;

  const minimum =
    filters.minDurationMs.trim() === "" ? null : BigInt(filters.minDurationMs);
  const maximum =
    filters.maxDurationMs.trim() === "" ? null : BigInt(filters.maxDurationMs);
  if (minimum !== null && maximum !== null && minimum > maximum) {
    return "Minimum latency must be less than or equal to maximum latency";
  }
  return null;
}

function toRequestLogFilters(
  filters: RequestFilterValues,
): RequestLogFilterParams {
  return {
    method: filters.method || undefined,
    path: filters.path || undefined,
    status: filters.status.trim() || undefined,
    statusClass: filters.statusClass || undefined,
    minDurationMs: filters.minDurationMs || undefined,
    maxDurationMs: filters.maxDurationMs || undefined,
    from: filters.from || undefined,
    to: filters.to || undefined,
  };
}

function toRequestLogParams(
  filters: RequestFilterValues,
  offset: number,
): ListRequestLogsParams {
  return {
    ...toRequestLogFilters(filters),
    limit: REQUEST_PAGE_SIZE,
    offset,
  };
}

function toSerializableRequestLog(row: RequestLogEntry) {
  return {
    id: row.id,
    timestamp: row.timestamp,
    method: row.method,
    path: row.path,
    status_code: row.status_code,
    duration_ms: row.duration_ms,
    user_id: row.user_id ?? null,
    api_key_id: row.api_key_id ?? null,
    request_size: row.request_size,
    response_size: row.response_size,
    ip_address: row.ip_address ?? null,
    request_id: row.request_id ?? null,
  };
}

function requestLogExportContent(
  format: RequestLogExportFormat,
  rows: RequestLogEntry[],
): { content: string; mimeType: string } {
  const serializableRows = rows.map(toSerializableRequestLog);
  if (format === "json") {
    return {
      content: JSON.stringify(serializableRows, null, 2),
      mimeType: "application/json",
    };
  }
  return {
    content: formatCSV([
      REQUEST_LOG_EXPORT_FIELDS,
      ...serializableRows.map((row) =>
        REQUEST_LOG_EXPORT_FIELDS.map((field) => row[field]),
      ),
    ]),
    mimeType: "text/csv",
  };
}

function downloadRequestLogExport(
  format: RequestLogExportFormat,
  rows: RequestLogEntry[],
): void {
  const { content, mimeType } = requestLogExportContent(format, rows);
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download =
      `request_logs_${new Date().toISOString().replace(/[:.]/g, "-")}.${format}`;
    link.click();
  } finally {
    URL.revokeObjectURL(url);
  }
}

export function Analytics() {
  const { addToast } = useAppToast();
  const [tab, setTab] = useState<Tab>("requests");
  const [requestData, setRequestData] = useState<RequestLogListResponse | null>(null);
  const [queryData, setQueryData] = useState<QueryAnalyticsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [requestError, setRequestError] = useState<string | null>(null);
  const [queryError, setQueryError] = useState<string | null>(null);
  const [filterError, setFilterError] = useState<string | null>(null);
  const [requestOffset, setRequestOffset] = useState(0);
  const [exportPending, setExportPending] = useState({
    json: false,
    csv: false,
  });
  const {
    draftValues: draftFilterValues,
    appliedValues: appliedFilterValues,
    setDraftValue: setDraftFilterValue,
    applyValues: applyFilterValues,
    resetValues: resetFilterValues,
  } = useDraftFilters(INITIAL_REQUEST_FILTER_VALUES);
  const [querySort, setQuerySort] = useState("total_time");
  const [selectedRequest, setSelectedRequest] = useState<RequestLogEntry | null>(null);
  const requestDetailsOriginRef = useRef<HTMLElement | null>(null);
  const requestTableCaptionRef = useRef<HTMLTableCaptionElement>(null);
  const requestLoadSequenceRef = useRef(0);

  const openRequestDetails = useCallback((row: RequestLogEntry, source: HTMLElement) => {
    requestDetailsOriginRef.current = source;
    setSelectedRequest(row);
  }, []);

  const closeRequestDetails = useCallback(() => {
    setSelectedRequest(null);
    window.setTimeout(() => requestDetailsOriginRef.current?.focus(), 0);
  }, []);

  const loadRequests = useCallback(async (
    filters: RequestFilterValues,
    offset: number,
  ) => {
    const sequence = ++requestLoadSequenceRef.current;
    setLoading(true);
    setRequestError(null);
    try {
      const result = await listRequestLogs(toRequestLogParams(filters, offset));
      if (sequence !== requestLoadSequenceRef.current) return;
      if (
        (result.count === 0 && result.offset > 0) ||
        (result.count > 0 && result.offset >= result.count)
      ) {
        const correctedOffset =
          result.count === 0
            ? 0
            : Math.floor((result.count - 1) / Math.max(result.limit, 1)) * result.limit;
        setRequestOffset(correctedOffset);
        return;
      }
      setRequestData(result);
    } catch (requestLoadError) {
      if (sequence !== requestLoadSequenceRef.current) return;
      setRequestError(
        requestLoadError instanceof Error
          ? requestLoadError.message
          : "Failed to load request logs",
      );
    } finally {
      if (sequence === requestLoadSequenceRef.current) setLoading(false);
    }
  }, []);

  const loadQueries = useCallback(async (sort: string) => {
    setLoading(true);
    setQueryError(null);
    try {
      setQueryData(await listQueryStats({ sort }));
    } catch (queryLoadError) {
      setQueryError(
        queryLoadError instanceof Error
          ? queryLoadError.message
          : "Failed to load query stats",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (tab === "requests") {
      void loadRequests(appliedFilterValues, requestOffset);
    } else {
      void loadQueries(querySort);
    }
  }, [
    appliedFilterValues,
    loadQueries,
    loadRequests,
    querySort,
    requestOffset,
    tab,
  ]);

  useEffect(() => {
    if (!selectedRequest || !requestData) return;
    const replacement = requestData.items.find((row) => row.id === selectedRequest.id);
    if (replacement) {
      if (replacement !== selectedRequest) setSelectedRequest(replacement);
      return;
    }
    setSelectedRequest(null);
    window.setTimeout(() => requestTableCaptionRef.current?.focus(), 0);
  }, [requestData, selectedRequest]);

  const applyRequestFilters = (values: Record<string, string>) => {
    const nextValues = values as RequestFilterValues;
    const validationError = validateLatencyFilters(nextValues);
    setFilterError(validationError);
    if (validationError) return;
    setRequestOffset(0);
    applyFilterValues(nextValues);
  };

  const resetRequestFilters = () => {
    setFilterError(null);
    setRequestOffset(0);
    resetFilterValues();
  };

  const exportRequestLogs = async (format: RequestLogExportFormat) => {
    setExportPending((current) => ({ ...current, [format]: true }));
    try {
      const result = await listAllRequestLogs(
        toRequestLogFilters(appliedFilterValues),
      );
      if (result.items.length === 0) {
        addToast("warning", "No matching request logs to export");
        return;
      }
      downloadRequestLogExport(format, result.items);
      addToast("success", `Exported ${result.items.length} request log(s)`);
    } catch (exportError) {
      addToast(
        "error",
        exportError instanceof Error
          ? exportError.message
          : "Failed to export request logs",
      );
    } finally {
      setExportPending((current) => ({ ...current, [format]: false }));
    }
  };

  const activeError = tab === "requests" ? requestError : queryError;
  const activeData = tab === "requests" ? requestData : queryData;

  if (loading && !activeData) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading analytics...
      </div>
    );
  }

  if (activeError && !activeData) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{activeError}</p>
          <button
            type="button"
            onClick={() => {
              if (tab === "requests") {
                void loadRequests(appliedFilterValues, requestOffset);
              } else {
                void loadQueries(querySort);
              }
            }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Analytics</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Request logs and query performance insights
        </p>
      </div>

      <div className="flex gap-1 mb-4 border-b">
        <button
          type="button"
          onClick={() => setTab("requests")}
          className={cn(TAB_CLASS, tab === "requests" ? TAB_ACTIVE : TAB_INACTIVE)}
        >
          Request Logs
        </button>
        <button
          type="button"
          onClick={() => setTab("queries")}
          className={cn(TAB_CLASS, tab === "queries" ? TAB_ACTIVE : TAB_INACTIVE)}
        >
          Query Performance
        </button>
      </div>

      {tab === "requests" ? (
        <>
          <div data-testid="request-logs-toolbar">
            <FilterBar
              fields={REQUEST_FILTER_FIELDS}
              values={draftFilterValues}
              onChange={setDraftFilterValue}
              onApply={applyRequestFilters}
              onReset={resetRequestFilters}
            />
            <div className="-mt-2 mb-4 flex justify-end gap-2">
              {(["json", "csv"] as const).map((format) => (
                <button
                  key={format}
                  type="button"
                  disabled={exportPending[format]}
                  onClick={() => void exportRequestLogs(format)}
                  className="rounded border border-gray-300 px-3 py-1.5 text-sm font-medium hover:bg-gray-50 disabled:cursor-wait disabled:opacity-60 dark:border-gray-600 dark:hover:bg-gray-800"
                >
                  {exportPending[format]
                    ? `Exporting ${format.toUpperCase()}...`
                    : `Export ${format.toUpperCase()}`}
                </button>
              ))}
            </div>
          </div>
          {filterError && (
            <p role="alert" className="mb-3 text-sm text-red-600">
              {filterError}
            </p>
          )}
          {requestError && requestData && (
            <div
              data-testid="request-logs-error"
              className="mb-3 flex items-center gap-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            >
              <span>{requestError}</span>
              <button
                type="button"
                onClick={() => void loadRequests(appliedFilterValues, requestOffset)}
                className="font-medium underline"
              >
                Retry
              </button>
            </div>
          )}
          {requestData && (
            <>
              <RequestLogsSummary
                data={requestData}
                appliedFilters={appliedFilterValues}
              />
              <RequestLogsTable
                rows={requestData.items}
                captionRef={requestTableCaptionRef}
                onOpenDetails={openRequestDetails}
              />
              <RequestLogsPager
                data={requestData}
                loading={loading}
                onPrevious={() =>
                  setRequestOffset(Math.max(0, requestData.offset - requestData.limit))
                }
                onNext={() =>
                  setRequestOffset(requestData.offset + requestData.limit)
                }
              />
            </>
          )}
        </>
      ) : (
        <>
          <div className="mb-4 flex items-center gap-2">
            <label htmlFor="query-sort" className="text-xs text-gray-600 dark:text-gray-300">
              Sort by
            </label>
            <select
              id="query-sort"
              value={querySort}
              onChange={(event) => setQuerySort(event.target.value)}
              className="border rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-800"
            >
              <option value="total_time">Total Time</option>
              <option value="calls">Calls</option>
              <option value="mean_time">Mean Time</option>
            </select>
          </div>
          <AdminTable
            columns={QUERY_COLUMNS}
            rows={queryData?.items ?? []}
            rowKey="queryid"
            emptyMessage="No query statistics available"
          />
        </>
      )}
      {selectedRequest && (
        <RequestLogDrawer row={selectedRequest} onClose={closeRequestDetails} />
      )}
    </div>
  );
}
