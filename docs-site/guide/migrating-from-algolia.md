<!-- audited 2026-06-04 -->

# Migrating from Algolia

This guide maps common Algolia search concepts to AYB's shipped PostgreSQL-backed collection list endpoint. It covers the non-vector search path that exists today: `search`, `fuzzy`, `filter`, and `facets` on `GET /api/collections/{table}` through either REST or the JavaScript SDK.

For the canonical AYB search behavior, examples, response shape, and RLS notes, use [Search](/guide/search). This page is a migration map, not a second search reference.

## Migration map

| Algolia concept | AYB path today |
| --- | --- |
| Index records | PostgreSQL table rows exposed through `GET /api/collections/{table}` |
| Query text | `search=<text>` on the collection list endpoint |
| Typo tolerance | `fuzzy=true` with a non-empty `search` value, backed by PostgreSQL `pg_trgm` |
| Typo-threshold tuning | `typoThreshold` in the JS SDK / `typo_threshold` in REST when `fuzzy=true` |
| Synonyms | Per-collection synonym groups configured through admin collection settings |
| Facets | `facets=column_a,column_b` for scalar column buckets in the list response |
| Filters | `filter=<expr>` using AYB's safe filter syntax |
| Highlight snippets | `highlight=true` returns `_highlight` in matching result items |
| SDK search request | `ayb.records.list("table", { search, fuzzy, typoThreshold, highlight, filter, facets })` |
| Result hits | `items` in the list response |
| Facet counts | `facets.<column>[]` buckets in the list response |

AYB search and facets run through the same scoped collection query path as normal record listing. Row-level security applies to returned `items`, totals, and facet counts.

## Query with the REST API

```bash
curl -s "http://127.0.0.1:8090/api/collections/products?search=keyboard&fuzzy=true&filter=status='active'&facets=brand,category" \
  -H "Authorization: Bearer $AYB_TOKEN" | jq '{items, facets}'
```

The request above uses the shipped non-vector list search surface:

- `search=keyboard` runs PostgreSQL full-text search across text columns.
- `fuzzy=true` adds typo-tolerant matching when `pg_trgm` is installed.
- `filter=status='active'` narrows the same list query.
- `facets=brand,category` returns scalar facet buckets scoped to the same search, filter, and RLS visibility.

## Query with the JavaScript SDK

```ts
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://127.0.0.1:8090");

const results = await ayb.records.list("products", {
  search: "keyboard",
  fuzzy: true,
  typoThreshold: 0.3,
  highlight: true,
  filter: "status='active'",
  facets: ["brand", "category"],
  perPage: 20,
});

console.log(results.items);
console.log(results.items[0]?._highlight);
console.log(results.facets?.brand);
```

The SDK forwards the same parameter names as the REST API. `highlight` is a
boolean toggle, and `typoThreshold` is only accepted when `fuzzy: true`. Facets
are returned under the optional `facets` response field, keyed by column name.

## Moving your data

Move Algolia records into PostgreSQL tables first, then query those tables through AYB's normal collection APIs. Current shipped ingest paths are:

- `POST /api/collections/{table}` for single-record creates
- `POST /api/collections/{table}/batch` for atomic create/update/delete batches
- `POST /api/collections/{table}/import` for CSV or JSON row import
- `ayb.records.create` and `ayb.records.batch` from the JavaScript SDK

This lane does not ship `ayb migrate algolia`, a dedicated Algolia importer, Algolia ranking-rule translation, hosted index operations, or dedicated importer automation. Export from Algolia, shape the records for your PostgreSQL schema, ingest them through one of the paths above, configure any per-collection synonym groups you need, then use [Search](/guide/search) to query them.

## Non-parity boundaries

AYB's shipped PostgreSQL search path is useful when your application can use database-owned search, filters, facets, and RLS-scoped counts from one API. It is not an Algolia feature clone.

AYB already ships typo-threshold tuning on fuzzy search, per-collection synonym groups, and `_highlight` snippets when you request `highlight=true`. The remaining gaps are:

- Algolia ranking-rule translation
- hosted index operations separate from PostgreSQL
- dedicated importer automation

Use Algolia when you still need Algolia-specific ranking controls, hosted search operations, or importer automation that are separate from your PostgreSQL data path.

## Related guides

- [Search](/guide/search)
- [REST API Reference](/guide/api-reference)
- [JavaScript SDK](/guide/javascript-sdk)
- [AI and Vector Search](/guide/ai-vector)
