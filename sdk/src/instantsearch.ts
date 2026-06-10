/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun05_pm_5_instantsearch_adapter_and_example/allyourbase_dev/sdk/src/instantsearch.ts.
 */
import type {
  ListParams,
  ListResponse,
  SearchHighlightResult,
  SearchHit,
} from "./types";

type InstantSearchRecord = SearchHit<Record<string, unknown>>;

type FacetFilterInput = string | Array<string | string[]>;

interface InstantSearchRecordsOwner {
  records: {
    list<T = SearchHit>(
      collection: string,
      params?: ListParams,
    ): Promise<ListResponse<T>>;
  };
}

export interface CreateInstantSearchClientOptions {
  client: InstantSearchRecordsOwner;
  objectIDField: string;
  defaultIndexName?: string;
  highlight?: boolean;
}

export interface InstantSearchSearchParams {
  query?: string;
  page?: number;
  hitsPerPage?: number;
  facets?: string[];
  facetFilters?: FacetFilterInput;
  filters?: string;
  highlightPreTag?: string;
  highlightPostTag?: string;
  [key: string]: unknown;
}

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
  page: number;
  nbHits: number;
  nbPages: number;
  hitsPerPage: number;
  processingTimeMS: number;
  query: string;
  params: string;
  exhaustiveNbHits: boolean;
}

export interface InstantSearchResponse {
  results: InstantSearchResult[];
}

export interface InstantSearchClient {
  search(requests: InstantSearchSearchRequest[]): Promise<InstantSearchResponse>;
  searchForFacetValues(_requests: unknown): Promise<never>;
}

const SUPPORTED_PARAM_KEYS = new Set([
  "query",
  "page",
  "hitsPerPage",
  "facets",
  "facetFilters",
  "filters",
  "highlightPreTag",
  "highlightPostTag",
]);

const DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG = "__ais-highlight__";
const DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG = "__/ais-highlight__";
const SAFE_HTML_HIGHLIGHT_PRE_TAG = "<mark>";
const SAFE_HTML_HIGHLIGHT_POST_TAG = "</mark>";

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
        const translated = translateRequest(request);
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

    async searchForFacetValues(): Promise<never> {
      throw new Error("searchForFacetValues is not supported");
    },
  };
}

function validateOptions(options: CreateInstantSearchClientOptions): void {
  if (!options.client?.records?.list) {
    throw new Error("client.records.list is required");
  }
  if (!options.objectIDField || typeof options.objectIDField !== "string") {
    throw new Error("objectIDField is required");
  }
}

