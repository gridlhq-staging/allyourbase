# Applications

## Task

Create, page through, inspect, and delete registered admin applications.

## Layout

1. Header with `Applications` title, rate-limit subtitle, and `Create App` action.
2. Main content area showing loading, error, empty, or populated application-list state.
3. Applications table with owner, rate-limit, creation, and action columns.
4. Pagination footer with total count, previous page, current page, total pages, and next page.
5. `Create Application` modal.
6. `Delete Application` confirmation modal.

## State contract

### Loading
- While `fetchApps` is waiting for the first `listApps` response, the screen shows a centered spinner and `Loading apps...`.
- Create and delete actions keep the current screen or modal visible while the active button is disabled and shows `Creating...` or `Deleting...`.

### Error
- When `listApps` fails before application data is available, the screen shows the error message or `Failed to load apps`.
- The error state includes `Retry`; clicking it sets loading true and reruns `fetchApps`.
- Create and delete failures keep the user on the current modal or screen and report the failure through a toast.

### Empty state
- When `data.items` is empty, the screen shows `No apps registered yet`.
- `Create your first app` opens the same create modal as the header `Create App` action.

### Populated table
- The table columns are `Name`, `Description`, `Owner`, `Rate Limit`, `Created`, and `Actions`.
- Each row shows the application name, ID, description or `-`, owner email when loaded or owner ID fallback, rate-limit summary or `none`, creation date, and a `Delete app` action.
- Rate limits render as `<rateLimitRps> req/<rateLimitWindowSeconds>s` when enabled.

### Pagination
- The footer shows the total application count and the current page as `<page> / <totalPages>`.
- Previous page is disabled on page 1.
- Next page is disabled on the last page.
- Changing pages reloads the table with the selected page.

### Create modal
- `Create App` opens `Create Application`.
- The modal includes `App name`, `Description`, and `Owner`.
- `Owner` is a select populated from loaded users when available; otherwise it falls back to an owner UUID text input.
- `Create` is disabled until app name and owner are present.
- `Create` calls `handleCreate`, closes and resets the modal on success, reloads the application list, and reports success through a toast.
- `Cancel` closes the modal and clears the draft fields without creating an application.

### Delete confirmation
- `Delete app` opens `Delete Application`.
- The confirmation names the selected application and explains that scoped API keys are revoked.
- `Delete` calls `handleDelete`, closes the modal on success, reloads the application list, and reports success through a toast.
- `Cancel` closes the confirmation without deleting the application.

## Navigation

- Route: `/admin/` with the `Applications` admin sidebar item selected.
- Entry: Select `Applications` from the `Admin` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Create and delete actions: stay on `Applications`.

## Acceptance criteria

- Given the admin app is loaded and an app is seeded, when the user selects `Applications`, then the `Applications` heading and seeded row are visible. Evidence: `ui/browser-tests-unmocked/smoke/apps-list.spec.ts`.
- Given a seeded app has a description and rate limit, when the applications table renders, then the row shows the description, `120 req/60s`, and `Delete app`. Evidence: `ui/browser-tests-unmocked/smoke/apps-list.spec.ts`.
- Given the applications table renders, when the user inspects the header row, then `Name`, `Description`, `Owner`, `Rate Limit`, `Created`, and `Actions` are visible. Evidence: `ui/browser-tests-unmocked/smoke/apps-list.spec.ts`.
- Given the apps service is available, when the user creates an app through `Create Application`, then the modal closes and the new row appears with its description. Evidence: `ui/browser-tests-unmocked/full/apps-lifecycle.spec.ts`.
- Given an existing app row, when the user confirms `Delete Application`, then the row is removed from the table. Evidence: `ui/browser-tests-unmocked/full/apps-lifecycle.spec.ts`.
- Given application data is loading, when the screen renders, then `Loading apps...` is visible. Evidence: `ui/src/components/__tests__/Apps.test.tsx`.
- Given the list request fails before data is available, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/Apps.test.tsx`.
- Given no apps are registered, when loading completes, then `No apps registered yet` and `Create your first app` are visible. Evidence: `ui/src/components/__tests__/Apps.test.tsx`.

## Edge cases

- User-list loading fails: owner cells fall back to owner IDs, and create owner entry falls back to a UUID text input.
- Rate limit disabled: the rate-limit cell shows `none`.
- Optional description absent: the description cell shows `-`.
- Apps endpoint unavailable in a test environment: existing browser proof skips only for explicit `404` or `501` service-unavailable probes.

## Current implementation gaps

None verified.
