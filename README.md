# 👾 Allyourbase

Open-source backend for PostgreSQL. Single binary. Auto-generated REST API, auth, realtime, storage, admin dashboard.

Like PocketBase, but Postgres.

```bash
curl -fsSL https://allyourbase.io/install.sh | sh
ayb start
```

No Docker. No config. On first run, AYB downloads a prebuilt PostgreSQL binary for your platform and manages it as a child process — no system install required.

## Who is this for?

- **Indie devs and small teams** who want a full backend without managing infrastructure. One binary, one command, done.
- **AI-first projects** building with Claude Code, Cursor, or Windsurf. The built-in MCP server gives AI tools direct access to your backend.
- **PocketBase graduates** who hit the SQLite ceiling and need Postgres — concurrent writes, RLS, extensions, horizontal scaling — without rewriting everything.

## Quickstart

Create a table:

```bash
ayb sql "CREATE TABLE posts (
  id serial PRIMARY KEY,
  title text NOT NULL,
  body text,
  created_at timestamptz DEFAULT now()
)"
```

You now have a full REST API:

```bash
# Create
curl -X POST http://localhost:8090/api/collections/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello world", "body": "First post"}'

# List (with sort, pagination)
curl 'http://localhost:8090/api/collections/posts?sort=-created_at&perPage=10'

# Admin dashboard
open http://localhost:8090/admin
```

Every table gets CRUD, filtering, sorting, pagination, full-text search, and FK expansion.

## Migration

Coming from another platform? Import your users, data, and policies:

```bash
# PocketBase — point at your pb_data directory
ayb start --from ./pb_data

# Supabase
ayb migrate supabase --source-url postgres://...supabase... --database-url postgres://localhost/mydb

# Firebase
ayb migrate firebase --auth-export users.json --firestore-export firestore.json --database-url postgres://localhost/mydb
```

Imports users (with password hashes), data, OAuth providers, and RLS policies.

## Features

- **REST API** — CRUD for every table. Filter, sort, paginate, full-text search, FK expand.
- **Auth** — email/password, JWT, OAuth (Google/GitHub), email verify, password reset
- **Realtime** — SSE subscriptions per table, filtered by RLS
- **Row-Level Security** — JWT claims mapped to Postgres session vars. Write policies in SQL.
- **Storage** — local disk or S3-compatible (R2, MinIO, DO Spaces, AWS)
- **Admin dashboard** — SQL editor, API explorer, schema browser, RLS manager, user management
- **RPC** — call Postgres functions via `POST /api/rpc/{function}`
- **Type generation** — `ayb types typescript` emits types from your schema
- **Embedded Postgres** — zero external dependencies for development
- **MCP server** — `ayb mcp` gives AI tools (Claude Code, Cursor, Windsurf) direct access to your schema, records, SQL, and RLS policies. 11 tools, 2 resources, 3 prompts.

Your data lives in standard PostgreSQL. No lock-in — take your database and go.

## Demos

Three example apps ship in [`/examples`](examples/). Each takes ~2 minutes to set up.

**[Live Polls](examples/live-polls/)** — real-time polling app. Create polls, vote, watch bar charts update live across all browsers via SSE. Shows off auth, RLS, realtime, and database RPC.

**[Pixel Canvas](examples/pixel-canvas/)** — collaborative r/place clone. 100x100 grid, 16 colors, real-time pixel placement. Good stress test of SSE with many concurrent updates.

**[Kanban Board](examples/kanban/)** — Trello-lite with drag-and-drop. Per-user boards via RLS, realtime sync across tabs.

```bash
cd examples/live-polls
psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql
npm install && npm run dev
```

## SDK

```bash
npm install @allyourbase/js
```

```typescript
import { AYBClient } from "@allyourbase/js";
const ayb = new AYBClient("http://localhost:8090");

// Records
const { items } = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
  expand: "author",
});
await ayb.records.create("posts", { title: "New post" });

// Auth
await ayb.auth.login("user@example.com", "password");

// Realtime
ayb.realtime.subscribe(["posts"], (e) => {
  console.log(e.action, e.record);
});
```

## Existing database

Point at any Postgres instance. Existing tables become API endpoints on startup.

```bash
ayb start --database-url postgresql://user:pass@localhost:5432/mydb
```

## Config

Zero config by default. Customize via `ayb.toml`, env vars (`AYB_` prefix), or CLI flags.

```toml
[server]
port = 8090

[database]
url = "postgresql://user:pass@localhost:5432/mydb"

[auth]
enabled = true

[storage]
backend = "s3"
```

Precedence: defaults → `ayb.toml` → env vars → CLI flags. Check resolved config: `ayb config`.

## CLI

```
ayb start                Start server (embedded or external Postgres)
ayb sql "..."            Execute SQL
ayb schema [table]       Inspect database schema
ayb migrate up           Apply pending migrations
ayb migrate create       Create a new migration
ayb migrate pocketbase   Import from PocketBase
ayb admin reset-password Reset admin password
ayb apikeys create       Create an API key
ayb types typescript     Generate TypeScript types
ayb mcp                  Start MCP server for AI tools
```

29 commands total. Run `ayb --help` or `ayb <command> --help` for the full list.

## vs. PocketBase vs. Supabase

| | PocketBase | Supabase (self-hosted) | Allyourbase |
|---|---|---|---|
| Database | SQLite | PostgreSQL | PostgreSQL |
| Deployment | Single binary | 10+ Docker containers | Single binary |
| Config | One file | Dozens of env vars | One file (or none) |
| Row-level security | No | Yes | Yes |
| Docker required | No | Yes | No |
| AI/MCP server | No | No | Yes |

[Full comparison →](https://allyourbase.io/guide/comparison)

## Install (Staging) options

```bash
# Homebrew
brew install gridlhq/tap/ayb

# Docker
docker run -p 8090:8090 ghcr.io/gridlhq-staging/allyourbase

# Go
go install github.com/allyourbase/ayb/cmd/ayb@latest

# From source
git clone https://github.com/gridlhq-staging/allyourbase.git && cd allyourbase && make build

# Specific version
curl -fsSL https://allyourbase.io/install.sh | sh -s -- v0.1.0
```

## License

[MIT](LICENSE)
