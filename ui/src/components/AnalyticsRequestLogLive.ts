import { useCallback, useEffect, useRef, useState } from "react";
import { streamRequestLogs, type RequestLogFilterParams } from "../api_analytics";
import type { RequestLogEntry, RequestLogListResponse } from "../types/analytics";

export type RequestLogLiveStatus = "off" | "connecting" | "live" | "error";
export type PendingRequestPageLiveRows = {
  sequence: number;
  rows: RequestLogEntry[];
  seenIds: Set<string>;
};

type UseRequestLogLiveOptions = {
  active: boolean;
  filters: RequestLogFilterParams;
  onRequestLog: (row: RequestLogEntry) => void;
  onResetToNewest: () => void;
};

export function mergePendingLiveRowsIntoRequestPage(
  page: RequestLogListResponse,
  pendingRows: RequestLogEntry[],
): RequestLogListResponse {
  if (page.offset !== 0 || pendingRows.length === 0) {
    return page;
  }

  const pageRowIds = new Set(page.items.map((item) => item.id));
  const missingPendingRows = pendingRows.filter((row) => !pageRowIds.has(row.id));
  if (missingPendingRows.length === 0) {
    return page;
  }

  return {
    ...page,
    items: [...missingPendingRows, ...page.items].slice(0, Math.max(page.limit, 1)),
    count: page.count + missingPendingRows.length,
    offset: 0,
  };
}

export function useRequestLogLive({
  active,
  filters,
  onRequestLog,
  onResetToNewest,
}: UseRequestLogLiveOptions) {
  const [enabled, setEnabled] = useState(false);
  const [status, setStatus] = useState<RequestLogLiveStatus>("off");
  const [error, setError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (active || !enabled) return;
    controllerRef.current?.abort();
    setEnabled(false);
    setStatus("off");
    setError(null);
  }, [active, enabled]);

  useEffect(() => {
    if (!enabled || !active) return;
    const controller = new AbortController();
    controllerRef.current?.abort();
    controllerRef.current = controller;
    onResetToNewest();
    setStatus("connecting");
    setError(null);

    void streamRequestLogs(filters, {
      signal: controller.signal,
      onReady: () => {
        if (!controller.signal.aborted) setStatus("live");
      },
      onRequestLog,
    }).catch((streamError) => {
      if (controller.signal.aborted) return;
      setStatus("error");
      setError(streamError instanceof Error ? streamError.message : "Request log stream failed");
    });

    return () => {
      controller.abort();
      if (controllerRef.current === controller) controllerRef.current = null;
    };
  }, [active, enabled, filters, onRequestLog, onResetToNewest]);

  const toggle = useCallback(() => {
    setEnabled((current) => {
      if (current) {
        controllerRef.current?.abort();
        setStatus("off");
        setError(null);
      } else {
        onResetToNewest();
        setStatus("connecting");
        setError(null);
      }
      return !current;
    });
  }, [onResetToNewest]);

  return { enabled, status, error, toggle };
}
