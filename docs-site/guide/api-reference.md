# REST API Reference

AYB auto-generates REST endpoints for every table in your PostgreSQL database.

## Collections (CRUD)

```
GET    /api/collections/{table}          List records
POST   /api/collections/{table}          Create record
POST   /api/collections/{table}/batch    Batch operations
GET    /api/collections/{table}/{id}     Get record
PATCH  /api/collections/{table}/{id}     Update record (partial)
DELETE /api/collections/{table}/{id}     Delete record
```

### List records

```bash
curl "http://localhost:8090/api/collections/posts?filter=status='active'&sort=-created_at&page=1&perPage=20"
```

**Response:**

```json
{
  "items": [
    { "id": 1, "title": "Hello", "published": true, "created_at": "2026-02-07T..." }
  ],
  "page": 1,
  "perPage": 20,
  "totalItems": 42,
  "totalPages": 3
}
```

### Query parameters

| Parameter | Example | Description |
|-----------|---------|-------------|
| `search` | `?search=hello world` | Full-text search across all text columns |
| `filter` | `?filter=status='active' AND age>21` | SQL-safe parameterized filtering |
| `sort` | `?sort=-created_at,+title` | Sort by fields (`-` desc, `+` asc) |
| `page` | `?page=2` | Page number (default: 1) |
| `perPage` | `?perPage=50` | Items per page (default: 20, max: 500) |
| `fields` | `?fields=id,name,email` | Select specific columns |
| `expand` | `?expand=author,category` | Expand foreign key relationships |
| `skipTotal` | `?skipTotal=true` | Skip COUNT query for faster responses |

### Filter syntax

Filters use a safe, parameterized syntax. All values are bound as query parameters — no SQL injection risk.

```
# Equality
?filter=status='active'

# Comparison
?filter=age>21
?filter=price<=100

# AND / OR (keywords or symbols)
?filter=status='active' AND category='tech'
?filter=role='admin' OR role='editor'
?filter=status='active' && category='tech'
?filter=role='admin' || role='editor'

# NULL checks (use =null or !=null)
?filter=deleted_at=null
?filter=email!=null

# Pattern matching (use ~ for LIKE, !~ for NOT LIKE)
?filter=name~'%john%'
?filter=name!~'%admin%'

# NOT equal
?filter=status!='draft'

# IN list
?filter=status IN ('active','pending','review')

# Grouping with parentheses
?filter=(status='active' OR status='pending') AND category='tech'

# Boolean and numeric values
?filter=published=true
?filter=age>21 AND score<=100
```

#### Operator reference

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal (or `IS NULL` when value is `null`) | `status='active'` |
| `!=` | Not equal (or `IS NOT NULL` when value is `null`) | `status!='draft'` |
| `>` | Greater than | `age>21` |
| `>=` | Greater than or equal | `score>=90` |
| `<` | Less than | `price<100` |
| `<=` | Less than or equal | `price<=50` |
| `~` | LIKE (pattern match) | `name~'%john%'` |
| `!~` | NOT LIKE | `name!~'%test%'` |
| `IN` | In list | `status IN ('a','b')` |
| `AND` / `&&` | Logical AND | `a='x' AND b='y'` |
| `OR` / `\|\|` | Logical OR | `a='x' OR a='y'` |

Values: strings in single quotes (`'hello'`), numbers (`42`, `3.14`), booleans (`true`, `false`), `null`.

### Full-text search

Use `?search=` to search across all text columns (`text`, `varchar`, `char`) in a table:

```bash
curl "http://localhost:8090/api/collections/posts?search=postgres database"
```

Search uses PostgreSQL's `websearch_to_tsquery`, so it supports natural search syntax:

```
# Simple search
?search=postgres

# Multi-word (AND by default)
?search=postgres database

# Exact phrase
?search="full text search"

# OR
?search=postgres or mysql

# Exclude terms
?search=postgres -mysql
```

Results are automatically ranked by relevance when no explicit `sort` is provided.

Search can be combined with filters:

```bash
curl "http://localhost:8090/api/collections/posts?search=postgres&filter=status='active'&perPage=10"
```

::: tip Performance
For tables with many rows, add a GIN index on text columns for faster search:

```sql
CREATE INDEX posts_fts_idx ON posts USING GIN (
  to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body, ''))
);
```
:::

