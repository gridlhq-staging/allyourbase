<!-- audited 2026-06-04 -->
# Search

AYB's collection list endpoints support full-text search, collection-scoped synonym expansion, typo-tolerant fuzzy matching, filters, highlighting, facet counts, and hybrid text/vector search on the same request path. This guide owns the behavioral search contract. For the compact query-parameter table and response field reference, see [REST API Reference](/guide/api-reference).

It is based on:

- `internal/api/handler_list.go`, `internal/api/handler_list_parse.go`, `internal/api/search.go`
- `internal/api/aggregate.go`, `internal/api/handler_list_facets.go`
- `ui/src/api_search.ts`, `ui/src/components/Search.tsx`, `ui/src/types/common.ts`
- Tests: `internal/api/search_test.go`, `internal/api/integration_rls_test.go`, `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`
- Screen spec: `docs/reference/screen_specs/search_playground.md`

## What search runs on

All examples in this guide use the standard collection list endpoint:

```text
GET /api/collections/{table}
```

The standard text search surface is:

- `search=<text>` for PostgreSQL full-text search with stemming enabled by default
- collection-scoped synonym expansion configured by admins
- `fuzzy=true` for `pg_trgm` typo tolerance
- `typo_threshold=<0..1>` to tune fuzzy matching when `fuzzy=true`
- `highlight=true` to request legacy `_highlight` snippets and `_highlightResult` metadata on matching rows
- `filter=<expr>` for safe predicate narrowing
- `facets=col_a,col_b` for facet buckets in the same response
- `disjunctiveFacets=col_a` to compute a facet's counts with its own equality predicate removed, which supports OR/multi-select facet UIs
- numeric facet stats (`facetStats`) for `min` / `max` bounds on requested numeric facets

Vector and hybrid search are documented separately in [AI and Vector Search](/guide/ai-vector).

## Full-text search

AYB searches across the table's text columns using PostgreSQL `websearch_to_tsquery`, so phrase search, `or`, and term exclusion work the same way they do in the REST reference. The default text-search configuration is English, so normal stemming is enabled by default: searches for terms such as `running` can match rows containing the same stem, such as `run`.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

You can combine search with filters and pagination:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres&filter=status='published'&perPage=10" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{totalItems, items}'
```

## Per-attribute relevance weighting

By default, AYB gives all searchable text columns equal weight. Collections with no search settings configured keep that existing behavior.

Admins can opt into per-attribute weighting through the admin API:

```text
GET /api/collections/{table}/search-settings
PUT /api/collections/{table}/search-settings
```

The request and response body shape is:

```json
{
  "attributes": [
    { "column": "title", "weight": "high" },
    { "column": "body", "weight": "low" }
  ]
}
```

An unset collection returns the same shape with an empty array:

```json
{
  "attributes": []
}
```

Example update:

```bash
curl -s -X PUT "http://127.0.0.1:8090/api/collections/posts/search-settings" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"attributes":[{"column":"title","weight":"high"},{"column":"body","weight":"low"}]}' | jq
```

AYB maps the configured weights onto PostgreSQL's weighted full-text labels and ranks matches with `ts_rank_cd`:

- `high` -> `A`
- `medium` -> `B`
- `low` -> `C`
- `lowest` -> `D`

Configured attributes use their mapped weight. Any other searchable text columns remain searchable and are appended at weight `D`, so those columns still match but rank behind configured attributes.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts/search-settings" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" | jq
```

A dashboard editor for search settings is coming; for now configure via the admin API.

## Custom ranking (secondary sort)

Admins can persist a `customRanking` chain on the same search-settings endpoint used for per-attribute relevance weighting:

```text
GET /api/collections/{table}/search-settings
PUT /api/collections/{table}/search-settings
```

Each `customRanking` entry names a rankable column and an `order` of `asc` or `desc`. The request and response keep the same `attributes` array from the relevance-weighting section and add `customRanking` beside it:

