/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun02_pm_4_search_facet_ui_docs/allyourbase_dev/ui/src/api_search.ts.
 */
import { request } from "./api_client";
import { asRecord } from "./lib/normalize";
import type {
  FacetBucketValue,
  FacetCounts,
  FacetValueCount,
  ListResponse,
} from "./types";

export interface SearchPlaygroundListParams {
  search?: string;
  fuzzy?: boolean;
  filter?: string;
  perPage?: number;
  facets?: string[];
}

export type SearchPlaygroundListResponse = ListResponse;

const FACET_COUNTS_SHAPE_ERROR =
  "Expected list response facets object with array buckets containing value and numeric count";

export function isSerializableFacetColumnName(facet: string): boolean {
  return facet.trim() !== "" && !facet.includes(",");
}

function collectionListPath(table: string, params: SearchPlaygroundListParams): string {
  const query = new URLSearchParams();
  const trimmedSearch = typeof params.search === "string" ? params.search.trim() : "";

  if (trimmedSearch !== "") {
    query.set("search", trimmedSearch);
  }
  if (typeof params.fuzzy === "boolean" && trimmedSearch !== "") {
    query.set("fuzzy", String(params.fuzzy));
  }
  if (typeof params.filter === "string" && params.filter !== "") {
    query.set("filter", params.filter);
  }
  if (Array.isArray(params.facets)) {
    const facets = params.facets
      .filter((facet): facet is string => typeof facet === "string")
      .map((facet) => facet.trim())
      .filter(isSerializableFacetColumnName);
    if (facets.length > 0) {
      query.set("facets", facets.join(","));
    }
  }
  if (typeof params.perPage === "number" && Number.isFinite(params.perPage)) {
    query.set("perPage", String(Math.trunc(params.perPage)));
  }

  const encodedTable = encodeURIComponent(table);
  const suffix = query.toString();
  return suffix === "" ? `/api/collections/${encodedTable}` : `/api/collections/${encodedTable}?${suffix}`;
}

function isIntegerNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value);
}

function isFacetBucketValue(value: unknown): value is FacetBucketValue {
  return (
    value === null ||
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  );
}

function normalizeFacetBucket(bucket: unknown): FacetValueCount {
  const record = asRecord(bucket);
  if (
    record === null ||
    !("value" in record) ||
    !isFacetBucketValue(record.value) ||
    !isIntegerNumber(record.count)
  ) {
    throw new Error(FACET_COUNTS_SHAPE_ERROR);
  }
  return {
    value: record.value,
    count: record.count,
  };
}

function normalizeFacetCounts(facets: unknown): FacetCounts {
  const record = asRecord(facets);
  if (record === null || Array.isArray(facets)) {
    throw new Error(FACET_COUNTS_SHAPE_ERROR);
  }

  const normalized: FacetCounts = {};
  for (const [column, buckets] of Object.entries(record)) {
    if (!Array.isArray(buckets)) {
      throw new Error(FACET_COUNTS_SHAPE_ERROR);
    }
    normalized[column] = buckets.map(normalizeFacetBucket);
  }
  return normalized;
}

function normalizeSearchPlaygroundListResponse(payload: unknown): SearchPlaygroundListResponse {
  const envelope = asRecord(payload);
  if (envelope === null) {
    throw new Error("Expected list response object");
  }
  if (
    !isIntegerNumber(envelope.page) ||
    !isIntegerNumber(envelope.perPage) ||
    !isIntegerNumber(envelope.totalItems) ||
    !isIntegerNumber(envelope.totalPages)
  ) {
    throw new Error("Expected integer list envelope fields: page, perPage, totalItems, totalPages");
  }
  if (!Array.isArray(envelope.items)) {
    throw new Error("Expected list response items array");
  }
  if (envelope.items.some((item) => asRecord(item) === null)) {
    throw new Error("Expected list response items to contain only objects");
  }

  const response: SearchPlaygroundListResponse = {
    page: envelope.page,
    perPage: envelope.perPage,
    totalItems: envelope.totalItems,
    totalPages: envelope.totalPages,
    items: envelope.items as Record<string, unknown>[],
  };
  if ("facets" in envelope) {
    response.facets = normalizeFacetCounts(envelope.facets);
  }

  return response;
}

export async function listSearchPlaygroundRecords(
  table: string,
  params: SearchPlaygroundListParams = {},
): Promise<SearchPlaygroundListResponse> {
  const payload = await request<unknown>(collectionListPath(table, params));
  return normalizeSearchPlaygroundListResponse(payload);
}
