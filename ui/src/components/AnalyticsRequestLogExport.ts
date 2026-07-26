import type { RequestLogEntry } from "../types/analytics";
import { formatCSV } from "./shared/format";

export type RequestLogExportFormat = "json" | "csv";

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

export function downloadRequestLogExport(
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
