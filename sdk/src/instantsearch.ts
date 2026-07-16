import type {
  FacetValueSearchParams,
  FacetValueSearchResponse,
  ListParams,
  ListResponse,
  SearchHighlightResult,
  SearchHit,
} from "./types";
import {
  DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG,
  DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG,
  SAFE_HTML_HIGHLIGHT_POST_TAG,
  SAFE_HTML_HIGHLIGHT_PRE_TAG,
  combineFilters,
  translateRequest,
  validateFacetArray,
  validateHighlightTags,
  type FacetFilterInput,
  type InstantSearchSearchParams,
  type NumericFilterInput,
  type TranslatedInstantSearchRequest,
} from "./instantsearch_filters";

type InstantSearchRecord = SearchHit<Record<string, unknown>>;

interface InstantSearchRecordsOwner {
  records: {
    list<T = SearchHit>(
      collection: string,
      params?: ListParams,
    ): Promise<ListResponse<T>>;
    searchFacetValues?(
      collection: string,
      column: string,
      params?: FacetValueSearchParams,
    ): Promise<FacetValueSearchResponse>;
  };
}

export interface CreateInstantSearchClientOptions {
  client: InstantSearchRecordsOwner;
  objectIDField: string;
  defaultIndexName?: string;
  highlight?: boolean;
  disjunctiveFacets?: string[];
}

export type { InstantSearchSearchParams } from "./instantsearch_filters";

export interface InstantSearchSearchRequest {
  indexName?: string;
  params?: InstantSearchSearchParams;
}

export interface InstantSearchHit extends InstantSearchRecord {
  objectID: string;
}

export interface InstantSearchResult {
  hits: InstantSearchHit[];
  facets?: Record<string, Record<string, number>>;
  disjunctiveFacets?: InstantSearchDisjunctiveFacet[];
  facetStats?: Record<string, { min: number; max: number }>;
  facets_stats?: Record<string, { min: number; max: number }>;
  page: number;
  nbHits: number;
  nbPages: number;
  hitsPerPage: number;
  processingTimeMS: number;
  query: string;
  params: string;
  exhaustiveNbHits: boolean;
}

export interface InstantSearchDisjunctiveFacet {
  name: string;
  data: Record<string, number>;
  stats?: { min: number; max: number };
}

export interface InstantSearchResponse {
  results: InstantSearchResult[];
}

/** Algolia-compatible params for `searchForFacetValues`. */
export interface InstantSearchFacetValueParams {
  facetName: string;
  facetQuery?: string;
  query?: string;
  maxFacetHits?: number;
  facetFilters?: FacetFilterInput;
  numericFilters?: NumericFilterInput;
  filters?: string;
  highlightPreTag?: string;
  highlightPostTag?: string;
}

/** Single request entry passed to `searchForFacetValues`. */
export interface InstantSearchFacetValueRequest {
  indexName?: string;
  params?: InstantSearchFacetValueParams;
}

/** Single facet-value hit returned to InstantSearch widgets. */
export interface InstantSearchFacetValueHit {
  value: string;
  highlighted: string;
  count: number;
}

/**
 * Per-request result returned by `searchForFacetValues`. Mirrors Algolia's
 * `SearchForFacetValuesResponse` shape (without analytics fields).
 */
export interface InstantSearchFacetValueResult {
  facetHits: InstantSearchFacetValueHit[];
  exhaustiveFacetsCount: boolean;
  processingTimeMS: number;
}

/** Multi-request envelope returned by `searchForFacetValues`. */
export type InstantSearchFacetValueResponse = InstantSearchFacetValueResult[];

export interface InstantSearchClient {
  search(requests: InstantSearchSearchRequest[]): Promise<InstantSearchResponse>;
  searchForFacetValues(
    requests: InstantSearchFacetValueRequest[],
  ): Promise<InstantSearchFacetValueResponse>;
}

const MAX_FACET_HITS = 100;

export function createInstantSearchClient(
  options: CreateInstantSearchClientOptions,
): InstantSearchClient {
  validateOptions(options);

  return {
    /**
     * TODO: Document search.
     */
    async search(requests: InstantSearchSearchRequest[]): Promise<InstantSearchResponse> {
      const indexNames = requests.map((request) => resolveIndexName(request, options));
      ensureSingleIndexName(indexNames);

      const results: InstantSearchResult[] = [];
      for (const [index, request] of requests.entries()) {
        const translated = translateRequest(request.params, options);
        const startedAt = Date.now();
        const list = await options.client.records.list<InstantSearchRecord>(
          indexNames[index],
          toListParams(translated, options),
        );
        const processingTimeMS = Math.max(0, Date.now() - startedAt);
        results.push(toInstantSearchResult(list, translated, options, processingTimeMS));
      }
      return { results };
    },

    /**
     * Adapter over `records.searchFacetValues()` returning Algolia-compatible
     * per-request facet-value results.
     */
    async searchForFacetValues(
      requests: InstantSearchFacetValueRequest[],
    ): Promise<InstantSearchFacetValueResponse> {
      if (typeof options.client.records.searchFacetValues !== "function") {
        throw new Error(
          "client.records.searchFacetValues is required for searchForFacetValues",
        );
      }
      const indexNames = requests.map((request) => resolveIndexName(request, options));
      ensureSingleIndexName(indexNames);

      const results: InstantSearchFacetValueResult[] = [];
      for (const [index, request] of requests.entries()) {
        results.push(
          await runFacetValueSearch(options.client.records, indexNames[index], request),
        );
      }
      return results;
    },
  };
}