### Batch operations

Perform multiple create, update, and delete operations in a single atomic transaction. If any operation fails, all changes are rolled back.

```bash
curl -X POST http://localhost:8090/api/collections/posts/batch \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"method": "create", "body": {"title": "Post A", "published": true}},
      {"method": "create", "body": {"title": "Post B", "published": false}},
      {"method": "update", "id": "42", "body": {"published": true}},
      {"method": "delete", "id": "99"}
    ]
  }'
```

**Request body:**

| Field | Type | Description |
|-------|------|-------------|
| `operations` | array | Array of operations (max 1000) |
| `operations[].method` | string | `"create"`, `"update"`, or `"delete"` |
| `operations[].id` | string | Record ID (required for update/delete) |
| `operations[].body` | object | Record data (required for create/update) |

**Response** (200 OK):

```json
[
  {"index": 0, "status": 201, "body": {"id": 100, "title": "Post A", "published": true}},
  {"index": 1, "status": 201, "body": {"id": 101, "title": "Post B", "published": false}},
  {"index": 2, "status": 200, "body": {"id": 42, "title": "Existing", "published": true}},
  {"index": 3, "status": 204}
]
```

All operations run in a single database transaction. RLS policies apply. Realtime and webhook events are published after successful commit.

### Create a record

```bash
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "New Post", "body": "Content", "published": false}'
```

**Response** (201 Created):

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": false,
  "created_at": "2026-02-07T22:00:00Z"
}
```

### Get a record

```bash
curl http://localhost:8090/api/collections/posts/42
```

**Response:**

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": false,
  "created_at": "2026-02-07T22:00:00Z"
}
```

Supports `?fields=` and `?expand=` query parameters.

### Update a record

```bash
curl -X PATCH http://localhost:8090/api/collections/posts/42 \
  -H "Content-Type: application/json" \
  -d '{"published": true}'
```

**Response:**

```json
{
  "id": 42,
  "title": "New Post",
  "body": "Content",
  "published": true,
  "created_at": "2026-02-07T22:00:00Z"
}
```

Only the specified fields are updated (partial update). The full updated record is returned.

### Delete a record

```bash
curl -X DELETE http://localhost:8090/api/collections/posts/42
```

Returns `204 No Content` on success.

### Expand foreign keys

If your `posts` table has an `author_id` column referencing `users(id)`:

```bash
curl "http://localhost:8090/api/collections/posts?expand=author"
```

**Response:**

```json
{
  "items": [
    {
      "id": 1,
      "title": "Hello",
      "author_id": 42,
      "expand": {
        "author": {
          "id": 42,
          "name": "Jane",
          "email": "jane@example.com"
        }
      }
    }
  ],
  "page": 1,
  "perPage": 20,
  "totalItems": 1,
  "totalPages": 1
}
```

Related records are nested under an `expand` key. For many-to-one relationships, the expanded value is a single object. For one-to-many, it's an array.

## Schema

```bash
curl http://localhost:8090/api/schema
```

Returns the full database schema as JSON including tables, columns, types, primary keys, and foreign key relationships.

## Health check

```bash
curl http://localhost:8090/health
```

Returns `200 OK` when the server is running and the database is reachable.

## Error format

All errors return a consistent JSON format:

```json
{
  "code": 404,
  "message": "collection not found: nonexistent",
  "doc_url": "https://allyourbase.io/guide/api-reference"
}
```

For validation errors (constraint violations), the response includes a `data` field with per-field detail:

```json
{
  "code": 409,
  "message": "unique constraint violation",
  "data": {
    "users_email_key": {
      "code": "unique_violation",
      "message": "Key (email)=(test@example.com) already exists."
    }
  },
  "doc_url": "https://allyourbase.io/guide/api-reference#error-format"
}
```

The `doc_url` field links to relevant documentation when available.

Common HTTP status codes:

| Status | Meaning |
|--------|---------|
| `400` | Invalid request (bad filter syntax, invalid JSON) |
| `401` | Unauthorized (missing or invalid JWT) |
| `404` | Collection or record not found |
| `409` | Conflict (unique constraint violation) |
| `422` | Validation error (NOT NULL violation, check constraint) |
| `500` | Internal server error |
