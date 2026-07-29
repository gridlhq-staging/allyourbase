# Sites

## Task

Create, page through, inspect, update, and delete hosted sites from the admin services tools.

## Layout

1. List view header with `Sites` title and `Add Site` action.
2. Optional inline `New Site` form.
3. Sites table with pagination, empty, loading, and action-failure states.
4. Delete site confirmation dialog.
5. Detail view for a selected site, including deploys and site settings.

## State contract

### Loading
- The list view uses `useAdminResource(() => listSites({ page }))` to load the current page.
- While the list is loading, the table area shows `Loading...`.
- Detail view loading is owned by the selected `SiteDetailView`.

### Error
- When the list load fails before any site-list data is available, the list view shows `Sites` and the error message.
- The list error includes `Retry`, which calls the existing site-list refresh owner for the current page.
- When an action fails while list data remains available, the error message appears above the table and the current list context remains visible.
- Detail view errors are owned by `SiteDetailView`.

### List view
- `viewState.kind === "list"` renders the site list.
- The table columns are `Name`, `Slug`, `Status`, `Created`, and actions.
- Empty list data shows `No sites configured`.
- Pagination uses the page, per-page, and total-count values returned by `listSites`.
- Page changes refresh the list after the initial mount and clamp the current page to the available total pages.

### Create form
- `Add Site` opens the inline `New Site` form.
- The form includes `Name`, `Slug`, and `SPA mode`.
- `Create` is disabled until trimmed name and slug are both present, and while an action is loading.
- `Create` calls `handleCreate`, closes and resets the form on success, and refreshes the list through the action owner.
- `Cancel` closes and resets the form without creating a site.

### Site detail
- Clicking a row `View <site>` action sets `viewState.kind === "detail"` with that site id.
- Detail view loads the selected site and deploys, then shows site metadata, settings, and deploy actions.
- Saving site settings updates the matching row in the existing list result.
- `Back` calls the list refresh function and returns to list view.

### Delete confirmation
- Clicking a row `Delete <site>` action opens `Delete Site`.
- The confirmation names the selected site and warns that deletion cannot be undone.
- Confirming `Delete` calls `handleDelete`, clears the delete target on success, and refreshes the list through the action owner.
- Canceling clears the delete target without deleting the site.

## Navigation

- Route: `/admin/` with the `Sites` admin sidebar item selected.
- Entry: Select `Sites` from the `Services` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Row `View`: stays under `Sites` and switches from list to detail view.
- Detail `Back`: refreshes the list and returns to `Sites` list view.

## Acceptance criteria

- Given Sites API is available and a site is seeded, when the user selects `Sites`, then the seeded site name, slug, `View <site>`, and `Delete <site>` actions are visible. Evidence: `ui/browser-tests-unmocked/smoke/sites-hosting.spec.ts`.
- Given the list is loading, when the screen renders, then `Loading...` is visible in the list area. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given list loading fails with no prior data, when the screen renders, then `Sites` and the error message are visible. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given no sites exist, when loading completes, then `No sites configured` is visible. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given the user opens `New Site`, enters a valid name and slug, chooses SPA mode, and clicks `Create`, then the form closes and the list refreshes with the new site.
- Given a site row, when the user clicks `View <site>`, then detail view opens for that site.
- Given the user returns from detail view, when `Back` is clicked, then the list refreshes before showing list view.
- Given a site row, when the user clicks `Delete <site>` and confirms, then the site is deleted and the list refreshes.

## Edge cases

- Page exceeds available pages after deletion: page is clamped to the last available page.
- Action failure with existing list data: the list remains visible with the error message above it.
- Blank create inputs: `Create` stays disabled after trimming whitespace.
- Detail view updates: saved site metadata updates the existing list cache when returning.

## Current implementation gaps

- Current: The smoke proof covers seeded list rendering and row action visibility, but does not exercise inline create, detail back-to-list refresh, pagination, or delete confirmation.
- Target: Existing Sites proof should cover at least create or delete confirmation through visible UI, with list/detail behavior asserted where seeded data makes it deterministic.
- Evidence: `ui/src/components/Sites.tsx:338-540` and `ui/browser-tests-unmocked/smoke/sites-hosting.spec.ts:31-57`.
