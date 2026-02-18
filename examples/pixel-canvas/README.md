# Pixel Art Canvas (r/place clone)

A collaborative pixel art canvas powered by AYB. Exercises REST API, Auth, Realtime SSE, RLS, and Database RPC.

Deploy at `place.allyourbase.io` for viral marketing.

## Features

- **100x100 grid** with 16-color classic r/place palette
- **Real-time updates** via AYB's SSE — see other users' pixels appear live
- **Auth-gated placement** — sign up to place pixels (showcases registration flow)
- **5-second cooldown** between placements
- **Zoom and pan** — mouse wheel + drag to navigate the canvas
- **Keyboard shortcuts** — keys 1-9, 0, a-f for quick color selection
- **Optimistic updates** — pixel appears instantly, reverts on server error
- **Atomic upsert** via `place_pixel()` RPC function (avoids race conditions)

## AYB Features Showcased

| Feature | How it's used |
|---------|--------------|
| REST API | Load all pixels on page load (`GET /api/collections/pixels`) |
| Auth | Email/password registration and login |
| Realtime SSE | Live pixel updates from other users |
| RLS | Public read, authenticated-only write |
| Database RPC | `place_pixel()` function for atomic upsert |

## Setup

```bash
# 1. Start AYB
ayb start

# 2. Apply schema
psql $DATABASE_URL -f schema.sql

# 3. Install and run
npm install
npm run dev
```

Open http://localhost:5174

## Testing

```bash
npm test        # 23 unit tests (vitest)
npm run test:watch  # watch mode
```

## Architecture

- **Canvas rendering**: HTML5 Canvas API (not DOM elements) for performance with 10,000 cells
- **Grid state**: `Int8Array` for memory efficiency (-1 = empty, 0-15 = color index)
- **Pixel placement**: Optimistic update → RPC call → revert on failure
- **Real-time sync**: SSE events update the local grid state on create/update
