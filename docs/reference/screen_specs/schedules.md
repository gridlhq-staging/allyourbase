# Schedules

## Task

Create, edit, enable, disable, and delete recurring job schedules.

## Layout

1. Header with `Job Schedules` title, cron/timezone subtitle, and `Create Schedule` action.
2. Main content area showing loading, error, empty, or populated schedule-list state.
3. Schedules table with schedule identity, cron, run timing, enabled state, and action columns.
4. `Create Schedule` modal.
5. `Edit Schedule` modal.
6. `Delete schedule?` confirmation modal.

## State contract

### Loading
- While `load` is waiting for the first `listSchedules` response, the screen shows a centered spinner and `Loading schedules...`.
- Save, delete, and enable/disable actions keep the current modal or row context visible while the active control is disabled.

### Error
- When `listSchedules` fails before schedule data is available, the screen shows the error message or `Failed to load schedules`.
- The error state includes `Retry`; clicking it sets loading true and reruns `load`.
- Save, delete, and enable/disable failures keep the user recoverable in the current context and report failure through a toast.

### Empty state
- When `data.items` is empty, the screen shows `No schedules configured yet`.
- `Create Schedule` remains available from the header.

### Populated table
- The table columns are `Name`, `Job Type`, `Cron`, `Last Run`, `Next Run`, `Enabled`, and `Actions`.
- Each row shows schedule name, job type, cron expression, timezone, formatted last-run timestamp, formatted next-run timestamp, enabled toggle, edit action, and delete action.
- Missing run timestamps use the shared date formatter fallback.

### Enable and disable
- Enabled schedules show an `On` toggle with `Disable schedule <id>` as the accessible action.
- Disabled schedules show an `Off` toggle with `Enable schedule <id>` as the accessible action.
- Toggling calls `handleToggle`, reloads the list on success, and reports failure through a toast.

### Create modal
- `Create Schedule` opens the create modal with name, job type, cron expression, timezone, payload JSON, and enabled controls.
- New schedules default to cron `0 * * * *`, timezone `UTC`, payload `{}`, and enabled.
- `Save` validates that cron has five fields and payload is a JSON object.
- Successful create closes the modal, reloads the list, and reports `Schedule created`.
- `Cancel` closes the modal without creating a schedule.

### Edit modal
- `Edit schedule` opens `Edit Schedule` populated from the selected schedule.
- Name and job type are read-only while editing.
- Cron expression, timezone, payload JSON, and enabled remain editable.
- `Save` validates that cron has five fields and payload is a JSON object.
- Successful edit closes the modal, reloads the list, and reports `Schedule updated`.
- `Cancel` closes the modal without saving changes.

### Delete confirmation
- `Delete schedule` opens `Delete schedule?`.
- The confirmation names the selected schedule.
- `Delete` calls `handleDelete`, closes the modal on success, reloads the list, and reports `Schedule deleted`.
- `Cancel` closes the confirmation without deleting the schedule.

## Navigation

- Route: `/admin/` with the `Schedules` admin sidebar item selected.
- Entry: Select `Schedules` from the `Admin` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Create, edit, toggle, and delete actions: stay on `Job Schedules`.

## Acceptance criteria

- Given the admin app is loaded and a schedule is seeded, when the user selects `Schedules`, then `Job Schedules`, the seeded row, cron expression, timezone, and management actions are visible. Evidence: `ui/browser-tests-unmocked/smoke/schedules-list.spec.ts`.
- Given the schedules table renders, when the user inspects the header row, then `Name`, `Job Type`, `Cron`, `Last Run`, `Next Run`, `Enabled`, and `Actions` are visible. Evidence: `ui/browser-tests-unmocked/smoke/schedules-list.spec.ts`.
- Given an enabled schedule row, when the user toggles it off and back on, then the row changes from `On` to `Off` and back to `On`. Evidence: `ui/browser-tests-unmocked/full/schedules-lifecycle.spec.ts`.
- Given valid create input, when the user saves `Create Schedule`, then `Schedule created` appears and the new row shows the created cron expression. Evidence: `ui/browser-tests-unmocked/full/schedules-lifecycle.spec.ts`.
- Given an existing schedule, when the user edits its cron expression and saves, then `Schedule updated` appears and the row shows the updated cron expression. Evidence: `ui/browser-tests-unmocked/full/schedules-lifecycle.spec.ts`.
- Given an existing schedule, when the user confirms `Delete schedule?`, then `Schedule deleted` appears and the row is removed. Evidence: `ui/browser-tests-unmocked/full/schedules-lifecycle.spec.ts`.
- Given schedule data is loading, when the screen renders, then `Loading schedules...` is visible. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given the list request fails before data is available, when the screen renders, then the error message and `Retry` action are visible. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given no schedules are configured, when loading completes, then `No schedules configured yet` is visible and `Create Schedule` remains available. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given a schedule form has a cron expression with other than five fields, when the user saves, then `Cron expression must have 5 fields.` is visible and no save request is sent. Evidence: `ui/src/components/__tests__/Schedules.test.tsx`.
- Given a schedule form has invalid payload JSON or a non-object JSON value, when the user saves, then `Payload must be a JSON object.` is visible and no save request is sent. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.

## Edge cases

- Schedules endpoint unavailable in a test environment: existing browser proof skips only for explicit `503`, `404`, or `501` service-unavailable probes.
- Empty timezone during save: the submitted timezone falls back to `UTC`.
- Edit mode preserves schedule identity by disabling name and job type fields.
- Payload must be a JSON object; arrays, scalars, and invalid JSON are rejected.

## Current implementation gaps

- Current: The current test corpus has no deterministic assertion for `Schedules` first-load loading, first-load error/retry, empty-list, or invalid-payload JSON states.
- Target: Add component or browser proof that asserts `Loading schedules...`, first-load error with `Retry`, `No schedules configured yet`, and `Payload must be a JSON object.` without changing this target spec.
- Evidence: stage 2 proof sweep over `ui/src/components/__tests__/Schedules.test.tsx`, `ui/browser-tests-unmocked/smoke/schedules-list.spec.ts`, and `ui/browser-tests-unmocked/full/schedules-lifecycle.spec.ts`.
