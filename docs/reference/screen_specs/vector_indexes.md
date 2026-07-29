# Vector Indexes

## Task

Inspect vector indexes and create new HNSW or IVFFlat indexes for vector columns.

## Layout

1. Header with `Vector Indexes` title and `Create Index` action.
2. Inline `New Vector Index` panel when creating an index.
3. Vector indexes table with `Name`, `Schema`, `Table`, and `Method` columns.

## State contract

### Loading
- Before vector index data is available, keep the header visible and show `Loading...`.

### Error
- List failure shows the `Vector Indexes` title, the returned error message, and `Retry`; retry calls the existing vector-index refresh owner.
- Create failures keep the table and create panel mounted while surfacing the returned error.

### Index table
- Rows show index name, schema, table, and method.
- Empty results show `No vector indexes found`.

### Create index
- `Create Index` opens a named `New Vector Index` region.
- The form includes `Schema`, `Table`, `Column`, `Method`, `Metric`, and optional `Index Name`.
- `Method` supports hnsw and ivfflat.
- `Create` is disabled until table, column, method, and metric are present.
- Successful create closes the panel, resets the form, and refreshes the list.
- `Cancel` closes the panel and resets draft values.

## Navigation

- Route: `/admin/` with admin view `vector-indexes`.
- Entry: Select `Vector Indexes` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create action: stay on `Vector Indexes`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Vector Indexes`, then the `Vector Indexes` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/vector-indexes.spec.ts`, `ui/browser-tests-unmocked/full/vector-indexes-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a vector index exists, when the table renders, then its name, schema, table, and method are visible. Evidence: `ui/src/components/__tests__/VectorIndexes.test.tsx`; `ui/browser-tests-unmocked/smoke/vector-indexes.spec.ts`.
- Given `Create Index` is opened with required fields missing, then `Create` is disabled. Evidence: `ui/src/components/__tests__/VectorIndexes.test.tsx`.
- Given valid create values, when the user submits, then the create API receives schema, table, column, method, metric, and optional index name. Evidence: `ui/src/components/__tests__/VectorIndexes.test.tsx`.
- Given a real vector table exists, when the user creates an index through the UI, then the index appears in the list with the selected method. Evidence: `ui/browser-tests-unmocked/full/vector-indexes-lifecycle.spec.ts`.
- Given list loading fails, then the returned error message is visible below the heading. Evidence: `ui/src/components/__tests__/VectorIndexes.test.tsx`.
- Given the Vector Indexes page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Vector index endpoint unavailable: unmocked browser tests skip only explicit 404, 501, or 503 probes.
- pgvector unavailable in a Postgres environment: browser lifecycle proof skips after checking extension availability.
- Optional index name may be omitted; the API request should omit it rather than submit an empty value.

## Current implementation gaps