```json
{
  "attributes": [
    { "column": "title", "weight": "high" },
    { "column": "body", "weight": "low" }
  ],
  "customRanking": [
    { "column": "popularity", "order": "desc" },
    { "column": "published_at", "order": "asc" }
  ]
}
```

Example update:

```bash
curl -s -X PUT "http://127.0.0.1:8090/api/collections/posts/search-settings" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"attributes":[{"column":"title","weight":"high"},{"column":"body","weight":"low"}],"customRanking":[{"column":"popularity","order":"desc"},{"column":"published_at","order":"asc"}]}' | jq
```

Persisted `customRanking` is the default tie-break after `ts_rank_cd` relevance when the request has no `sort` parameter. Search relevance remains primary; the persisted chain only orders rows that have the same full-text rank.

This request uses the persisted default:

```text
GET /api/collections/{table}?search=<term>
```

Example:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

An explicit `?sort=` suppresses the persisted `customRanking` chain for that request only. Use it when a single list request needs a different tie-break while keeping the saved collection default for later searches:

```text
GET /api/collections/{table}?search=<term>&sort=published_at
```

Example:

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres&sort=published_at" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

For this beta, searchable-attribute weighting and custom ranking are API-only. Geo ranking, `distinct`, and query-rules-style behavior remain unsupported; see [Beta Limitations](/guide/beta-limitations) for the current boundary list.

## Synonym expansion

Admins can configure synonym groups for one collection at a time. When a group exists, AYB expands matching search terms before evaluating the normal full-text predicate. A row that stores `science fiction` can therefore match a caller searching for `scifi` when that collection has a `["scifi", "science fiction"]` group.

Synonyms do not add a separate search endpoint. Search consumers still use:

```text
GET /api/collections/{table}?search=<text>
```

The JavaScript SDK keeps using `records.list`:

```ts
const response = await ayb.records.list("posts", {
  search: "scifi",
});
```

Admin setup and replacement semantics are documented in [Search Synonyms](/guide/synonyms).

Hybrid search with `search=<text>&semantic=true` uses the same full-text search builder for its text leg, so configured synonym groups also expand that text leg. Hybrid results are ranked by the fused text/vector rank and then paginated from the fused set. The vector leg and fusion rules remain documented in [AI and Vector Search](/guide/ai-vector).

## Fuzzy matching

