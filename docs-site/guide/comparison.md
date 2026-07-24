<!-- audited 2026-06-04 -->

# Comparison

How Allyourbase compares to PocketBase and Supabase (self-hosted).

This matrix is intentionally conservative: AYB is marked `✅` only for features that are shipped in code today.

For measured binary size, startup time, memory usage, and API benchmark methodology, see [Performance](/guide/performance).

## Feature matrix

| Feature | PocketBase | Supabase (self-hosted) | Allyourbase |
| --- | --- | --- | --- |
| **Database** | SQLite | PostgreSQL | PostgreSQL |
| **Deployment model** | Single binary | Multi-container stack | Single binary |
| **Docker required** | No | Yes (typical self-host setup) | No |
| **Configuration surface** | Small config | Many services/env vars | One file (`ayb.toml`) |
| **Admin dashboard surface** | Core admin UI | Full admin UI | **Comprehensive built-in admin views + dynamic table browser** |
| **OAuth providers (built-in)** | Limited set | Configurable via GoTrue/providers | Google, GitHub, Microsoft, Apple, Discord, Twitter, Facebook, LinkedIn, Spotify, Twitch, GitLab, Bitbucket, Slack, Zoom, Figma, Notion |
| **Row-level security (RLS)** | No | Yes | Yes |
| **SAML / SSO** | No | Available with self-host setup constraints | Shipped for login, but assertion-signature verification is not yet performed ([Guide](/guide/saml)) |
| **Read replicas** | No | Manual PostgreSQL operations | ✅ |
| **Database branching** | No | Not built-in for self-host | ✅ |
| **Backups + PITR** | External tooling | PostgreSQL/infra dependent | ✅ |
| **AI assistant workflows** | No | No built-in assistant surface | ✅ |
| **Vector indexes / vector search** | No | Yes (`pgvector`) | ✅ ([Guide](/guide/ai-vector)) |
| **Custom domains** | Reverse proxy/manual | Reverse proxy/manual | ✅ ([Guide](/guide/custom-domains)) |
| **Log drains** | External tooling | External tooling | ✅ ([Guide](/guide/log-drains)) |
| **Audit logging** | Basic logs | Partial | ✅ |
| **MCP server** | No | No | ✅ |
| **Migration tools (source importers)** | No built-in import suite | SQL migration workflow | PocketBase, Supabase, Directus, Appwrite, Nhost, and one-index Algolia imports; Firebase importer is retired |
| **PostGIS spatial** | No | Yes | Requires external PostgreSQL with PostGIS; the default managed build excludes PostGIS binaries |
| **Push notifications** | No | External integration | ✅ |
| **SMS operations** | No | Auth OTP focused | ✅ |
| **Email templates** | No | Auth templates | ✅ |
| **Edge functions** | No | Yes | ✅ |
| **Materialized views tooling** | No | PostgreSQL-native/manual | ✅ |

## Migration tools

AYB ships built-in migration/import flows for these source platforms:

- PocketBase
- Supabase
- Algolia (`ayb migrate algolia` for one-index record imports)
- Directus
- Appwrite
- Nhost

Firebase importer is retired.

## Leaving PocketBase for Allyourbase

Use the [PocketBase migration guide](/guide/migrations#pocketbase) for the
supported one-command and explicit import flows. Review schema constraints,
relation handling, and file migration behavior before switching production
traffic.

## Leaving Supabase for Allyourbase

Use the [Supabase migration guide](/guide/migrations#supabase) for the supported
PostgreSQL import path. Database objects can move without implying parity with
every Supabase-hosted service, so inventory service dependencies separately.

## Leaving Firebase for Allyourbase

Firebase importer is retired. AYB does not provide Firestore-style offline/local-first sync. Use the [Firebase migration guide](/guide/migrations#firebase) for historical migration notes, then plan a custom export into PostgreSQL and rehearse the relational schema and identity mapping against a non-production export first.

## Leaving Algolia for Allyourbase

Use the [Algolia migration guide](/guide/migrating-from-algolia) for the shipped
one-index record importer and query mapping. Algolia-specific ranking controls
and hosted index operations do not have direct AYB equivalents.

## Leaving Directus for Allyourbase

Use the [Directus migration guide](/guide/migrations#directus) for database
metadata and content import behavior. Verify permissions and application-facing
queries independently before cutover.

## Leaving Appwrite for Allyourbase

Use the [Appwrite migration guide](/guide/migrations#appwrite) for database,
identity, and storage migration boundaries. Confirm unsupported source features
before choosing the final cutover sequence.

## Leaving Nhost for Allyourbase

Use the [Nhost migration guide](/guide/migrations#nhost) for the supported
PostgreSQL import path. Treat GraphQL, authentication, and operational service
parity as separate acceptance checks.

## Honest beta limits

Allyourbase is still beta software. Read the [beta limitations](/guide/beta-limitations)
before a production evaluation, and validate recovery, upgrades, migration
coverage, and every application-critical integration in your own environment.

## Search and Algolia

AYB's shipped search path is PostgreSQL search on the standard collection list endpoint, not a hosted-search replacement for every Algolia workflow. It is a fit when records already live in PostgreSQL and you want one API path for full-text `search`, per-collection synonym groups, optional `fuzzy=true` typo tolerance through `pg_trgm`, safe `filter` expressions, scalar `facets`, pagination, and RLS-scoped counts.

Keep Algolia when your product depends on Algolia-specific relevance controls,
Algolia ranking-rule translation, or hosted index operations. AYB does ship `ayb
migrate algolia` for one-index record imports, per-collection synonym groups,
fuzzy typo-threshold tuning, and `highlight=true` `_highlight` snippets; hosted
index operations and Algolia-specific ranking controls remain outside the AYB
search path.

For an Algolia-oriented migration map, see [Migrating from Algolia](/guide/migrating-from-algolia). For the canonical shipped AYB search behavior, see [Search](/guide/search).

## When to use Allyourbase

Choose AYB when you want a PostgreSQL backend platform that runs as a single binary while still shipping advanced admin capabilities (RLS, branching, replicas, backups/PITR, AI/vector tooling, audit logs, and operational controls). SAML login is available, but assertion-signature verification is not yet performed.

## When to use PocketBase

Choose PocketBase when SQLite is sufficient and you want the smallest operational footprint with minimal moving parts.

## When to use Supabase (self-hosted)

Choose Supabase self-hosted when you specifically want the Supabase ecosystem and are comfortable operating a multi-service container stack.
