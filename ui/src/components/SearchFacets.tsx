import type { Column, FacetBucketValue, FacetCounts } from "../types";

const FACETABLE_JSON_TYPES = new Set(["string", "number", "integer", "boolean"]);
const NON_FACETABLE_TYPE_PATTERNS = ["json", "vector", "geometry", "geography", "raster"];
const FILTER_IDENTIFIER_PATTERN = /^[A-Za-z_][A-Za-z0-9_.]*$/;
const FILTER_IDENTIFIER_KEYWORDS = new Set(["AND", "OR", "IN", "TRUE", "FALSE", "NULL"]);

interface FacetPanel {
  column: string;
  buckets: FacetCounts[string];
}

interface SearchFacetControlsProps {
  eligibleFacetColumns: Column[];
  selectedFacetColumns: string[];
  facetPanels: FacetPanel[];
  onToggleFacetColumn: (columnName: string) => void;
  onFilterSelected: (expression: string) => void;
}

export function isFacetEligibleColumn(column: Column): boolean {
  const normalizedType = column.type.trim().toLowerCase();
  if (normalizedType.endsWith("[]")) {
    return false;
  }
  if (NON_FACETABLE_TYPE_PATTERNS.some((pattern) => normalizedType.includes(pattern))) {
    return false;
  }
  if (Array.isArray(column.enumValues) && column.enumValues.length > 0) {
    return true;
  }
  const normalizedJsonType = column.jsonType.trim().toLowerCase();
  return FACETABLE_JSON_TYPES.has(normalizedJsonType);
}

export function selectedFacetPanels(
  selectedFacetColumns: string[],
  facets: FacetCounts | undefined,
): FacetPanel[] {
  if (!facets) {
    return [];
  }
  return selectedFacetColumns
    .map((column) => {
      const buckets = facets[column];
      return buckets ? { column, buckets } : null;
    })
    .filter((panel): panel is FacetPanel => panel !== null);
}

export function SearchFacetControls({
  eligibleFacetColumns,
  selectedFacetColumns,
  facetPanels,
  onToggleFacetColumn,
  onFilterSelected,
}: SearchFacetControlsProps) {
  if (eligibleFacetColumns.length === 0) {
    return null;
  }

  return (
    <div
      className="mb-4 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/40 p-4 space-y-4"
      data-testid="search-facet-controls"
    >
      <fieldset>
        <legend className="text-sm font-medium text-gray-800 dark:text-gray-100">
          Facet columns
        </legend>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-300">
          Choose scalar columns to return live bucket counts with the current result set.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          {eligibleFacetColumns.map((column) => {
            const checked = selectedFacetColumns.includes(column.name);
            return (
              <label
                key={column.name}
                data-testid={`search-facet-option-${column.name}`}
                className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs ${checked ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-950/40 dark:text-blue-200" : "border-gray-300 bg-white text-gray-700 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-200"}`}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => onToggleFacetColumn(column.name)}
                  aria-label={column.name}
                />
                <span>{column.name}</span>
              </label>
            );
          })}
        </div>
      </fieldset>

      {facetPanels.length > 0 && (
        <div className="space-y-3">
          <div>
            <h2 className="text-sm font-medium text-gray-800 dark:text-gray-100">Facet buckets</h2>
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-300">
              Bucket counts match the current search and filter exactly. Clicking a bucket rewrites the filter expression.
            </p>
          </div>

          {facetPanels.map(({ column, buckets }) => (
            <FacetBucketPanel
              key={column}
              column={column}
              buckets={buckets}
              onFilterSelected={onFilterSelected}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function FacetBucketPanel({
  column,
  buckets,
  onFilterSelected,
}: {
  column: string;
  buckets: FacetCounts[string];
  onFilterSelected: (expression: string) => void;
}) {
  return (
    <section
      className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-3"
      data-testid={`search-facet-panel-${column}`}
    >
      <h3 className="text-sm font-medium text-gray-800 dark:text-gray-100">{column}</h3>
      {buckets.length === 0 ? (
        <p className="mt-2 text-xs text-gray-500 dark:text-gray-300">
          No facet buckets for the current result set.
        </p>
      ) : (
        <div className="mt-3 flex flex-wrap gap-2">
          {buckets.map((bucket) => (
            <FacetBucketButton
              key={`${column}-${formatFacetValue(bucket.value)}-${bucket.count}`}
              column={column}
              bucket={bucket}
              onFilterSelected={onFilterSelected}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function FacetBucketButton({
  column,
  bucket,
  onFilterSelected,
}: {
  column: string;
  bucket: FacetCounts[string][number];
  onFilterSelected: (expression: string) => void;
}) {
  const valueLabel = formatFacetValue(bucket.value);
  const expression =
    bucket.value === null ? null : buildFacetFilterExpression(column, bucket.value);
  const isClickable = expression !== null;

  return (
    <button
      type="button"
      disabled={!isClickable}
      onClick={() => expression && onFilterSelected(expression)}
      data-testid={`search-facet-bucket-${column}-${toFacetTestIDSegment(bucket.value)}`}
      className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs ${isClickable ? "border-gray-300 bg-white text-gray-700 hover:border-blue-400 hover:text-blue-700 dark:border-gray-600 dark:bg-gray-950 dark:text-gray-200 dark:hover:border-blue-400 dark:hover:text-blue-200" : "cursor-not-allowed border-gray-200 bg-gray-100 text-gray-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400"}`}
    >
      <span>{valueLabel}</span>
      <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-gray-800 dark:text-gray-300">
        {bucket.count}
      </span>
    </button>
  );
}

function formatFacetValue(value: FacetBucketValue): string {
  if (value === null) {
    return "null";
  }
  return String(value);
}

function toFacetTestIDSegment(value: FacetBucketValue): string {
  if (value === null) {
    return "null";
  }
  const normalized = String(value)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return normalized === "" ? "value" : normalized;
}

function isFilterIdentifierCompatible(column: string): boolean {
  return (
    FILTER_IDENTIFIER_PATTERN.test(column) && !FILTER_IDENTIFIER_KEYWORDS.has(column.toUpperCase())
  );
}

function escapeFilterStringLiteral(value: string): string {
  return value.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
}

function buildFacetFilterExpression(column: string, value: Exclude<FacetBucketValue, null>): string | null {
  if (!isFilterIdentifierCompatible(column)) {
    return null;
  }
  if (typeof value === "string") {
    return `${column}='${escapeFilterStringLiteral(value)}'`;
  }
  return `${column}=${String(value)}`;
}
