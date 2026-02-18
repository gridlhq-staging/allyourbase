# Kanban Board Demo

A realtime collaborative Kanban board (Trello-lite) built with Allyourbase. Demonstrates:

- **REST API** — CRUD operations on boards, columns, and cards
- **Authentication** — Email/password registration and login with JWT
- **Row-Level Security** — Users only see their own boards
- **Realtime SSE** — Live updates when cards are created, moved, or deleted
- **Foreign Key relationships** — Boards → Columns → Cards with cascading deletes
- **Drag-and-drop** — Move cards between columns with instant persistence

## Quick Start

### 1. Start Allyourbase

```bash
# Install AYB (if you haven't already)
curl -fsSL https://allyourbase.io/install.sh | sh

# Enable auth in ayb.toml
cat > ayb.toml <<EOF
[server]
host = "0.0.0.0"
port = 8090
cors_allowed_origins = ["*"]

[auth]
enabled = true
jwt_secret = "change-me-to-a-secret-at-least-32-chars-long!!"

[database]
# Leave empty for managed PostgreSQL (zero config)
EOF

# Start AYB
ayb start
```

### 2. Create the database tables

```bash
psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql
```

### 3. Run the demo

```bash
npm install
npm run dev
```

Open http://localhost:5173, register an account, and start creating boards!

## Features

### Drag & Drop
Move cards between columns. Changes persist instantly and sync across tabs via SSE.

### Realtime
Open two browser tabs. Create or move a card in one — it appears in the other instantly.

### Row-Level Security
Register two different users. Each user only sees their own boards. This is enforced at the Postgres level via RLS policies.

## Tech Stack

- React 18 + TypeScript + Vite
- Tailwind CSS
- [@allyourbase/js](https://www.npmjs.com/package/@allyourbase/js) SDK
- [@hello-pangea/dnd](https://github.com/hello-pangea/dnd) for drag-and-drop

## Schema

See [schema.sql](./schema.sql) for the complete database schema including RLS policies.

```
boards (id, title, user_id)
  └── columns (id, board_id, title, position)
       └── cards (id, column_id, title, description, position)
```
