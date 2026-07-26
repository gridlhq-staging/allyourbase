import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Search as SearchIcon, TableProperties } from "lucide-react";
import type { ListResponse, SchemaCache, Table } from "../types";
import { isSerializableFacetColumnName, listSearchPlaygroundRecords } from "../api_search";
import {
  SearchFacetControls,
  isFacetEligibleColumn,
  selectedFacetPanels,
} from "./SearchFacets";
import {
  SEARCH_HIGHLIGHT_RESPONSE_FIELD,
  SearchHighlightResults,
  gridDataWithoutSyntheticSearchFields,
  searchHighlightSnippets,
} from "./SearchHighlights";
import {
  SEARCH_RANK_RESPONSE_FIELD,
  SearchRankResults,
  searchRankScores,
} from "./SearchRankResults";
import { TableBrowserGrid } from "./TableBrowserGrid";

const DEFAULT_PER_PAGE = 20;

function toCollectionKey(table: Pick<Table, "schema" | "name">): string {
  return table.schema === "public" ? table.name : `${table.schema}.${table.name}`;
}

function toCollectionLabel(table: Pick<Table, "schema" | "name">): string {
  return toCollectionKey(table);
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
  const [showHighlightedMatches, setShowHighlightedMatches] = useState(true);
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
  const hasAppliedSearch = appliedSearch !== "";
  const selectedTableHasHighlightColumn = useMemo(
    () =>
      selectedTable?.columns.some((column) => column.name === SEARCH_HIGHLIGHT_RESPONSE_FIELD) ??
      false,
    [selectedTable],
  );
  const shouldRequestHighlights =
    hasAppliedSearch && showHighlightedMatches && !selectedTableHasHighlightColumn;
  const highlightSnippets = useMemo(
    () => (shouldRequestHighlights ? searchHighlightSnippets(data) : []),
    [data, shouldRequestHighlights],
  );
  // The backend emits a synthetic `_rank` for every non-empty text search, except when
  // the table owns a real `_rank` column — in which case the value is user data and must
  // stay in the grid untouched rather than being read as a relevance score.
  const selectedTableHasRankColumn = useMemo(
    () => selectedTable?.columns.some((column) => column.name === SEARCH_RANK_RESPONSE_FIELD) ?? false,
    [selectedTable],
  );
  const showsRelevanceScores = hasAppliedSearch && !selectedTableHasRankColumn;
  const rankScores = useMemo(
    () => (showsRelevanceScores ? searchRankScores(data) : []),
    [data, showsRelevanceScores],
  );
  const syntheticSearchFields = useMemo(() => {
    const fields: string[] = [];
    if (shouldRequestHighlights) {
      fields.push(SEARCH_HIGHLIGHT_RESPONSE_FIELD);
    }
    if (showsRelevanceScores) {
      fields.push(SEARCH_RANK_RESPONSE_FIELD);
    }
    return fields;
  }, [shouldRequestHighlights, showsRelevanceScores]);
  const gridData = useMemo(
    () => gridDataWithoutSyntheticSearchFields(data, syntheticSearchFields),
    [data, syntheticSearchFields],
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
    setShowHighlightedMatches(true);
    setPerPage(DEFAULT_PER_PAGE);
  }, []);

  const toggleFacetColumn = useCallback((columnName: string) => {
    setSelectedFacetColumns((previous) =>
      previous.includes(columnName)
        ? previous.filter((column) => column !== columnName)
        : [...previous, columnName],
    );
  }, []);

  const handleFacetFilterSelected = useCallback((expression: string) => {
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
        highlight: shouldRequestHighlights || undefined,
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
  }, [
    selectedTable,
    appliedSearch,
    fuzzy,
    shouldRequestHighlights,
    appliedFilter,
    selectedFacetColumns,
    perPage,
  ]);

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

        <label className="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input
            type="checkbox"
            checked={showHighlightedMatches}
            onChange={(event) => setShowHighlightedMatches(event.target.checked)}
            aria-label="Show highlighted matches"
          />
          Show highlighted matches
        </label>

        <button
          onClick={handleSubmit}
          className="px-3 py-1.5 text-xs bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 rounded font-medium"
        >
          Search
        </button>
      </div>

      <SearchFacetControls
        eligibleFacetColumns={eligibleFacetColumns}
        selectedFacetColumns={selectedFacetColumns}
        facetPanels={facetPanels}
        onToggleFacetColumn={toggleFacetColumn}
        onFilterSelected={handleFacetFilterSelected}
      />

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

      <SearchRankResults scores={rankScores} />

      <SearchHighlightResults snippets={highlightSnippets} />

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
            data={gridData}
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
