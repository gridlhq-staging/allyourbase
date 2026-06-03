<!-- audited 2026-06-03 -->
# Search

AYB's collection list endpoints support full-text search, typo-tolerant fuzzy matching, filters, and facet counts on the same request path. This guide covers the shipped non-vector collection search workflow. For the canonical query-parameter table and response field reference, see [REST API Reference](/guide/api-reference).

It is based on:

- `internal/api/handler_list.go`, `internal/api/handler_list_parse.go`, `internal/api/search.go`
- `internal/api/aggregate.go`, `internal/api/handler_list_facets.go`
- `ui/src/api_search.ts`, `ui/src/components/Search.tsx`, `ui/src/types/common.ts`
- Tests: `internal/api/search_test.go`, `internal/api/integration_rls_test.go`, `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`
- Screen spec: `docs/screen_specs/search_playground.md`

## What search runs on

All examples in this guide use the standard collection list endpoint:

```text
GET /api/collections/{table}
```

The non-vector search surface is:

- `search=<text>` for PostgreSQL full-text search
- `fuzzy=true` for `pg_trgm` typo tolerance
- `filter=<expr>` for safe predicate narrowing
- `facets=col_a,col_b` for facet buckets in the same response

Vector and hybrid search are documented separately in [AI and Vector Search](/guide/ai-vector).

## Full-text search

AYB searches across the table's text columns using PostgreSQL `websearch_to_tsquery`, so phrase search, `or`, and term exclusion work the same way they do in the REST reference.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

You can combine search with filters and pagination:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres&filter=status='published'&perPage=10" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{totalItems, items}'
```

## Fuzzy matching

`fuzzy=true` adds typo-tolerant matching on top of a non-empty `search` value. AYB keeps the normal full-text predicate and adds trigram similarity checks for the searched terms. This requires the PostgreSQL `pg_trgm` extension to be available.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postres&fuzzy=true" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

If `pg_trgm` is unavailable, AYB fails closed with a `400` and an explanatory message. `fuzzy` requires `search`, and invalid `fuzzy` values are rejected. The [REST API Reference](/guide/api-reference) owns the parameter boundary details.

## Facets

`facets` asks AYB to return grouped count buckets for one or more scalar columns alongside the normal list response. The backend accepts scalar facet columns and rejects object-shaped or spatial/vector columns such as JSON, arrays, geometry, geography, vector, and raster fields. The counts are scoped to the same search text, filter, and RLS visibility as the returned rows.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=post&facets=status,category" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.facets'
```

Example response shape:

```json
{
  "status": [
    { "value": "published", "count": 2 },
    { "value": "draft", "count": 1 },
    { "value": null, "count": 1 }
  ],
  "category": [
    { "value": "guides", "count": 2 },
    { "value": "ops", "count": 1 }
  ]
}
```

Buckets are returned exactly as the backend grouped them:

- `value` stays typed: string, number, boolean, or `null`
- `count` is the exact row count for that bucket inside the current result set
- `null` buckets are valid and indicate matching rows where that column is null

## Drill in with filters

The usual drill-in pattern is:

1. Run a search with `facets`.
2. Read a bucket from the response.
3. Reissue the same list request with a filter expression for that bucket.

String and enum buckets use single quotes:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=post&filter=status='published'&facets=status" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{totalItems, facets}'
```

Numeric and boolean buckets stay unquoted:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?filter=rank=1&facets=rank,published" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{totalItems, facets}'
```

## RLS behavior

Search and facets both respect row-level security because they run on the same scoped collection query path as normal record listing. Facet counts use the same parsed filter and search predicates as the list query, then execute through the same RLS-scoped request context. Two users can issue the same `search` or `facets` request and get different counts if their RLS policies expose different rows.

That applies to:

- returned `items`
- `totalItems` and `totalPages`
- each `facets.<column>[].count`

## JavaScript SDK

The JavaScript SDK wires the same query params through `records.list`.

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://127.0.0.1:8090");

const response = await ayb.records.list("posts", {
  search: "postgres",
  fuzzy: true,
  filter: "status='published'",
  facets: ["status", "category"],
  perPage: 10,
});

console.log(response.items);
console.log(response.facets?.status);
```

## Admin Search view

The admin dashboard includes a `Search` view that runs the same collection list endpoint through a UI:

- collection picker
- search text
- fuzzy toggle
- filter expression
- facet column selector
- facet buckets that rewrite the filter input when clicked
- null facet buckets that are displayed with their counts but are not clickable

That screen is a thin client over the same `GET /api/collections/{table}` contract shown above. It does not use a separate search-only endpoint.

## Vector boundary

Facets are part of AYB's non-vector list/search path. They are not supported on vector list modes such as:

- `nearest=[...]`
- `semantic_query=<text>`
- `search=<text>&semantic=true`

Use [AI and Vector Search](/guide/ai-vector) for those query modes and their compatibility rules.

## Related guides

- [REST API Reference](/guide/api-reference)
- [JavaScript SDK](/guide/javascript-sdk)
- [Authentication](/guide/authentication)
- [AI and Vector Search](/guide/ai-vector)
