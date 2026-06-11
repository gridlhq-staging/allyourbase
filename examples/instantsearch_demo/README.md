# AYB InstantSearch Demo

This source-only Vite example exercises the `@allyourbase/js/instantsearch` adapter against a local AYB server. It is not bundled into the `ayb demo` binary path.

## Setup

```bash
ayb start
ayb sql < schema.sql
ayb sql < seed.sql
npm install
npm run dev
```

The demo runs at `http://127.0.0.1:8096` and points at AYB on `http://127.0.0.1:8090` by default. Override the API URL when needed:

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

Keep this example source-only. Do not register it in `examples/embed.go` or `internal/cli/demo.go`; bundled demo ownership belongs to a later stage.
