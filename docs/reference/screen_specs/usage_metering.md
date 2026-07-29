# Usage Metering

## Task

Review tenant usage totals, filter the reporting period and metric, inspect trend and breakdown charts, and inspect limits for the selected tenant.

## Layout

1. Header with `Usage Metering` title, shared-contract subtitle, and `Refresh` action.
2. Usage filter controls for period, metric, granularity, and breakdown.
3. `Aggregate Usage` section with row count, tenant table, sortable tenant column, and pagination actions.
4. `Usage Trend` chart section.
5. `Usage Breakdown` chart section.
6. `Tenant Limits` panel for the currently selected tenant.
7. Inline partial refresh or partial error banner below the main sections when stale data remains visible.

## State contract

### Loading
- Before list, trend, or breakdown data exists, the screen shows a centered spinner with `Loading usage metering...`.
- When existing data is visible and a refresh or filter change is loading, the screen keeps the current sections visible and shows `Refreshing usage data...`.
- Tenant-limit refreshes show loading state inside the tenant-limits panel without replacing aggregate usage.

### Error
- Initial usage load failure shows a centered error state containing the returned error message, or `Failed to load usage data` when the thrown value is not an `Error`.
- The initial error state includes `Retry usage data`, which reruns the full usage load.
- If a later usage refresh fails after list data exists, the stale sections remain visible and an amber inline error banner shows the returned message.
- Tenant-limit failures stay scoped to `Tenant Limits` and show `Failed to load tenant limits` or the returned message.

### Filters
- `Period` supports today, last 7 days, and last 30 days.
- `Metric` supports API requests, storage bytes, bandwidth bytes, and function invocations.
- API requests support hour, day, week, and month granularity and tenant, endpoint, or status-code breakdown.
- Non-request metrics support day, week, and month granularity and tenant breakdown.
- Changing metric coerces unsupported granularity or breakdown values back to valid choices and resets pagination offset.
- Changing period resets pagination offset.

### Aggregate usage
- The aggregate heading is `Aggregate Usage` and the count reads `Showing <visible> of <total> tenants`.
- Empty aggregate results show `No tenant usage rows`.
- Rows show tenant name, request count, storage bytes, bandwidth bytes, and function invocations.
- Clicking a tenant row selects it for the tenant-limits panel.
- The tenant header sorts by tenant name, toggling ascending and descending order and resetting pagination offset.
- `Previous page` and `Next page` update the offset and are disabled at page boundaries.

### Charts and limits
- `Usage Trend` shows an SVG chart with `Usage trend chart` label when trend points exist, or `No trend data`.
- `Usage Breakdown` shows bar rows with `Usage breakdown chart` labels when breakdown entries exist, or `No breakdown data`.
- `Tenant Limits` prompts `Select a tenant to view limits` until a tenant is selected.
- When limits are available, the panel lists each metric with used, limit, and remaining values.

## Navigation

- Route: `/admin/` with admin view `usage`.
- Entry: Select `Usage Metering` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Refresh: Stays on `Usage Metering` and reloads usage data for the current query.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Usage Metering`, then the `Usage Metering` heading is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/usage-metering.spec.ts` and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given usage aggregation is configured and a tenant has seeded usage rows, when the user opens `Usage Metering`, then the seeded tenant name appears in the aggregate table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/usage-metering.spec.ts`.
- Given usage aggregation is configured, when the user opens `Usage Metering`, then `Usage Trend`, `Usage Breakdown`, and `Tenant Limits` headings are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/usage-metering.spec.ts`.
- Given usage aggregation is not configured and the API returns 503, when the user opens `Usage Metering`, then a usage-not-configured message is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/usage-metering.spec.ts`.
- Given the Usage page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Usage aggregation 503: show the service-provided not-configured message and allow the unmocked test to treat it as valid environment evidence.
- No tenant rows: show `No tenant usage rows`, disable next-page navigation, and show no selected tenant limits.
- Unsupported metric/filter combinations: coerce granularity and breakdown to valid choices.
- Stale refresh result: ignore stale async responses that lose the load sequence race.
- Limit-only failure: keep aggregate data, trend, and breakdown visible while the tenant-limits panel reports the limits error.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove filter coercion, sort toggling, pagination buttons, partial refresh banner, stale-response protection, empty aggregate state, chart data rendering, selected-tenant row styling, or tenant-limit values.
- Target: Acceptance evidence should cover those behaviors when fixture data can produce deterministic totals and multiple pages without excessive runtime.
- Evidence: `ui/src/components/UsageMetering.tsx`; `ui/src/components/UsageMeteringSections.tsx`; `ui/browser-tests-unmocked/smoke/usage-metering.spec.ts`.
