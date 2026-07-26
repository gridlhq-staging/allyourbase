import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import type {
  QueryAnalyticsResponse,
  RequestLogAggregateBucket,
  RequestLogEntry,
  RequestLogListResponse,
} from "../types/analytics";
import {
  listAllRequestLogs,
  listQueryStats,
  listRequestLogAggregates,
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
import {
  downloadRequestLogExport,
  type RequestLogExportFormat,
} from "./AnalyticsRequestLogExport";
import {
  mergePendingLiveRowsIntoRequestPage,
  type PendingRequestPageLiveRows,
  useRequestLogLive,
} from "./AnalyticsRequestLogLive";
import { AnalyticsCharts } from "./AnalyticsCharts";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { useAppToast } from "./ToastProvider";

type Tab = "requests" | "queries";

const REQUEST_PAGE_SIZE = 25;

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

const TAB_CLASS = "px-4 py-2 text-sm font-medium rounded-t border-b-2 transition-colors";
const TAB_ACTIVE = "border-blue-600 text-blue-600 dark:text-blue-400 dark:border-blue-400";
const TAB_INACTIVE = "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400";
const MAX_INT64_TEXT = "9223372036854775807";

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

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

export function Analytics() {
  const { addToast } = useAppToast();
  const [tab, setTab] = useState<Tab>("requests");
  const [requestData, setRequestData] = useState<RequestLogListResponse | null>(null);
  const [aggregateItems, setAggregateItems] = useState<RequestLogAggregateBucket[] | null>(null);
  const [aggregateLoading, setAggregateLoading] = useState(true);
  const [aggregateError, setAggregateError] = useState<string | null>(null);
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
  const pendingRequestPageLiveRowsRef = useRef<PendingRequestPageLiveRows | null>(null);
  const aggregateLoadSequenceRef = useRef(0);

  const openRequestDetails = useCallback((row: RequestLogEntry, source: HTMLElement) => {
    requestDetailsOriginRef.current = source;
    setSelectedRequest(row);
  }, []);

  const closeRequestDetails = useCallback(() => {
    setSelectedRequest(null);
    window.setTimeout(() => requestDetailsOriginRef.current?.focus(), 0);
  }, []);

  const loadRequestPage = useCallback(async (filters: RequestFilterValues, offset: number) => {
    const sequence = ++requestLoadSequenceRef.current;
    pendingRequestPageLiveRowsRef.current = offset === 0
      ? { sequence, rows: [], seenIds: new Set<string>() }
      : null;
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
      const pendingLiveRows = pendingRequestPageLiveRowsRef.current?.sequence === sequence
        ? pendingRequestPageLiveRowsRef.current.rows
        : [];
      pendingRequestPageLiveRowsRef.current = null;
      setRequestData(mergePendingLiveRowsIntoRequestPage(result, pendingLiveRows));
    } catch (requestLoadError) {
      if (sequence !== requestLoadSequenceRef.current) return;
      pendingRequestPageLiveRowsRef.current = null;
      setRequestError(errorMessage(requestLoadError, "Failed to load request logs"));
    } finally {
      if (sequence === requestLoadSequenceRef.current) setLoading(false);
    }
  }, []);

  const loadRequestAggregates = useCallback(async (filters: RequestFilterValues) => {
    const sequence = ++aggregateLoadSequenceRef.current;
    setAggregateLoading(true);
    setAggregateError(null);
    try {
      const result =
        await listRequestLogAggregates(toRequestLogFilters(filters));
      if (sequence === aggregateLoadSequenceRef.current) {
        setAggregateItems(result.items);
      }
    } catch (aggregateLoadError) {
      if (sequence === aggregateLoadSequenceRef.current) {
        setAggregateError(errorMessage(
          aggregateLoadError, "Failed to load request aggregates",
        ));
      }
    } finally {
      if (sequence === aggregateLoadSequenceRef.current) setAggregateLoading(false);
    }
  }, []);

  const loadQueries = useCallback(async (sort: string) => {
    setLoading(true);
    setQueryError(null);
    try {
      setQueryData(await listQueryStats({ sort }));
    } catch (queryLoadError) {
      setQueryError(errorMessage(queryLoadError, "Failed to load query stats"));
    } finally {
      setLoading(false);
    }
  }, []);

  const acceptLiveRequestLog = useCallback((row: RequestLogEntry) => {
    setRequestData((current) => {
      if (!current || current.items.some((item) => item.id === row.id)) {
        return current;
      }
      const pendingRequestPageLiveRows = pendingRequestPageLiveRowsRef.current;
      if (
        pendingRequestPageLiveRows &&
        !pendingRequestPageLiveRows.seenIds.has(row.id)
      ) {
        pendingRequestPageLiveRows.seenIds.add(row.id);
        pendingRequestPageLiveRows.rows.unshift(row);
      }
      setRequestOffset(0);
      const limit = Math.max(current.limit, 1);
      return {
        ...current,
        items: [row, ...current.items].slice(0, limit),
        count: current.count + 1,
        offset: 0,
      };
    });
  }, []);

  const resetLiveRequestLogsToNewest = useCallback(() => setRequestOffset(0), []);
  const liveRequestLogFilters = useMemo(
    () => toRequestLogFilters(appliedFilterValues),
    [appliedFilterValues],
  );
  const requestLogLive = useRequestLogLive({
    active: tab === "requests",
    filters: liveRequestLogFilters,
    onRequestLog: acceptLiveRequestLog,
    onResetToNewest: resetLiveRequestLogsToNewest,
  });

  useEffect(() => {
    if (tab === "requests") void loadRequestPage(appliedFilterValues, requestOffset);
  }, [appliedFilterValues, loadRequestPage, requestOffset, tab]);

  useEffect(() => {
    if (tab === "requests") void loadRequestAggregates(appliedFilterValues);
  }, [appliedFilterValues, loadRequestAggregates, tab]);

  useEffect(() => {
    if (tab === "queries") void loadQueries(querySort);
  }, [loadQueries, querySort, tab]);

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
      addToast("error", errorMessage(exportError, "Failed to export request logs"));
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
                void loadRequestPage(appliedFilterValues, requestOffset);
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
                onClick={() =>
                  void loadRequestPage(appliedFilterValues, requestOffset)
                }
                className="font-medium underline"
              >
                Retry
              </button>
            </div>
          )}
          {requestData && (
            <>
              <AnalyticsCharts
                items={aggregateItems}
                loading={aggregateLoading}
                error={aggregateError}
                onRetry={() => void loadRequestAggregates(appliedFilterValues)}
              />
              <RequestLogsSummary
                data={requestData}
                appliedFilters={appliedFilterValues}
                liveEnabled={requestLogLive.enabled}
                liveStatus={requestLogLive.status}
                liveError={requestLogLive.error}
                onLiveToggle={requestLogLive.toggle}
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
