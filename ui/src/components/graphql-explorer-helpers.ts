export interface GraphqlHistoryEntry {
  query: string;
  variablesText: string;
  status: number;
  durationMs: number;
  timestamp: string;
}

export interface GraphqlSchemaArgument {
  name: string;
  description: string | null;
  type: string;
  defaultValue: string | null;
}

export interface GraphqlSchemaField {
  name: string;
  description: string | null;
  type: string;
  arguments: GraphqlSchemaArgument[];
  isDeprecated: boolean;
  deprecationReason: string | null;
}

export interface GraphqlSchemaType {
  kind: string;
  name: string;
  description: string | null;
  fields: GraphqlSchemaField[];
}

export const SCHEMA_INTROSPECTION_QUERY = `query SchemaIntrospection {
  __schema {
    types {
      kind
      name
      description
      fields(includeDeprecated: true) {
        name
        description
        args {
          name
          description
          defaultValue
          type { ...SchemaTypeReference }
        }
        type { ...SchemaTypeReference }
        isDeprecated
        deprecationReason
      }
    }
  }
}

fragment SchemaTypeReference on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
          }
        }
      }
    }
  }
}`;

export const GRAPHQL_HISTORY_KEY = "ayb_graphql_explorer_history";
export const GRAPHQL_HISTORY_LIMIT = 20;
const PERSISTED_VARIABLES_TEXT = "";

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : null;
}

function nullableString(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function formatGraphqlType(value: unknown, depth = 0): string | null {
  const reference = asRecord(value);
  if (!reference || depth > 8) {
    return null;
  }

  if (reference.kind === "NON_NULL") {
    const innerType = formatGraphqlType(reference.ofType, depth + 1);
    return innerType ? `${innerType}!` : null;
  }
  if (reference.kind === "LIST") {
    const innerType = formatGraphqlType(reference.ofType, depth + 1);
    return innerType ? `[${innerType}]` : null;
  }
  return nullableString(reference.name);
}

function parseSchemaArgument(value: unknown): GraphqlSchemaArgument | null {
  const argument = asRecord(value);
  const name = nullableString(argument?.name);
  const type = formatGraphqlType(argument?.type);
  if (!name || !type) {
    return null;
  }

  return {
    name,
    description: nullableString(argument?.description),
    type,
    defaultValue: nullableString(argument?.defaultValue),
  };
}

function parseSchemaField(value: unknown): GraphqlSchemaField | null {
  const field = asRecord(value);
  const name = nullableString(field?.name);
  const type = formatGraphqlType(field?.type);
  if (!name || !type) {
    return null;
  }

  const rawArguments = Array.isArray(field?.args) ? field.args : [];
  return {
    name,
    description: nullableString(field?.description),
    type,
    arguments: rawArguments
      .map(parseSchemaArgument)
      .filter((argument): argument is GraphqlSchemaArgument => argument !== null),
    isDeprecated: field?.isDeprecated === true,
    deprecationReason: nullableString(field?.deprecationReason),
  };
}

function parseSchemaType(value: unknown): GraphqlSchemaType | null {
  const type = asRecord(value);
  const kind = nullableString(type?.kind);
  const name = nullableString(type?.name);
  if (!kind || !name || name.startsWith("__")) {
    return null;
  }

  const rawFields = Array.isArray(type?.fields) ? type.fields : [];
  return {
    kind,
    name,
    description: nullableString(type?.description),
    fields: rawFields
      .map(parseSchemaField)
      .filter((field): field is GraphqlSchemaField => field !== null),
  };
}

export function parseGraphqlIntrospectionSchema(
  response: unknown,
): GraphqlSchemaType[] {
  const data = asRecord(asRecord(response)?.data);
  const schema = asRecord(data?.__schema);
  if (!Array.isArray(schema?.types)) {
    return [];
  }

  return schema.types
    .map(parseSchemaType)
    .filter((type): type is GraphqlSchemaType => type !== null);
}

function isGraphqlHistoryEntry(value: unknown): value is GraphqlHistoryEntry {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const entry = value as Record<string, unknown>;
  return (
    typeof entry.query === "string" &&
    typeof entry.variablesText === "string" &&
    typeof entry.status === "number" &&
    typeof entry.durationMs === "number" &&
    typeof entry.timestamp === "string"
  );
}

function sanitizePersistedHistoryEntry(entry: GraphqlHistoryEntry): GraphqlHistoryEntry {
  return {
    ...entry,
    variablesText: PERSISTED_VARIABLES_TEXT,
  };
}

export function loadGraphqlHistory(): GraphqlHistoryEntry[] {
  try {
    const raw = localStorage.getItem(GRAPHQL_HISTORY_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    const sanitized = parsed
      .filter(isGraphqlHistoryEntry)
      .slice(0, GRAPHQL_HISTORY_LIMIT)
      .map(sanitizePersistedHistoryEntry);
    const normalized = JSON.stringify(sanitized);
    if (normalized !== raw) {
      try {
        localStorage.setItem(GRAPHQL_HISTORY_KEY, normalized);
      } catch {
        // Best-effort scrub only; reads must still succeed if storage is unavailable.
      }
    }
    return sanitized;
  } catch {
    return [];
  }
}

export function saveGraphqlHistory(history: readonly GraphqlHistoryEntry[]) {
  try {
    localStorage.setItem(
      GRAPHQL_HISTORY_KEY,
      JSON.stringify(
        history.slice(0, GRAPHQL_HISTORY_LIMIT).map(sanitizePersistedHistoryEntry),
      ),
    );
  } catch {
    // History persistence is best-effort; completed requests must still render.
  }
}

export function insertGraphqlHistoryEntry(
  history: readonly GraphqlHistoryEntry[],
  entry: GraphqlHistoryEntry,
): GraphqlHistoryEntry[] {
  return [
    entry,
    ...history.filter(
      (item) => item.query !== entry.query || item.variablesText !== entry.variablesText,
    ),
  ].slice(0, GRAPHQL_HISTORY_LIMIT);
}
