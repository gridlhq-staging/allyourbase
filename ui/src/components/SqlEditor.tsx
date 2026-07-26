import { useState, useCallback, useMemo } from "react";
import CodeMirror, { keymap, EditorView } from "@uiw/react-codemirror";
import { sql, PostgreSQL } from "@codemirror/lang-sql";
import { executeSQL, ApiError } from "../api";
import type { SqlResult } from "../types";
import {
  Play,
  Clock,
  CheckCircle2,
  FileJson,
  FileSpreadsheet,
} from "lucide-react";
import { useCodeMirrorTheme } from "./codeMirrorTheme";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { ErrorNotice } from "./ErrorNotice";
import { formatCSV } from "./shared/format";

const DESTRUCTIVE_SQL_KEYWORDS = new Set(["DELETE", "DROP", "TRUNCATE"]);

function skipLineComment(query: string, index: number): number {
  while (index < query.length && query[index] !== "\n" && query[index] !== "\r") {
    index += 1;
  }
  if (query[index] === "\r" && query[index + 1] === "\n") return index + 2;
  return index + 1;
}

function skipBlockComment(query: string, index: number): number {
  let depth = 1;

  while (index < query.length - 1) {
    if (query[index] === "/" && query[index + 1] === "*") {
      depth += 1;
      index += 2;
      continue;
    }
    if (query[index] === "*" && query[index + 1] === "/") {
      depth -= 1;
      index += 2;
      if (depth === 0) return index;
      continue;
    }
    index += 1;
  }

  return query.length;
}

function skipQuotedText(query: string, index: number, quote: "'" | '"'): number {
  index += 1;
  while (index < query.length) {
    if (query[index] !== quote) {
      index += 1;
      continue;
    }
    if (query[index + 1] === quote) {
      index += 2;
      continue;
    }
    return index + 1;
  }
  return index;
}

function nextSQLKeyword(
  query: string,
  startIndex = 0,
): { keyword: string; nextIndex: number } | null {
  let index = startIndex;

  while (index < query.length) {
    const current = query[index];
    const next = query[index + 1];

    if (/\s/.test(current)) {
      index += 1;
      continue;
    }
    if (current === "-" && next === "-") {
      index = skipLineComment(query, index + 2);
      continue;
    }
    if (current === "/" && next === "*") {
      index = skipBlockComment(query, index + 2);
      continue;
    }
    if (current === "'" || current === '"') {
      index = skipQuotedText(query, index, current);
      continue;
    }
    if (/[A-Za-z_]/.test(current)) {
      let end = index + 1;
      while (/[A-Za-z0-9_$]/.test(query[end] || "")) {
        end += 1;
      }
      return {
        keyword: query.slice(index, end).toUpperCase(),
        nextIndex: end,
      };
    }

    index += 1;
  }

  return null;
}

function firstSQLKeyword(query: string): string | null {
  return nextSQLKeyword(query)?.keyword ?? null;
}

function classifyQuery(q: string): "select" | "dml" | "ddl" | "other" {
  const first = firstSQLKeyword(q);
  if (!first) return "other";
  if (first === "SELECT" || first === "WITH" || first === "TABLE" || first === "VALUES")
    return "select";
  if (["INSERT", "UPDATE", "DELETE", "MERGE"].includes(first)) return "dml";
  if (
    ["CREATE", "ALTER", "DROP", "TRUNCATE", "GRANT", "REVOKE", "COMMENT"].includes(
      first,
    )
  )
    return "ddl";
  return "other";
}

function isDestructiveQuery(q: string): boolean {
  const first = firstSQLKeyword(q);
  if (!first) return false;
  if (DESTRUCTIVE_SQL_KEYWORDS.has(first)) return true;
  if (first !== "WITH") return false;

  let keyword = nextSQLKeyword(q);
  while (keyword) {
    if (DESTRUCTIVE_SQL_KEYWORDS.has(keyword.keyword)) return true;
    keyword = nextSQLKeyword(q, keyword.nextIndex);
  }

  return false;
}

