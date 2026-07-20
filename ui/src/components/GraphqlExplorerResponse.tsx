import { AlertCircle, Clock } from "lucide-react";
import type { GraphqlTransportResult } from "../api_admin";
import { cn } from "../lib/utils";
import { statusColor } from "./api-explorer-helpers";

interface GraphqlExplorerResponseProps {
  response: GraphqlTransportResult | null;
  error: string | null;
}

export function GraphqlExplorerResponse({
  response,
  error,
}: GraphqlExplorerResponseProps) {
  return (
    <div className="flex-1 overflow-auto">
      {error && (
        <div
          className="m-4 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3"
          role="alert"
        >
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
          <pre className="whitespace-pre-wrap font-mono text-sm text-red-700">
            {error}
          </pre>
        </div>
      )}

      {response && (
        <div className="p-4">
          <div className="mb-3 flex items-center gap-3">
            <span className={cn("text-sm font-bold", statusColor(response.status))}>
              {response.status} {response.statusText}
            </span>
            <span className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-300">
              <Clock className="h-3 w-3" />
              {Math.round(response.durationMs)}ms
            </span>
          </div>

          <div className="overflow-hidden rounded-lg border">
            <div className="border-b bg-gray-50 px-3 py-1.5 text-xs font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-300">
              Response Body
            </div>
            <pre
              className="max-h-96 overflow-x-auto whitespace-pre-wrap bg-white p-3 font-mono text-xs dark:bg-gray-800"
              data-testid="graphql-response-body"
            >
              {JSON.stringify(response.body, null, 2)}
            </pre>
          </div>
        </div>
      )}

      {!response && !error && (
        <div className="flex h-48 flex-1 items-center justify-center text-sm text-gray-500 dark:text-gray-300">
          Send a query to see the response
        </div>
      )}
    </div>
  );
}
