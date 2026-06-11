<!-- audited 2026-06-09 -->

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
| Highlight snippets | `highlight=true` returns `_highlight` and `_highlightResult` in matching result items |
| SDK search request | `ayb.records.list("table", { search, fuzzy, typoThreshold, highlight, filter, facets })` |
| Result hits | `items` in the list response |
| Facet counts | `facets.<column>[]` buckets in the list response |

AYB search and facets run through the same scoped collection query path as normal record listing. Row-level security applies to returned `items`, totals, and facet counts. [Search](/guide/search) owns the detailed AYB behavior for stemming, relevance-first ordering, pagination, highlight response shape, and hybrid-mode boundaries.

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
console.log(results.items[0]?._highlightResult);
console.log(results.facets?.brand);
```

The SDK forwards the same parameter names as the REST API. `highlight` is a
boolean toggle, and `typoThreshold` is only accepted when `fuzzy: true`. Facets
are returned under the optional `facets` response field, keyed by column name.
For the exact highlight metadata shape and search-mode compatibility rules, use
[Search](/guide/search).

## Moving your data

Use `ayb migrate algolia` as the primary data-move path for one Algolia index. The importer browses the source index, infers one PostgreSQL table, creates that table when needed, imports the browsed records, and then queries the table through AYB's normal collection APIs.

Dry-run first to inspect the inferred schema and counts without writing rows:

```bash
ayb migrate algolia \
  --app-id "$ALGOLIA_APP_ID" \
  --api-key "$ALGOLIA_API_KEY" \
  --index products \
  --database-url "$DATABASE_URL" \
  --table products \
  --dry-run
```

Run a confirmed import with `-y, --yes` when the dry-run migration report matches the target table you expect:

```bash
ayb migrate algolia \
  --app-id "$ALGOLIA_APP_ID" \
  --api-key "$ALGOLIA_API_KEY" \
  --index products \
  --database-url "$DATABASE_URL" \
  --table products \
  --include-synonyms \
  -y
```

Required flags are `--app-id`, `--api-key`, `--index`, `--database-url`, and `--table`. `--dry-run` previews the plan without writes, `--include-synonyms` also reads Algolia synonyms when the API key has settings access, `-y, --yes` skips the confirmation prompt, and `--json` writes machine-readable import stats instead of the human report.

Human output reuses the shared migration report before the import and the validation summary after a confirmed import. JSON mode emits the importer stats directly for automation. Stage validation distinguishes live Algolia browse and synonym verification from fixture-backed acceptance: when live credentials or ACLs are unavailable, acceptance is tied to the committed browse and synonym fixtures plus the `CheckRecordParity` parity check instead of claiming a live Algolia run.

With `--include-synonyms`, AYB carries over only supported equivalent Algolia synonym groups into AYB per-collection synonym groups. During synonym import, unsupported synonym types and missing settings ACL are reported as skipped rather than blocking record import.

## Non-parity boundaries

AYB's shipped PostgreSQL search path is useful when your application can use database-owned search, filters, facets, and RLS-scoped counts from one API. It is not an Algolia feature clone.

AYB already ships typo-threshold tuning on fuzzy search, per-collection synonym groups, and `_highlight` / `_highlightResult` snippets when you request `highlight=true`. The remaining gaps are:

- Algolia ranking-rule translation
- hosted index operations separate from PostgreSQL
- Algolia geo controls: `aroundLatLng`, `insideBoundingBox`, and other geo / spatial filters are not shipped through the AYB collection list endpoint
- Synonym-type parity: `--include-synonyms` carries over only equivalent Algolia synonym groups into AYB per-collection synonym groups; unsupported synonym types are reported as skipped rather than blocking record import

Use Algolia when you still need Algolia-specific ranking controls, geo / spatial search, or hosted search operations that are separate from your PostgreSQL data path.

## Related guides

- [Search](/guide/search)
- [REST API Reference](/guide/api-reference)
- [JavaScript SDK](/guide/javascript-sdk)
- [AI and Vector Search](/guide/ai-vector)
