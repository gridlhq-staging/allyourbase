# Analytics

## Task

Inspect request traffic in a rich request-log viewer while preserving the existing query-performance workflow.

## Layout

1. Header: `Analytics` title and `Request logs and query performance insights` subtitle.
2. Tabs: `Request Logs` is the default rich-viewer tab; `Query Performance` remains available with the same sort choices, statistics columns, empty state, and index-suggestion presentation as today.
3. Request-log toolbar (`data-testid="request-logs-toolbar"`): draft filters for method, path, exact status code, status class, from date, to date, minimum latency, and maximum latency; `Apply Filters`, `Reset`, `Export JSON`, and `Export CSV` actions.
4. Request-log charts (`data-testid="request-log-aggregate-charts"`): responsive inline-SVG request volume and status-class panels sit between the toolbar and results summary. The volume chart uses `items[].bucket` and `items[].count`; the stacked status chart uses `status_2xx` through `status_5xx`, with distinct theme-aware colors and a visible legend. Both charts render at most 120 marks: longer aggregate responses are folded into equal contiguous minute groups whose counts are summed, and the grouping is disclosed in `data-testid="request-log-chart-bucket-reduction"`.
5. Request-log results summary (`data-testid="request-logs-summary"`): shows total matching rows, current page range, page size, and the active applied-filter summary. The same summary row contains the `Live (periodic refresh)` toggle and `request-logs-live-status`.
6. Request-log table (`data-testid="request-logs-table"`): columns are `Time`, `Method`, `Path`, `Status`, `Duration`, `Size`, and `Identity`. Each row is focusable, has `data-testid="request-log-row-<id>"`, and opens the details drawer on click, Enter, or Space. The row action is also exposed as a `View details` button with `data-testid="request-log-view-details-<id>"`.
7. Request-log table allocation: table cells show `timestamp` as localized date/time, `method`, `path`, `status_code` with the existing status-badge semantics, `duration_ms` in milliseconds, request/response byte summary from `request_size` and `response_size`, and identity summary from `user_id` or `api_key_id`.
8. Pager (`data-testid="request-logs-pager"`): previous and next buttons plus visible limit/offset-derived range. Buttons use accessible labels `Previous request-log page` and `Next request-log page`.
9. Request details drawer (`data-testid="request-log-drawer"`): opens from the right without leaving `Analytics`, traps focus while open, returns focus to the originating row/action on close, closes on `Close`, Escape, or backdrop click, and presents all API fields: `id`, `timestamp`, `method`, `path`, `status_code`, `duration_ms`, `user_id`, `api_key_id`, `ip_address`, `request_id`, `request_size`, and `response_size`.
10. Drawer field allocation: drawer fields use stable test IDs `request-log-detail-id`, `request-log-detail-timestamp`, `request-log-detail-method`, `request-log-detail-path`, `request-log-detail-status-code`, `request-log-detail-duration-ms`, `request-log-detail-user-id`, `request-log-detail-api-key-id`, `request-log-detail-ip-address`, `request-log-detail-request-id`, `request-log-detail-request-size`, and `request-log-detail-response-size`.
11. Query-performance panel: `Sort by` select supports `Total Time`, `Calls`, and `Mean Time`; rows show query text, calls, average execution time, total execution time, rows, and index-suggestion statements with confidence labels. This tab keeps the current `adminQueryAnalyticsResponse` contract unchanged.

## State contract

### Loading
- Initial load shows a centered spinner with `Loading analytics...`.
- Request-log filter, reset, pagination, retry, and tab-load requests set a visible loading state while preserving the last successful page until replacement data arrives.
- Request charts show `Loading request charts…` before the first aggregate response and keep the last successful marks visible during subsequent reloads.
- Export actions show a pending state only for the export button that was invoked; the table remains readable.
- Live starts in `Off`; enabling it moves through `Connecting` to `Live` after the filtered SSE stream reports ready. Stream errors move to `Error` and keep existing rows visible. Turning Live off, leaving the tab, or unmounting aborts the stream and returns to `Off`.

### Error
- Initial request-log or query-stat load failure shows a centered error state containing the returned error message, or `Failed to load request logs` / `Failed to load query stats` for non-`Error` throws.
- Retry is visible as a button, reloads the current tab with the current applied request filters or query sort, and preserves draft filter values.
- Subsequent request-log failures keep the last successful table visible, show a retryable inline error with `data-testid="request-logs-error"`, and do not silently clear applied filters or the selected page.
- Aggregate failures show the returned message in the chart region, preserve existing chart marks when available, and expose `Retry request charts` without replacing the request-log table.
- An aggregate error outranks both the chart loading and chart empty states, so a failed refresh after an empty result shows the message and `Retry request charts` instead of reporting no activity.

