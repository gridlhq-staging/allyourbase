# AYB InstantSearch Demo

Run the bundled demo with one command:

```bash
ayb demo instantsearch
```

The bundled path starts AYB, applies the schema and seed data, and serves the
pre-built frontend without requiring Node.js at runtime.

## Develop the frontend

For local frontend development, start AYB and Vite separately:

```bash
ayb start
ayb sql < schema.sql
ayb sql < seed.sql
npm ci
npm run dev
```

The demo runs at `http://127.0.0.1:5179` and points at AYB on `http://127.0.0.1:8090` by default. Override the API URL when needed:

```bash
VITE_AYB_URL=http://127.0.0.1:8092 npm run dev
```

Browser-unmocked validation uses the same local ports:

```bash
npm run lint:browser-tests
npm run test:browser-tests
```

## Scope

The widget tree supplies the `instantsearch_products` index name. The demo proves category facets, a numeric `price_cents` range filter, highlighting, and pagination through the shared `@allyourbase/js/instantsearch` adapter. `src/lib/ayb.ts` owns only the AYB base URL, `AYBClient` construction, and adapter options for `objectIDField` and highlighting.
