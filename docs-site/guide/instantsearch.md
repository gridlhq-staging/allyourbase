<!-- audited 2026-06-09 -->
# InstantSearch

AYB ships a JavaScript InstantSearch adapter at `@allyourbase/js/instantsearch`. It lets an existing `instantsearch.js` or `react-instantsearch` UI point at AYB's collection list/search path instead of Algolia's hosted index API.

This guide owns the adapter contract. For the underlying backend search behavior, use [Search](/guide/search). For the migration map from Algolia concepts to AYB concepts, use [Migrating from Algolia](/guide/migrating-from-algolia).

## Install

```bash
npm install @allyourbase/js react-instantsearch
```

## Create the search client

```ts
import { AYBClient } from "@allyourbase/js";
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";

const ayb = new AYBClient("http://127.0.0.1:8090");

const searchClient = createInstantSearchClient({
  client: ayb,
  objectIDField: "slug",
  defaultIndexName: "products",
  disjunctiveFacets: ["category"],
});
```

`objectIDField` is required. AYB rows are arbitrary PostgreSQL records, so the adapter fails closed if a returned hit is missing that field or if the value is `null`.
`disjunctiveFacets` is optional. Use it for facets that should keep their full bucket counts while a user multi-selects values in the same facet.

## Minimal React wiring

```tsx
import {
  Configure,
  Highlight,
  Hits,
  InstantSearch,
  Pagination,
  RangeInput,
  RefinementList,
  SearchBox,
  Stats,
} from "react-instantsearch";

function ProductHit({ hit }) {
  return (
    <article>
      <h2>
        <Highlight attribute="title" hit={hit} />
      </h2>
      <p>
        <Highlight attribute="description" hit={hit} />
      </p>
      <small>{hit.category}</small>
    </article>
  );
}

export function ProductSearch({ searchClient }) {
  return (
    <InstantSearch searchClient={searchClient} indexName="products">
      <Configure hitsPerPage={6} facets={["category", "price_cents"]} />
      <SearchBox placeholder="Search products" />
      <Stats />
      <RefinementList attribute="category" operator="or" />
      <RangeInput attribute="price_cents" />
      <Hits hitComponent={ProductHit} />
      <Pagination />
    </InstantSearch>
  );
}
```

The adapter does not add a second transport. It calls `client.records.list()` and maps AYB's list response into the InstantSearch `search(requests)` result shape, and delegates `searchForFacetValues(requests)` through `client.records.searchFacetValues()`. This is the shipped one-index adapter contract; `records.list` remains the transport owner for `search`, and `records.searchFacetValues` is the transport owner for facet-value search.

## Supported request mapping

The adapter supports one-index `search(requests)` calls with these fields:

- `indexName` -> AYB collection/table name
- `params.query` -> `search`
- `params.page` -> AYB `page` after zero-based `page` to one-based conversion
- `params.hitsPerPage` -> `perPage`
- `params.facets` -> concrete attribute names in `facets`
- `params.disjunctiveFacets` -> `disjunctiveFacets`
- `params.facetFilters` -> AYB `filter` for the documented `attribute:value` subset
- `params.numericFilters` -> AYB range comparisons for the documented `attribute>=number` / `attribute<=number` subset
- `params.filters` -> AYB `filter` for the documented comparison subset
- `params.highlightPreTag` and `params.highlightPostTag` -> accepted only as the InstantSearch placeholder pair (`__ais-highlight__` / `__/ais-highlight__`) or the equivalent `<mark>` / `</mark>` pair; when omitted, the adapter emits InstantSearch's default highlight placeholders

Response mapping keeps the AYB row fields, adds `objectID`, passes through `_highlightResult`, performs facet-map transposition from AYB bucket arrays into InstantSearch count maps, maps AYB `facetStats` to both `facetStats` and `facets_stats`, emits `disjunctiveFacets` entries for configured OR facets, and keeps params echoing on each result for widget compatibility.

## Widget and parameter matrix