### Request logs
- `Request Logs` is the default tab and owns the rich request-log workflow.
- Filter controls edit draft values only. API reloads happen only when the user chooses `Apply Filters`, `Reset`, pager controls, retry, or export.
- Applying filters commits the draft filter set, resets `offset` to `0`, and reloads page one.
- Reset clears all draft and applied filters, resets `offset` to `0`, and reloads the unfiltered page.
- Existing exact filters remain: method, path with `*` wildcard semantics, exact `status_code`, from date, and to date.
- Target filters add status class (`2xx`, `3xx`, `4xx`, `5xx`) plus minimum and maximum latency in milliseconds. Invalid latency ranges show an inline validation message and do not call `listRequestLogs`.
- `count`, `limit`, and `offset` drive visible pagination. Previous is disabled at `offset <= 0`; next is disabled when `offset + items.length >= count`. A stale page where `offset >= count` and `count > 0` resets to the last valid page and announces the corrected range.
- Empty request-log results show `No request logs found` and keep the filter toolbar and export actions visible.
- Empty aggregate results with no pending error show `No request activity matches these filters.`; charts never render fabricated bars or sample series.
- Both charts have accessible image names, and each bucket mark exposes its timestamp and exact count through SVG title text and stable `data-testid`/`data-count` attributes.
- Bucket labels are UTC. When the rendered buckets all fall on one UTC day, labels show time only (`12:00 PM UTC`); when they span more than one UTC day, both visible labels and SVG title text include the date (`Jul 27, 12:00 AM UTC`). Folded marks label their span as `<start> – <end> UTC`.
- Visible tick labels are thinned to at most 12 per chart (`data-testid="request-log-bucket-label"`) while every mark keeps its full timestamp in accessible title text.
- Optional identity/network fields render deterministically: missing `user_id`, `api_key_id`, `ip_address`, or `request_id` display `-` in the table or drawer, never blank space.
- Byte fields render in human-readable units while preserving exact bytes in drawer labels; time fields render localized display text while preserving the original timestamp in the drawer.
- JSON and CSV exports include all rows matching the applied filters, not only the visible page. CSV string cells with spreadsheet-formula prefixes are neutralized before download. Empty exports do not download a file and show `No matching request logs to export`.
- Export filenames include `request_logs`, the format extension, and an ISO-like timestamp. Export conventions follow the established in-browser Blob download pattern and toast/error behavior in `AdminLogs.tsx`; `AdminLogs.tsx` is not a second owner for request-log schema or filtering.
- `Live (periodic refresh)` uses the currently applied filters, not draft edits. The SSE transport is backed by periodic database polling on the server; delivered rows prepend into the descending newest-first table, deduplicate by `id`, cap to the current page size, increment `count` once per accepted row, and reset the visible offset to `0`.

### Request details drawer
- Opening a row shows the drawer with page-body-unique values from that row, including values not present as table columns.
- The drawer has accessible role/name semantics (`dialog`, `Request details`) and a labeled close button.
- The drawer exposes a visible `Copy request ID` action with `data-testid="request-log-copy-request-id"` that writes the row's `request_id` to the clipboard; the action is disabled when `request_id` is missing.
- Closing the drawer does not change filters, pagination, selected tab, or loaded rows.
- If the selected row disappears after a reload, the drawer closes and focus moves to the table caption or results summary.

### Query performance
- Selecting `Query Performance` loads query statistics without changing request-log draft filters, applied filters, selected request-log page, or open/closed drawer state once the user returns to `Request Logs`.
- `Sort by` supports `Total Time`, `Calls`, and `Mean Time`.
- Changing `Sort by` reloads query statistics with the selected sort.
- Query rows show query text, call count, average execution time, total execution time, rows, and index-suggestion statements with confidence labels when present.
- Empty query-stat results show `No query statistics available`.

## Navigation

- Route: `/admin/` with admin view `analytics`.
- Entry: Select `Analytics` from the visible Admin sidebar entry.
- Back: Browser back follows the admin app history; opening and closing the request details drawer does not create a separate route.
- Tab change: Stays on `Analytics` and swaps the visible analytics data set.
- Request rows: stay on `Analytics` and open the request details drawer.
- Export: downloads a file and stays on `Analytics`.

## Acceptance criteria

