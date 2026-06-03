import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Search as SearchIcon, TableProperties } from "lucide-react";
import type {
  Column,
  FacetBucketValue,
  FacetCounts,
  ListResponse,
  SchemaCache,
  Table,
} from "../types";
import { isSerializableFacetColumnName, listSearchPlaygroundRecords } from "../api_search";
import { TableBrowserGrid } from "./TableBrowserGrid";

const DEFAULT_PER_PAGE = 20;
const FACETABLE_JSON_TYPES = new Set(["string", "number", "integer", "boolean"]);
const NON_FACETABLE_TYPE_PATTERNS = ["json", "vector", "geometry", "geography", "raster"];
const FILTER_IDENTIFIER_PATTERN = /^[A-Za-z_][A-Za-z0-9_.]*$/;
const FILTER_IDENTIFIER_KEYWORDS = new Set(["AND", "OR", "IN", "TRUE", "FALSE", "NULL"]);

function toCollectionKey(table: Pick<Table, "schema" | "name">): string {
  return table.schema === "public" ? table.name : `${table.schema}.${table.name}`;
}

function toCollectionLabel(table: Pick<Table, "schema" | "name">): string {
  return toCollectionKey(table);
}

function isFacetEligibleColumn(column: Column): boolean {
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

// Facet clicks must rewrite the existing filter field so the UI keeps one
// canonical narrowing expression instead of introducing hidden extra state.
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

function selectedFacetPanels(
  selectedFacetColumns: string[],
  facets: FacetCounts | undefined,
): Array<{ column: string; buckets: FacetCounts[string] }> {
  if (!facets) {
    return [];
  }
  return selectedFacetColumns
    .map((column) => {
      const buckets = facets[column];
      return buckets ? { column, buckets } : null;
    })
    .filter((panel): panel is { column: string; buckets: FacetCounts[string] } => panel !== null);
}

interface SearchProps {
  schema: SchemaCache;
}

export function Search({ schema }: SearchProps) {
  const collections = useMemo(
    () =>
      Object.values(schema.tables).sort((left, right) =>
        toCollectionKey(left).localeCompare(toCollectionKey(right)),
      ),
    [schema.tables],
  );
  const [selectedCollection, setSelectedCollection] = useState(
    collections[0] ? toCollectionKey(collections[0]) : "",
  );
  const [data, setData] = useState<ListResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [appliedSearch, setAppliedSearch] = useState("");
  const [filter, setFilter] = useState("");
  const [appliedFilter, setAppliedFilter] = useState("");
  const [selectedFacetColumns, setSelectedFacetColumns] = useState<string[]>([]);
  const [fuzzy, setFuzzy] = useState(false);
  const [perPage, setPerPage] = useState(DEFAULT_PER_PAGE);
  const fetchRunRef = useRef(0);

  useEffect(() => {
    if (collections.length === 0) {
      setSelectedCollection("");
      return;
    }
    const selectedStillExists = collections.some(
      (collection) => toCollectionKey(collection) === selectedCollection,
    );
    if (!selectedStillExists) {
      setSelectedCollection(toCollectionKey(collections[0]));
    }
  }, [collections, selectedCollection]);

  const selectedTable = useMemo(
    () =>
      collections.find((collection) => toCollectionKey(collection) === selectedCollection) ?? null,
    [collections, selectedCollection],
  );
  const eligibleFacetColumns = useMemo(
    () =>
      (selectedTable?.columns ?? [])
        .filter((column) => isFacetEligibleColumn(column) && isSerializableFacetColumnName(column.name))
        .sort(
          (left, right) => left.position - right.position || left.name.localeCompare(right.name),
        ),
    [selectedTable],
  );
  const facetPanels = useMemo(
    () => selectedFacetPanels(selectedFacetColumns, data?.facets),
    [selectedFacetColumns, data?.facets],
  );

  const handleSubmit = useCallback(() => {
    setAppliedSearch(search.trim());
    setAppliedFilter(filter.trim());
  }, [search, filter]);

  const handleCollectionChange = useCallback((value: string) => {
    setSelectedCollection(value);
    setSearch("");
    setAppliedSearch("");
    setFilter("");
    setAppliedFilter("");
    setSelectedFacetColumns([]);
    setFuzzy(false);
    setPerPage(DEFAULT_PER_PAGE);
  }, []);

  const toggleFacetColumn = useCallback((columnName: string) => {
    setSelectedFacetColumns((previous) =>
      previous.includes(columnName)
        ? previous.filter((column) => column !== columnName)
        : [...previous, columnName],
    );
  }, []);

  const handleFacetBucketClick = useCallback((column: string, value: FacetBucketValue) => {
    if (value === null) {
      return;
    }
    const expression = buildFacetFilterExpression(column, value);
    if (!expression) {
      return;
    }
    setFilter(expression);
    setAppliedFilter(expression);
  }, []);

  useEffect(() => {
    const eligibleColumnNames = new Set(eligibleFacetColumns.map((column) => column.name));
    setSelectedFacetColumns((previous) => {
      const next = previous.filter((columnName) => eligibleColumnNames.has(columnName));
      return next.length === previous.length ? previous : next;
    });
  }, [eligibleFacetColumns]);

  const fetchData = useCallback(async () => {
    const runID = ++fetchRunRef.current;
    const isCurrentRun = () => fetchRunRef.current === runID;
    if (!selectedTable) {
      if (isCurrentRun()) {
        setData(null);
        setLoading(false);
      }
      return;
    }
    setLoading(true);
    setError(null);
    setData(null);
    try {
      const response = await listSearchPlaygroundRecords(toCollectionKey(selectedTable), {
        search: appliedSearch || undefined,
        fuzzy,
        filter: appliedFilter || undefined,
        facets: selectedFacetColumns.length > 0 ? selectedFacetColumns : undefined,
        perPage,
      });
      if (isCurrentRun()) {
        setData(response);
      }
    } catch (fetchError) {
      if (isCurrentRun()) {
        setData(null);
        setError(fetchError instanceof Error ? fetchError.message : "Failed to load search results");
      }
    } finally {
      if (isCurrentRun()) {
        setLoading(false);
      }
    }
  }, [selectedTable, appliedSearch, fuzzy, appliedFilter, selectedFacetColumns, perPage]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  if (collections.length === 0) {
    return (
      <div className="p-6">
        <div className="text-center py-16 border rounded-lg bg-gray-50 dark:bg-gray-800">
          <TableProperties className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-gray-300 text-sm">No collections available for search</p>
          <p className="text-gray-600 dark:text-gray-300 text-xs mt-1">
            Create a table first, then come back to run search queries.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 h-full flex flex-col">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Search</h1>
        <p className="text-sm text-gray-500 dark:text-gray-300 mt-0.5">
          Query collection records with optional fuzzy matching, filters, and facet buckets.
        </p>
      </div>

      <div className="mb-4 grid gap-3 md:grid-cols-2">
        <label className="text-sm text-gray-700 dark:text-gray-200">
          Collection
          <select
            className="mt-1 w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
            value={selectedCollection}
            onChange={(event) => handleCollectionChange(event.target.value)}
            aria-label="Collection"
          >
            {collections.map((collection) => {
              const value = toCollectionKey(collection);
              return (
                <option key={value} value={value}>
                  {toCollectionLabel(collection)}
                </option>
              );
            })}
          </select>
        </label>

        <label className="text-sm text-gray-700 dark:text-gray-200">
          Results per page
          <input
            type="number"
            min={1}
            step={1}
            className="mt-1 w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
            value={perPage}
            onChange={(event) => {
              const parsed = Number.parseInt(event.target.value, 10);
              setPerPage(Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_PER_PAGE);
            }}
            aria-label="Results per page"
          />
        </label>

        <label className="text-sm text-gray-700 dark:text-gray-200">
          Search query
          <div className="mt-1 flex items-center border rounded px-3 py-2 bg-white dark:bg-gray-800">
            <SearchIcon className="w-4 h-4 text-gray-400 dark:text-gray-500 mr-2 shrink-0" />
            <input
              type="text"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onKeyDown={(event) => event.key === "Enter" && handleSubmit()}
              className="w-full text-sm bg-transparent outline-none"
              placeholder="Search records..."
              aria-label="Search query"
            />
          </div>
        </label>

        <label className="text-sm text-gray-700 dark:text-gray-200">
          Filter expression
          <input
            type="text"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            onKeyDown={(event) => event.key === "Enter" && handleSubmit()}
            className="mt-1 w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
            placeholder="status='active'"
            aria-label="Filter expression"
          />
        </label>
      </div>

      <div className="mb-4 flex items-center gap-3">
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input
            type="checkbox"
            checked={fuzzy}
            onChange={(event) => setFuzzy(event.target.checked)}
            aria-label="Use fuzzy matching"
          />
          Use fuzzy matching
        </label>

        <button
          onClick={handleSubmit}
          className="px-3 py-1.5 text-xs bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 rounded font-medium"
        >
          Search
        </button>
      </div>

      {eligibleFacetColumns.length > 0 && (
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
                      onChange={() => toggleFacetColumn(column.name)}
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
                <section
                  key={column}
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
                      {buckets.map((bucket) => {
                        const valueLabel = formatFacetValue(bucket.value);
                        const isClickable =
                          bucket.value !== null && isFilterIdentifierCompatible(column);
                        return (
                          <button
                            key={`${column}-${valueLabel}-${bucket.count}`}
                            type="button"
                            disabled={!isClickable}
                            onClick={() => handleFacetBucketClick(column, bucket.value)}
                            data-testid={`search-facet-bucket-${column}-${toFacetTestIDSegment(bucket.value)}`}
                            className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs ${isClickable ? "border-gray-300 bg-white text-gray-700 hover:border-blue-400 hover:text-blue-700 dark:border-gray-600 dark:bg-gray-950 dark:text-gray-200 dark:hover:border-blue-400 dark:hover:text-blue-200" : "cursor-not-allowed border-gray-200 bg-gray-100 text-gray-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400"}`}
                          >
                            <span>{valueLabel}</span>
                            <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                              {bucket.count}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  )}
                </section>
              ))}
            </div>
          )}
        </div>
      )}

      {error && !data && (
        <div className="m-1 mb-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 shrink-0" />
          <div>
            <p className="text-sm text-red-700">{error}</p>
            <button onClick={fetchData} className="mt-2 text-xs text-red-600 hover:text-red-800 underline">
              Retry
            </button>
          </div>
        </div>
      )}

      <div className="border rounded-lg overflow-hidden bg-white dark:bg-gray-900 flex-1 min-h-0">
        {data?.items.length === 0 && !loading && (appliedSearch || appliedFilter) ? (
          <div className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
            <p className="text-sm font-medium text-gray-600 dark:text-gray-300">
              No results matched this search
            </p>
            <p className="text-sm mt-1">Try adjusting your search query or filter expression.</p>
          </div>
        ) : (
          <TableBrowserGrid
            data={data}
            loading={loading}
            columns={selectedTable?.columns ?? []}
            expandColumns={[]}
            sort={null}
            toggleSort={() => {}}
            showCheckboxes={false}
            isWritable={false}
            hasPK={false}
            selectedIds={new Set<string>()}
            toggleSelectAll={() => {}}
            toggleSelect={() => {}}
            pkId={() => ""}
            onRowClick={() => {}}
            onEdit={() => {}}
            onDelete={() => {}}
            page={1}
            setPage={() => {}}
            enableSorting={false}
            enableRowClick={false}
            enablePagination={false}
          />
        )}
      </div>
    </div>
  );
}
