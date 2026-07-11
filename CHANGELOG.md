# Changelog

All notable changes to Allyourbase will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Real AI token streaming: `GenerateTextStream` is now implemented for Ollama (NDJSON) and Anthropic (SSE), so chat surfaces stream tokens as they are generated instead of delivering one blob at the end. An OpenAI adapter ships alongside them.
- Multi-node enablers over a shared Postgres LISTEN/NOTIFY bus (`internal/pgnotify`): realtime events fan out across nodes, and token/session revocations propagate durably. AYB can run as N nodes behind a load balancer against one Postgres.
- Hosted/pooled multi-tenant mode: a `server.require_resolved_tenant` setting (env `AYB_SERVER_REQUIRE_RESOLVED_TENANT`, default off) makes an anonymous request whose tenant cannot be resolved fail closed instead of reading the ambient namespace. Single-tenant self-host behavior is unchanged.
- `make test-multinode`: an unattended two-node end-to-end lane (shared external Postgres + MinIO, OS-assigned ports) proving cross-node realtime delivery, cross-node session revocation, and per-tenant storage isolation.

### Changed

- Retired the legacy direct sync pipeline and Makefile guard in favor of Debbie-owned public sync.
- Resumable uploads stage their partial data in the configured storage backend rather than a node-local temp directory, so an upload started on one node can be completed by another.

### Fixed

- Storage is now tenant-isolated: object metadata and physical keys are scoped by `tenant_id` and signed URLs are bound to their tenant, fixing a cross-tenant overwrite where two tenants uploading the same bucket+name would clobber each other's bytes.
- Realtime subscriptions are tenant-scoped: a subscriber for one tenant no longer receives another tenant's row events for a same-named table, and delete events now respect row-level security instead of broadcasting to every subscriber of the table.
- Storage usage is accounted per tenant: two tenants sharing a user id no longer share one quota balance.

## [0.0.14-beta] - 2026-07-08

### Added

- Demo launch smoke coverage now has a shared `make test-demo-launch` owner for Kanban, Live Polls, and Movies, and the public release candidate gate runs it before publish.

### Changed

- CI and release workflows now run the demo launch smoke gate so demo startup regressions block both continuous validation and release publication.
- Debbie staging sync now resolves the active dev worktree from `.debbie.toml` instead of a hard-coded checkout path, so parallel release worktrees sync their own `HEAD`.

### Fixed

- `ayb demo movies` now pins `AYB_SERVER_PORT` to the port from the resolved demo URL, keeping the auto-started AYB server aligned with the CLI-selected base URL instead of the embedded Movies config port.

## [0.0.13-beta] - 2026-07-06

### Changed

- Project home moved to the `AllyourbaseHQ` GitHub organization. Source, releases, and container images now live at `github.com/AllyourbaseHQ/allyourbase` and `ghcr.io/allyourbasehq/allyourbase`; the `install.allyourbase.io` installer and all documentation point to the new location. Existing `0.0.12-beta` and earlier artifacts remain available under the previous org.

## [0.0.12-beta] - 2026-06-10

### Added

- Dashboard collection Search Settings UI now edits weighted searchable attributes and custom-ranking order.
- `ayb migrate algolia --include-settings` fetches Algolia index search settings and persists `searchableAttributes` as AYB search attribute weights and `customRanking` as persisted secondary sort through the existing `GET/PUT /api/collections/{table}/search-settings` owner; `attributesForFaceting` are reported as advisory-only skips.
- Persisted `customRanking` on `GET/PUT /api/collections/{table}/search-settings` now provides the default tie-break after search relevance, while a request-level `?sort=` overrides that saved ranking chain for that one request.
- JavaScript SDK `ayb.searchSettings` now exposes persisted collection search settings and synonym group management with typed `SearchSettings`, `SearchableAttribute`, `CustomRankingTie`, `SearchSynonymGroup`, `SearchSynonymsRequest`, and `SearchSynonymsResponse` exports.
- Facet value search: new `GET /api/collections/{table}/facets/{column}/search` endpoint returns prefix-matched bucket values with backend `<mark>` highlighting, RLS-scoped counts, and `maxFacetHits`/`filter`/`search` scoping; JS SDK `records.searchFacetValues()` and the InstantSearch adapter's `searchForFacetValues(requests)` ship over this endpoint, unlocking `RefinementList` searchable facet widgets against AYB.

### Changed

### Fixed

## [0.0.11-beta] - 2026-06-09

### Added

- Search relevance weighting: opt-in per-collection searchable-attributes config (`GET/PUT /api/collections/{table}/search-settings`) drives weighted `to_tsvector` ranking (`ts_rank_cd`), so a title match can outrank a body match; equal-weight behavior is unchanged when unconfigured. Custom-ranking secondary sort after relevance is supported.
- Disjunctive (OR) faceting: new `disjunctiveFacets` list parameter computes each listed facet's counts with its own equality predicate removed (multi-select RefinementList no longer collapses), plus per-numeric-facet `min`/`max` stats for range UIs.
- InstantSearch adapter depth: `@allyourbase/js/instantsearch` now wires OR-facets through `disjunctiveFacets` and translates numeric range refinements into `>=`/`<=` filters; the `examples/instantsearch_demo` app exercises multi-select facets + a range filter under browser-unmocked coverage.
- SDK install truthfulness: added READMEs for the Go, Python, Swift, and Kotlin SDKs and install sections for the React/SSR SDKs, each documenting the working from-source install.
- Public-exposure safety: the startup banner now warns when the server binds a non-loopback interface (e.g. `0.0.0.0`) with auth disabled — the open-database footgun. Auth remains open-by-default for local dev; this is a warning only.

### Changed

