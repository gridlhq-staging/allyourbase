# Live Polls (Slido-lite)

A real-time polling app powered by AYB. Audience members vote on polls and see live results update via SSE.

## Quick Start

```bash
ayb demo live-polls
```

Open http://localhost:5175, register an account, and start creating polls!

### Manual Setup

```bash
printf '\n[graphql]\nenabled = true\n' >> ayb.toml
ayb start
ayb sql < schema.sql
npm install
npm run dev
```

Verify GraphQL is mounted before running the frontend or E2E tests:

```bash
curl -s -o /dev/null -w %{http_code} http://localhost:8090/api/graphql
```

The status must be non-`404`.

## Features

- **Create polls** with 2-10 options
- **Real-time results** — vote counts and bar charts update live via SSE
- **One vote per user per poll** — enforced by DB constraint + RPC function
- **Change your vote** — `cast_vote()` upserts, so users can switch their answer
- **Close polls** — poll creator can close voting
- **Auth-gated** — sign up to create polls and vote

## Demonstrates

| Feature | How it's used |
|---------|--------------|
| REST API | CRUD for polls, options; list votes for bar charts |
| Auth | Email/password registration and login |
| Realtime SSE | Live vote count updates across all connected clients |
| RLS | Public read; only creator can update polls; one vote per user |
| Database RPC | `cast_vote()` function for atomic vote with duplicate enforcement |

## Testing

```bash
npm test            # 26 unit/component tests (vitest)
npm run test:watch  # watch mode
```

Playwright assumes GraphQL is enabled before the frontend starts (see Manual Setup above).

## Architecture

- **Schema**: `polls` → `poll_options` (1:N) → `votes` (1:N, unique per user per poll)
- **RLS policies**: Public read on all tables; insert-gated by `ayb.user_id`; only poll owner can close
- **`cast_vote()` RPC**: Atomic upsert — inserts a vote or updates the option if user already voted; rejects closed polls
- **Real-time sync**: Subscribe to `polls`, `poll_options`, `votes` tables; update local state on every SSE event
- **Bar chart**: CSS width transitions for smooth percentage bar animation