const SUPPORTED_FACET_VALUE_PARAM_KEYS = new Set([
  "facetName",
  "facetQuery",
  "query",
  "maxFacetHits",
  "facetFilters",
  "numericFilters",
  "filters",
  "highlightPreTag",
  "highlightPostTag",
]);

async function runFacetValueSearch(
  records: InstantSearchRecordsOwner["records"],
  collection: string,
  request: InstantSearchFacetValueRequest,
): Promise<InstantSearchFacetValueResult> {
  if (typeof records.searchFacetValues !== "function") {
    throw new Error(
      "client.records.searchFacetValues is required for searchForFacetValues",
    );
  }

  const params = request.params ?? ({} as InstantSearchFacetValueParams);
  rejectUnsupportedFacetValueParams(params);
  validateFacetName(params.facetName);
  const facetQuery = normalizeFacetQuery(params.facetQuery);
  const maxFacetHits = validateMaxFacetHits(params.maxFacetHits);
  validateHighlightTags(params);

  const filter = combineFilters(
    params.facetFilters,
    params.numericFilters,
    params.filters,
    undefined,
  );

  const startedAt = Date.now();
  const response = await records.searchFacetValues(
    collection,
    params.facetName,
    buildFacetValueRequestParams(facetQuery, params.query, maxFacetHits, filter),
  );
  const processingTimeMS = Math.max(0, Date.now() - startedAt);

  const highlightPreTag =
    params.highlightPreTag ?? DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG;
  const highlightPostTag =
    params.highlightPostTag ?? DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG;

  return {
    facetHits: response.facetHits.map((hit) => ({
      value: hit.value,
      highlighted: replaceHighlightMarkers(
        hit.highlighted,
        highlightPreTag,
        highlightPostTag,
        SAFE_HTML_HIGHLIGHT_PRE_TAG,
        SAFE_HTML_HIGHLIGHT_POST_TAG,
      ),
      count: hit.count,
    })),
    exhaustiveFacetsCount: response.exhaustiveFacetsCount,
    processingTimeMS,
  };
}

function buildFacetValueRequestParams(
  facetQuery: string | undefined,
  query: string | undefined,
  maxFacetHits: number | undefined,
  filter: string | undefined,
): FacetValueSearchParams {
  const params: FacetValueSearchParams = {};
  if (facetQuery !== undefined) params.q = facetQuery;
  if (query !== undefined) params.search = query;
  if (maxFacetHits !== undefined) params.maxFacetHits = maxFacetHits;
  if (filter !== undefined) params.filter = filter;
  return params;
}

function rejectUnsupportedFacetValueParams(
  params: InstantSearchFacetValueParams,
): void {
  for (const key of Object.keys(params)) {
    if (!SUPPORTED_FACET_VALUE_PARAM_KEYS.has(key)) {
      throw new Error(`unsupported searchForFacetValues parameter: ${key}`);
    }
  }
}

function validateFacetName(facetName: unknown): void {
  if (facetName == null || facetName === "") {
    throw new Error("facetName is required");
  }
  if (typeof facetName !== "string") {
    throw new Error("facetName must be a string");
  }
}

function normalizeFacetQuery(facetQuery: unknown): string | undefined {
  if (facetQuery == null) return undefined;
  if (typeof facetQuery !== "string") {
    throw new Error("facetQuery must be a string");
  }
  return facetQuery;
}

function validateMaxFacetHits(maxFacetHits: unknown): number | undefined {
  if (maxFacetHits == null) return undefined;
  if (!Number.isInteger(maxFacetHits) || Number(maxFacetHits) <= 0) {
    throw new Error("maxFacetHits must be a positive integer");
  }
  if (Number(maxFacetHits) > MAX_FACET_HITS) {
    throw new Error(`maxFacetHits must be less than or equal to ${MAX_FACET_HITS}`);
  }
  return Number(maxFacetHits);
}

function validateOptions(options: CreateInstantSearchClientOptions): void {
  if (!options.client?.records?.list) {
    throw new Error("client.records.list is required");
  }
  if (!options.objectIDField || typeof options.objectIDField !== "string") {
    throw new Error("objectIDField is required");
  }
  validateFacetArray(options.disjunctiveFacets);
}

function resolveIndexName(
  request: { indexName?: string },
  options: CreateInstantSearchClientOptions,
): string {
  const indexName = request.indexName ?? options.defaultIndexName;
  if (!indexName) {
    throw new Error("indexName or defaultIndexName is required");
  }
  return indexName;
}

function ensureSingleIndexName(indexNames: string[]): void {
  const unique = new Set(indexNames);
  if (unique.size > 1) {
    throw new Error("mixed indexName requests are not supported");
  }
}

