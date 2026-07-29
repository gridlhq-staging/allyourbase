# Secrets
<!-- audited 2026-07-27 -->

AYB's vault stores application secrets encrypted in PostgreSQL and exposes them through admin-authenticated APIs and the `ayb secrets` CLI. Vault secrets can also become environment variables for [Edge Functions](/guide/edge-functions).

## Master key setup

Vault data is encrypted before it is written to `_ayb_vault_secrets`. AYB resolves the master key in this order:

1. `AYB_VAULT_MASTER_KEY`
2. `[vault].master_key` in `ayb.toml`
3. `~/.ayb/vault-key`

If none is configured, AYB generates a random key and persists it at `~/.ayb/vault-key` with owner-only permissions.

For production, provide a stable key through your secret-management system:

```bash
export AYB_VAULT_MASTER_KEY="<at-least-16-bytes-of-key-material>"
ayb start
```

Alternatively, configure it in TOML:

```toml
[vault]
master_key = "<at-least-16-bytes-of-key-material>"
```

::: warning Back up the master key
The master key is required to decrypt every stored value. Keep the same key across restarts and replicas, and back it up separately from the database. Losing or changing it makes existing ciphertext unreadable. Avoid committing a key in `ayb.toml`.
:::

The key may be hex, standard or URL-safe base64, or raw text. After decoding, it must contain at least 16 bytes. AYB derives a distinct AES-256-GCM key for each secret.

## CLI

The CLI connects to `http://127.0.0.1:8090` by default. It resolves admin authentication from `--admin-token`, `AYB_ADMIN_TOKEN`, or the local `~/.ayb/admin-token` file. Use `--url` for another server.

### Create or update

`ayb secrets set` creates a secret and updates it when the name already exists:

```bash
ayb secrets set PAYMENT_API_KEY "development-key"
printf '%s\n' "value-from-stdin" | ayb secrets set WEBHOOK_SECRET -
```

### Read

`ayb secrets get` masks the value by default. Add `--reveal` only when plaintext output is required:

```bash
ayb secrets get PAYMENT_API_KEY
# PAYMENT_API_KEY=****

ayb secrets get PAYMENT_API_KEY --reveal
# PAYMENT_API_KEY=development-key
```

Revealed values can be captured by shell history, terminal logs, or CI output. Prefer consuming them without printing when possible.

### List

`ayb secrets list` returns names and timestamps, never values:

```bash
ayb secrets list
ayb secrets list --json
ayb secrets list --output csv
```

### Delete

`ayb secrets delete` prompts for confirmation. Automated workflows can pass `--yes`:

```bash
ayb secrets delete PAYMENT_API_KEY
ayb secrets delete PAYMENT_API_KEY --yes
```

## Admin API

All routes require an admin bearer token:

```bash
export AYB_URL="http://127.0.0.1:8090"
export AYB_ADMIN_TOKEN="<admin-token>"
```

Create:

```bash
curl -sS -X POST "$AYB_URL/api/admin/secrets" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"PAYMENT_API_KEY","value":"development-key"}'
```

List metadata:

```bash
curl -sS "$AYB_URL/api/admin/secrets" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

`GET /api/admin/secrets` returns only `name`, `created_at`, and `updated_at`. It never returns secret values.

Read one plaintext value:

```bash
curl -sS "$AYB_URL/api/admin/secrets/PAYMENT_API_KEY" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

Update:

```bash
curl -sS -X PUT "$AYB_URL/api/admin/secrets/PAYMENT_API_KEY" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"value":"replacement-key"}'
```

Delete:

```bash
curl -sS -X DELETE "$AYB_URL/api/admin/secrets/PAYMENT_API_KEY" \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN"
```

The complete CRUD surface is:

- `GET /api/admin/secrets`
- `POST /api/admin/secrets`
- `GET /api/admin/secrets/{name}`
- `PUT /api/admin/secrets/{name}`
- `DELETE /api/admin/secrets/{name}`

## Secret names

Names are trimmed and may contain letters, numbers, underscore, hyphen, and dot. They cannot be empty or contain `..`.

Examples:

- Valid: `API_KEY`, `stripe-live`, `service.token`
- Invalid: `service/key`, `two words`, `..`

## Edge Function environment

At invocation time, AYB loads all vault secrets and merges them into the Edge Function environment. Function-level environment variables take precedence over vault secrets with the same name. This lets a function override a shared value without changing the vault entry used by other functions.

Vault secrets are also used internally by configured field encryption and selected server integrations. Those consumers use the same vault owner; they do not create a second plaintext secret store.
