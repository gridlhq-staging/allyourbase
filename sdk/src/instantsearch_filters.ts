export type FacetFilterInput = string | Array<string | string[]>;
export type NumericFilterInput = string | Array<string | string[]>;

export interface InstantSearchSearchParams {
  query?: string;
  page?: number;
  hitsPerPage?: number;
  facets?: string[] | string;
  disjunctiveFacets?: string[];
  facetFilters?: FacetFilterInput;
  numericFilters?: NumericFilterInput;
  filters?: string;
  analytics?: boolean;
  clickAnalytics?: boolean;
  highlightPreTag?: string;
  highlightPostTag?: string;
  [key: string]: unknown;
}

export interface TranslatedInstantSearchRequest {
  query: string;
  page: number;
  hitsPerPage?: number;
  facets?: string[];
  disjunctiveFacets?: string[];
  filter?: string;
  highlightPreTag?: string;
  highlightPostTag?: string;
  echoedParams: string;
}

export interface InstantSearchTranslationOptions {
  disjunctiveFacets?: string[];
}

export const DEFAULT_INSTANTSEARCH_HIGHLIGHT_PRE_TAG = "__ais-highlight__";
export const DEFAULT_INSTANTSEARCH_HIGHLIGHT_POST_TAG = "__/ais-highlight__";
export const SAFE_HTML_HIGHLIGHT_PRE_TAG = "<mark>";
export const SAFE_HTML_HIGHLIGHT_POST_TAG = "</mark>";

const SUPPORTED_PARAM_KEYS = new Set([
  "query",
  "page",
  "hitsPerPage",
  "facets",
  "disjunctiveFacets",
  "facetFilters",
  "numericFilters",
  "filters",
  "analytics",
  "clickAnalytics",
  "highlightPreTag",
  "highlightPostTag",
]);

interface ParsedFacetFilter {
  attribute: string;
  filter: string;
}

export function validateFacetArray(facets: unknown): void {
  if (facets == null) return;
  if (!Array.isArray(facets) || facets.some((facet) => typeof facet !== "string")) {
    throw new Error("facets must be an array of concrete attribute names");
  }
  if (facets.includes("*")) {
    throw new Error('wildcard facets ["*"] are not supported');
  }
}