- Given at least one request-log row is arranged with `seedRequestLogEntry`, when the user navigates through the visible Analytics sidebar entry, then the seeded row is asserted in the request-log table body before any drawer, filter, pager, or export action runs.
- Given a seeded request row includes page-body-unique values for request ID, IP address, request size, response size, and duration, when the user opens its details drawer, then the drawer shows every `RequestLogEntry` field with the correct formatting and `-` for any missing optional field.
- Given more matching request rows exist than one page limit, when the user goes to the next page through the visible pager, then the table shows a row unique to the second page and the previous/next disabled states match `count`, `limit`, and `offset`.
- Given draft filters are edited, when the user has not applied them, then the current table does not reload; when the user applies them, then the first page reloads with method, path, exact status, status class, from/to date, and latency filters.
- Given aggregate rows with known minute buckets and status-class counts, when `AnalyticsCharts` renders, then the volume-bar dimensions and stacked status marks encode those exact values, both charts have accessible names, and the status legend labels all four classes.
- Given 500 aggregate minute buckets, when `AnalyticsCharts` renders, then each chart draws 100 marks folded from 5 minutes each with summed counts, at most 12 visible labels, and a visible grouping note.
- Given aggregate buckets on two different UTC days, when `AnalyticsCharts` renders, then each visible label and mark title includes its UTC calendar date.
- Given an empty aggregate result followed by a failed aggregate refresh, when `AnalyticsCharts` renders, then the error message and `Retry request charts` are visible and the no-activity message is not.
- Given visible filters isolate one seeded request row, when the unmocked aggregate endpoint responds, then the volume and matching status-class marks show count `1`, and the status class of a nonmatching seeded row shows count `0`.
- Given an invalid minimum/maximum latency range, when the user applies filters, then a visible validation error appears and `listRequestLogs` is not called.
- Given applied filters match no request rows, when the user exports JSON or CSV, then no download starts and the empty-export message is visible.
- Given applied filters match rows beyond the visible page, when the user exports JSON or CSV, then the downloaded content includes all matching rows and field values, not only the current page.
- Given `Query Performance` is selected, when the user changes `Sort by` to `Mean Time`, then either the query table or `No query statistics available` is visible and existing query columns plus index suggestions remain unchanged.
- Given request-log browser coverage runs against the unmocked server, genuine 404, 501, or 503 endpoint unavailability may skip the scenario, but HTTP 500 is a failure.
- Given downstream browser tests interact with the rich request-log viewer, actions and assertions use accessible roles/labels or the stable `data-testid` contract in this spec; XPath and CSS selectors are prohibited for request-log actions/assertions.
- Given cleanup runs after seeded request-log browser scenarios, then individual seeded paths use `cleanupRequestLogsByPath`; the 501-row export fixture uses the lifecycle spec's prefix cleanup because enumerating every seeded path would be needlessly expensive.

## Edge cases

- Analytics service not configured: unmocked request-log tests may skip only for genuine 404, 501, or 503 endpoint unavailability.
- HTTP 500 from request-log endpoints: fail tests and show retryable error UI; do not treat it as an unavailable-feature skip.
- No request logs: show `No request logs found`.
- No aggregate rows: show `No request activity matches these filters.` without fake marks.
- No query statistics: show `No query statistics available`.
- Draft filter values: changing filter inputs does not reload until the user applies them.
- Invalid status code: show validation from the request-log API or client-side validation if available; preserve draft values for correction.
- Invalid latency range: show deterministic inline validation and do not issue the request.
- Missing optional identity/network fields: display `-` in both table summaries and drawer details.
- Stale page after deletion or filtering: move to the last valid page when possible, otherwise show the empty request-log state at offset `0`.
- Component ownership: `Analytics.tsx` owns applied-filter state and aggregate loading; `AnalyticsCharts.tsx` exclusively owns prop-driven chart rendering and chart-specific transforms. Request query construction remains shared by `listRequestLogs` and `listRequestLogAggregates`, response types remain in `types/analytics.ts`, and filter/count behavior remains in `admin_request_logs_handler.go`.
- Stage boundary: this spec does not prescribe schema changes, migrations, query-log live-tail, chart dependencies, sample-data fallbacks, deployment, unrelated observability docs, or changes to `adminQueryAnalyticsResponse`.

## Current implementation gaps

- Current: The request-log tab renders real minute-bucket volume and status-class aggregation data in accessible inline SVG charts, with loading, empty, retryable error, and retained-data reload states; chart DOM size is bounded by folding oversized responses into at most 120 marks, and labels keep UTC date context across day boundaries.
- Target: Chart rendering remains prop-driven and dependency-free, while `Analytics.tsx` applies the exact same request-log filter object to both table and aggregate requests.
- Evidence: `ui/src/components/AnalyticsCharts.tsx` owns SVG marks, `reduceAggregateBuckets` bucket folding, label formatting, and states; `ui/src/api_analytics.ts:listRequestLogAggregates` owns aggregate query serialization; `ui/src/components/__tests__/AnalyticsCharts.test.tsx` verifies exact mark geometry, legend, bounded folding of a 500-bucket response, multi-day label dates, error-over-empty precedence, and states; `ui/src/components/__tests__/AnalyticsAggregates.test.tsx` verifies applied-filter wiring; `ui/browser-tests-unmocked/smoke/analytics.spec.ts` verifies filtered aggregate values from the real server.

