<!-- audited 2026-03-20 -->
# Migrations

AYB supports migration from 6 source platforms and ongoing SQL migration workflows after import.

Firebase migration support remains historical in-tree and retired from roadmap scope. No new
Firebase live-validation or expansion work is scheduled.

## Migrate in one command (`ayb start --from`)

For the fastest path, start AYB and migrate in one step:

```bash
# PocketBase (path detection)
ayb start --from ./pb_data

# Supabase (postgres URL detection)
ayb start --from postgres://postgres:password@db.xxx.supabase.co:5432/postgres

# Firebase auth export (json detection)
ayb start --from ./auth-export.json
```

`ayb start --from` auto-detects only:

- PocketBase path (`pb_data`)
- Supabase Postgres URL
- Firebase `.json` auth export

Appwrite, Directus, and Nhost require explicit `ayb migrate <source>` commands.

For first-time setup context, see [Getting Started](/guide/getting-started).

## Shared migration flow

All source migrations follow the same pattern:

1. Pre-flight analysis (`Analyze`) to count tables/users/policies/files.
2. Confirmation prompt (skip with `-y`/`--yes`).
3. Transactional database migration execution.
4. Post-commit storage copy when the source includes files.
5. Validation summary comparing expected vs migrated totals.

## Shared flags (all source migrations)

| Flag | Description |
|---|---|
| `--dry-run` | Analyze and preview without applying changes |
| `--verbose` | Emit detailed migration progress |
| `--yes`, `-y` | Skip confirmation prompt |
| `--json` | Machine-readable stats output |

## PocketBase

```bash
ayb migrate pocketbase --source ./pb_data --database-url "$DATABASE_URL"
```

What is migrated:

- Collections -> PostgreSQL tables
- Collection fields -> columns
- API rules -> RLS policies
- Auth users -> `_ayb_users`
- Records -> table data
- Files -> AYB storage (optional)

Source-specific flags:

- `--source` (required)
- `--database-url` (required for standalone `ayb migrate pocketbase`; managed Postgres is used automatically with `ayb start --from`)
- `--skip-files`
- `--force`

## Supabase

`ayb migrate supabase` connects directly to a Supabase PostgreSQL database, recreates public-schema tables and rows, migrates email auth users plus non-email OAuth identity links, rewrites supported public-schema RLS policies, and optionally copies exported storage files. When storage is included, AYB copies files from the local export, normalizes bucket names, and registers bucket and object metadata in `_ayb_storage_buckets` and `_ayb_storage_objects` for AYB storage access.

Current limitations are code-verified: user metadata, phone-only users, MFA factors, non-public schemas, secondary indexes, functions/triggers, and full custom-type fidelity are not migrated yet. Storage is omitted unless a local export is supplied.

For the runnable procedure, exact flags, verification commands, and failure boundaries, see [Migrating from Supabase](/guide/migrating-from-supabase).

## Firebase

```bash
ayb migrate firebase \
  --auth-export ./auth-export.json \
  --firestore-export ./firestore-export \
  --rtdb-export ./rtdb-export.json \
  --storage-export ./storage-export \
  --database-url "$DATABASE_URL"
```

What is migrated:

- Auth users with Firebase scrypt hash preservation
- OAuth links -> `_ayb_oauth_accounts`
- Firestore exports -> JSONB tables + GIN indexes
- RTDB exports -> JSONB tables + GIN indexes
- Cloud Storage files -> AYB storage

Source-specific flags:

- `--auth-export`
- `--firestore-export`
- `--rtdb-export`
- `--storage-export`
- `--storage-path`
- `--database-url` (required)

At least one export source flag must be provided.

## Appwrite

```bash
ayb migrate appwrite --export ./appwrite-export.json --database-url "$DATABASE_URL"
```

What is migrated:

- Appwrite export schema -> tables
- Appwrite records -> table data (unless skipped)
- Generated RLS policies (unless skipped)

Source-specific flags:

- `--export` (required)
- `--database-url` (required)
- `--skip-rls`
- `--skip-data`

## Directus

```bash
ayb migrate directus --snapshot ./directus-snapshot.json --database-url "$DATABASE_URL"
```

What is migrated:

- Directus schema snapshot -> tables/columns
- Generated RLS policies (unless skipped)

Source-specific flags:

- `--snapshot` (required)
- `--database-url` (required)
- `--skip-rls`

## Nhost

```bash
ayb migrate nhost \
  --hasura-metadata ./metadata \
  --pg-dump ./dump.sql \
  --database-url "$DATABASE_URL"
```

What is migrated:

- Hasura metadata + pg_dump -> AYB schema/data
- Generated RLS policies (unless skipped)

Source-specific flags:

- `--hasura-metadata` (required)
- `--pg-dump` (required)
- `--database-url` (required)
- `--skip-rls`

## SQL migration workflow after import

After initial platform migration, use standard SQL migrations for ongoing changes:

```bash
ayb migrate create add_orders_table
ayb migrate up --database-url "$DATABASE_URL"
ayb migrate status --database-url "$DATABASE_URL"
ayb migrate diff --database-url "$DATABASE_URL"
ayb migrate generate add_indexes --database-url "$DATABASE_URL"
```

Management commands:

- `migrate create <name>`
- `migrate up`
- `migrate status`
- `migrate diff`
- `migrate generate <name>`

Full command tree reference: [CLI Reference](/guide/cli).

## Operational recommendations

- Take a backup before large migrations. See [Backups](/guide/backups).
- Start with `--dry-run` to validate source shape and row counts.
- Use `--json` in CI or scripted migration jobs.
- Review validation summaries and resolve mismatches before cutover.
