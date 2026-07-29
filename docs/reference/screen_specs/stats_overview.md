# Stats Overview

## Task

Review live server runtime, memory, and optional database-pool metrics from the admin dashboard.

## Layout

1. Header with `Stats` title.
2. Loading or error text below the title when data is unavailable.
3. Runtime metric grid with `Uptime`, `Go Version`, `Goroutines`, and `GC Cycles` cards.
4. `Memory` section with `Alloc` and `Sys` cards.
5. Optional `DB Pool` section with `Total`, `Idle`, `In Use`, and `Max` cards when pool data is present.

## State contract

### Loading
- Before stats data is available, the screen shows the `Stats` heading and `Loading...`.
- No stat cards are shown until data has loaded.

### Error
- Request failure shows the `Stats` heading and red text containing `error.message`, or `Failed to load` when the thrown value is not an `Error`.
- The error notice includes a `Retry` action that reruns the existing stats refresh callback.
- No stat cards are shown while the error state is active.

### Runtime metrics
- `stats-card-uptime` displays `uptime_seconds` formatted as minutes, hours/minutes, or days/hours/minutes.
- `stats-card-go-version` displays the exact Go version string.
- `stats-card-goroutines` and `stats-card-gc-cycles` display numeric runtime counts.

### Memory metrics
- `stats-card-alloc` and `stats-card-sys` display byte values as `B`, `KB`, or `MB` using one decimal place for KB/MB.
- The `Memory` heading separates runtime cards from memory cards.

### DB pool metrics
- When `db_pool_max` is present, show `DB Pool` with `stats-card-total`, `stats-card-idle`, `stats-card-in-use`, and `stats-card-max`.
- Missing optional pool values default to `0` for total, idle, and in-use; max uses the supplied value.
- When `db_pool_max` is absent, omit the DB pool section.

## Navigation

- Route: `/admin/` with admin view `stats`.
- Entry: Select `Stats` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Auto-refresh: Stays on `Stats` and polls the stats endpoint every five seconds.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Stats`, then the `Stats` heading is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/stats.spec.ts` and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a live admin stats snapshot is available, when the Stats page renders, then the uptime card contains the hand-formatted uptime derived from `uptime_seconds`. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/stats.spec.ts`.
- Given a live admin stats snapshot is available, when the Stats page renders, then the Go version, goroutine count, GC cycle count, allocated memory, and system memory cards match the snapshot values. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/stats.spec.ts`.
- Given the snapshot includes database-pool data, when the Stats page renders, then `Total`, `Idle`, `In Use`, and `Max` cards match the snapshot values. Evidence owner: existing conditional assertions in `ui/browser-tests-unmocked/smoke/stats.spec.ts`.
- Given the Stats page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Stats endpoint unavailable with status 404, 501, or 503: unmocked smoke coverage may skip because the runtime surface is not available in that environment.
- Uptime below one hour: display whole minutes.
- Uptime above one hour: display hours/minutes, or days/hours/minutes after 24 hours.
- Memory below 1024 bytes: display bytes without decimal places.
- Optional DB pool section absent: keep the runtime and memory cards visible without placeholder pool cards.
- Non-`Error` thrown value: show `Failed to load`.

## Current implementation gaps

- Evidence: `ui/src/components/StatsOverview.tsx`.
- Current: Unmocked browser coverage proves rendered live values and error retry recovery, but does not prove loading, endpoint-unavailable UI, or absent-DB-pool rendering.
- Target: Acceptance evidence should cover those visible states when deterministic unmocked runtime fixtures can exercise them without adding mocked-only coverage or a parallel test file.
- Evidence: `ui/browser-tests-unmocked/smoke/stats.spec.ts`.
