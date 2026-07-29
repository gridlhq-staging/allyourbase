# Edge Functions

## Task

Deploy, browse, edit, invoke, and delete server-side edge functions, and inspect each function's execution logs.

## Layout

1. List view with the `Edge Functions` title and a `New Function` action.
2. Function table showing each function's name, access, timeout, created date, and last-invoked time.
3. Create view (`Deploy New Function`) with name, source editor, entry point, timeout, public toggle, and `Deploy` action.
4. Detail view with a back button, function name heading, access badge, and `Editor` / `Logs` / `Invoke` / `Triggers` tabs.

## State contract

### Loading
- The list view shows a spinner with `Loading edge functions...` while `listEdgeFunctions` resolves.
- The detail view shows a spinner with `Loading function...` while the function and its logs load.

### Error
- A list-load failure shows an inline red alert containing the returned error message, or `Failed to load edge functions` when the thrown value is not an `Error`.
- The list-load alert includes a `Retry` action that reruns the existing function-list fetch while preserving the `Edge Functions` header and create action.
- A detail-load failure shows an error toast `Failed to load function details` and returns the user to the list view.
- A failed save surfaces as an error toast; compile/transpile/syntax failures additionally show an inline `Deploy error` banner in the editor.
- A failed invoke surfaces as an error toast and leaves the response panel empty.

### Empty list
- When no functions exist, the list view shows `No edge functions deployed yet`, guidance text, and a `Deploy Function` action.

### Populated list
- The table columns are `Name`, `Access`, `Timeout`, `Created`, and `Last Invoked`.
- Each row shows the function name, a `Public` or `Private` access badge, formatted timeout, created date, and last-invoked time.
- Clicking a row opens that function's detail view.

### Create
- `New Function` (and the empty-state `Deploy Function`) opens the `Deploy New Function` view.
- The view has a `Name` field, a code editor seeded with default source, an entry point (default `handler`), a timeout (default `5000` ms), and a `Public` toggle (default on).
- `Deploy` is disabled until the name is non-blank; a successful deploy shows a success toast naming the function and returns to the list.

### Detail — Editor tab
- The editor tab shows the source in a CodeMirror editor plus entry point, timeout, public toggle, and environment variables.
- Unsaved edits show an `Unsaved changes` indicator with a `Revert` action.
- `Save` persists the changes and shows a `Function saved` toast.

### Detail — Logs tab
- The logs tab shows status and trigger-type filters and a paged execution-log table with `Status`, `Duration`, `Method`, `Path`, `Trigger`, and `Time` columns.
- Each successful invocation appears as a row with a success indicator, duration, request method, request path, and trigger-type badge.
- With no logs, the tab shows `No execution logs yet.`; with filters that match nothing, it shows `No matching logs for the selected filters.`.
- Rows with captured output are expandable to show `stdout` and `error` detail.

### Detail — Invoke tab
- The invoke tab has an HTTP method selector, request path (default `/<name>`), optional headers, an optional request body for `POST`/`PUT`/`PATCH`, and a `Send` action.
- A completed invoke shows a `Response` panel with the status code badge, duration, response headers, and response body, and refreshes the logs.

### Delete
- The editor tab `Delete` action reveals an inline `Are you sure?` confirmation with `Confirm` and `Cancel` controls.
- `Confirm` deletes the function, shows a `Function deleted` toast, and returns to the list where the function no longer appears.
- `Cancel` dismisses the confirmation and leaves the function and its editor state intact.

## Navigation

- Route: `/admin/` with the `Edge Functions` sidebar item selected.
- Entry: Select `Edge Functions` from the admin sidebar.
- Back: The detail and create views' back button returns to the list and reloads it.
- New Function: opens the `Deploy New Function` create view.
- Row: opens the selected function's detail view.

## Acceptance criteria

- Given a public function is seeded via the admin API, when the user opens `Edge Functions`, then the seeded function name appears in the table and its access badge reads `Public`. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.
- Given the create view is open, when the user enters a name and clicks `Deploy`, then the new function appears as a row in the list. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.
- Given a function detail is open on the `Invoke` tab, when the user clicks `Send`, then a response with status code `200` is shown. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.
- Given a function has been invoked, when the user opens the `Logs` tab, then a log row shows the `GET` method, the invoked path containing the function name, a duration, and an `http` trigger badge. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.
- Given the delete confirmation is showing on the editor tab, when the user clicks `Cancel`, then the function is not deleted and the editor remains open. Evidence owner: assertion added in this stage in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.
- Given the delete confirmation is showing, when the user clicks `Confirm`, then the user returns to the list and the function row is gone. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/edge-functions-crud.spec.ts`.

## Edge cases

- Empty list: show `No edge functions deployed yet` with a `Deploy Function` action.
- Blank name: keep `Deploy` disabled until a name is entered.
- Compile failure: show the inline `Deploy error` banner in addition to the error toast.
- No logs: show `No execution logs yet.`; filtered-empty shows `No matching logs for the selected filters.`.
- Cancelled delete: leave the function and its unsaved editor state intact.
- Detail-load failure: toast the error and return to the list rather than showing a broken detail view.

## Current implementation gaps

- Current: The editor's delete confirmation shows only the text `Are you sure?` and does not name the target function, so the confirmation cannot prove which function is about to be deleted.
- Target: The destructive delete confirmation should name the exact function being deleted, matching the exact-target confirmation pattern used by the Storage delete dialog.
- Evidence: `ui/src/components/edge-functions/FunctionEditor.tsx` (the `confirmDelete` block renders a bare `Are you sure?` with no function name).
