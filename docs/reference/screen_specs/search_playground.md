# Search Playground

## Task

Run text search and filter queries against one collection, then narrow the same result set by verified facet buckets.

## Layout

1. Page header: `Search` title and short helper text for querying collection records.
2. Query controls: collection picker, results-per-page input, search query field, filter expression field, fuzzy matching toggle, and `Search` submit button. These remain the existing screen skeleton owned by `ui/src/components/Search.tsx:441-543`.
3. Facet controls: compact selector for eligible scalar columns in the selected collection, hidden when no eligible columns exist. Eligibility is computed in the UI from `Column.type`, `Column.jsonType`, and `enumValues` in `ui/src/types/schema.ts:20-29`; this spec does not introduce a backend `facetable` flag or second schema metadata contract.
4. Facet bucket panel: shown only after the current list response includes facet data. Each selected facet column shows buckets with exact `value` and `count` from the shared list response.
5. Error banner: same placement and retry action as the existing search screen.
6. Relevance score panel: shown above the results grid after a text search is applied. Lists one entry per returned row, labelled `Result <n>` in backend result order, with that row's backend relevance score. Owned by `ui/src/components/SearchRankResults.tsx`; it never adds a `_rank` column to the results grid.
7. Results grid: existing `TableBrowserGrid` result area remains below the query and facet controls.

## State contract

### Loading
- Query controls remain visible but the results grid shows its existing loading state through `TableBrowserGrid`.
- Facet buckets from a previous response are not shown while a new request is in flight.

### Error
- Show the existing red error banner with message and `Retry` button.
- `Retry` reuses the current applied search, filter, fuzzy, per-page, and selected facet parameters.

### Empty eligibility
- When the selected collection has no eligible scalar columns, the facet selector and bucket panel are hidden.
- Search, filter, fuzzy, per-page, and results grid behavior is unchanged.

### Populated facets
- The facet selector lists eligible scalar columns only. Text, enum, numeric, and boolean-like scalar columns qualify; JSON, array, vector, spatial, raster, and other object-shaped columns do not.
- The request uses the canonical list endpoint syntax documented in `docs-site/guide/api-reference.md:37-167`, including `search`, `filter`, `fuzzy`, `perPage`, and `facets`.
- Bucket counts render exactly as returned by the backend envelope: `facets.<column>[]` entries with `{ value, count }`, matching `internal/api/response.go:14-40` and `internal/api/handler_list_facets.go:12-49`.
- Clicking a non-null bucket rewrites the existing `filter` and `appliedFilter` owners, then reuses the same submit/fetch path owned by `ui/src/components/Search.tsx:333-399`. It must not create a second narrowing state or parallel query model.
- The generated filter expression replaces any pre-existing filter string. String and enum values are single-quoted, numeric and boolean values are unquoted, and all expressions follow the documented filter syntax.

### Null bucket
- Buckets whose `value` is `null` are displayed with their count and a neutral label.
- Null buckets are not clickable in this target flow, so they never rewrite the filter field.

### Populated relevance scores
- After a non-empty text search is applied, each returned row's synthetic `_rank` value from the list response renders in the relevance panel at that row's position, formatted to four significant digits.
- The panel never reorders rows or scores. Backend list ordering is the single owner of relevance order, so panel entry `Result <n>` always describes the nth row in the response.
- Rows whose `_rank` is missing or not a finite number contribute no panel entry; the panel is hidden when no row yields a score.
- The synthetic `_rank` field is stripped from the rows handed to the results grid, so it never appears as a grid column.

### Real `_rank` column
- When the selected collection declares its own `_rank` column, the backend does not emit a synthetic score, the relevance panel is hidden, and the user's `_rank` values stay in the results grid untouched with their own column header.
- The API client passes `_rank` through without narrowing or validation, so a user-owned column of any type is never rejected or reinterpreted as a score.

### No applied search
- With no submitted text search, the relevance panel is hidden regardless of what the list response contains.

### Empty results
- If a submitted search or filter returns no rows, show the existing empty-results panel: `No results matched this search` plus the adjustment hint.
- If facets are requested on an empty result set, show no clickable buckets and keep the results empty state visible.

## Navigation

- Route: dashboard `Search` screen.
- Entry: dashboard navigation item for `Search`.
- Back: browser or shell back behavior remains unchanged; query state is not URL-driven in this spec.
- Search: stays on `Search Playground` and refreshes the shared list response.

## Acceptance criteria

- Given a selected collection with no eligible scalar columns, when the screen renders, then the facet selector and bucket panel are hidden. Evidence: `ui/src/components/__tests__/Search.test.tsx`.
- Given a selected collection with eligible scalar columns, when the user selects `status` as a facet and submits `search=post`, then the request uses the shared list endpoint with `facets=status`. Evidence: `ui/src/components/__tests__/Search.test.tsx`; `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`.
- Given a list response containing `facets.status`, when buckets render, then each visible bucket count equals the returned `count` value. Evidence: `ui/src/components/__tests__/Search.test.tsx`; `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`.
- Given a bucket whose value is `null`, when buckets render, then that bucket is visible with its count but cannot be clicked. Evidence: `ui/src/components/__tests__/Search.test.tsx`.
- Given an existing filter string and a non-null facet bucket click, when narrowing runs, then the existing filter string is replaced with one valid filter expression for that bucket. Evidence: `ui/src/components/__tests__/Search.test.tsx`; `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`.
- Given a facet bucket click, when results refresh, then the results table narrows through the same list endpoint rather than a special search-only path. Evidence: `ui/src/components/__tests__/Search.test.tsx`; `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`.
- Given an applied text search against a collection without a `_rank` column, when results render, then each returned row's backend relevance score is shown in the relevance panel at that row's position, in non-increasing order, and no `_rank` grid column appears. Evidence: `ui/src/components/__tests__/Search.test.tsx`; `ui/browser-tests-unmocked/full/search-playground-journey.spec.ts`.
- Given a collection that owns a real `_rank` column, when an applied text search renders, then the relevance panel is hidden and the stored `_rank` values remain visible in the results grid. Evidence: `ui/src/components/__tests__/Search.test.tsx`.

## Edge cases

- No collections: keep the existing no-collections panel and show no facet controls.
- No eligible scalar columns: hide facet controls instead of showing disabled empty chrome.
- Empty result set with requested facets: show the empty-results panel and no clickable buckets.
- Null facet values: display counts but keep the bucket non-clickable.
- Backend rejects a requested facet column: show the existing error banner and retry action.

- Shipped facet behavior: `ui/src/components/Search.tsx:545-620` renders the eligible-column selector and bucket panel above the results grid, `ui/src/api_search.ts` sends selected columns through the canonical `facets` query parameter and normalizes returned `facets`, and the backend response envelope is owned by `internal/api/response.go`.

## Current implementation gaps

- Current: Relevance scores render in a panel above the grid, keyed to rows by a `Result <n>` label rather than sitting on the matching grid row.
- Target: Each result row shows its own relevance score inline, so score-to-row association needs no positional counting.
- Evidence: `ui/src/components/Search.tsx` renders `<SearchRankResults>` above `<TableBrowserGrid>`, and `ui/src/components/SearchRankResults.tsx` labels entries by result position; grid columns are owned by `ui/src/components/TableBrowserGrid.tsx`, which this surface does not extend.
