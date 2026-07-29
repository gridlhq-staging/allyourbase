# Performance Advisor

## Task

Review slow-query telemetry, change the analysis time range, page through query rows, and inspect query details.

## Layout

1. Header row with `Performance Advisor` title and `Time range` select.
2. Loading, error, or stale telemetry banner below the header when applicable.
3. Empty-state text when no slow queries are available for the selected range.
4. Query table with `Fingerprint`, `Mean ms`, `Total ms`, `Calls`, and `Rows` columns.
5. Pagination controls when query count exceeds one page.
6. Optional query-detail panel for the selected fingerprint.

## State contract

### Loading
- The panel remains mounted with test id `performance-advisor-panel`.
- Before the first report for the selected range is available, show `Loading performance telemetry...`.
- The `Time range` control remains visible while loading.

### Error
- Request failure shows red inline text from `toPanelError(error)`.
- The error notice includes a `Retry` action that reruns the existing polling refresh while preserving the range control and advisor panel shell.
- Existing report data remains visible if a later poll fails.

### Time range
- `Time range` defaults to `1h` unless URL query param `perfRange` provides a range.
- Valid ranges are `15m`, `1h`, `6h`, `24h`, and `7d`.
- Changing the range updates `perfRange` in the URL and refreshes advisor data for that range.

### Query table
- When `data.stale` is true, show `Telemetry may be stale`.
- When the current page has no rows, show `No slow queries`.
- Rows are paginated at `PAGE_SIZE=20`.
- Table columns are `Fingerprint`, `Mean ms`, `Total ms`, `Calls`, and `Rows`.
- Fingerprint cells are buttons that select a row for detail display.

### Pagination and detail
- Show `Prev`, `<page>/<totalPages>`, and `Next` only when `totalPages > 1`.
- `Prev` is disabled on the first page; `Next` is disabled on the last page.
- If the current page becomes greater than the total page count after data changes, clamp it to the last page.
- Selecting a fingerprint shows `Query Detail: <fingerprint>`, normalized SQL, trend, and endpoint list.

## Navigation

- Route: `/admin/` with admin view `performance-advisor`.
- Entry: Select `Performance Advisor` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Time range: Stays on `Performance Advisor` and updates URL query param `perfRange`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Performance Advisor`, then the `Performance Advisor` heading and `performance-advisor-panel` are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/performance-advisor-view.spec.ts`, `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a live performance advisor report is available, when the screen renders for `1h`, then the `Time range` select is visible and has the report range. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/performance-advisor-view.spec.ts`.
- Given a live performance advisor report is available, when the screen renders, then it shows either the `Fingerprint` table header or `No slow queries`, matching whether the report has query rows. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/performance-advisor-view.spec.ts`.
- Given the user selects `24h`, when the report has query rows, then the URL includes `perfRange=24h` and the table row count equals the live report row count capped to `PAGE_SIZE=20`. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the selected range has query rows, when the user selects a fingerprint, then `Query Detail: <fingerprint>` and the normalized query text are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the selected range has no query rows, when the screen renders, then `No slow queries` is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the Performance Advisor page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Advisor endpoint unavailable with status 404, 501, or 503: unmocked coverage may skip because the runtime surface is not available in that environment.
- Empty selected range: show `No slow queries`.
- More than 20 queries: show only the selected page and expose pagination controls.
- Range change while on a high page: clamp the current page to the last available page for the new result set.
- Stale telemetry: show `Telemetry may be stale` without hiding the report.
- Query with no endpoints: show an empty endpoint list under the detail panel.

## Current implementation gaps

- Evidence: `ui/src/components/PerformanceAdvisor.tsx`.
- Current: Unmocked browser coverage proves live content, range switching, URL persistence, row count, detail expansion, empty state, error retry recovery, and accessibility, but does not prove loading, stale banner, multi-page Prev/Next behavior, page clamping after range changes, or endpoint-list edge cases.
- Target: Acceptance evidence should cover those visible states when deterministic unmocked advisor fixtures can exercise them without adding mocked-only coverage or a parallel test file.
- Evidence: `ui/browser-tests-unmocked/smoke/performance-advisor-view.spec.ts`; `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
