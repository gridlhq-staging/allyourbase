# SMS Health

## Task

Review sent/confirmed/failed SMS counts and conversion rates across the today, last-7-day, and last-30-day windows to spot delivery problems.

## Layout

1. `SMS Health` heading.
2. Optional warning badge shown above the stat grid when the health response carries a `warning`.
3. Responsive grid of three stat cards: `Today`, `Last 7 Days`, `Last 30 Days`.
4. Each card lists `Sent`, `Confirmed`, `Failed`, and a `Conversion Rate` percentage divided from the counts by a top border.

## State contract

### Loading
- While `getSMSHealth` is in flight and before any data is rendered, the screen shows a centered spinner with `Loading...`.
- The loading view replaces the whole screen; no stat cards or heading are shown yet.

### Error
- When `getSMSHealth` rejects, the screen shows a centered error state with an alert icon and the thrown error message, or `Failed to load SMS health` when the thrown value is not an `Error`.
- The error state includes a `Retry` action.
- Clicking `Retry` clears the error, sets loading true, and reruns `getSMSHealth`.

### Populated stats
- The three cards render in fixed order: `Today`, `Last 7 Days`, `Last 30 Days`.
- Each card shows the window's `Sent`, `Confirmed` (green), and `Failed` (red) counts and a `Conversion Rate` rendered to one decimal place with a `%` suffix.
- A window with zero sent renders `0.0%` rather than `NaN`.

### Warning
- When the health response includes a `warning` string, a warning badge with an alert icon and the warning text is shown above the stat grid.
- When no `warning` is present, no warning badge is rendered.

## Navigation

- Route: `/admin/` with the `SMS Health` sidebar item selected (view id `sms-health`).
- Entry: Select `SMS Health` from the admin sidebar messaging group.
- Back: Browser back follows the admin app history.
- Retry: stays on `SMS Health` and reloads the health data.

## Acceptance criteria

- Given seeded SMS daily counts, when the user opens `SMS Health`, then the `SMS Health` heading and the `Today`, `Last 7 Days`, and `Last 30 Days` card labels are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/sms-health.spec.ts`.
- Given seeded daily counts, when the user opens `SMS Health`, then the seeded `Sent`, `Confirmed`, `Failed`, and conversion-rate values render in each window card. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given a low conversion rate below the warning threshold, when the user opens `SMS Health`, then the warning badge is visible with the warning text. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given a healthy conversion rate above the threshold, when the user opens `SMS Health`, then no warning badge is shown. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given a window with zero sent messages, when the user views the card, then the conversion rate shows `0.0%` and never `NaN`. Evidence owner: existing assertion in `ui/src/components/__tests__/SMSHealth.test.tsx`.
- Given the health request fails, when the error state renders, then the error message and a `Retry` action are visible and `Retry` re-fetches. Evidence owner: existing assertions in `ui/src/components/__tests__/SMSHealth.test.tsx`.

## Edge cases

- Zero-activity window: show `0` counts and a `0.0%` conversion rate instead of an empty or `NaN` value.
- SMS provider not configured: the smoke test skips seeded-value assertions when the provider probe reports the provider is unconfigured.
- Load failure: replace the stat grid with the error state and offer `Retry` rather than showing stale cards.
- Warning absent: omit the warning badge entirely rather than rendering an empty badge.

## Current implementation gaps

- Current: The initial `Loading...` state has no dedicated unmocked browser assertion; it is only covered by the `ui/src/components/__tests__/SMSHealth.test.tsx` render test.
- Target: An unmocked probe could assert the loading spinner if a stable slow-response fixture becomes available without mocked routes.
- Evidence: `ui/src/components/SMSHealth.tsx`; `ui/src/components/__tests__/SMSHealth.test.tsx`; `ui/browser-tests-unmocked/smoke/sms-health.spec.ts`.