`fuzzy=true` adds typo-tolerant matching on top of a non-empty `search` value. AYB keeps the normal full-text predicate and adds trigram similarity checks for the searched terms. This requires the PostgreSQL `pg_trgm` extension to be available.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postres&fuzzy=true" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items'
```

If `pg_trgm` is unavailable, AYB fails closed with a `400` and an explanatory message. `fuzzy` requires `search`, and invalid `fuzzy` values are rejected. The [REST API Reference](/guide/api-reference) owns the parameter boundary details.

`typo_threshold` tunes the trigram threshold AYB uses for fuzzy matches. It
must be a number between `0` and `1`, and the backend rejects it unless
`fuzzy=true` is also present.

## Highlighting

`highlight=true` asks AYB to return both highlight response fields on matching
items:

- `_highlight`: the legacy combined excerpt string.
- `_highlightResult`: a map keyed by searchable attribute. Each entry contains
  `value`, the HTML-escaped highlighted attribute value, and `matchLevel`, which
  is `full` when that attribute matched the query and `none` otherwise.

AYB HTML-escapes the source text before `ts_headline` inserts `<b>` and `</b>`,
so the only HTML the server adds is those bold tags around matched terms.

```bash
curl -s "http://127.0.0.1:8090/api/collections/posts?search=postgres&highlight=true" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '.items[0] | {title, _highlight, _highlightResult}'
```

Example item shape:

```json
{
  "id": 1,
  "title": "Postgres guide",
  "_highlight": "<b>Postgres</b> guide",
  "_highlightResult": {
    "title": {
      "value": "<b>Postgres</b> guide",
      "matchLevel": "full"
    },
    "body": {
      "value": "Database notes",
      "matchLevel": "none"
    }
  }
}
```

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

Numeric facets also return `facetStats` with exact `min` and `max` bounds for range widgets:

```json
{
  "facetStats": {
    "price_cents": { "min": 799, "max": 8999 }
  }
}
```

For multi-select OR facets, include the same column in `facets` and `disjunctiveFacets`. AYB evaluates the page query with the full filter, then evaluates that facet's buckets with the facet's own equality predicate removed:

```bash
curl -s "http://127.0.0.1:8090/api/collections/products?filter=category='books'&facets=category,brand&disjunctiveFacets=category" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{facets, facetStats}'
```

## Search facet values

For searchable facet widgets (large category lists where the user types into the
facet itself), use the dedicated facet-value search endpoint:

```text
GET /api/collections/{table}/facets/{column}/search
```

This is the owner of facet-value bucket search and is separate from the list
endpoint's `facets=` per-value counts described above. It returns one column's
buckets, optionally filtered by a typed prefix, and scoped to the same
visibility predicates as the list endpoint. This path is for text facet columns:
AYB rejects non-text columns up front, and `NULL` facet values are omitted from
`facetHits`.

Supported query parameters:

- `q=<text>` — case-insensitive prefix match against the facet column. LIKE
  metacharacters in `q` are escaped, so `q=50%` matches a literal `50%` prefix.
- `search=<text>` — scopes the buckets to the same full-text search predicate
  the list endpoint accepts, so values not present in the search result set are
  filtered out.
- `filter=<expr>` — scopes the buckets to the same filter predicate the list
  endpoint accepts.
- `maxFacetHits=<n>` — caps the returned `facetHits` array. It defaults to
  `10`, must be greater than `0`, and cannot exceed `100`.

Each `facetHits` entry returns:

- `value` — the non-null text bucket value as stored.
- `highlighted` — the same value with the matched prefix wrapped in literal
  `<mark>...</mark>` tags by the backend. When `q` is empty, no `<mark>` tags
  are added.
- `count` — the exact row count for that bucket inside the current
  `search`/`filter`-scoped result set.

The envelope also carries `exhaustiveFacetsCount`, which is `false` only when
the backend truncated the result set at `maxFacetHits`.

```bash
curl -s "http://127.0.0.1:8090/api/collections/products/facets/category/search?q=st&maxFacetHits=20" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq
```

Example response:

```json
{
  "facetHits": [
    { "value": "Stationery", "highlighted": "<mark>St</mark>ationery", "count": 3 }
  ],
  "exhaustiveFacetsCount": true
}
```

Counts respect row-level security the same way the rest of search and faceting
do; two users running the same `q`/`search`/`filter` combination can see
different `count` values when their RLS policies expose different rows.

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
- each `facetStats.<column>.min` / `facetStats.<column>.max`

## JavaScript SDK

The JavaScript SDK wires the same query params through `records.list`.

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://127.0.0.1:8090");

const response = await ayb.records.list("posts", {
  search: "postgres",
  fuzzy: true,
  typoThreshold: 0.3,
  highlight: true,
  filter: "status='published'",
  facets: ["status", "category"],
  disjunctiveFacets: ["category"],
  perPage: 10,
});

console.log(response.items);
console.log(response.items[0]?._highlight);
console.log(response.items[0]?._highlightResult?.title);
console.log(response.facets?.status);
console.log(response.facetStats);
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

Facets, fuzzy matching, typo-threshold tuning, and highlight metadata are part of AYB's non-vector list/search path. They are rejected on vector list modes such as:

- `nearest=[...]`
- `semantic_query=<text>`
- `search=<text>&semantic=true`

Use [AI and Vector Search](/guide/ai-vector) for those query modes and their compatibility rules.

## Related guides

- [REST API Reference](/guide/api-reference)
- [JavaScript SDK](/guide/javascript-sdk)
- [Search Synonyms](/guide/synonyms)
- [Authentication](/guide/authentication)
- [AI and Vector Search](/guide/ai-vector)
