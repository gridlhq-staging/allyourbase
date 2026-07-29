<!-- audited 2026-07-26 -->

# Migrating from Supabase

This guide covers the Supabase migration behavior implemented by `ayb migrate supabase`. It uses direct PostgreSQL connections for database content. Storage can come from either a local `--storage-export` tree or direct S3 pull credentials, while `--storage-path` remains the AYB local destination for copied bytes.

For the high-level migration matrix, see [Migrations](/guide/migrations). This page owns the runnable Supabase procedure.

## Prerequisites

- A direct Supabase PostgreSQL connection URL for the source database. Use the database connection on port 5432, not the pooler on port 6543.
- An initialized AYB target database. The target must already contain `_ayb_users`; run `ayb start` or `ayb migrate up` before importing into an empty database.
- A target PostgreSQL connection URL for AYB.
- An optional storage source: either export files arranged as `<export>/<source bucket>/<object name>` or Supabase S3 endpoint and access credentials. For the local flow, the source object `avatars/users/alice.png` in bucket `avatars` must exist at `./supabase-storage-export/avatars/users/alice.png`.
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

## Local storage export

### Dry run

Preview the source shape and planned migration without changing the database or storage:

```bash
ayb migrate supabase \
  --source-url "$SUPABASE_DB_URL" \
  --database-url "$AYB_DATABASE_URL" \
  --storage-export ./supabase-storage-export \
  --dry-run
```

`--dry-run` analyzes and reports the plan, then rolls back the database transaction and skips storage writes. Neither the AYB database nor the storage directory changes.

### Confirmed run

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

## Direct S3 pull

Use Supabase's S3-compatible endpoint and S3 access credentials to pull object
bytes without creating a local source export. Enable the S3 protocol, then copy the endpoint and region from your project's Storage S3 configuration. Read those
values and the generated credentials without putting them in shell history:

```bash
read -rp 'Supabase S3 endpoint: ' SUPABASE_S3_ENDPOINT && export SUPABASE_S3_ENDPOINT
read -rp 'Supabase S3 region: ' SUPABASE_S3_REGION && export SUPABASE_S3_REGION
read -rp 'Supabase S3 access key: ' SUPABASE_S3_ACCESS_KEY && export SUPABASE_S3_ACCESS_KEY
read -rsp 'Supabase S3 secret key: ' SUPABASE_S3_SECRET_KEY && export SUPABASE_S3_SECRET_KEY
printf '\n'
```

The migration reads source bucket names from the Supabase PostgreSQL
`storage.buckets` inventory, so there is intentionally no
`--storage-s3-bucket` flag. `--storage-export` and `--storage-s3-*` are mutually
exclusive, and `--skip-storage` skips both the local-export and direct-S3
sources.

Preview the direct pull without changing the target database or destination:

```bash
ayb migrate supabase \
  --source-url "$SUPABASE_DB_URL" \
  --database-url "$AYB_DATABASE_URL" \
  --storage-s3-endpoint "$SUPABASE_S3_ENDPOINT" \
  --storage-s3-region "$SUPABASE_S3_REGION" \
  --storage-s3-access-key "$SUPABASE_S3_ACCESS_KEY" \
  --storage-s3-secret-key "$SUPABASE_S3_SECRET_KEY" \
  --storage-s3-use-ssl=true \
  --storage-path "$AYB_STORAGE_LOCAL_PATH" \
  --dry-run
```

After the dry-run report matches the source inventory, run the confirmed
migration:

```bash
ayb migrate supabase \
  --source-url "$SUPABASE_DB_URL" \
  --database-url "$AYB_DATABASE_URL" \
  --storage-s3-endpoint "$SUPABASE_S3_ENDPOINT" \
  --storage-s3-region "$SUPABASE_S3_REGION" \
  --storage-s3-access-key "$SUPABASE_S3_ACCESS_KEY" \
  --storage-s3-secret-key "$SUPABASE_S3_SECRET_KEY" \
  --storage-s3-use-ssl=true \
  --storage-path "$AYB_STORAGE_LOCAL_PATH" \
  -y
```

