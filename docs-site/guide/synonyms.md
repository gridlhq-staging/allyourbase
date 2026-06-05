<!-- audited 2026-06-05 -->

# Search Synonyms

Search synonyms are admin-owned configuration for normal and hybrid collection search. Each synonym group belongs to one collection, and expansion is applied when callers search that collection through the list path.

## Scope

A synonym group belongs to one resolved collection. Terms in the same group are treated as alternatives during full-text search expansion, so a group such as `["scifi", "science fiction"]` lets `search=scifi` match rows that contain `science fiction` in that collection.

The admin path accepts an unqualified table name in `{table}`. It prefers `public.<table>` when that table exists, otherwise it falls back to a matching table name from the schema cache. The shipped admin route does not accept a schema-qualified table target, so avoid duplicate exposed table names when configuring synonyms.

The same group applies only to the resolved collection. It does not automatically apply to other collections with similar text columns.

Hybrid search with `search=<text>&semantic=true` also uses this expansion for its full-text search leg. Synonyms do not change the vector leg or ranking-fusion mechanics; they only expand the text query that feeds the full-text portion.

## Admin contract

Synonym management is an admin/server configuration surface:

```text
GET /api/collections/{table}/synonyms
PUT /api/collections/{table}/synonyms
```

`GET` returns the configured groups:

```json
{
  "groups": [
    { "terms": ["science fiction", "scifi"] }
  ]
}
```

PUT replaces the full synonym group set for that collection:

```bash
curl -s -X PUT "http://127.0.0.1:8090/api/collections/posts/synonyms" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"groups":[{"terms":["scifi","science fiction"]}]}' | jq
```

Every group must contain at least two non-empty terms. Terms are normalized by the server before storage, and the response returns the saved groups.

## Search consumption

Search consumers do not call a separate synonym endpoint. They keep using the standard collection list/search path:

```text
GET /api/collections/{table}?search=scifi
GET /api/collections/{table}?search=scifi&semantic=true
```

In the JavaScript SDK, use `records.list`:

```ts
const response = await ayb.records.list("posts", {
  search: "scifi",
});
```

AYB does not ship a typed JavaScript SDK synonym-management method on `client.records`. Admin configuration stays in the admin/server layer; application callers consume the result through normal list/search APIs.

## Related guides

- [Search](/guide/search)
- [REST API Reference](/guide/api-reference)
- [JavaScript SDK](/guide/javascript-sdk)
