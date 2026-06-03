export interface SqlResult {
  columns: string[];
  rows: unknown[][];
  rowCount: number;
  durationMs: number;
}

export type FacetBucketValue = string | number | boolean | null;

export interface FacetValueCount {
  value: FacetBucketValue;
  count: number;
}

export type FacetCounts = Record<string, FacetValueCount[]>;

export interface ListResponse {
  items: Record<string, unknown>[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
  facets?: FacetCounts;
}