- `ayb demo` auto-recovers when it detects an AYB-owned auth-disabled server on its target port (the README quickstart sequence now works end-to-end). `ayb init` scaffolds the current `@allyourbase/js` major.

### Fixed

- Removed leaked absolute developer worktree paths from shipped source headers; fixed the docs-site GitHub links (`gridlhq` → `griddlehq`) and the default `ayb.toml` documentation URL; corrected the Dart SDK README's unresolvable pub.dev install claim; refreshed the two stale search screen specs.
- Eliminated a duplicated `Error:` prefix on the port-in-use CLI path.
- Path-leak hygiene: leaked worktree paths are now guarded out of shipped source-doc surfaces (Go doc-comment headers, generated `DIRMAP.md` rows) by the codehealth `TestNoLeakedWorktreePaths` guard, and a `make check-hygiene` / `make hygiene` flow wraps `scripts/strip-leaked-paths.sh` so contributors can re-strip leaks idempotently before pushing.

## [0.0.10-beta] - 2026-06-09

### Added

- Search-engine depth: collection search now ships `_highlightResult` metadata beside legacy `_highlight`, default English stemming, relevance-first ordering, hybrid search pagination after text/vector fusion, and semantic-mode rejection of incompatible `highlight`, `fuzzy`, `facets`, and `typo_threshold` modifiers across backend behavior, JS SDK types, OpenAPI, and the public search guide.
- Algolia importer: `ayb migrate algolia` imports one Algolia index into PostgreSQL with dry-run, JSON output, fixture-backed acceptance, optional supported synonym import, and public migration guidance.
- InstantSearch support: `@allyourbase/js/instantsearch` ships a one-index adapter over `records.list`, with empty-query browsing, facet transposition, `_highlightResult` passthrough, strict unsupported-feature rejection, a source-only `examples/instantsearch_demo` proof app, browser-unmocked coverage, and the dedicated public guide at `docs-site/guide/instantsearch.md`.

### Changed

- Beta limitations now live in `docs-site/guide/beta-limitations.md`, with the README linking there instead of duplicating caveats; codehealth status wording also reflects the live allowlisted Go file-size baseline and the empty function-size allowlist instead of claiming no oversized-file debt.
- Search helper parity is now documented as shipped across the JavaScript, Go, Python, Dart, Kotlin, Swift, and React-facing surfaces where those SDKs expose collection list helpers.
- Passkey helper parity remains open outside the JavaScript and React SDKs; resident-key passkey registration remains a backend limitation.

### Fixed

## [0.0.9-beta] - 2026-06-03

### Added
- Web hosting MVP: `ayb sites deploy` uploads static sites with deploy/promote/rollback lifecycle, host-based routing, and SPA fallback serving via the admin API
- Admin dashboard: new panels for organizations, tenants, usage metering, request logs, realtime event inspector, storage browser with CDN purge, and site/deploy management
- CLI UX overhaul: shared design system with branded output, animated step spinners, and styled error suggestions with fix hints
- `ayb demo` serves pre-built example apps with no Node.js dependency — embedded assets, API reverse proxy, and SPA fallback in a single command
- MCP server for AI coding tools (`ayb mcp`) — 13 tools, 2 resources, 3 prompts
- `ayb init` project scaffolding with 4 templates (React, Next.js, Express, plain TS)
- `ayb db backup` and `ayb db restore` commands
- `ayb stats` for server statistics
- `ayb rpc` for calling PostgreSQL functions from CLI
- `ayb query` for querying records from CLI
- Faceted search: REST `facets` query parameter returns per-value counts honoring RLS scoping; dashboard search playground exposes a faceted filter UI; JS SDK `ListParams` adds `facets` with `FacetCounts` response typing
- Login rate-limiting middleware throttles password and anonymous-auth attempts to mitigate credential-stuffing
- Search facet user guide published at `docs-site/guide/search.md`
- Security audit: auth bypass, RLS enforcement, API key scoping, secrets handling
- Performance baseline: 1.9K–21K req/s, 310ms startup, 20.5MB RSS
- OpenAPI spec served at `/api/openapi.yaml`

### Changed
- HTTP timeout hardening: read-header, idle, and TLS redirect timeouts on the server; 30-second default on CLI HTTP client; config file written with 0600 permissions
- Go 1.25 (upgraded from 1.24)
- License clarified as MIT across all artifacts
- Documentation expanded across guides, API references, and feature inventory

## [0.1.0] - 2026-02-08

Initial release.

### Added
- Single Go binary with embedded admin dashboard
- Auto-generated REST API from PostgreSQL schema (CRUD, filter, sort, search, pagination, FK expand, batch)
- Auth: email/password, JWT, OAuth (Google, GitHub), password reset, email verification
- Row-Level Security via Postgres RLS with JWT claims injected into session vars
- Realtime via Server-Sent Events with RLS-filtered change subscriptions
- File storage on local disk or S3-compatible object stores with signed URLs
- Webhooks with HMAC-SHA256 signing, retry with exponential backoff
- TypeScript SDK with auth state management, realtime subscriptions, OAuth flows
- CLI coverage for core operational workflows (start, stop, config, migrate, types, webhooks, storage, users, apikeys)
- Managed PostgreSQL for zero-config development (`ayb start` downloads Postgres automatically)
- Migration tools: PocketBase, Supabase, Firebase — one-command import with auth user preservation
- Non-expiring API keys with scope enforcement (readonly/readwrite/full, per-table restrictions)
- Full-text search via Postgres tsquery with relevance ranking
- Type generation from live schema (`ayb types typescript`)
- Email backends: log, SMTP, webhook
- Password hashing: argon2id, bcrypt, firebase-scrypt with progressive re-hashing
- Two example apps: Live Polls, Kanban Board