function resolveIndexName(
  request: InstantSearchSearchRequest,
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

function translateRequest(request: InstantSearchSearchRequest): RequiredRequest {
  const params = request.params ?? {};
  rejectUnsupportedParams(params);
  validatePage(params.page);
  validateHitsPerPage(params.hitsPerPage);
  validateFacets(params.facets);
  validateHighlightTags(params);

  return {
    query: params.query ?? "",
    page: params.page ?? 0,
    hitsPerPage: params.hitsPerPage,
    facets: params.facets,
    filter: combineFilters(params.facetFilters, params.filters),
    highlightPreTag: params.highlightPreTag,
    highlightPostTag: params.highlightPostTag,
    echoedParams: buildEchoedParams(params),
  };
}

interface RequiredRequest {
  query: string;
  page: number;
  hitsPerPage?: number;
  facets?: string[];
  filter?: string;
  highlightPreTag?: string;
  highlightPostTag?: string;
  echoedParams: string;
}

function rejectUnsupportedParams(params: InstantSearchSearchParams): void {
  for (const key of Object.keys(params)) {
    if (!SUPPORTED_PARAM_KEYS.has(key)) {
      throw new Error(`unsupported InstantSearch parameter: ${key}`);
    }
  }
  if (params.query != null && typeof params.query !== "string") {
    throw new Error("query must be a string");
  }
  if (params.filters != null && typeof params.filters !== "string") {
    throw new Error("filters must be a string");
  }
}

function validatePage(page: unknown): void {
  if (page == null) return;
  if (!Number.isInteger(page) || Number(page) < 0) {
    throw new Error("page must be a zero-based integer");
  }
}

function validateHitsPerPage(hitsPerPage: unknown): void {
  if (hitsPerPage == null) return;
  if (!Number.isInteger(hitsPerPage) || Number(hitsPerPage) <= 0) {
    throw new Error("hitsPerPage must be a positive integer");
  }
}

function validateFacets(facets: unknown): void {
  if (facets == null) return;
  if (!Array.isArray(facets) || facets.some((facet) => typeof facet !== "string")) {
    throw new Error("facets must be an array of concrete attribute names");
  }
  if (facets.includes("*")) {
    throw new Error('wildcard facets ["*"] are not supported');
  }
}

function validateHighlightTags(params: InstantSearchSearchParams): void {
  const { highlightPreTag, highlightPostTag } = params;
  if (highlightPreTag == null && highlightPostTag == null) return;

  if (highlightPreTag != null && typeof highlightPreTag !== "string") {
    throw new Error("highlightPreTag must be a string");
  }
  if (highlightPostTag != null && typeof highlightPostTag !== "string") {
    throw new Error("highlightPostTag must be a string");
  }
  if (highlightPreTag == null || highlightPostTag == null) {
    throw new Error("highlightPreTag and highlightPostTag must be provided together");
  }
  if (!isSupportedHighlightTagPair(highlightPreTag, highlightPostTag)) {
    throw new Error(
      "highlight tags must use InstantSearch placeholders or <mark> wrappers",
    );
  }
}

function isSupportedHighlightTagPair(
  highlightPreTag: string,
  highlightPostTag: string,
): boolean {
  return (
    (highlightPreTag === DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG &&
      highlightPostTag === DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG) ||
    (highlightPreTag === SAFE_HTML_HIGHLIGHT_PRE_TAG &&
      highlightPostTag === SAFE_HTML_HIGHLIGHT_POST_TAG)
  );
}

function toListParams(
  request: RequiredRequest,
  options: CreateInstantSearchClientOptions,
): ListParams {
  const params: ListParams = { page: request.page + 1 };
  if (request.hitsPerPage != null) params.perPage = request.hitsPerPage;
  if (request.query !== "") params.search = request.query;
  if (request.facets?.length) params.facets = request.facets;
  if (request.filter) params.filter = request.filter;
  if (options.highlight ?? true) params.highlight = true;
  return params;
}

function combineFilters(facetFilters: unknown, filters: unknown): string | undefined {
  const translated = [
    translateFacetFilters(facetFilters),
    translateFilters(filters),
  ].filter((filter): filter is string => Boolean(filter));
  return translated.length ? translated.join(" AND ") : undefined;
}

function translateFacetFilters(input: unknown): string | undefined {
  if (input == null) return undefined;
  if (typeof input === "string") return translateFacetFilterValue(input);
  if (!Array.isArray(input)) {
    throw new Error("facetFilters must be a string or array");
  }
  const clauses = input.map((entry) => {
    if (typeof entry === "string") return translateFacetFilterValue(entry);
    if (!Array.isArray(entry)) {
      throw new Error("facetFilters entries must be strings or one-level arrays");
    }
    if (entry.some((nested) => Array.isArray(nested))) {
      throw new Error("nested facetFilters deeper than one level are not supported");
    }
    if (entry.some((nested) => typeof nested !== "string")) {
      throw new Error("facetFilters arrays must contain strings");
    }
    if (entry.length === 0) {
      throw new Error("facetFilters OR groups must not be empty");
    }
    const orClauses = entry.map((nested) => translateFacetFilterValue(nested));
    return orClauses.length === 1 ? orClauses[0] : `(${orClauses.join(" OR ")})`;
  });
  return clauses.join(" AND ");
}

function translateFacetFilterValue(input: string): string {
  const separator = input.indexOf(":");
  if (separator <= 0) {
    throw new Error("facetFilters must use attribute:value form");
  }
  const attribute = input.slice(0, separator);
  const value = input.slice(separator + 1);
  validateAttribute(attribute);
  if (value.startsWith("-") || value.startsWith("\\-")) {
    throw new Error("negative facetFilters are not supported");
  }
  return `${attribute}=${emitLiteral(value, "=")}`;
}

function translateFilters(input: unknown): string | undefined {
  if (input == null || input === "") return undefined;
  const filter = String(input);
  rejectUnsupportedFilterSyntax(filter);
  const tokens = tokenizeFilter(filter);
  const parsed = parseFilterExpression(tokens, 0);
  if (parsed.nextIndex !== tokens.length) throwMalformedBooleanFilter();
  const translated = parsed.emitted.join(" ");
  return tokens.some(isBooleanToken) ? `(${translated})` : translated;
}

interface ParsedFilterExpression {
  emitted: string[];
  nextIndex: number;
}

function parseFilterExpression(tokens: string[], startIndex: number): ParsedFilterExpression {
  const emitted: string[] = [];
  let index = startIndex;
  let expectOperand = true;

  while (index < tokens.length) {
    const token = tokens[index];
    if (token === ")") {
      if (expectOperand) throwMalformedBooleanFilter();
      return { emitted, nextIndex: index };
    }
    if (expectOperand) {
      const parsedOperand = parseFilterOperand(tokens, index);
      emitted.push(...parsedOperand.emitted);
      index = parsedOperand.nextIndex;
      expectOperand = false;
      continue;
    }
    if (!isBooleanToken(token)) throwMalformedBooleanFilter();
    emitted.push(token.toUpperCase());
    index += 1;
    expectOperand = true;
  }

  if (expectOperand) throwMalformedBooleanFilter();
  return { emitted, nextIndex: index };
}

function parseFilterOperand(tokens: string[], index: number): ParsedFilterExpression {
  const token = tokens[index];
  if (isBooleanToken(token) || token === ")") throwMalformedBooleanFilter();
  if (token === "(") {
    const nested = parseFilterExpression(tokens, index + 1);
    if (nested.nextIndex >= tokens.length || tokens[nested.nextIndex] !== ")") {
      throwMalformedBooleanFilter();
    }
    return { emitted: ["(", ...nested.emitted, ")"], nextIndex: nested.nextIndex + 1 };
  }
  return parseFilterComparison(tokens, index);
}

function parseFilterComparison(tokens: string[], index: number): ParsedFilterExpression {
  const attribute = tokens[index];
  const operator = tokens[index + 1];
  const value = tokens[index + 2];
  if (!operator || !value) throw new Error("bare tag filters are not supported");
  validateAttribute(attribute);
  if (attribute.toLowerCase() === "_tags") {
    throw new Error("_tags filters are not supported");
  }
  validateFilterOperator(operator);
  return {
    emitted: [`${attribute}${operator === ":" ? "=" : operator}${emitLiteral(value, operator)}`],
    nextIndex: index + 3,
  };
}

function throwMalformedBooleanFilter(): never {
  throw new Error("malformed boolean filters are not supported");
}

function rejectUnsupportedFilterSyntax(filter: string): void {
  if (/\bNOT\b/i.test(filter)) throw new Error("NOT filters are not supported");
  if (/\bTO\b/i.test(filter)) throw new Error("numeric range filters are not supported");
  if (filter.includes("[") || filter.includes("]")) {
    throw new Error("array filters are not supported");
  }
  if (/[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z0-9_]/.test(filter)) {
    throw new Error("nested attributes are not supported");
  }
}

function tokenizeFilter(input: string): string[] {
  const tokens: string[] = [];
  const matcher =
    /\s*(>=|<=|!=|=|<|>|:|\(|\)|'([^'\\]|\\.)*'|"([^"\\]|\\.)*"|\bAND\b|\bOR\b|[^\s():=<>!]+)\s*/giy;
  let match: RegExpExecArray | null;
  let consumed = 0;
  while ((match = matcher.exec(input)) !== null) {
    tokens.push(match[1]);
    consumed = matcher.lastIndex;
  }
  if (tokens.length === 0 || input.slice(consumed).trim() !== "") {
    throw new Error("unsupported filters syntax");
  }
  return tokens;
}