The flags expand the S3 credentials into the `ayb` process arguments, so run
the command on a trusted administrative host with dedicated, short-lived
credentials. Remove them from the environment afterward:

```bash
unset SUPABASE_S3_ACCESS_KEY SUPABASE_S3_SECRET_KEY
```

## CLI flags

Command-specific flags:

| Flag | Required | Default | Description |
| --- | --- | --- | --- |
| `--source-url` | Yes | empty | Supabase PostgreSQL source connection URL. |
| `--database-url` | Yes | empty | AYB PostgreSQL target connection URL. |
| `--storage-export` | No | empty | Local Supabase Storage export root laid out as `<export>/<source bucket>/<object name>`. |
| `--storage-path` | No | `./ayb_storage` | Destination directory for AYB local storage files. |
| `--storage-s3-endpoint` | No | empty | S3-compatible source endpoint. |
| `--storage-s3-region` | No | empty | S3-compatible source region. |
| `--storage-s3-access-key` | No | empty | S3-compatible source access key. |
| `--storage-s3-secret-key` | No | empty | S3-compatible source secret key. |
| `--storage-s3-use-ssl` | No | `true` | Use HTTPS when the S3 endpoint has no explicit scheme; set `false` only for an HTTP endpoint. |
| `--dry-run` | No | `false` | Preview analysis and migration phases without database or storage changes. |
| `--force` | No | `false` | Allow migration when `_ayb_users` already contains users. |
| `--verbose` | No | `false` | Show detailed progress. |
| `--skip-rls` | No | `false` | Skip RLS policy rewriting. |
| `--skip-oauth` | No | `false` | Skip OAuth identity migration. |
| `--skip-data` | No | `false` | Skip data table migration (auth and RLS only); this skips user-schema table creation and row migration. |
| `--skip-storage` | No | `false` | Skip storage file migration. |
| `--include-anonymous` | No | `false` | Include anonymous Supabase users in the source query; users with no email are still skipped. |
| `--yes`, `-y` | No | `false` | Skip the confirmation prompt. |
| `--json` | No | `false` | Write migration stats as JSON. |

