import type {
  GraphqlSchemaField,
  GraphqlSchemaType,
} from "./graphql-explorer-helpers";

interface GraphqlExplorerSchemaProps {
  schema: readonly GraphqlSchemaType[] | null;
  loading: boolean;
  message: string | null;
  onLoad: () => void;
}

function GraphqlSchemaFieldDetails({
  field,
  typeName,
}: {
  field: GraphqlSchemaField;
  typeName: string;
}) {
  return (
    <div
      className="border-t px-3 py-2 text-xs first:border-t-0"
      data-testid={`graphql-schema-field-${typeName}-${field.name}`}
    >
      <div className="flex flex-wrap items-baseline gap-1 font-mono">
        <span className="font-semibold">{field.name}</span>
        <span className="text-blue-600 dark:text-blue-300">{field.type}</span>
      </div>
      {field.description && (
        <p className="mt-1 text-gray-500 dark:text-gray-300">{field.description}</p>
      )}
      {field.isDeprecated && (
        <p className="mt-1 text-amber-700 dark:text-amber-300">
          Deprecated{field.deprecationReason ? `: ${field.deprecationReason}` : ""}
        </p>
      )}
      {field.arguments.length > 0 && (
        <ul className="mt-2 space-y-1 border-l pl-3" aria-label={`${field.name} arguments`}>
          {field.arguments.map((argument) => (
            <li
              data-testid={`graphql-schema-argument-${typeName}-${field.name}-${argument.name}`}
              key={argument.name}
            >
              <code className="font-semibold">{argument.name}</code>{" "}
              <code className="text-blue-600 dark:text-blue-300">{argument.type}</code>
              {argument.defaultValue !== null && (
                <span className="text-gray-500 dark:text-gray-300">
                  {` = ${argument.defaultValue}`}
                </span>
              )}
              {argument.description && (
                <span className="ml-2 text-gray-500 dark:text-gray-300">
                  {argument.description}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function GraphqlSchemaTypes({ schema }: { schema: readonly GraphqlSchemaType[] }) {
  if (schema.length === 0) {
    return <p className="px-4 py-3 text-xs text-gray-500">No schema types available</p>;
  }

  return (
    <div className="max-h-72 overflow-auto border-t">
      {schema.map((type) => (
        <details className="border-b last:border-b-0" key={type.name}>
          <summary className="flex cursor-pointer items-center gap-2 px-4 py-2 text-sm">
            <span className="font-mono font-semibold">{type.name}</span>
            <span className="text-[10px] uppercase text-gray-500 dark:text-gray-300">
              {type.kind}
            </span>
          </summary>
          {type.description && (
            <p className="border-t px-3 py-2 text-xs text-gray-500 dark:text-gray-300">
              {type.description}
            </p>
          )}
          <div className="bg-gray-50 dark:bg-gray-900">
            {type.fields.map((field) => (
              <GraphqlSchemaFieldDetails
                field={field}
                key={field.name}
                typeName={type.name}
              />
            ))}
          </div>
        </details>
      ))}
    </div>
  );
}

export function GraphqlExplorerSchema({
  schema,
  loading,
  message,
  onLoad,
}: GraphqlExplorerSchemaProps) {
  return (
    <section aria-label="GraphQL schema" className="border-b">
      <div className="flex items-center gap-3 px-4 py-2">
        <h2 className="text-sm font-semibold">Schema</h2>
        <button
          className="ml-auto rounded border px-3 py-1 text-xs hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-gray-800"
          disabled={loading}
          onClick={onLoad}
        >
          {loading ? "Loading schema..." : "Load schema"}
        </button>
      </div>
      {message && (
        <p className="border-t px-4 py-3 text-xs text-gray-600 dark:text-gray-300" role="status">
          {message}
        </p>
      )}
      {!message && schema && <GraphqlSchemaTypes schema={schema} />}
    </section>
  );
}
