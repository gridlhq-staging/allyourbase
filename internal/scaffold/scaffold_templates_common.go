// Package scaffold.
package scaffold

import (
	"fmt"
	"strings"
)

// aybToml returns the default ayb.toml configuration file content with server, database, auth, storage, and admin settings.
func aybToml(opts Options) string {
	return `[server]
host = "127.0.0.1"
port = 8090

[database]
# Leave empty for managed Postgres (zero-config dev mode)
# url = "postgresql://user:pass@localhost:5432/mydb"

[auth]
enabled = true

[storage]
enabled = true
backend = "local"

[admin]
enabled = true
`
}

// schemaSQLFile returns the default PostgreSQL schema with an example items table and row-level security policies that restrict access by owner.
func schemaSQLFile() string {
	return `-- AYB Schema
-- Run with: psql $DATABASE_URL -f schema.sql
-- Or paste into the admin SQL editor at http://localhost:8090/admin
-- Starter search examples query the items table by name or description.

-- Example: users table with RLS
CREATE TABLE IF NOT EXISTS items (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    description TEXT,
    owner_id   UUID REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enable Row-Level Security
ALTER TABLE items ENABLE ROW LEVEL SECURITY;

-- Users can only see their own items
CREATE POLICY items_select ON items FOR SELECT
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only insert items they own
CREATE POLICY items_insert ON items FOR INSERT
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only update their own items
CREATE POLICY items_update ON items FOR UPDATE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only delete their own items
CREATE POLICY items_delete ON items FOR DELETE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);
`
}

// envFile returns the .env template documenting AYB environment variables for server port, database URL, authentication, and admin settings.
func envFile() string {
	return `# AYB environment variables
# Copy to .env.local for overrides

# Server
AYB_SERVER_PORT=8090

# Database (leave empty for managed Postgres)
# AYB_DATABASE_URL=postgresql://user:pass@localhost:5432/mydb

# Auth
AYB_AUTH_ENABLED=true
# AYB_AUTH_JWT_SECRET=  # auto-generated if not set

# Admin
AYB_ADMIN_ENABLED=true
# AYB_ADMIN_PASSWORD=  # set for admin dashboard access
`
}

func gitignoreFile(tmpl Template) string {
	base := `node_modules/
dist/
.env.local
.env.*.local
*.log
.DS_Store
`
	switch tmpl {
	case TemplateNext:
		base += ".next/\n"
	}
	return base
}

// claudeMD returns the project CLAUDE.md documentation with quick start instructions, API reference links, and SDK usage examples.
func claudeMD(opts Options) string {
	return fmt.Sprintf(`# %s

Built with [Allyourbase](https://allyourbase.io) - Backend-as-a-Service for PostgreSQL.

## Quick Start

`+"```"+`bash
# Start AYB (managed Postgres, zero config)
ayb start

# Apply schema
ayb sql < schema.sql

# Generate TypeScript types
ayb types typescript -o src/types/ayb.d.ts
`+"```"+`

## API Reference

- **REST API**: http://localhost:8090/api
- **Admin Dashboard**: http://localhost:8090/admin
- **API Schema**: http://localhost:8090/api/schema

## AYB SDK

Run `+"`"+`ayb sql < schema.sql`+"`"+` first so the `+"`"+`items`+"`"+` table exists before trying the starter search example.

`+"```"+`ts
import { AYBClient } from "@allyourbase/js";
const ayb = new AYBClient("http://localhost:8090");

// Search records with AYB's shipped list-search path
const search = "demo";
const { items } = await ayb.records.list("items", { search, fuzzy: true });

// CRUD
const item = await ayb.records.create("items", { name: "New Item" });
await ayb.records.update("items", item.id, { name: "Updated" });
await ayb.records.delete("items", item.id);

// Auth
await ayb.auth.login("user@example.com", "password");
const me = await ayb.auth.me();
`+"```"+`
`, opts.Name)
}

// packageJSON returns a package.json string for the given scaffold template (react, next, or plain/express), including the appropriate dependencies and build scripts.
func packageJSON(opts Options, tmpl string) string {
	name := strings.ToLower(opts.Name)

	switch tmpl {
	case "react":
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@allyourbase/js": "^0.2.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^6.0.0"
  }
}
`, name)
	case "next":
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start"
  },
  "dependencies": {
    "@allyourbase/js": "^0.2.0",
    "next": "^15.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "typescript": "^5.0.0"
  }
}
`, name)
	default:
		return nodePackageJSON(name)
	}
}

// nodePackageJSON returns a minimal package.json for plain Node.js and Express scaffold templates with tsx for development and tsc for production builds.
func nodePackageJSON(name string) string {
	return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "tsx watch src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@allyourbase/js": "^0.2.0"
  },
  "devDependencies": {
    "@types/node": "^22.0.0",
    "tsx": "^4.0.0",
    "typescript": "^5.0.0"
  }
}
`, name)
}

// aybClient returns the TypeScript source for a shared AYB client module that initializes the SDK with the server URL and provides in-memory session token management.
func aybClient() string {
	return `/// <reference types="vite/client" />
import { AYBClient } from "@allyourbase/js";
` + scaffoldAYBClientTypes() + `

const AYB_URL = import.meta.env.VITE_AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL) as unknown as ScaffoldAYBClient;

// Keep auth tokens in memory by default. Persisting bearer tokens in
// localStorage makes XSS impact much worse for scaffolded browser apps.
export function setSessionTokens(token: string, refreshToken: string) {
  ayb.setTokens(token, refreshToken);
}

export function clearSessionTokens() {
  ayb.clearTokens();
}

export function isLoggedIn(): boolean {
  return typeof ayb.token === "string" && typeof ayb.refreshToken === "string";
}
`
}

func aybClientNode() string {
	return `import { AYBClient } from "@allyourbase/js";
` + scaffoldAYBClientTypes() + `

const AYB_URL = process.env.AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL) as unknown as ScaffoldAYBClient;
`
}

func scaffoldAYBClientTypes() string {
	return `
type ScaffoldFacetValue = string | number | boolean | null;

type ScaffoldFacetBucket = {
  value: ScaffoldFacetValue;
  count: number;
};

type ScaffoldListParams = {
  search?: string;
  fuzzy?: boolean;
  filter?: string;
  facets?: string[];
  page?: number;
  perPage?: number;
  sort?: string;
  [key: string]: unknown;
};

type ScaffoldListResponse<T = Record<string, unknown>> = {
  items: T[];
  page?: number;
  perPage?: number;
  totalItems?: number;
  totalPages?: number;
  facets?: Record<string, ScaffoldFacetBucket[]>;
};

type ScaffoldAYBClient = AYBClient & {
  health(): Promise<{ status: string }>;
  records: AYBClient["records"] & {
    list<T = Record<string, unknown>>(
      collection: string,
      params?: ScaffoldListParams,
    ): Promise<ScaffoldListResponse<T>>;
  };
};
`
}
