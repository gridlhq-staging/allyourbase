<!-- audited 2026-07-26 -->

# Migrating from Supabase

This guide covers the Supabase migration behavior implemented by `ayb migrate supabase`. It uses direct PostgreSQL connections for database content and an optional local export directory for Supabase Storage files.

For the high-level migration matrix, see [Migrations](/guide/migrations). This page owns the runnable Supabase procedure.

## Prerequisites

- A direct Supabase PostgreSQL connection URL for the source database. Use the database connection on port 5432, not the pooler on port 6543.
- An initialized AYB target database. The target must already contain `_ayb_users`; run `ayb start` or `ayb migrate up` before importing into an empty database.
- A target PostgreSQL connection URL for AYB.
- Optional storage export files arranged as `<export>/<source bucket>/<object name>`. For example, the source object `avatars/users/alice.png` in bucket `avatars` must exist at `./supabase-storage-export/avatars/users/alice.png`.
- Optional local destination path for copied storage files. The migration command writes directly to the local backend rooted at `--storage-path`; it does not read or change the running AYB server's storage configuration.
- To verify migrated public objects through `/api/storage/...`, run AYB with storage enabled, the local backend, and `storage.local_path` / `AYB_STORAGE_LOCAL_PATH` resolving to the same directory passed to `--storage-path`. A disabled or S3-backed AYB server cannot serve the files copied by this guide's local storage migration.

Read connection URLs without echoing them or recording them in shell history, then
export the non-secret settings:

```bash
read -rsp 'Supabase database URL: ' SUPABASE_DB_URL && export SUPABASE_DB_URL
printf '\n'
read -rsp 'AYB target database URL: ' AYB_DATABASE_URL && export AYB_DATABASE_URL
printf '\n'
export AYB_BASE_URL='http://127.0.0.1:8090'
export AYB_STORAGE_LOCAL_PATH="$PWD/ayb_storage"
```

The silent reads protect terminal output and shell history, but the migration flags
below still expand both URLs into the `ayb` process arguments. Run the migration on
a trusted administrative host with dedicated, short-lived database credentials.
After the migration and verification steps, remove the credentials from the shell
environment with `unset SUPABASE_DB_URL AYB_DATABASE_URL`.

Use the storage variable for `--storage-path`. For the optional `/api/storage/...`
verification probe, also keep `AYB_STORAGE_LOCAL_PATH` in the environment used to
run `ayb start`, with storage enabled and the local backend selected. The equivalent
`ayb.toml` storage configuration is:

```toml
[storage]
enabled = true
backend = "local"
local_path = "/absolute/path/to/ayb_storage"
```

In either form, `storage.local_path` / `AYB_STORAGE_LOCAL_PATH` and
`--storage-path` must resolve to the same directory.

## Dry run

Preview the source shape and planned migration without changing the database or storage:

```bash
ayb migrate supabase \
  --source-url "$SUPABASE_DB_URL" \
  --database-url "$AYB_DATABASE_URL" \
  --storage-export ./supabase-storage-export \
  --dry-run
```

`--dry-run` analyzes and reports the plan, then rolls back the database transaction and skips storage writes. Neither the AYB database nor the storage directory changes.

## Confirmed run

Run the migration after the dry-run report matches the source you intend to move:

```bash
ayb migrate supabase \
  --source-url "$SUPABASE_DB_URL" \
  --database-url "$AYB_DATABASE_URL" \
  --storage-export ./supabase-storage-export \
  --storage-path "$AYB_STORAGE_LOCAL_PATH" \
  -y
```

Storage is copied only when `--storage-export` is set and `--skip-storage` is not set. Omitting `--storage-export` leaves storage files untouched; add `--skip-storage` when that omission is intentional.

## CLI flags

Command-specific flags:

| Flag | Required | Default | Description |
| --- | --- | --- | --- |
| `--source-url` | Yes | empty | Supabase PostgreSQL source connection URL. |
| `--database-url` | Yes | empty | AYB PostgreSQL target connection URL. |
| `--storage-export` | No | empty | Local Supabase Storage export root laid out as `<export>/<source bucket>/<object name>`. |
| `--storage-path` | No | `./ayb_storage` | Destination directory for AYB local storage files. |
| `--dry-run` | No | `false` | Preview analysis and migration phases without database or storage changes. |
| `--force` | No | `false` | Allow migration when `_ayb_users` already contains users. |
| `--verbose` | No | `false` | Show detailed progress. |
| `--skip-rls` | No | `false` | Skip RLS policy rewriting. |
| `--skip-oauth` | No | `false` | Skip OAuth identity migration. |
| `--skip-data` | No | `false` | Skip public table schema and row migration; auth still runs. |
| `--skip-storage` | No | `false` | Skip storage file migration. |
| `--include-anonymous` | No | `false` | Include anonymous Supabase users in the source query; users with no email are still skipped. |
| `--yes`, `-y` | No | `false` | Skip the confirmation prompt. |
| `--json` | No | `false` | Write migration stats as JSON. |

