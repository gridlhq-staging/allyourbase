# Realtime Inspector

## Task

Inspect live realtime connection metrics, subscription counts, dropped-message counters, and heartbeat failures.

## Layout

1. Header row with `Realtime Inspector` title and `Refresh` action.
2. Inline red error text when realtime telemetry fails to load.
3. Loading text while the first realtime snapshot is waiting.
4. Five metric cards for total, SSE, WebSocket, dropped messages, and heartbeat failures.
5. `Subscriptions` panel with `Filter subscriptions` input.
6. Subscription table with `Name`, `Type`, and `Count` columns, or an empty-state message.

## State contract

### Loading
- The panel remains mounted with test id `realtime-inspector-panel`.
- Before the first snapshot is available, show `Loading realtime telemetry...`.
- Metric cards remain visible and display `0` while data is absent.

### Error
- Request failure shows red inline text from `toPanelError(error)`.
- The error notice includes a `Retry` action that reruns the polling request; the `Refresh` button remains visible and can also retry.
- Existing metric and subscription data remains visible if a later poll fails.

### Metrics
- `realtime-total-metric` and `realtime-total-metric-value` show total realtime connections.
- `realtime-sse-metric` and `realtime-sse-metric-value` show SSE connections.
- `realtime-ws-metric` and `realtime-ws-metric-value` show WebSocket connections.
- `realtime-dropped-metric` and `realtime-dropped-metric-value` show dropped messages.
- `realtime-heartbeat-failures-metric` and `realtime-heartbeat-failures-metric-value` show heartbeat failures.

### Subscriptions
- `Filter subscriptions` filters the loaded subscription list by subscription name.
- Subscriptions are sorted by descending count.
- When data has loaded and no subscriptions match, show `No active subscriptions`.
- When subscriptions are present, show `Name`, `Type`, and `Count` columns with type badges.

### Refresh
- `Refresh` reruns the realtime stats request and keeps the user on the same screen.
- After refresh, the metric cards and subscription table reflect the latest snapshot.

## Navigation

- Route: `/admin/` with admin view `realtime-inspector`.
- Entry: Select `Realtime Inspector` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Refresh: Stays on `Realtime Inspector` and reloads telemetry.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Realtime Inspector`, then the `Realtime Inspector` heading and `realtime-inspector-panel` are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/realtime-inspector.spec.ts`, `ui/browser-tests-unmocked/smoke/realtime-inspector-view.spec.ts`, `ui/browser-tests-unmocked/full/realtime-inspector-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a live realtime snapshot is available, when the screen renders, then total, SSE, WebSocket, dropped-message, and heartbeat-failure metric values match the snapshot. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/realtime-inspector.spec.ts` and `ui/browser-tests-unmocked/full/realtime-inspector-lifecycle.spec.ts`.
- Given a live realtime snapshot is available, when the screen renders, then the subscriptions section shows either the `Name` table header or `No active subscriptions`. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/realtime-inspector-view.spec.ts` and `ui/browser-tests-unmocked/full/realtime-inspector-lifecycle.spec.ts`.
- Given a WebSocket subscription is opened through the real realtime path, when the inspector refreshes, then WebSocket, total, and `users` table subscription counts increase and return to baseline after cleanup. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/realtime-inspector.spec.ts`.
- Given the user clicks `Refresh`, when the realtime stats response succeeds, then the heading remains visible and the metric values remain visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/realtime-inspector.spec.ts` and `ui/browser-tests-unmocked/full/realtime-inspector-lifecycle.spec.ts`.
- Given the Realtime Inspector page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Realtime endpoint unavailable with status 404, 501, or 503: unmocked coverage may skip because the runtime surface is not available in that environment.
- Empty subscriptions: show `No active subscriptions`.
- Filter with no matching subscription names: show `No active subscriptions`.
- Missing initial data: metric cards display `0` until the first snapshot arrives.
- Later polling failure: show the error while keeping the panel controls available.

## Current implementation gaps

- Current: Unmocked browser coverage proves live metrics, refresh/retry recovery, subscriptions table-or-empty, deterministic no-match filtering, and WebSocket lifecycle counts, but does not prove loading, sort order, or every subscription filter branch.
- Target: Acceptance evidence should cover those visible states when deterministic unmocked realtime fixtures can exercise them without adding mocked-only coverage or a parallel test file.
- Evidence: `ui/src/components/RealtimeInspector.tsx`; `ui/browser-tests-unmocked/smoke/realtime-inspector.spec.ts`; `ui/browser-tests-unmocked/smoke/realtime-inspector-view.spec.ts`; `ui/browser-tests-unmocked/full/realtime-inspector-lifecycle.spec.ts`.