Global flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--output` | `table` | Output format for shared CLI commands: `table`, `json`, or `csv`. |

## What is migrated

- Base tables in `public` and other user-owned schemas admitted by `isAdmittedUserSchema` in `internal/sbmigrate/migrator_helpers.go` are recreated in AYB. The admission rule excludes `auth`, `storage`, `realtime`, `extensions`, `graphql`, `graphql_public`, `vault`, `pgsodium`, `cron`, `net`, `pgbouncer`, `pgsodium_masks`, `pgtle`, `_analytics`, `_realtime`, `_supabase`, and `information_schema`, plus schemas whose names start with `pg_` or `supabase_`. AYB creates each admitted non-`public` schema in the target without granting it to `ayb_authenticated`; review the migrated RLS policies and grant only the access your application requires.
- Table rows in admitted user schemas are streamed into the target, with deferred retries for foreign-key dependencies. The Supabase-internal-table and `_ayb_*` name filters apply within `public`; every base table in an admitted non-`public` schema is migrated.
- Supported primary keys and foreign keys are recreated. After row copy, AYB attempts
  to reset recognized serial sequences on supported primary-key columns; reset failures
  are reported as warnings.
- Views in admitted user schemas are created on a best-effort basis; views that cannot be created are skipped with a warning.
- AYB recreates admitted user-schema `plpgsql` and `sql` functions whose signatures are safe to install before tables and views and whose definitions do not reference excluded schemas. Function definitions, overload identity arguments, and supported attributes come from the source PostgreSQL catalog.
- After tables and data are migrated, AYB recreates ordinary table triggers whose handlers were migrated. Trigger definitions and enabled or disabled states come from the source PostgreSQL catalog, including supported triggers on partitioned tables.
- Email-based `auth.users` rows are inserted into `_ayb_users` with preserved UUIDs, lower-cased email addresses, email verification state, timestamps, and existing password hashes. Supabase `auth.users.raw_user_meta_data` maps to AYB `_ayb_users.raw_user_meta_data`, and Supabase `auth.users.raw_app_meta_data` maps to AYB `_ayb_users.raw_app_meta_data`. Nested JSON is preserved verbatim as JSONB rather than expanded into first-class auth fields or exposed through the user-facing auth API. Supabase bcrypt password hashes continue to verify through AYB login and are upgraded after successful login.
- OAuth identities from `auth.identities` are inserted into `_ayb_oauth_accounts` for non-email providers when a provider user ID is available.
- RLS policies are recreated on migrated tables in admitted user schemas after applying the four rewrite rules owned by `RewriteRLSExpression`: text-cast `auth.uid()` / `uid()` becomes `current_setting('ayb.user_id', true)`, UUID `auth.uid()` / `uid()` becomes `current_setting('ayb.user_id', true)::uuid`, `auth.role()` / `role()` becomes `current_setting('ayb.user_role', true)`, and `auth.jwt() ->> 'email'` / `jwt() ->> 'email'` becomes `current_setting('ayb.user_email', true)`.
- Storage files are copied from either the local export or direct S3 source into the AYB storage backend when one source is configured. Bucket names are normalized to AYB-compatible names, bucket metadata is registered in `_ayb_storage_buckets`, and object metadata is registered in `_ayb_storage_objects`.

## What is not migrated yet

- Phone-only users and MFA factors.
- Email-less users, even with `--include-anonymous`; the flag only includes anonymous rows in the source query before the no-email skip is applied.
- Secondary indexes.
- Functions with excluded-schema body references; C, `internal`, and unknown function languages; extension-owned functions; aggregate, window, and operator implementation functions; procedures; and functions whose signatures depend on table- or view-defined composite types are skipped and reported.
- Event triggers and constraint triggers are skipped and reported. Ordinary table triggers are also skipped when their handlers were skipped or not migrated, or when their definitions reference an excluded schema.
- Full custom-type fidelity beyond the types handled by the table DDL generator.
- Source schema, table, and sequence grants. Non-`public` schemas remain inaccessible
  to `ayb_authenticated` until an operator explicitly grants the required privileges.
- Direct S3 pull is shipped. Its credential-gated live confirmation against `https://<project>.storage.supabase.co/storage/v1/s3` requires operator credentials and does not gate downstream AI work.

## Post-migration verification

Choose named supported specimens before the migration and assert their exact expected results afterward. Do not turn the absence of a specimen into a passing check.

Before the storage checks, start or restart AYB with `AYB_STORAGE_ENABLED=true`,
`AYB_STORAGE_BACKEND=local`, and `AYB_STORAGE_LOCAL_PATH` set to the same directory
passed to `--storage-path`. The migration command creates its own local backend at
that path; it does not change the running server's storage configuration. With
storage disabled the server does not expose the storage routes, and with an S3
backend or a different local path the `curl` probe reads a different destination.

Compare one selected admitted user-schema table's source and target row counts. The
default below checks `public.todos`; for a non-public specimen, use values such as
`SCHEMA='billing'` and `TABLE='invoices'`:

