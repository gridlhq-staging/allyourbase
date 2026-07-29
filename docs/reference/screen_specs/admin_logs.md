# Admin Logs

## Task

Filter server admin logs, inspect request attributes, copy rows, export filtered results, and refresh the live log buffer.

## Layout

1. Header row with `Admin Logs` title, auto-refresh status, pause/resume auto-refresh action, `Export filtered JSON`, and `Refresh`.
2. Inline error text below the header when the admin-log request fails.
3. Loading text below the header while the first log request is waiting.
4. Optional amber buffering message when log buffering is disabled.
5. Filter grid with `Search logs`, `Level`, `From`, and `To` controls.
6. Empty-state text when loaded data has no rows after filtering.
7. Log table with `Time`, `Level`, `Message`, `Attributes`, and `Actions` columns.
8. Per-row attribute summary, optional `Inspect JSON` toggle, and `Copy` row action.

## State contract

### Loading
- The panel remains mounted with test id `admin-logs-panel`.
- Before the first payload is available, the screen shows `Loading admin logs...`.
- Header actions and filters remain visible during loading.

### Error
- Request failure shows red inline text from `toPanelError(error)`.
- The error notice includes a `Retry` action that reruns the polling request; the `Refresh` button remains visible and can also retry.
- Existing data is not forcibly cleared by the error state.

### Filters
- `Search logs` filters against normalized message and attribute search text.
- `Level` supports all levels, DEBUG, INFO, WARN, ERROR, and UNKNOWN.
- `From` and `To` are datetime-local controls; rows without parseable timestamps do not match active time filters.
- Filtering is client-side against the loaded buffer and sorts matching entries newest first.

### Log buffer state
- When `data.bufferingEnabled` is false, show amber text with `data.message` or fallback `Log buffering is not enabled` in `admin-logs-buffering-message`.
- When loaded filtering returns no rows, show `No log entries found`.
- When rows are present, show the log table with localized time, upper-case level label, message or `-`, attribute summary or `-`, and row actions.

### Row actions
- `Inspect JSON` appears only when the row has non-empty attributes.
- Expanding attributes sets `aria-expanded` and shows formatted JSON in `admin-log-attrs-json-<id>`.
- `Copy log <id>` writes the serialized row to the clipboard and emits a success or failure toast.
- `Export filtered JSON` downloads only the currently filtered rows and warns when no rows are available.

## Navigation

- Route: `/admin/` with admin view `admin-logs`.
- Entry: Select `Admin Logs` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Refresh: Stays on `Admin Logs` and reloads the current log snapshot.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Admin Logs`, then the `Admin Logs` heading and `admin-logs-panel` are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/admin-logs.spec.ts` and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given the admin-log endpoint reports buffering unavailable, when the screen opens, then the buffering-disabled message is visible instead of fabricated table evidence. Evidence owner: existing fallback assertion in `ui/browser-tests-unmocked/smoke/admin-logs.spec.ts`.
- Given buffering is available, when an in-run admin stats request is triggered and `Refresh` is clicked, then a real log row containing the request id, `request`, and `/api/admin/stats` is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/admin-logs.spec.ts`.
- Given the Admin Logs page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Buffering disabled: show the backend message or fallback `Log buffering is not enabled`.
- Empty filtered result: show `No log entries found` instead of an empty table.
- Empty attributes: show `-` and omit the JSON inspector action.
- Invalid or missing timestamp with active time filters: exclude the row from the filtered list.
- Clipboard unavailable or rejected: keep the row visible and show an error toast.
- Export with no rows: do not download a file; show a warning toast.
- Polling paused: show `Auto-refresh paused` and let `Resume auto-refresh` re-enable polling.

## Current implementation gaps

- Current: Unmocked browser coverage proves the known-row-or-buffering fallback, refresh/retry path, and deterministic empty filtered state, but does not prove level/date filters, pause/resume polling, export, copy, or attribute expansion.
- Target: Acceptance evidence should cover those visible states when deterministic unmocked log fixtures can exercise them without adding mocked-only coverage or a parallel test file.
- Evidence: `ui/src/components/AdminLogs.tsx`; `ui/browser-tests-unmocked/smoke/admin-logs.spec.ts`.
