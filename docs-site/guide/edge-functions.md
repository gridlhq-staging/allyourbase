# Edge Functions

<!-- audited 2026-03-23 -->

AYB edge functions have two surfaces:

1. Admin management and testing: `/api/admin/functions/*` (admin token required)
2. Public HTTP invoke path: `/functions/v1/{name}` and `/functions/v1/{name}/*`

## Admin API contract

Admin routes:

- `GET /api/admin/functions`
- `POST /api/admin/functions`
- `GET /api/admin/functions/by-name/{name}`
- `GET /api/admin/functions/{id}`
- `PUT /api/admin/functions/{id}`
- `DELETE /api/admin/functions/{id}`
- `GET /api/admin/functions/{id}/logs`
- `POST /api/admin/functions/{id}/invoke`

Trigger management routes are nested under each function:

- Base prefix: `/api/admin/functions/{id}/triggers`
- Total sub-routes: `19` (`6` DB + `7` cron + `6` storage)

Database triggers (`/{id}/triggers/db`):

- `GET /api/admin/functions/{id}/triggers/db`
- `POST /api/admin/functions/{id}/triggers/db`
- `GET /api/admin/functions/{id}/triggers/db/{triggerId}`
- `DELETE /api/admin/functions/{id}/triggers/db/{triggerId}`
- `POST /api/admin/functions/{id}/triggers/db/{triggerId}/enable`
- `POST /api/admin/functions/{id}/triggers/db/{triggerId}/disable`

Cron triggers (`/{id}/triggers/cron`):

- `GET /api/admin/functions/{id}/triggers/cron`
- `POST /api/admin/functions/{id}/triggers/cron`
- `GET /api/admin/functions/{id}/triggers/cron/{triggerId}`
- `DELETE /api/admin/functions/{id}/triggers/cron/{triggerId}`
- `POST /api/admin/functions/{id}/triggers/cron/{triggerId}/enable`
- `POST /api/admin/functions/{id}/triggers/cron/{triggerId}/disable`
- `POST /api/admin/functions/{id}/triggers/cron/{triggerId}/run` (manual run)

Storage triggers (`/{id}/triggers/storage`):

- `GET /api/admin/functions/{id}/triggers/storage`
- `POST /api/admin/functions/{id}/triggers/storage`
- `GET /api/admin/functions/{id}/triggers/storage/{triggerId}`
- `DELETE /api/admin/functions/{id}/triggers/storage/{triggerId}`
- `POST /api/admin/functions/{id}/triggers/storage/{triggerId}/enable`
- `POST /api/admin/functions/{id}/triggers/storage/{triggerId}/disable`

Deploy/update request fields map to the server request structs:

- `name`
- `source`
- `entry_point`
- `timeout_ms`
- `env_vars`
- `public`

Behavior notes:

- Empty `entry_point` defaults to `handler`.
- Deploy with `timeout_ms <= 0` falls back to the default timeout.
- Update only changes timeout when `timeout_ms > 0`.

## Dashboard workflows

The dashboard uses the same admin APIs and supports:

- Create/deploy with name, source, entry point, timeout, visibility, env vars
- Edit/revert flow with unsaved-state handling
- Admin test invoke (`POST /api/admin/functions/{id}/invoke`)
- Logs filtering via query params on `GET /api/admin/functions/{id}/logs`

## Admin invoke and logs filters

Admin invoke request body:

```json
{
  "method": "POST",
  "path": "/my-function/run",
  "headers": { "Content-Type": ["application/json"] },
  "body": "{\"hello\":\"world\"}"
}
```

Admin invoke response:

```json
{
  "statusCode": 200,
  "headers": { "content-type": ["application/json"] },
  "body": "{\"ok\":true}"
}
```

Logs endpoint accepts:

- `page`
- `perPage`
- `limit` (takes precedence over `page`/`perPage`)
- `status` (`success` or `error`)
- `trigger_type` (`http`, `db`, `cron`, `storage`, `function`)
- `since` / `until` (RFC3339)

Normalization:

- default page: `1`
- default per-page: `50`
- per-page maximum: `1000`

## Public invoke path (`/functions/v1/{name}`)

Public invoke supports any HTTP method and CORS preflight.

Current behavior:

- Routes: `/functions/v1/{name}` and `/functions/v1/{name}/*`
- Request body default cap: `1 MiB`
- Query strings and subpaths are forwarded to the function request context
- Response status defaults to `200` if function status is `0`
- Private functions require a bearer token

