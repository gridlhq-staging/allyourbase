# Demos

Explore the live demo landing page at [https://demo.allyourbase.io](https://demo.allyourbase.io). All three demos run against the same AYB backend at [https://api.allyourbase.io](https://api.allyourbase.io).

## Kanban

A collaborative Kanban board demo that shows realtime board workflows built on AYB.

- Deployed app: [https://kanban.demo.allyourbase.io](https://kanban.demo.allyourbase.io)
- Source: [github.com/griddlehq/allyourbase/examples/kanban](https://github.com/griddlehq/allyourbase/tree/main/examples/kanban)

```bash
ayb demo kanban
# Open http://localhost:5173
```

What it shows:

- REST API CRUD for boards, columns, and cards
- Email/password authentication with JWT sessions
- Row-level security enforcement for user data boundaries
- Realtime SSE updates for create, move, and delete events
- Foreign key relationships from boards -> columns -> cards
- Drag-and-drop card movement with persisted ordering

For the full step-by-step build walkthrough, see [Tutorial: Realtime Kanban Board](/guide/tutorial-kanban).

## Live Polls

A realtime polling demo for creating polls and streaming live vote results to connected clients.

- Deployed app: [https://polls.demo.allyourbase.io](https://polls.demo.allyourbase.io)
- Source: [github.com/griddlehq/allyourbase/examples/live-polls](https://github.com/griddlehq/allyourbase/tree/main/examples/live-polls)

```bash
ayb demo live-polls
# Open http://localhost:5175
```

What it shows:

- REST API CRUD for polls and options
- Email/password authentication for poll creation and voting
- Realtime SSE vote-count updates across active clients
- Row-level security for poll ownership and write controls
- Database RPC (`cast_vote()`) for atomic one-vote-per-user behavior

## Movies

A vector-search demo that combines semantic movie search, note embedding, and BYOK chat over retrieved context.

- Deployed app: [https://movies.demo.allyourbase.io](https://movies.demo.allyourbase.io)
- Source: [github.com/griddlehq/allyourbase/examples/movies](https://github.com/griddlehq/allyourbase/tree/main/examples/movies)

```bash
ayb demo movies
# Open http://localhost:5177
```

What it shows:

- Vector search against a movies corpus using AYB vector capabilities
- Embedding user notes tied to selected movies
- Streaming chat responses over retrieved movie context
- BYOK provider-key flows for AI provider integration

- Scope: this demo indexes the deterministic seed corpus in [`examples/movies/seed.sql`](https://github.com/griddlehq/allyourbase/blob/main/examples/movies/seed.sql) with exactly three records (Inception, Arrival, Moonlight). It is intentionally small and illustrative, not a production search index.