Global flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--output` | `table` | Output format for shared CLI commands: `table`, `json`, or `csv`. |

## What is migrated

- Public-schema base tables are recreated in AYB, excluding Supabase and AYB internal tables.
- Public-schema table rows are streamed into the target, with deferred retries for foreign-key dependencies.
- Supported primary keys and foreign keys are recreated. After row copy, AYB attempts
  to reset recognized serial sequences on supported primary-key columns; reset failures
  are reported as warnings.
- Public-schema views are created on a best-effort basis; views that cannot be created are skipped with a warning.
- Email-based `auth.users` rows are inserted into `_ayb_users` with preserved UUIDs, lower-cased email addresses, email verification state, timestamps, and existing password hashes. Supabase bcrypt password hashes continue to verify through AYB login and are upgraded after successful login.
- OAuth identities from `auth.identities` are inserted into `_ayb_oauth_accounts` for non-email providers when a provider user ID is available.
- Public-schema RLS policies are recreated on migrated tables after applying the four rewrite rules owned by `RewriteRLSExpression`: text-cast `auth.uid()` / `uid()` becomes `current_setting('ayb.user_id', true)`, UUID `auth.uid()` / `uid()` becomes `current_setting('ayb.user_id', true)::uuid`, `auth.role()` / `role()` becomes `current_setting('ayb.user_role', true)`, and `auth.jwt() ->> 'email'` / `jwt() ->> 'email'` becomes `current_setting('ayb.user_email', true)`.
- Storage files are copied from the local export into the AYB storage backend when `--storage-export` is provided. Bucket names are normalized to AYB-compatible names, bucket metadata is registered in `_ayb_storage_buckets`, and object metadata is registered in `_ayb_storage_objects`.

## What is not migrated yet

- Supabase user metadata from `auth.users`.
- Phone-only users and MFA factors.
- Email-less users, even with `--include-anonymous`; the flag only includes anonymous rows in the source query before the no-email skip is applied.
- Non-public schemas.
- Secondary indexes.
- Functions and triggers.
- Full custom-type fidelity beyond the types handled by the table DDL generator.

## Post-migration verification

Choose named supported specimens before the migration and assert their exact expected results afterward. Do not turn the absence of a specimen into a passing check.

Before the storage checks, start or restart AYB with `AYB_STORAGE_ENABLED=true`,
`AYB_STORAGE_BACKEND=local`, and `AYB_STORAGE_LOCAL_PATH` set to the same directory
passed to `--storage-path`. The migration command creates its own local backend at
that path; it does not change the running server's storage configuration. With
storage disabled the server does not expose the storage routes, and with an S3
backend or a different local path the `curl` probe reads a different destination.

Compare one selected public table's source and target row counts:

```bash
(
set -euo pipefail

TABLE='todos'
SOURCE_COUNT=$(printf '%s\n' 'SELECT COUNT(*) FROM public.:"table";' |
  psql -X -v ON_ERROR_STOP=1 -v table="$TABLE" "$SUPABASE_DB_URL" -At) || exit 1
TARGET_COUNT=$(printf '%s\n' 'SELECT COUNT(*) FROM public.:"table";' |
  psql -X -v ON_ERROR_STOP=1 -v table="$TABLE" "$AYB_DATABASE_URL" -At) || exit 1
test "$SOURCE_COUNT" = "$TARGET_COUNT"
)
```

Inspect one selected migrated RLS policy and require the exact expression expected
after AYB's rewrite. Set `EXPECTED_POLICY_EXPRESSION` from the source policy and the
documented rewrite rules before running the check; the example value assumes a source
expression of `auth.uid() = user_id`:

```bash
(
set -euo pipefail

TABLE='todos'
POLICY='todos_owner_select'
EXPECTED_POLICY_EXPRESSION="((current_setting('ayb.user_id'::text, true))::uuid = user_id)"
ACTUAL_POLICY_EXPRESSION=$(psql -X -v ON_ERROR_STOP=1 \
  -v table="$TABLE" -v policy="$POLICY" "$AYB_DATABASE_URL" -At <<'SQL'
SELECT pg_get_expr(pol.polqual, pol.polrelid)
FROM pg_policy pol
JOIN pg_class c ON c.oid = pol.polrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relname = :'table'
  AND pol.polname = :'policy';
SQL
) || exit 1
test "$ACTUAL_POLICY_EXPRESSION" = "$EXPECTED_POLICY_EXPRESSION"
)
```

### How AYB derives the stored bucket name

AYB does not store the Supabase bucket name verbatim. During migration each source
bucket name is normalized to an AYB-compatible name, and that normalized name is what
appears in `_ayb_storage_buckets`, in `_ayb_storage_objects.bucket`, and in the
`/api/storage/<bucket>/<object>` path. The normalization, owned by
`normalizeBucketName` in `internal/sbmigrate/storage.go`, applies these rules in order:

1. Lower-case the whole name.
2. Keep only ASCII letters, digits, hyphens (`-`), and underscores (`_`).
3. Replace each space and each dot (`.`) with a single hyphen (`-`).
4. Drop every other character entirely (no replacement).
5. Truncate the result to at most 63 characters.
6. If nothing survives, use the literal name `default`.

So `Avatars` becomes `avatars`, `User Photos` becomes `user-photos`, `my.files`
becomes `my-files`, and a name of only unsupported characters becomes `default`.
When your source bucket name already matches the rules the normalized name is identical.

Before the metadata and `curl` checks, choose one Supabase source bucket and set
`NORMALIZED_BUCKET` to the exact AYB bucket name produced by those six rules for that
specimen. For example, use `avatars` for `Avatars`, `user-photos` for
`User Photos`, `my-files` for `my.files`, and `default` when the source name drops to
an empty result.

Inspect one selected bucket and object metadata row. Query by the normalized bucket name
that AYB creates from the Supabase bucket name:

```bash
(
set -euo pipefail

SOURCE_BUCKET='Avatars'
NORMALIZED_BUCKET='avatars'
OBJECT='users/alice.png'
EXPECTED_PUBLIC='true'
EXPECTED_OBJECT_METADATA='12345|image/png'
psql -X -v ON_ERROR_STOP=1 -v bucket="$NORMALIZED_BUCKET" \
  "$AYB_DATABASE_URL" -At <<'SQL' |
SELECT public::text FROM _ayb_storage_buckets
WHERE tenant_id = '' AND name = :'bucket';
SQL
  rg -x "$EXPECTED_PUBLIC" || exit 1

psql -X -v ON_ERROR_STOP=1 -v bucket="$NORMALIZED_BUCKET" -v object="$OBJECT" \
  "$AYB_DATABASE_URL" -At <<'SQL' |
SELECT size, content_type FROM _ayb_storage_objects
WHERE tenant_id = '' AND bucket = :'bucket' AND name = :'object';
SQL
  rg -Fx "$EXPECTED_OBJECT_METADATA" || exit 1
)
```

For a selected public storage object, require a source bucket with `public=true`, download the AYB object, and compare bytes with the exported file:

```bash
(
set -euo pipefail

SOURCE_BUCKET='Avatars'
NORMALIZED_BUCKET='avatars'
OBJECT='users/alice.png'
EXPORT_FILE="./supabase-storage-export/$SOURCE_BUCKET/$OBJECT"
DOWNLOADED_FILE=$(mktemp) || exit 1
trap 'rm -f "$DOWNLOADED_FILE"' EXIT

psql -X -v ON_ERROR_STOP=1 -v bucket="$SOURCE_BUCKET" \
  "$SUPABASE_DB_URL" -At <<'SQL' |
SELECT public::text FROM storage.buckets WHERE name = :'bucket';
SQL
  rg -x 'true' || exit 1

curl --fail --show-error \
  "$AYB_BASE_URL/api/storage/$NORMALIZED_BUCKET/$OBJECT" \
  -o "$DOWNLOADED_FILE" || exit 1
cmp "$EXPORT_FILE" "$DOWNLOADED_FILE"
)
```

## Failure boundaries

The database migration phases run in one target transaction. On a confirmed run, AYB commits that transaction before storage starts. Storage copy and metadata registration run outside the database transaction.

During storage migration, per-object path validation, backup, and file-copy errors are recorded while later objects continue. Bucket metadata-registration errors, object metadata-registration errors, storage bucket name validation errors, storage export path validation errors, and storage backend setup errors stop the storage phase. A fatal storage error does not roll back committed database work, and it does not roll back storage objects whose metadata registration already completed.

## Related guides

- [Migrations](/guide/migrations)
- [File Storage](/guide/file-storage)
- [Authentication](/guide/authentication)
- [Security](/guide/security)