function isBooleanToken(token: string): boolean {
  return token.toUpperCase() === "AND" || token.toUpperCase() === "OR";
}

function validateFilterOperator(operator: string): void {
  if (![":", "=", "!=", "<", "<=", ">", ">="].includes(operator)) {
    throw new Error("filters must use supported comparison operators");
  }
}

function validateAttribute(attribute: string): void {
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(attribute)) {
    throw new Error("attributes with spaces or unsupported characters are not supported");
  }
}

function emitLiteral(rawValue: string, operator: string): string {
  const value = stripQuotes(rawValue);
  if (/^-?\d+(\.\d+)?$/.test(value)) return value;
  if (/^(true|false)$/i.test(value)) return value.toLowerCase();
  if (/^null$/i.test(value)) {
    if (operator !== "=" && operator !== ":" && operator !== "!=") {
      throw new Error("null filters require = or !=");
    }
    return "null";
  }
  return `'${escapeAYBString(value)}'`;
}

function stripQuotes(value: string): string {
  if (
    (value.startsWith("'") && value.endsWith("'")) ||
    (value.startsWith('"') && value.endsWith('"'))
  ) {
    return value.slice(1, -1);
  }
  return value;
}

function escapeAYBString(value: string): string {
  return value.replace(/\\/g, "\\\\").replace(/'/g, "\\'");
}

function buildEchoedParams(params: InstantSearchSearchParams): string {
  const echoed = new URLSearchParams();
  echoed.set("query", params.query ?? "");
  if (params.page != null) echoed.set("page", String(params.page));
  if (params.hitsPerPage != null) echoed.set("hitsPerPage", String(params.hitsPerPage));
  if (params.facets != null) echoed.set("facets", JSON.stringify(params.facets));
  if (params.facetFilters != null) {
    echoed.set("facetFilters", JSON.stringify(params.facetFilters));
  }
  if (params.filters != null) echoed.set("filters", params.filters);
  if (params.highlightPreTag != null) echoed.set("highlightPreTag", params.highlightPreTag);
  if (params.highlightPostTag != null) echoed.set("highlightPostTag", params.highlightPostTag);
  return echoed.toString();
}

function toInstantSearchResult(
  list: ListResponse<InstantSearchRecord>,
  request: RequiredRequest,
  options: CreateInstantSearchClientOptions,
  processingTimeMS: number,
): InstantSearchResult {
  const result: InstantSearchResult = {
    hits: list.items.map((item) =>
      toInstantSearchHit(item, options.objectIDField, request),
    ),
    page: list.page - 1,
    nbHits: list.totalItems,
    nbPages: list.totalPages,
    hitsPerPage: list.perPage,
    processingTimeMS,
    query: request.query,
    params: request.echoedParams,
    exhaustiveNbHits: true,
  };
  if (list.facets) result.facets = mapFacets(list.facets);
  return result;
}

function toInstantSearchHit(
  item: InstantSearchRecord,
  objectIDField: string,
  request: RequiredRequest,
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
  request: RequiredRequest,
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
): string {
  return value.split("<b>").join(highlightPreTag).split("</b>").join(highlightPostTag);
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
