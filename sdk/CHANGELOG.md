# Changelog

## 0.2.0 (2026-06-03)

Release metadata refresh for the existing SDK public surface.

- Records search: documents the shipped `ListParams` search options `search`, `fuzzy`, `typoThreshold`, boolean `highlight` (`highlight: true` returns `_highlight` snippets), `facets`, `semantic`, `semanticQuery`, `nearest`, `vectorColumn`, and `distance`, plus the `SearchHit` highlight type and `FacetCounts` response envelope.
- InstantSearch: adds the `@allyourbase/js/instantsearch` subpath with `createInstantSearchClient`, a thin `records.list` adapter that requires `objectIDField`, preserves empty query browsing, supports concrete facets, `disjunctiveFacets`, `numericFilters` range comparisons, `facetStats` / `facets_stats`, plus the documented `facetFilters` and `filters` subset, and rejects unsupported cases such as mixed indexes, wildcard facets, negative facet filters, malformed numeric filters, and nested/tag filters. Searchable facet values are supported through `searchClient.searchForFacetValues(requests)`, which delegates to `client.records.searchFacetValues(collection, column, params)` against the new `GET /api/collections/{table}/facets/{column}/search` backend endpoint and returns Algolia-shaped `{ facetHits, exhaustiveFacetsCount, processingTimeMS }`.
- Passkeys: documents the existing WebAuthn entry points `beginWebAuthnLogin`, `finishWebAuthnLogin`, `signInWithPasskey`, `enrollPasskey`, and `verifyPasskey`.
- TypeScript: confirms the public barrel exports canonical auth, record, storage, realtime, OAuth, admin, RPC, GraphQL, search, and WebAuthn types from `sdk/src/index.ts`.

## 0.1.0 (2026-02-07)

Initial release.

- Records: list, get, create, update, delete with filtering, sorting, pagination, FK expansion
- Auth: register, login, refresh, logout, me, password reset, email verification
- Storage: upload, download URL, delete
- Realtime: SSE subscriptions with table filtering
- TypeScript types for all API responses
- Zero runtime dependencies
