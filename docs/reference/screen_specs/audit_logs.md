# Audit Logs

## Task

Filter audit-log entries, page through results, and expand an entry to inspect old and new change payloads.

## Layout

1. Header with `Audit Logs` title.
2. Audit filter bar with table, operation, from-date, to-date, apply, and reset controls.
3. Audit-log table with `Operation`, `Table`, `Timestamp`, `User`, and `IP` columns.
4. Pagination controls from the shared table component.
5. Per-entry change toggle buttons below the table.
6. `Audit change details` region showing old and new JSON payloads for the expanded entry.

## State contract

### Loading
- Before audit entries load, the screen keeps the title and filters visible and shows `Loading...` below the filter bar.
- Filter changes and pagination set loading while the title and filter controls remain visible.

### Error
- Audit-list failure shows the `Audit Logs` title and the returned error message, or `Failed to load` when the thrown value is not an `Error`.
- The table error includes `Retry`, which calls the existing `fetchEntries(page * 100)` owner with the current applied filters.
- The title and filter controls remain mounted while the table shows loading, error, or empty state.

### Filters
- `Table` accepts a table-name string.
- `Operation` supports all operations, INSERT, UPDATE, and DELETE.
- `From` and `To` accept date values.
- `Apply Filters` resets to page 1 and reloads with the applied filter values.
- `Reset` clears all filters, resets to page 1, and reloads unfiltered audit logs.

### Audit table and pagination
- Requests use limit `100` and offset `page * 100`.
- Empty results show `No audit log entries found`.
- Rows show operation, table name, localized timestamp, user id or `-`, and IP address or `-`.
- Pagination uses the shared table controls and clamps total pages to at least 1.
- `Next page` and `Previous page` change the page and reload with the current applied filters.

### Change details
- When entries are visible, each entry has a button labeled `Show changes for <short id>`.
- Clicking a show button toggles that row and updates `aria-expanded`.
- The expanded region has role `region`, label `Audit change details`, and includes `Old Values:` and `New Values:` JSON blocks.
- Clicking the expanded entry's button again hides the region.

## Navigation

- Route: `/admin/` with admin view `audit-logs`.
- Entry: Select `Audit Logs` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Pagination: Stays on `Audit Logs` and reloads entries for the selected page.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Audit Logs`, then the `Audit Logs` heading is visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/audit-logs.spec.ts`, `ui/browser-tests-unmocked/full/audit-logs-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given an audit-log entry has been seeded, when the user opens `Audit Logs`, then the audit table shows `Operation`, `Table`, and `Timestamp` columns. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/audit-logs.spec.ts`.
- Given an audit-log entry has been seeded, when the user filters by its table name, then the matching operation and table cells are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/audit-logs.spec.ts`.
- Given a seeded update entry has old and new values, when the user clicks `Show changes for <short id>`, then the `Audit change details` region contains both the old and new status values. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/audit-logs.spec.ts` and `ui/browser-tests-unmocked/full/audit-logs-lifecycle.spec.ts`.
- Given seeded INSERT, UPDATE, and DELETE audit rows exist, when the user filters by their table name, then exactly one row for each operation is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/audit-logs-lifecycle.spec.ts`.
- Given more than 100 audit rows exist for a table, when the user filters by that table, then `Next page` is visible and either moves to the second page or is disabled when there is only one effective page. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/audit-logs-lifecycle.spec.ts`.
- Given the Audit Logs page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- No entries: show `No audit log entries found`.
- Null user or IP values: render `-`.
- Null old or new payload: render JSON `null` in the details region.
- Filter apply/reset: always return to page 1 before loading.
- Page count of zero: display at least one page through the shared table total-page clamp.
- Error state: show the title and error message, but no retry button is currently shipped.

## Current implementation gaps

- Current: Unmocked browser coverage proves isolated seeded content, an empty filtered result, exact backend error, retry recovery, and change-detail expansion, but does not prove date filters, reset behavior, null user/IP rendering, or details collapse.
- Target: Acceptance evidence should cover those remaining visible states when deterministic audit fixtures can exercise them without adding mocked-only coverage.
- Evidence: `ui/browser-tests-unmocked/smoke/audit-logs.spec.ts`; `ui/browser-tests-unmocked/full/audit-logs-lifecycle.spec.ts`.