function toListParams(
  request: TranslatedInstantSearchRequest,
  options: CreateInstantSearchClientOptions,
): ListParams {
  const params: ListParams = { page: request.page + 1 };
  if (request.hitsPerPage != null) {
    params.perPage = request.hitsPerPage === 0 ? 1 : request.hitsPerPage;
  }
  if (request.query !== "") params.search = request.query;
  if (request.facets?.length) params.facets = request.facets;
  if (request.disjunctiveFacets?.length) {
    params.disjunctiveFacets = request.disjunctiveFacets;
  }
  if (request.filter) params.filter = request.filter;
  if (options.highlight ?? true) params.highlight = true;
  return params;
}

function toInstantSearchResult(
  list: ListResponse<InstantSearchRecord>,
  request: TranslatedInstantSearchRequest,
  options: CreateInstantSearchClientOptions,
  processingTimeMS: number,
): InstantSearchResult {
  const result: InstantSearchResult = {
    hits:
      request.hitsPerPage === 0
        ? []
        : list.items.map((item) =>
            toInstantSearchHit(item, options.objectIDField, request),
          ),
    page: list.page - 1,
    nbHits: list.totalItems,
    nbPages: request.hitsPerPage === 0 ? 0 : list.totalPages,
    hitsPerPage: request.hitsPerPage ?? list.perPage,
    processingTimeMS,
    query: request.query,
    params: request.echoedParams,
    exhaustiveNbHits: true,
  };
  const mappedFacets = list.facets ? mapFacets(list.facets) : undefined;
  if (mappedFacets) result.facets = mappedFacets;
  if (list.facetStats) {
    const mappedFacetStats = mapFacetStats(list.facetStats);
    result.facetStats = mappedFacetStats;
    result.facets_stats = mappedFacetStats;
  }
  if (request.disjunctiveFacets?.length) {
    result.disjunctiveFacets = mapDisjunctiveFacets(
      request.disjunctiveFacets,
      mappedFacets,
      result.facetStats,
    );
  }
  return result;
}

function toInstantSearchHit(
  item: InstantSearchRecord,
  objectIDField: string,
  request: TranslatedInstantSearchRequest,
): InstantSearchHit {
  if (!Object.prototype.hasOwnProperty.call(item, objectIDField) || item[objectIDField] === undefined) {
    throw new Error(`objectIDField ${objectIDField} is missing from a returned row`);
  }
  if (item[objectIDField] === null) {
    throw new Error(`objectIDField ${objectIDField} is null on a returned row`);
  }
  const highlightResult = item._highlightResult
    ? remapHighlightResult(item._highlightResult, request)
    : undefined;
  return {
    ...item,
    _highlightResult: highlightResult,
    objectID: String(item[objectIDField]),
  };
}

function remapHighlightResult(
  highlightResult: SearchHighlightResult,
  request: TranslatedInstantSearchRequest,
): SearchHighlightResult {
  const highlightPreTag =
    request.highlightPreTag ?? DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG;
  const highlightPostTag =
    request.highlightPostTag ?? DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG;
  const mapped: SearchHighlightResult = {};

  for (const [attribute, entry] of Object.entries(highlightResult)) {
    mapped[attribute] = {
      ...entry,
      value: replaceHighlightMarkers(entry.value, highlightPreTag, highlightPostTag),
    };
  }

  return mapped;
}

function replaceHighlightMarkers(
  value: string,
  highlightPreTag: string,
  highlightPostTag: string,
  sourcePreTag = "<b>",
  sourcePostTag = "</b>",
): string {
  return value
    .split(sourcePreTag)
    .join(highlightPreTag)
    .split(sourcePostTag)
    .join(highlightPostTag);
}

function mapFacets(
  facets: NonNullable<ListResponse<InstantSearchRecord>["facets"]>,
): Record<string, Record<string, number>> {
  const mapped: Record<string, Record<string, number>> = {};
  for (const [name, buckets] of Object.entries(facets)) {
    mapped[name] = {};
    for (const bucket of buckets) {
      mapped[name][bucket.value === null ? "null" : String(bucket.value)] = bucket.count;
    }
  }
  return mapped;
}

function mapFacetStats(
  facetStats: NonNullable<ListResponse<InstantSearchRecord>["facetStats"]>,
): Record<string, { min: number; max: number }> {
  const mapped: Record<string, { min: number; max: number }> = {};
  for (const [name, bounds] of Object.entries(facetStats)) {
    const min = Number(bounds.min);
    const max = Number(bounds.max);
    if (!Number.isFinite(min) || !Number.isFinite(max)) {
      throw new Error("facetStats bounds must be numeric");
    }
    mapped[name] = { min, max };
  }
  return mapped;
}

function mapDisjunctiveFacets(
  facetNames: string[],
  facets: Record<string, Record<string, number>> | undefined,
  facetStats: Record<string, { min: number; max: number }> | undefined,
): InstantSearchDisjunctiveFacet[] {
  return facetNames.map((name) => ({
    name,
    data: facets?.[name] ?? {},
    ...(facetStats?.[name] ? { stats: facetStats[name] } : {}),
  }));
}