Bearer token validation accepts:

- OAuth access tokens
- API keys
- JWTs

## CLI command reference

All commands are under `ayb functions` and use the same admin API.

```bash
export AYB_URL="http://127.0.0.1:8090"
export AYB_ADMIN_TOKEN="<admin-token>"
```

## Tutorial: write, deploy, and invoke

Start AYB locally with `ayb start`. While the server is running, it stores the loopback admin token in `~/.ayb/admin-token`. The examples below keep the base URL and token explicit so the same commands work from a temporary directory.

```bash
export AYB_BASE_URL="http://127.0.0.1:8090"
export AYB_ADMIN_TOKEN="$(cat ~/.ayb/admin-token)"
workdir="$(mktemp -d)"

cat > "$workdir/hello_docs.js" <<'JS'
export default function handler(req) {
  const payload = req.body ? JSON.parse(req.body) : {};
  const name = payload.name || "friend";

  return {
    statusCode: 201,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      ok: true,
      message: `Hello, ${name}!`,
      path: req.path,
      method: req.method,
    }),
  };
}
JS

ayb functions deploy hello-docs \
  --source "$workdir/hello_docs.js" \
  --public \
  --url "$AYB_BASE_URL" \
  --admin-token "$AYB_ADMIN_TOKEN"

ayb functions invoke hello-docs \
  --method POST \
  --path /hello-docs/admin \
  --header "Content-Type:application/json" \
  --body '{"name":"Admin"}' \
  --url "$AYB_BASE_URL" \
  --admin-token "$AYB_ADMIN_TOKEN"

curl -sS -X POST "$AYB_BASE_URL/functions/v1/hello-docs/public" \
  -H "Content-Type: application/json" \
  -d '{"name":"Public"}'
```

The admin CLI invocation calls `POST /api/admin/functions/{id}/invoke`. It is for management and testing, so it always requires the admin token. The public invocation calls `/functions/v1/hello-docs/public`; because the deployment used `--public`, callers do not need a bearer token. Private functions use the same public URL shape but require a bearer token accepted by the auth service.

Expected semantic output from the two invocations:

```text
Status: 201
Body:
{"ok":true,"message":"Hello, Admin!","path":"/hello-docs/admin","method":"POST"}
```

```json
{"ok":true,"message":"Hello, Public!","path":"/hello-docs/public","method":"POST"}
```

Clean up the tutorial function and temporary source when you are done:

```bash
ayb functions delete hello-docs --force \
  --url "$AYB_BASE_URL" \
  --admin-token "$AYB_ADMIN_TOKEN"

rm -rf "$workdir"
```

### List

```bash
ayb functions list --url "$AYB_URL" --admin-token "$AYB_ADMIN_TOKEN"
ayb functions list --page 2 --per-page 25 --json
```

### Get

```bash
ayb functions get hello-world --url "$AYB_URL" --admin-token "$AYB_ADMIN_TOKEN"
ayb functions get aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa --json
```

### Scaffold

```bash
ayb functions new hello-world
ayb functions new hello-world --typescript
```

### Deploy

```bash
ayb functions deploy hello-world \
  --source ./hello-world.js \
  --url "$AYB_URL" \
  --admin-token "$AYB_ADMIN_TOKEN"

ayb functions deploy hello-world \
  --source ./hello-world.ts \
  --entry-point handler \
  --timeout 10000 \
  --public
```

### Delete

```bash
ayb functions delete hello-world --force --url "$AYB_URL" --admin-token "$AYB_ADMIN_TOKEN"
```

### Invoke (admin)

```bash
ayb functions invoke hello-world \
  --method POST \
  --path /hello-world/run \
  --header "Content-Type:application/json" \
  --body '{"name":"AYB"}' \
  --url "$AYB_URL" \
  --admin-token "$AYB_ADMIN_TOKEN"
```

### Logs

```bash
ayb functions logs hello-world --limit 50 --url "$AYB_URL" --admin-token "$AYB_ADMIN_TOKEN"
ayb functions logs hello-world --status error --trigger-type cron --limit 10
ayb functions logs hello-world --follow
```

## Operational limits

| Area | Current behavior |
|---|---|
| Admin deploy/update payload body | `1 MiB` request body cap |
| Public invoke request body | `1 MiB` default cap (`MaxEdgeFuncBodySize`) |
| Default timeout | `5s` when unset (`DefaultTimeout`) |
| Log list default | `perPage=50` |
| Log list max | `perPage`/`limit` clamped to `1000` |
