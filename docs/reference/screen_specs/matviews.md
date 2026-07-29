# Materialized Views

## Task

Register, inspect, refresh, edit, and unregister materialized views from the admin database tools.

## Layout

1. Header with `Materialized Views` title, registry subtitle, and `Register Matview` action.
2. Main content area showing loading, error, empty, or populated registry state.
3. Registered materialized-view table with metadata and row actions.
4. Register materialized view modal.
5. Edit refresh-mode modal.
6. Unregister confirmation modal.

## State contract

### Loading
- While `load` is waiting for the first `listMatviews` response, the screen shows a centered spinner and `Loading materialized views...`.
- Row refresh, register, update, and unregister actions keep the current screen context visible while their buttons are disabled or show per-action loading state.

### Error
- When `load` fails before registry data is available, the screen shows the error message or `Failed to load materialized views`.
- The error state includes `Retry`; clicking it sets loading true and reruns `load`.
- Refresh, register, update, and unregister failures stay on the current screen or modal and report the failure through a toast.

### Empty state
- When `data.items` is empty, the screen shows `No materialized views registered`.
- `Register Matview` remains available from the header.

### Populated table
- The table columns are `Schema`, `View Name`, `Mode`, `Status`, `Last Refresh`, `Duration`, `Error`, and `Actions`.
- Each row shows the registered schema, view name, refresh mode, last-refresh status, formatted last-refresh time, duration in milliseconds or `-`, and a truncated error preview or `-`.
- Row actions are `Refresh`, edit refresh mode, and delete/unregister.
- `Refresh` calls `handleRefresh`, disables only the active row refresh button, shows a spinning refresh icon for that row, reloads registry data on success, and reports success or failure through a toast.

### Register modal
- `Register Matview` opens `Register Materialized View`.
- `View` is populated from schema tables whose kind is `materialized_view`, sorted by `<schema>.<name>`.
- `Refresh Mode` offers `standard` and `concurrent`.
- `Register` calls `handleRegister`, closes the modal and reloads registry data on success, and leaves the user recoverable on failure.
- `Cancel` closes the modal without changing registry data.

### Edit modal
- Editing a row opens `Edit Refresh Mode` with the row's current refresh mode selected.
- `Save` calls `handleUpdate`, closes the modal and reloads registry data on success, and leaves the modal recoverable on failure.
- `Cancel` closes the modal without changing the row.

### Unregister confirmation
- Deleting a row opens `Unregister materialized view?`.
- The confirmation names the selected view and explains that the materialized view itself is not dropped.
- `Unregister` calls `handleDelete`, closes the modal and reloads registry data on success, and leaves the modal recoverable on failure.
- `Cancel` closes the confirmation without unregistering.

## Navigation

- Route: `/admin/` with the `Materialized Views` admin sidebar item selected.
- Entry: Select `Materialized Views` from the `Database` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Register, edit, refresh, and unregister: stay on `Materialized Views`.

## Acceptance criteria

- Given the admin app is loaded, when the user selects `Materialized Views`, then the `Materialized Views` heading and `Register Matview` action are visible. Evidence: `ui/browser-tests-unmocked/smoke/matviews-list.spec.ts`.
- Given registry data is still loading, when the screen renders, then `Loading materialized views...` is visible. Evidence: `ui/src/components/__tests__/MatviewsAdmin.test.tsx`.
- Given the registry request fails before data is available, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/MatviewsAdmin.test.tsx`.
- Given no materialized views are registered, when loading completes, then `No materialized views registered` is visible and `Register Matview` remains available. Evidence: `ui/src/components/__tests__/MatviewsAdmin.test.tsx`; `ui/browser-tests-unmocked/smoke/matviews-list.spec.ts`.
- Given registered materialized views exist, when loading completes, then the table shows the canonical columns and row actions. Evidence: `ui/browser-tests-unmocked/smoke/matviews-list.spec.ts`.
- Given discovered materialized views exist, when the user opens `Register Materialized View`, chooses a view and refresh mode, and clicks `Register`, then the modal closes and the refreshed registry includes the registered view.
- Given an existing registry row, when the user edits its refresh mode and saves, then the modal closes and the row reloads with the updated mode.
- Given an existing registry row, when the user confirms `Unregister`, then the registry reloads without that row while the underlying materialized view is not dropped.

## Edge cases

- No materialized views discovered in schema: the register modal has no usable view option, and registration cannot produce a valid schema/view pair.
- Last refresh metadata absent: status, date, duration, and error cells fall back to `-`.
- Long refresh errors: the table truncates previews past 80 characters.
- Concurrent refresh mode requires the backing database constraints needed by PostgreSQL; failures are reported as action toasts.

## Current implementation gaps

None verified.