- Current: The shipped rich viewer keeps a concise request-log table and exposes every `RequestLogEntry` field through its accessible row actions and focus-trapped details drawer, with deterministic optional-field and byte formatting.
- Target: The table/drawer allocation remains the canonical presentation for every request-log field, including accessible open, close, focus-return, and copy-request-ID behavior.
- Evidence: `ui/src/components/AnalyticsRequestLogs.tsx` owns the Stage 2 table and drawer presentation; `ui/src/components/__tests__/Analytics.test.tsx` verifies every displayed field, drawer behavior, clipboard action, and focus return; `ui/browser-tests-unmocked/smoke/analytics.spec.ts` verifies seeded drawer values against the live server.

- Current: The shipped request-log toolbar includes draft method, wildcard path, exact status, status-class, date, and inclusive latency filters; applying or resetting returns to page one, and exact backend `count`, `limit`, and `offset` drive the summary, pager boundaries, and stale-page correction.
- Target: Draft/applied filter separation and exact-total paging remain owned by the existing Analytics state, request API, response type, and server filter/count contracts.
- Evidence: `ui/src/components/Analytics.tsx` owns the Stage 4 filter/page state; `ui/src/api_analytics.ts:listRequestLogs` owns query serialization; `ui/src/types/analytics.ts:RequestLogListResponse` owns the paging response; `internal/server/admin_request_logs_handler.go` owns the Stage 3 exact-count and filter contract; `ui/src/components/__tests__/Analytics.test.tsx` verifies draft/apply/reset, validation, ranges, boundaries, and stale-page behavior.

- Current: The shipped `Export JSON` and `Export CSV` actions fetch every row matching only the applied filters, serialize the canonical request-log field order, escape CSV values through the shared formatter, timestamp filenames, and report empty or failed exports through toasts without replacing the visible page.
- Target: All-match export remains in `Analytics.tsx` for UI state and `api_analytics.ts` for request pagination, while generic CSV formatting remains shared with SQL Editor and Table Browser.
- Evidence: `ui/src/components/Analytics.tsx` owns export state, serialization, and Blob cleanup; `ui/src/api_analytics.ts:listAllRequestLogs` owns all-pages fetching; `ui/src/components/shared/format.ts` owns CSV escaping; `ui/src/components/__tests__/Analytics.test.tsx`, `SqlEditor.test.tsx`, and `TableBrowser.test.tsx` verify exact content and preserved shared behavior.

- Current: Automated component and unmocked browser coverage asserts the seeded load-first row before interactions, drawer fields, exact status-class and inclusive latency results, total-driven second-page boundaries, parsed all-match JSON/CSV downloads, stable hooks and accessible controls, and deterministic failure for HTTP 500 while preserving the Query Performance workflow.
- Target: The same visible, content-focused evidence remains required for future changes; only genuine HTTP 404, 501, or 503 endpoint unavailability may skip the unmocked lifecycle.
- Evidence: `ui/src/components/__tests__/Analytics.test.tsx` covers the fast state and export contracts; `ui/browser-tests-unmocked/full/analytics-lifecycle.spec.ts` uses the same 501-row export set to prove ordered 25-row page transitions, parses both downloaded formats, and owns prefix cleanup for that bulk set; `ui/browser-tests-unmocked/smoke/analytics.spec.ts` covers the complete seeded drawer; `ui/browser-tests-unmocked/fixtures/admin.ts` owns deterministic single-row seeding and per-path cleanup.

- Current: The request-log table exposes a `Live (periodic refresh)` control with Off, Connecting, Live, and Error status text. It consumes authenticated filtered SSE through `fetchAdmin`, and the server stream is a polling-backed request-log tail.
- Target: Live stays lifecycle-safe and scoped to the existing request-log state owner: applied filters only, no URL token, no native `EventSource`, no chart refresh, no query-log tail, no row duplication, and no leaked stream after toggle-off, tab change, or unmount.
- Evidence: `ui/src/__tests__/api_analytics.test.ts` verifies stream filter serialization, authenticated `fetchAdmin` use, fragmented and multiple SSE frame decoding, error events, and aborts; `ui/src/components/__tests__/Analytics.test.tsx` verifies Live ready/delivery, dedupe, cap/count updates, applied-filter reconnects, error preservation, and abort lifecycle; `ui/browser-tests-unmocked/smoke/analytics.spec.ts` verifies a real logged 404 appears without reload and Live-off stops appends.
