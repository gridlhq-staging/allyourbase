import CodeMirror, { EditorView } from "@uiw/react-codemirror";
import { Clock, Play } from "lucide-react";
import { useCodeMirrorTheme } from "./codeMirrorTheme";

interface GraphqlExplorerRequestProps {
  query: string;
  variablesText: string;
  loading: boolean;
  historyCount: number;
  validationError: string | null;
  onQueryChange: (query: string) => void;
  onVariablesTextChange: (variablesText: string) => void;
  onExecute: () => void;
  onToggleHistory: () => void;
}

export function GraphqlExplorerRequest({
  query,
  variablesText,
  loading,
  historyCount,
  validationError,
  onQueryChange,
  onVariablesTextChange,
  onExecute,
  onToggleHistory,
}: GraphqlExplorerRequestProps) {
  const codeMirrorTheme = useCodeMirrorTheme();

  return (
    <div className="border-b px-6 py-4">
      <div className="mb-3 flex items-center gap-3">
        <h1 className="text-lg font-semibold">GraphQL</h1>
        <button
          className="ml-auto flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-200 dark:hover:text-gray-200"
          onClick={onToggleHistory}
        >
          <Clock className="h-3 w-3" />
          History ({historyCount})
        </button>
      </div>

      <div className="overflow-hidden rounded-lg border">
        <CodeMirror
          basicSetup={{
            lineNumbers: true,
            foldGutter: false,
            highlightActiveLine: true,
            bracketMatching: true,
            autocompletion: false,
          }}
          extensions={[
            EditorView.contentAttributes.of({ "aria-label": "GraphQL query" }),
          ]}
          height="180px"
          maxHeight="440px"
          minHeight="120px"
          onChange={onQueryChange}
          placeholder="query Example { __typename }"
          theme={codeMirrorTheme}
          value={query}
        />
      </div>

      <div className="mt-3">
        <label
          className="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-300"
          htmlFor="graphql-variables"
        >
          Variables (JSON)
        </label>
        <textarea
          aria-describedby={validationError ? "graphql-variables-error" : undefined}
          aria-invalid={validationError ? "true" : "false"}
          aria-label="GraphQL variables"
          className="h-24 w-full resize-y rounded-lg border bg-gray-50 px-3 py-2 font-mono text-xs focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-800"
          id="graphql-variables"
          onChange={(event) => onVariablesTextChange(event.target.value)}
          placeholder='{"limit": 20}'
          spellCheck={false}
          value={variablesText}
        />
        {validationError && (
          <p
            className="mt-1 text-xs text-red-600 dark:text-red-300"
            id="graphql-variables-error"
          >
            {validationError}
          </p>
        )}
      </div>

      <div className="mt-3 flex items-center gap-3">
        <button
          className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          disabled={loading || !query.trim()}
          onClick={onExecute}
        >
          <Play className="h-3.5 w-3.5" />
          {loading ? "Sending..." : "Send"}
        </button>
        <span className="text-xs text-gray-500 dark:text-gray-300">
          Cmd/Ctrl+Enter to send
        </span>
      </div>
    </div>
  );
}
