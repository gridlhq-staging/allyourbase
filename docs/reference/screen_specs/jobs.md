# Jobs

## Task

Monitor queue health, filter jobs, retry failed jobs, cancel queued jobs, and inspect run history from one admin screen.

## Layout

1. Header with `Job Queue` title and queue-health subtitle.
2. Optional queue-stat cards.
3. Filter form with state selector, type input, and `Apply Filters`.
4. Main content area showing loading, error, empty, filtered-empty, or populated job-list state.
5. Jobs table with state, type, creation, attempts, last-error, and action columns.
6. Inline run-history panel for the selected job.

## State contract

### Loading
- While `load` is waiting for the first `listJobs` response, the screen shows a centered spinner and `Loading jobs...`.
- Applying filters, clearing filters, and refreshing an empty queue set loading true and keep the screen recoverable while data reloads.
- Retrying or canceling a row disables only that row's active action until the request resolves.
- Opening run history shows `Loading run history...` inside the run-history panel.

### Error
- When `listJobs` fails before job data is available, the screen shows the error message or `Failed to load jobs`.
- The error state includes `Retry`; clicking it sets loading true and reruns `load` with the current applied filters.
- Queue-stat failure alone does not fail the screen; the job list remains visible and stats are omitted.
- Retry and cancel failures keep the job row visible and report the failure through a toast.
- Run-history failures show the error message inside the run-history panel with `Retry run history`.

### Queue stats
- When queue stats load, the screen shows `Queued`, `Running`, `Completed`, `Failed`, `Canceled`, and `Oldest queued age` cards.
- Missing oldest queued age shows `-`.

### Filters
- State offers `All states`, `queued`, `running`, `completed`, `failed`, and `canceled`.
- Type accepts a job-type filter string.
- `Apply Filters` applies non-empty state and type values to the next job-list request.
- When filters are applied and no rows match, the screen shows `No jobs match these filters`, explanatory text, and `Clear filters`.
- `Clear filters` resets state and type filters and reloads all jobs.

### Empty state
- When no jobs exist and no filters are applied, the screen shows `No jobs in queue yet`.
- The empty state includes `Refresh jobs`; clicking it reloads jobs with the current filters.

### Jobs table
- The table columns are `State`, `Type`, `Created`, `Attempts`, `Last Error`, and `Actions`.
- Each row shows state badge, job type, job ID, created date, attempts as `<attempts> / <maxAttempts>`, last-error preview or `-`, and row actions.
- Last-error previews longer than 90 characters are truncated.
- Every row has `View Runs` or `Hide Runs`.
- Failed rows show `Retry`.
- Queued rows show `Cancel`.
- Other states do not show retry or cancel.

### Retry and cancel
- `Retry` calls `handleRetry`, reports success through a toast, reloads jobs and stats, and updates the row state from the server response.
- `Cancel` calls `handleCancel`, reports success through a toast, reloads jobs and stats, and updates the row state from the server response.

### Run history
- `View Runs` opens the inline `Run history for job <id>` panel under the jobs table and changes the row action to `Hide Runs`.
- The panel shows the job type and `Close`.
- The run-history table columns are `Attempt`, `Status`, `Started`, `Finished`, `Duration`, and `Error`.
- Each run row shows attempt, status, formatted timestamps, duration in milliseconds, and error or `-`.
- When no runs exist, the panel shows `No run history found for this job.`
- `Close` or `Hide Runs` closes the panel.
- If the selected job disappears after reload, the run-history panel closes.

## Navigation

- Route: `/admin/` with the `Jobs` admin sidebar item selected.
- Entry: Select `Jobs` from the `Admin` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Filtering, retrying, canceling, refreshing, and viewing run history: stay on `Job Queue`.

## Acceptance criteria

- Given the admin app is loaded and a failed job is seeded, when the user selects `Jobs`, then `Job Queue`, the seeded row, failed state, last error, and `Retry job` action are visible. Evidence: `ui/browser-tests-unmocked/smoke/jobs-list.spec.ts`.
- Given the jobs table renders, when the user inspects the header row, then `State`, `Type`, `Created`, `Attempts`, `Last Error`, and `Actions` are visible. Evidence: `ui/browser-tests-unmocked/smoke/jobs-list.spec.ts`.
- Given a failed job has one attempt out of three, when the row renders, then `1 / 3`, `View runs for job`, and `Retry job` are visible. Evidence: `ui/browser-tests-unmocked/smoke/jobs-list.spec.ts`.
- Given queue stats load, when the full jobs screen renders with seeded queued and failed jobs, then queued, failed, and oldest queued age cards are visible. Evidence: `ui/browser-tests-unmocked/full/jobs-management.spec.ts`.
- Given a failed job has seeded run history, when the user opens `View Runs`, then `Run history for job <id>` shows the seeded attempts, statuses, durations, and errors. Evidence: `ui/browser-tests-unmocked/full/jobs-management.spec.ts`.
- Given a failed job row, when the user clicks `Retry`, then a success toast appears and the row updates to `queued`. Evidence: `ui/browser-tests-unmocked/full/jobs-management.spec.ts`.
- Given a queued job row, when the user clicks `Cancel`, then a success toast appears and the row updates to `canceled`. Evidence: `ui/browser-tests-unmocked/full/jobs-management.spec.ts`.
- Given job data is loading, when the screen renders, then `Loading jobs...` is visible. Evidence: `ui/src/components/__tests__/Jobs.test.tsx`.
- Given the list request fails before data is available, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/Jobs.test.tsx`.
- Given a run-history request fails, when the panel renders, then the error message and `Retry run history` action are visible. Evidence: `ui/src/components/__tests__/Jobs.test.tsx`.

## Edge cases

- Job queue endpoint unavailable in a test environment: existing browser proof skips only for explicit `503`, `404`, or `501` service-unavailable probes.
- Queue stats unavailable while jobs load: omit stat cards without failing the list screen.
- No jobs with filters applied: show the filtered-empty state and `Clear filters`.
- No jobs without filters: show the queue-empty state and `Refresh jobs`.
- Selected job removed by reload: close the run-history panel.

## Current implementation gaps

None verified.