Supported and proven in the shipped example:

- `SearchBox`
- `Hits`
- `Highlight`
- `RefinementList`
- `RangeInput`
- `Pagination`
- `Stats`
- Multi-select OR facets through `disjunctiveFacets`
- Numeric range refinements through `numericFilters`
- Empty-query browsing with facets on first render
- Searchable facet values through `searchClient.searchForFacetValues(requests)` (for example, `RefinementList` with `searchable`)

Supported request parameters:

- `query`
- `page`
- `hitsPerPage`
- `facets`
- `disjunctiveFacets`
- `facetFilters`
- `numericFilters`
- `filters`
- `highlightPreTag`
- `highlightPostTag`

## Disjunctive facets and numeric range refinements

InstantSearch widgets such as `RefinementList` with `operator="or"` expect a selected facet to keep showing bucket counts as if that facet's own predicate were removed. Configure those attributes as `disjunctiveFacets`; AYB computes those counts through the backend `disjunctiveFacets` request parameter.

`RangeInput` emits `numericFilters`. The adapter supports range comparisons (`>=`, `>`, `<=`, `<`) and combines them with facet filters into AYB's `filter` expression. Backend numeric bounds are returned as `facetStats`; the adapter exposes them under both `facetStats` and InstantSearch's `facets_stats` name for widget compatibility.

## Searchable facet values

`searchClient.searchForFacetValues(requests)` is supported for live searchable facet values lookups, and delegates each request through `client.records.searchFacetValues(collection, facetName, params)`, which calls `GET /api/collections/{table}/facets/{column}/search` (see [Search](/guide/search#search-facet-values) for the backend contract). `facetName` must reference a text facet column.

```ts
const results = await searchClient.searchForFacetValues([
  {
    indexName: "products",
    params: {
      facetName: "category",
      facetQuery: "st",
      maxFacetHits: 10,
    },
  },
]);
const [{ facetHits, exhaustiveFacetsCount }] = results;
```

Supported per-request params: `facetName`, `facetQuery`, `query`, `maxFacetHits`, `facetFilters`, `numericFilters`, `filters`, `highlightPreTag`, `highlightPostTag`. `maxFacetHits` defaults to 10 and is capped at 100. The same one-index adapter boundary applies; mixed-index requests still fail closed before AYB is called.

Each `facetHits` entry has the Algolia shape: `value`, `highlighted`, `count`. The backend wraps the matched prefix in `<mark>...</mark>`, and the adapter remaps those wrappers onto the caller's `highlightPreTag`/`highlightPostTag` (defaulting to the InstantSearch `__ais-highlight__` placeholders). `exhaustiveFacetsCount` is forwarded from the backend response.

## Current boundaries

Unsupported cases fail closed before AYB is called:

- federated/multi-index search and other mixed-index requests
- wildcard facets such as `["*"]`
- negative `facetFilters`
- custom highlight tags outside InstantSearch placeholders or `<mark>` wrappers
- nested attributes, `_tags`, `NOT`, arrays, equality-style numeric filters, and numeric filter OR groups deeper than one level
- vector-mode query parameters

Not yet supported on this path:

- rules or merchandising
- analytics events
- personalization

## Empty queries

The adapter intentionally keeps InstantSearch's initial empty query as a real AYB list request. It omits the `search` parameter and still requests the configured facets, so a first render can show browsable results and `RefinementList` counts without a separate bootstrap call.

## Example app

The source-only reference app lives at `examples/instantsearch_demo/`. It runs on `http://127.0.0.1:8096`, points at a local AYB server on `http://127.0.0.1:8090`, and uses `react-instantsearch` widgets against the shipped adapter. Its browser-unmocked proof is `examples/instantsearch_demo/browser-tests-unmocked/smoke/search.spec.ts`.

## Related guides

- [Search](/guide/search)
- [Migrating from Algolia](/guide/migrating-from-algolia)
- [JavaScript SDK](/guide/javascript-sdk)
