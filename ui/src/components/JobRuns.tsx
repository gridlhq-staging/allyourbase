import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, Loader2, XCircle } from "lucide-react";
import { listJobRuns } from "../api";
import type { JobResponse, JobRunListResponse } from "../types";
import { formatDate } from "./shared/format";

interface JobRunsProps {
  job: JobResponse;
  onClose: () => void;
}

export function JobRuns({ job, onClose }: JobRunsProps) {
  const [runs, setRuns] = useState<JobRunListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestIdRef = useRef(0);
  const mountedRef = useRef(true);

  const loadRuns = useCallback(async () => {
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;
    setLoading(true);
    setError(null);
    setRuns(null);
    try {
      const response = await listJobRuns(job.id);
      if (!mountedRef.current || requestId !== requestIdRef.current) return;
      setRuns(response);
    } catch (e) {
      if (!mountedRef.current || requestId !== requestIdRef.current) return;
      setError(e instanceof Error ? e.message : "Failed to load run history");
    } finally {
      if (!mountedRef.current || requestId !== requestIdRef.current) return;
      setLoading(false);
    }
  }, [job.id]);

  useEffect(() => {
    mountedRef.current = true;
    loadRuns();

    return () => {
      mountedRef.current = false;
    };
  }, [loadRuns]);

  return (
    <section className="mt-4 border rounded-lg bg-white dark:bg-gray-900 p-4">
      <div className="flex items-start justify-between gap-3 mb-3">
        <div>
          <h2 className="text-sm font-semibold">Run history for job {job.id}</h2>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{job.type}</p>
        </div>
        <button
          onClick={onClose}
          aria-label="Close run history"
          className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600"
        >
          <XCircle className="w-3.5 h-3.5" />
          Close
        </button>
      </div>

      {loading ? (
        <div className="flex items-center text-sm text-gray-500 dark:text-gray-400 py-2">
          <Loader2 className="w-4 h-4 animate-spin mr-2" />
          Loading run history...
        </div>
      ) : null}

      {!loading && error ? (
        <div className="border rounded p-3 bg-red-50 dark:bg-red-950/30">
          <div className="flex items-center text-red-700 dark:text-red-300 text-sm">
            <AlertCircle className="w-4 h-4 mr-2" />
            {error}
          </div>
          <button
            onClick={loadRuns}
            className="mt-2 text-xs text-blue-600 hover:underline"
          >
            Retry run history
          </button>
        </div>
      ) : null}

      {!loading && !error && runs && runs.items.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400 py-2">
          No run history found for this job.
        </p>
      ) : null}

      {!loading && !error && runs && runs.items.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-xs border rounded overflow-hidden">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Attempt</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Started</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Finished</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Duration</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Error</th>
              </tr>
            </thead>
            <tbody>
              {runs.items.map((run) => (
                <tr key={`${run.attempt}-${run.startedAt}`} className="border-b last:border-0">
                  <td className="px-3 py-2">{run.attempt}</td>
                  <td className="px-3 py-2">{run.status}</td>
                  <td className="px-3 py-2 text-gray-500 dark:text-gray-400">{formatDate(run.startedAt)}</td>
                  <td className="px-3 py-2 text-gray-500 dark:text-gray-400">{formatDate(run.finishedAt)}</td>
                  <td className="px-3 py-2">{run.durationMs} ms</td>
                  <td className="px-3 py-2 text-gray-500 dark:text-gray-400">{run.error ?? "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
