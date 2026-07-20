import { useCallback, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { executeGraphql } from "../api";
import type { GraphqlTransportResult } from "../api_admin";
import { GraphqlExplorerHistory } from "./GraphqlExplorerHistory";
import { GraphqlExplorerRequest } from "./GraphqlExplorerRequest";
import { GraphqlExplorerResponse } from "./GraphqlExplorerResponse";
import { GraphqlExplorerSchema } from "./GraphqlExplorerSchema";
import {
  SCHEMA_INTROSPECTION_QUERY,
  insertGraphqlHistoryEntry,
  loadGraphqlHistory,
  parseGraphqlIntrospectionSchema,
  saveGraphqlHistory,
  type GraphqlHistoryEntry,
  type GraphqlSchemaType,
} from "./graphql-explorer-helpers";

const DEFAULT_QUERY = `query Example {
  __typename
}`;

const VARIABLES_ERROR = "Variables must be valid JSON object text.";
const SCHEMA_DISABLED_MESSAGE = "GraphQL is not enabled on this server";
const SCHEMA_FORBIDDEN_MESSAGE =
  "Schema browsing requires admin access or is disabled";
const SCHEMA_LOAD_ERROR_MESSAGE = "Unable to load the GraphQL schema";

function parseVariablesText(
  variablesText: string,
): { variables?: Record<string, unknown>; error: string | null } {
  const trimmed = variablesText.trim();
  if (!trimmed) {
    return { error: null };
  }

  try {
    const parsed = JSON.parse(trimmed);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return { error: VARIABLES_ERROR };
    }
    return { variables: parsed as Record<string, unknown>, error: null };
  } catch {
    return { error: VARIABLES_ERROR };
  }
}

function requestErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function schemaDegradationMessage(result: GraphqlTransportResult): string | null {
  if (result.status === 404) {
    return SCHEMA_DISABLED_MESSAGE;
  }
  if (
    result.status === 403 ||
    (result.status === 200 &&
      Array.isArray(result.body.errors) &&
      result.body.errors.length > 0)
  ) {
    return SCHEMA_FORBIDDEN_MESSAGE;
  }
  return result.status >= 200 && result.status < 300
    ? null
    : SCHEMA_LOAD_ERROR_MESSAGE;
}

export function GraphqlExplorer() {
  const [query, setQuery] = useState(DEFAULT_QUERY);
  const [variablesText, setVariablesText] = useState("");
  const [loading, setLoading] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [response, setResponse] = useState<GraphqlTransportResult | null>(null);
  const [requestError, setRequestError] = useState<string | null>(null);
  const [history, setHistory] = useState<GraphqlHistoryEntry[]>(loadGraphqlHistory);
  const [showHistory, setShowHistory] = useState(false);
  const [schema, setSchema] = useState<GraphqlSchemaType[] | null>(null);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [schemaMessage, setSchemaMessage] = useState<string | null>(null);
  const inFlightRequest = useRef(false);
  const inFlightSchemaRequest = useRef(false);

  const recordHistory = useCallback(
    (result: GraphqlTransportResult, submittedQuery: string, submittedVariables: string) => {
      const entry: GraphqlHistoryEntry = {
        query: submittedQuery,
        variablesText: submittedVariables,
        status: result.status,
        durationMs: result.durationMs,
        timestamp: new Date().toISOString(),
      };
      setHistory((current) => {
        const updated = insertGraphqlHistoryEntry(current, entry);
        saveGraphqlHistory(updated);
        return updated;
      });
    },
    [],
  );

  const execute = useCallback(async () => {
    if (inFlightRequest.current) {
      return;
    }

    const submittedQuery = query;
    if (!submittedQuery.trim()) {
      return;
    }

    const parsed = parseVariablesText(variablesText);
    if (parsed.error) {
      setValidationError(parsed.error);
      setRequestError(null);
      return;
    }

    inFlightRequest.current = true;
    setLoading(true);
    setValidationError(null);
    setRequestError(null);
    setResponse(null);

    try {
      const result = await executeGraphql(submittedQuery, parsed.variables);
      setResponse(result);
      recordHistory(result, submittedQuery, variablesText);
    } catch (error) {
      setRequestError(requestErrorMessage(error));
    } finally {
      inFlightRequest.current = false;
      setLoading(false);
    }
  }, [query, recordHistory, variablesText]);

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
        event.preventDefault();
        execute();
      }
    },
    [execute],
  );

  const handleHistorySelect = useCallback((entry: GraphqlHistoryEntry) => {
    setQuery(entry.query);
    setVariablesText(entry.variablesText);
    setValidationError(null);
    setRequestError(null);
    setShowHistory(false);
  }, []);

  const clearHistory = useCallback(() => {
    setHistory([]);
    saveGraphqlHistory([]);
  }, []);

  const loadSchema = useCallback(async () => {
    if (inFlightSchemaRequest.current) {
      return;
    }

    inFlightSchemaRequest.current = true;
    setSchemaLoading(true);
    setSchemaMessage(null);
    try {
      const result = await executeGraphql(SCHEMA_INTROSPECTION_QUERY);
      const degradationMessage = schemaDegradationMessage(result);
      if (degradationMessage) {
        setSchema(null);
        setSchemaMessage(degradationMessage);
        return;
      }
      setSchema(parseGraphqlIntrospectionSchema(result.body));
    } catch {
      setSchema(null);
      setSchemaMessage(SCHEMA_LOAD_ERROR_MESSAGE);
    } finally {
      inFlightSchemaRequest.current = false;
      setSchemaLoading(false);
    }
  }, []);

  return (
    <div
      className="flex h-full flex-col"
      data-testid="graphql-explorer"
      onKeyDown={handleKeyDown}
    >
      <GraphqlExplorerRequest
        historyCount={history.length}
        loading={loading}
        onExecute={execute}
        onQueryChange={setQuery}
        onToggleHistory={() => setShowHistory((current) => !current)}
        onVariablesTextChange={setVariablesText}
        query={query}
        validationError={validationError}
        variablesText={variablesText}
      />

      <GraphqlExplorerSchema
        loading={schemaLoading}
        message={schemaMessage}
        onLoad={loadSchema}
        schema={schema}
      />

      {showHistory && (
        <GraphqlExplorerHistory
          history={history}
          onClear={clearHistory}
          onSelect={handleHistorySelect}
        />
      )}

      <GraphqlExplorerResponse error={requestError} response={response} />
    </div>
  );
}
