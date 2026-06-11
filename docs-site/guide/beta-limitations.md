# Beta Limitations

AYB is beta software. The core runtime, APIs, SDKs, and docs are usable today, but a few shipped surfaces intentionally have bounded behavior while the project closes parity and operations gaps.

## Managed PostgreSQL extension boundaries

Managed PostgreSQL is the zero-config local path. If you need extensions beyond the managed build's default set, such as PostGIS, use an external PostgreSQL instance unless your managed PostgreSQL build explicitly includes them.

## Algolia importer automation

`ayb migrate algolia` ships for one-index Algolia record imports into PostgreSQL, with dry-run planning, JSON output, and optional supported synonym import. Algolia query-code migration is documented through AYB's shipped collection list API: `search`, `fuzzy`, `filter`, `facets`, result highlighting, typo-threshold controls, and operator-defined synonyms. AYB does not translate Algolia ranking rules or hosted index operations.

## Passkey resident-key registration

First-factor passkey login is shipped through the JavaScript SDK and React SDK, but resident-key / discoverable-credential registration is not yet enabled. The backend registration owner still calls `BeginRegistration` without resident-key options, so usernameless passkey login remains open.

## Other-language SDK passkey helpers

Search helper parity is shipped across the JavaScript, Go, Python, Dart, Kotlin, and Swift SDK list surfaces. Other-language SDK passkey helpers remain open: Go, Python, Dart, Kotlin, and Swift do not yet expose equivalents for the JavaScript SDK's `beginWebAuthnLogin`, `finishWebAuthnLogin`, and `signInWithPasskey` helpers.

## Local Supabase Export Caveat

This caveat only affects local development on macOS with Colima and does not affect customer cloud or self-hosted migrations. `supabase start` may fail on a Docker socket mount for Logflare/Vector; the local workaround is `supabase start -x logflare,vector`.

## Collection list endpoint scope

Geo / spatial search is not a shipped parity target on `GET /api/collections/{table}`; PostGIS-backed geo queries remain an external-extension path rather than a list-endpoint parameter. `distinct` / dedup is likewise not a shipped list-endpoint parameter; consumers needing distinct result sets rely on SQL views or post-processing.

## Dashboard search administration

The dashboard search-settings UI is limited to the shipped Search Playground and the per-collection Synonyms tab. A broader admin search configuration screen (index-wide ranking knobs, tokenizer overrides, etc.) is not shipped today.

## Screen-spec coverage

Screen-spec backfill in `docs/reference/screen_specs/` remains partial across the dashboard surface; not every shipped screen has a matching spec yet.