export function resultToCSV(result: SqlResult): string {
  return formatCSV([result.columns, ...result.rows]);
}

export function resultToJSON(result: SqlResult): string {
  const objects = result.rows.map((row) => {
    const obj: Record<string, unknown> = {};
    result.columns.forEach((col, i) => {
      obj[col] = row[i];
    });
    return obj;
  });
  return JSON.stringify(objects, null, 2);
}

interface SqlEditorProps {
  onSchemaChange?: () => void | Promise<void>;
}

export function SqlEditor({ onSchemaChange }: SqlEditorProps) {
  const [query, setQuery] = useState(
    () => localStorage.getItem("ayb_sql_query") || "SELECT 1 AS hello;",
  );
  const [result, setResult] = useState<SqlResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [lastQuery, setLastQuery] = useState<string>("");
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [pendingDestructiveQuery, setPendingDestructiveQuery] = useState<string | null>(null);
  const codeMirrorTheme = useCodeMirrorTheme();

  const executeQuery = useCallback(async (trimmed: string) => {
    if (!trimmed) return;

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const res = await executeSQL(trimmed);
      setResult(res);
      setLastQuery(trimmed);
      localStorage.setItem("ayb_sql_query", trimmed);
      // Auto-refresh schema after DDL (CREATE, ALTER, DROP, etc.)
      if (classifyQuery(trimmed) === "ddl" && onSchemaChange) {
        await onSchemaChange();
      }
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError(String(err));
      }
    } finally {
      setLoading(false);
    }
  }, [onSchemaChange]);

  const execute = useCallback(async () => {
    const trimmed = query.trim();
    if (!trimmed) return;

    if (isDestructiveQuery(trimmed)) {
      setPendingDestructiveQuery(trimmed);
      return;
    }

    await executeQuery(trimmed);
  }, [query, executeQuery]);

  const confirmDestructiveQuery = useCallback(async () => {
    if (!pendingDestructiveQuery) return;
    const confirmedQuery = pendingDestructiveQuery;
    setPendingDestructiveQuery(null);
    await executeQuery(confirmedQuery);
  }, [pendingDestructiveQuery, executeQuery]);

  const cancelDestructiveQuery = useCallback(() => {
    setPendingDestructiveQuery(null);
  }, []);

  // Stable reference for Cmd+Enter keymap — reads current query via closure
  // but the keymap extension itself is only created once.
  const executeKeymapExt = useMemo(() => {
    // We use a Prec-less keymap; Mod-Enter is unlikely to collide.
    return keymap.of([
      {
        key: "Mod-Enter",
        run: () => {
          execute();
          return true;
        },
      },
    ]);
    // execute is stable (useCallback) so this is fine
  }, [execute]);

  const extensions = useMemo(
    () => [sql({ dialect: PostgreSQL }), executeKeymapExt, EditorView.contentAttributes.of({ "aria-label": "SQL query" })],
    [executeKeymapExt],
  );

  const copyToClipboard = useCallback(
    async (format: "csv" | "json") => {
      if (!result) return;
      const text = format === "csv" ? resultToCSV(result) : resultToJSON(result);
      try {
        await navigator.clipboard.writeText(text);
      } catch {
        // Fallback for environments without clipboard API
      }
      setCopyFeedback(format === "csv" ? "CSV copied!" : "JSON copied!");
      setTimeout(() => setCopyFeedback(null), 2000);
    },
    [result],
  );

  const feedbackMessage = useMemo(() => {
    if (!result) return null;
    const qtype = classifyQuery(lastQuery);
    const dur = `${result.durationMs}ms`;

    if (result.columns.length > 0) {
      // SELECT-style query that returned rows
      return {
        icon: "clock" as const,
        text: `${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} in ${dur}`,
      };
    }

    // No columns — DDL/DML
    if (qtype === "ddl") {
      return {
        icon: "check" as const,
        text: `Statement executed successfully in ${dur}`,
      };
    }
    if (qtype === "dml") {
      return {
        icon: "check" as const,
        text: `${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} affected in ${dur}`,
      };
    }
    // fallback
    return {
      icon: "check" as const,
      text: `Query OK — ${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} affected in ${dur}`,
    };
  }, [result, lastQuery]);

  return (
    <div className="flex flex-col h-full">
      <ConfirmDialog
        open={pendingDestructiveQuery !== null}
        title="Confirm destructive SQL"
        message="This SQL statement can permanently change or remove data. Confirm before executing it."
        confirmLabel="Execute destructive SQL"
        destructive
        loading={loading}
        onConfirm={confirmDestructiveQuery}
        onCancel={cancelDestructiveQuery}
      />

      {/* Editor area */}
      <div className="border-b p-4">
        <div className="border rounded-lg overflow-hidden">
          <CodeMirror
            value={query}
            onChange={setQuery}
            extensions={extensions}
            theme={codeMirrorTheme}
            height="160px"
            minHeight="80px"
            maxHeight="400px"
            placeholder="Enter SQL query..."
            basicSetup={{
              lineNumbers: true,
              foldGutter: false,
              highlightActiveLine: true,
              bracketMatching: true,
              autocompletion: true,
            }}
          />
        </div>
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={execute}
            disabled={loading || !query.trim()}
            className="px-4 py-1.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
          >
            <Play className="w-3.5 h-3.5" />
            {loading ? "Running..." : "Execute"}
          </button>
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {navigator.platform.includes("Mac") ? "\u2318" : "Ctrl"}+Enter to run
          </span>
          {feedbackMessage && feedbackMessage.icon === "clock" && (
            <span className="ml-auto text-xs text-gray-500 dark:text-gray-400 flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {feedbackMessage.text}
            </span>
          )}
        </div>
      </div>

      {/* Results area */}
      <div className="flex-1 overflow-auto">
        {error && (
          <div className="m-4">
            <ErrorNotice message={error} docsPath="/guide/patterns" />
          </div>
        )}

        {result && result.columns.length > 0 && (
          <div className="relative">
            {/* Copy buttons */}
            <div className="absolute top-2 right-2 flex items-center gap-1 z-10">
              {copyFeedback && (
                <span className="text-xs text-green-600 mr-1">{copyFeedback}</span>
              )}
              <button
                onClick={() => copyToClipboard("csv")}
                title="Copy as CSV"
                className="p-1.5 text-gray-500 dark:text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded transition-colors"
              >
                <FileSpreadsheet className="w-4 h-4" />
              </button>
              <button
                onClick={() => copyToClipboard("json")}
                title="Copy as JSON"
                className="p-1.5 text-gray-500 dark:text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded transition-colors"
              >
                <FileJson className="w-4 h-4" />
              </button>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-gray-50 dark:bg-gray-800">
                    {result.columns.map((col) => (
                      <th
                        key={col}
                        className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300 whitespace-nowrap"
                      >
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {result.rows.map((row, i) => (
                    <tr key={i} className="border-b hover:bg-gray-50 dark:hover:bg-gray-800">
                      {row.map((cell, j) => (
                        <td
                          key={j}
                          className="px-4 py-2 whitespace-nowrap font-mono text-xs"
                        >
                          {cell === null ? (
                            <span className="text-gray-300 dark:text-gray-500 italic">null</span>
                          ) : typeof cell === "object" ? (
                            JSON.stringify(cell)
                          ) : (
                            String(cell)
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {result && result.columns.length === 0 && feedbackMessage && (
          <div className="m-4 p-3 bg-green-50 border border-green-200 rounded-lg flex items-center gap-2">
            <CheckCircle2 className="w-4 h-4 text-green-600 shrink-0" />
            <span className="text-sm text-green-700">{feedbackMessage.text}</span>
          </div>
        )}

        {!result && !error && (
          <div className="flex-1 flex items-center justify-center text-gray-500 dark:text-gray-400 text-sm h-48">
            Run a query to see results
          </div>
        )}
      </div>
    </div>
  );
}