```bash
(
set -euo pipefail

SCHEMA='public'
TABLE='todos'
SOURCE_COUNT=$(printf '%s\n' 'SELECT COUNT(*) FROM :"schema".:"table";' |
  psql -X -v ON_ERROR_STOP=1 -v schema="$SCHEMA" -v table="$TABLE" "$SUPABASE_DB_URL" -At) || exit 1
TARGET_COUNT=$(printf '%s\n' 'SELECT COUNT(*) FROM :"schema".:"table";' |
  psql -X -v ON_ERROR_STOP=1 -v schema="$SCHEMA" -v table="$TABLE" "$AYB_DATABASE_URL" -At) || exit 1
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

SCHEMA='public'
TABLE='todos'
POLICY='todos_owner_select'
EXPECTED_POLICY_EXPRESSION="((current_setting('ayb.user_id'::text, true))::uuid = user_id)"
ACTUAL_POLICY_EXPRESSION=$(psql -X -v ON_ERROR_STOP=1 \
  -v schema="$SCHEMA" -v table="$TABLE" -v policy="$POLICY" "$AYB_DATABASE_URL" -At <<'SQL'
SELECT pg_get_expr(pol.polqual, pol.polrelid)
FROM pg_policy pol
JOIN pg_class c ON c.oid = pol.polrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = :'schema'
  AND c.relname = :'table'
  AND pol.polname = :'policy';
SQL
) || exit 1
test "$ACTUAL_POLICY_EXPRESSION" = "$EXPECTED_POLICY_EXPRESSION"
)
```

Inspect one named migrated function and one named migrated trigger, and require
their exact expected definitions. Choose supported specimens before migration,
capture the expected `pg_get_functiondef` and `pg_get_triggerdef` values from the
source, and export them as `EXPECTED_FUNCTION_DEFINITION` and
`EXPECTED_TRIGGER_DEFINITION`. The sample identities below match the Stage 4
integration specimens; replace them with specimens from your source:

```bash
(
set -euo pipefail

FUNCTION_IDENTITY='public.plpgsql_increment(integer)'
: "${EXPECTED_FUNCTION_DEFINITION:?export the exact expected pg_get_functiondef value}"
ACTUAL_FUNCTION_DEFINITION=$(psql -X -v ON_ERROR_STOP=1 \
  -v function_identity="$FUNCTION_IDENTITY" "$AYB_DATABASE_URL" -At <<'SQL'
SELECT pg_get_functiondef(to_regprocedure(:'function_identity'));
SQL
) || exit 1
test "$ACTUAL_FUNCTION_DEFINITION" = "$EXPECTED_FUNCTION_DEFINITION"

TRIGGER_SCHEMA='public'
TRIGGER_TABLE='trigger_specimens'
TRIGGER_NAME='trigger_specimens_before_insert'
: "${EXPECTED_TRIGGER_DEFINITION:?export the exact expected pg_get_triggerdef value}"
ACTUAL_TRIGGER_DEFINITION=$(psql -X -v ON_ERROR_STOP=1 \
  -v schema="$TRIGGER_SCHEMA" -v table="$TRIGGER_TABLE" -v trigger="$TRIGGER_NAME" \
  "$AYB_DATABASE_URL" -At <<'SQL'
SELECT pg_get_triggerdef(t.oid)
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = :'schema'
  AND c.relname = :'table'
  AND t.tgname = :'trigger'
  AND NOT t.tgisinternal;
SQL
) || exit 1
test "$ACTUAL_TRIGGER_DEFINITION" = "$EXPECTED_TRIGGER_DEFINITION"
)
```

For a selected migrated non-`public` schema, require that migration did not
automatically expose it to AYB's authenticated role:

```bash
(
set -euo pipefail

SCHEMA='billing'
psql -X -v ON_ERROR_STOP=1 -v schema="$SCHEMA" "$AYB_DATABASE_URL" -At <<'SQL' |
SELECT has_schema_privilege('ayb_authenticated', :'schema', 'USAGE')::text;
SQL
  rg -x 'false' || exit 1
)
```

After reviewing the schema's tables and migrated RLS policies, grant only the
privileges the application requires. For example, a read-only schema exposure can
be enabled explicitly with:

```sql
GRANT USAGE ON SCHEMA billing TO ayb_authenticated;
GRANT SELECT ON ALL TABLES IN SCHEMA billing TO ayb_authenticated;
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

For a selected public storage object migrated through the local-export flow,
require a source bucket with `public=true`, download the AYB object, and compare
bytes with the exported file. This export-file comparison is local-export-specific;
the direct S3 procedure does not create a local source file:

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