export function validateHighlightTags(params: {
  highlightPreTag?: string;
  highlightPostTag?: string;
}): void {
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

export function translateRequest(
  params: InstantSearchSearchParams | undefined,
  options: InstantSearchTranslationOptions,
): TranslatedInstantSearchRequest {
  const searchParams = params ?? {};
  rejectUnsupportedParams(searchParams);
  validatePage(searchParams.page);
  validateHitsPerPage(searchParams.hitsPerPage);
  const facets = normalizeFacets(searchParams.facets);
  const requestDisjunctiveFacets = normalizeFacets(searchParams.disjunctiveFacets);
  validateHighlightTags(searchParams);

  const disjunctiveFacets = mergeDisjunctiveFacets(
    mergeDisjunctiveFacets(options.disjunctiveFacets, requestDisjunctiveFacets),
    deriveDisjunctiveFacets(facets, searchParams.facetFilters),
  );

  return {
    query: searchParams.query ?? "",
    page: searchParams.page ?? 0,
    hitsPerPage: searchParams.hitsPerPage,
    facets,
    disjunctiveFacets,
    filter: combineFilters(
      searchParams.facetFilters,
      searchParams.numericFilters,
      searchParams.filters,
      disjunctiveFacets,
    ),
    highlightPreTag: searchParams.highlightPreTag,
    highlightPostTag: searchParams.highlightPostTag,
    echoedParams: buildEchoedParams(searchParams),
  };
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
  if (params.analytics != null && typeof params.analytics !== "boolean") {
    throw new Error("analytics must be a boolean");
  }
  if (params.clickAnalytics != null && typeof params.clickAnalytics !== "boolean") {
    throw new Error("clickAnalytics must be a boolean");
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
  if (!Number.isInteger(hitsPerPage) || Number(hitsPerPage) < 0) {
    throw new Error("hitsPerPage must be a non-negative integer");
  }
}

function normalizeFacets(facets: unknown): string[] | undefined {
  if (facets == null) return;
  const normalized = typeof facets === "string" ? [facets] : facets;
  validateFacetArray(normalized);
  return normalized as string[];
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

export function combineFilters(
  facetFilters: unknown,
  numericFilters: unknown,
  filters: unknown,
  disjunctiveFacets: string[] | undefined,
): string | undefined {
  const translated = [
    translateFacetFilters(facetFilters, disjunctiveFacets),
    translateNumericFilters(numericFilters),
    translateFilters(filters),
  ].filter((filter): filter is string => Boolean(filter));
  return translated.length ? translated.join(" AND ") : undefined;
}

function translateFacetFilters(
  input: unknown,
  disjunctiveFacets: string[] | undefined,
): string | undefined {
  if (input == null) return undefined;
  if (typeof input === "string") return translateFacetFilterValue(input);
  if (!Array.isArray(input)) {
    throw new Error("facetFilters must be a string or array");
  }
  const disjunctiveFacetSet = new Set(disjunctiveFacets ?? []);
  const groupedFilters = new Map<string, string[]>();
  const clauses = input.map((entry) => {
    if (typeof entry === "string") {
      const parsed = parseFacetFilter(entry);
      if (!disjunctiveFacetSet.has(parsed.attribute)) return parsed.filter;
      if (!groupedFilters.has(parsed.attribute)) groupedFilters.set(parsed.attribute, []);
      groupedFilters.get(parsed.attribute)?.push(parsed.filter);
      return { attribute: parsed.attribute };
    }
    if (!Array.isArray(entry)) {
      throw new Error("facetFilters entries must be strings or one-level arrays");
    }
    const group = validateFacetFilterGroup(entry);
    const orClauses = group.map((nested) => translateFacetFilterValue(nested));
    return orClauses.length === 1 ? orClauses[0] : `(${orClauses.join(" OR ")})`;
  });
  return clauses
    .map((clause) => {
      if (typeof clause === "string") return clause;
      const filters = groupedFilters.get(clause.attribute) ?? [];
      return filters.length === 1 ? filters[0] : `(${filters.join(" OR ")})`;
    })
    .filter((clause, index, allClauses) => allClauses.indexOf(clause) === index)
    .join(" AND ");
}

function translateFacetFilterValue(input: string): string {
  return parseFacetFilter(input).filter;
}

function parseFacetFilter(input: string): ParsedFacetFilter {
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
  return {
    attribute,
    filter: `${attribute}=${emitLiteral(value, "=")}`,
  };
}

function validateFacetFilterGroup(entry: unknown[]): string[] {
  if (entry.some((nested) => Array.isArray(nested))) {
    throw new Error("nested facetFilters deeper than one level are not supported");
  }
  if (entry.some((nested) => typeof nested !== "string")) {
    throw new Error("facetFilters arrays must contain strings");
  }
  if (entry.length === 0) {
    throw new Error("facetFilters OR groups must not be empty");
  }
  return entry as string[];
}

function deriveDisjunctiveFacets(
  facets: string[] | undefined,
  facetFilters: unknown,
): string[] | undefined {
  if (!facets?.length || !Array.isArray(facetFilters)) return undefined;

  const requestedFacets = new Set(facets);
  const disjunctiveFacets: string[] = [];
  for (const entry of facetFilters) {
    if (!Array.isArray(entry)) continue;
    const group = validateFacetFilterGroup(entry);
    for (const filter of group) {
      const attribute = parseFacetFilter(filter).attribute;
      if (requestedFacets.has(attribute) && !disjunctiveFacets.includes(attribute)) {
        disjunctiveFacets.push(attribute);
      }
    }
  }
  return disjunctiveFacets.length ? disjunctiveFacets : undefined;
}

function mergeDisjunctiveFacets(
  explicitFacets: string[] | undefined,
  derivedFacets: string[] | undefined,
): string[] | undefined {
  const merged = [...(explicitFacets ?? []), ...(derivedFacets ?? [])];
  return merged.length ? Array.from(new Set(merged)) : undefined;
}

function translateNumericFilters(input: unknown): string | undefined {
  if (input == null) return undefined;
  if (typeof input === "string") return translateNumericFilterValue(input);
  if (!Array.isArray(input)) {
    throw new Error("numericFilters must be a string or array");
  }
  const clauses = input.map((entry) => {
    if (typeof entry === "string") return translateNumericFilterValue(entry);
    if (!Array.isArray(entry)) {
      throw new Error("numericFilters entries must be strings or one-level arrays");
    }
    const group = validateNumericFilterGroup(entry);
    const orClauses = group.map((nested) => translateNumericFilterValue(nested));
    return orClauses.length === 1 ? orClauses[0] : `(${orClauses.join(" OR ")})`;
  });
  return clauses.join(" AND ");
}

function translateNumericFilterValue(input: string): string {
  const compact = input.replace(/\s+/g, "");
  const match = compact.match(/^([A-Za-z_][A-Za-z0-9_]*)(<=|>=|<|>)(-?\d+(\.\d+)?)$/);
  if (!match) {
    if (/^[A-Za-z_][A-Za-z0-9_]*(=|!=)/.test(compact)) {
      throw new Error("numericFilters must use range comparison operators");
    }
    throw new Error("numericFilters must use attribute<op>number form");
  }
  const [, attribute, operator, rawValue] = match;
  validateAttribute(attribute);
  return `${attribute}${operator}${emitNumericLiteral(rawValue)}`;
}

function validateNumericFilterGroup(entry: unknown[]): string[] {
  if (entry.some((nested) => Array.isArray(nested))) {
    throw new Error("nested numericFilters deeper than one level are not supported");
  }
  if (entry.some((nested) => typeof nested !== "string")) {
    throw new Error("numericFilters arrays must contain strings");
  }
  if (entry.length === 0) {
    throw new Error("numericFilters OR groups must not be empty");
  }
  return entry as string[];
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
  if (tokens[index + 3]?.toUpperCase() === "TO") {
    return parseNumericRangeComparison(attribute, operator, value, tokens[index + 4], index);
  }
  validateFilterOperator(operator);
  return {
    emitted: [`${attribute}${operator === ":" ? "=" : operator}${emitLiteral(value, operator)}`],
    nextIndex: index + 3,
  };
}

function parseNumericRangeComparison(
  attribute: string,
  operator: string,
  lowerBoundRaw: string,
  upperBoundRaw: string | undefined,
  index: number,
): ParsedFilterExpression {
  if (operator !== ":" || upperBoundRaw == null) {
    throw new Error("numeric range filters must use attribute:value TO value");
  }
  return {
    emitted: [
      `(${attribute}>=${emitNumericLiteral(lowerBoundRaw)}`,
      "AND",
      `${attribute}<=${emitNumericLiteral(upperBoundRaw)})`,
    ],
    nextIndex: index + 5,
  };
}

function throwMalformedBooleanFilter(): never {
  throw new Error("malformed boolean filters are not supported");
}

function rejectUnsupportedFilterSyntax(filter: string): void {
  if (/\bNOT\b/i.test(filter)) throw new Error("NOT filters are not supported");
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

function emitNumericLiteral(rawValue: string): string {
  const value = stripQuotes(rawValue);
  if (!/^-?\d+(\.\d+)?$/.test(value)) {
    throw new Error("numeric range filters require numeric bounds");
  }
  return value;
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
  if (params.disjunctiveFacets != null) {
    echoed.set("disjunctiveFacets", JSON.stringify(params.disjunctiveFacets));
  }
  if (params.facetFilters != null) {
    echoed.set("facetFilters", JSON.stringify(params.facetFilters));
  }
  if (params.numericFilters != null) {
    echoed.set("numericFilters", JSON.stringify(params.numericFilters));
  }
  if (params.filters != null) echoed.set("filters", params.filters);
  if (params.highlightPreTag != null) echoed.set("highlightPreTag", params.highlightPreTag);
  if (params.highlightPostTag != null) echoed.set("highlightPostTag", params.highlightPostTag);
  return echoed.toString();
}
