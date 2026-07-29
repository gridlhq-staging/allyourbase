# Backups & PITR

## Task

Inspect database backups, filter backup history, trigger a backup, validate point-in-time recovery, start dry-run or real restores, and abandon running restore jobs.

## Layout

1. Header with `Backups & PITR` title, operational subtitle, and `Trigger Backup` action.
2. Backup filter bar with `Status`, `Type`, `Apply Filters`, and `Reset` controls.
3. Backup table with `Status`, `Type`, `Database`, `Size`, `Started`, and `Triggered By` columns.
4. Restore context selector when more than one project/database context is available, or an explanatory restore-context message when no backup exposes context metadata.
5. `Point-In-Time Recovery` panel with target-time input, validation action, dry-run checkbox, restore action, and validation result summary.
6. `Restore Jobs` section when restore jobs exist for the selected context.
7. `Abandon Restore Job` confirmation dialog for running restore jobs.

## State contract

### Loading
- Before backup data is available, the screen shows a centered spinner with `Loading backups...`.
- Loading triggered by filter changes keeps the current page shell stable once data has already rendered.

### Error
- Initial backup-list failure shows a centered error state containing the returned error message, or `Failed to load backups` when the thrown value is not an `Error`.
- The initial error state includes a `Retry` action that reloads backups with the currently applied filters.
- Trigger, PITR validation, restore, and abandon failures surface as toast errors without replacing the backup table.

### Backup list and filters
- The header title is `Backups & PITR` and the subtitle is `Manage database backups and point-in-time recovery`.
- `Trigger Backup` calls the backup trigger endpoint, shows `Backup triggered` on success, and reloads the current filtered backup list.
- `Status` supports all statuses, completed, running, failed, and pending.
- `Type` filters the returned rows client-side by exact lowercased backup type after server status filtering.
- Applying filters reloads the table; resetting filters clears both status and type.
- The empty backup table message is `No backups found`.

### Point-in-time recovery
- PITR controls are disabled until a restore context is available.
- `Target Time` accepts a local date-time value.
- `Validate PITR` normalizes the local target time, requires a selected context and target time, and shows earliest recoverable time, latest recoverable time, estimated WAL bytes, and WAL segment count after validation.
- Invalid local target time shows `Target time must be a valid local date and time`.
- `Dry run` toggles whether `Start Restore` requests a dry run.
- `Start Restore` is disabled until a PITR validation result exists; a dry run keeps the validation result visible and a real restore refreshes restore jobs.

### Restore jobs
- Restore jobs are loaded for the selected project/database context and refreshed after restore or abandon actions.
- The restore jobs table shows `Status`, `Phase`, `Started`, and `Actions`.
- Running restore jobs show an `Abandon Job` action.
- Abandoning opens `Abandon Restore Job`, states that the operation cannot be undone, and requires `Abandon` confirmation before calling the abandon endpoint.

## Navigation

- Route: `/admin/` with admin view `backups`.
- Entry: Select `Backups & PITR` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Restore actions: Stay on `Backups & PITR` and refresh PITR/restore-job state.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Backups & PITR`, then the `Backups & PITR` heading is visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/backups.spec.ts`, `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a backup row has been seeded, when the user opens `Backups & PITR`, then the seeded database name, status, and backup type appear in the table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/backups.spec.ts`.
- Given the backups screen is loaded, when the user inspects the table, then the `Status`, `Type`, `Database`, and `Size` columns are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/backups.spec.ts` and `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`.
- Given the backup trigger endpoint is available, when the user clicks `Trigger Backup`, then a backup-triggered toast or pending/running backup result is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`.
- Given the backup table is visible, when the user filters by `completed` status and resets filters, then the backup table remains visible after both operations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`.
- Given the PITR panel is visible, when the user inspects restore controls, then `Target Time`, `Validate PITR`, `Start Restore`, and `Dry run` are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`.
- Given the Backups page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Backup service not configured: unmocked tests may skip for 501, 404, or 503 service probes.
- No backups: show `No backups found` and keep PITR disabled with restore-context guidance.
- Multiple restore contexts: require the user to choose the project/database context before PITR or restore-job actions are enabled.
- Invalid PITR target time: show a toast error and do not call validate or restore.
- Trigger or restore API failure: keep current rows and surface the failure as a toast.
- Abandon failure: keep the current confirmation context recoverable and surface the failure as a toast.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove initial load-error retry, no-backups empty state, multiple restore-context selection, PITR validation result rendering, real restore-job refresh, or abandon confirmation.
- Target: Acceptance evidence should cover those visible states when a stable unmocked fixture can exercise them without mocked routes or a parallel harness.
- Evidence: `ui/src/components/Backups.tsx`; `ui/src/components/backups/PITRPanel.tsx`; `ui/src/components/backups/RestoreContextSelector.tsx`; `ui/src/components/backups/RestoreJobsSection.tsx`; `ui/browser-tests-unmocked/smoke/backups.spec.ts`; `ui/browser-tests-unmocked/full/backups-lifecycle.spec.ts`.
