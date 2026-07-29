# Users

## Task

Find, inspect, page through, and delete registered user accounts from the admin dashboard.

## Layout

1. Screen header with `Users` title and `Manage registered user accounts` subtitle.
2. Search row with email search input, clear-search action, and `Search` button.
3. Main content area showing loading, error, empty, search-empty, or populated table state.
4. Pagination row with total user count, previous page action, current page indicator, total pages, and next page action.
5. Delete confirmation dialog for the selected user.

## State contract

### Loading
- When `fetchUsers` is loading before any data has been rendered, the screen shows a centered spinner with `Loading users...`.
- The initial request uses page `1`, `PER_PAGE` value `20`, and no search parameter until a search is applied.

### Error
- When `fetchUsers` fails before data is available, the screen shows a centered error state with the returned error message, or `Failed to load users` when the thrown value is not an `Error`.
- The error state includes a `Retry` action.
- Clicking `Retry` sets loading true and reruns `fetchUsers` with the current page and applied search.
- When `fetchUsers` fails, existing data is cleared so stale table rows are not shown with the error state.

### Search
- The search input is labeled `Search users` and has placeholder `Search by email...`.
- Typing text updates the draft search value without reloading the table.
- Pressing Enter or clicking `Search` applies the draft search, resets the page to `1`, and reloads users with the search term.
- When the draft search is non-empty, the clear-search action is visible.
- Clicking clear search clears both the draft and applied search, resets the page to `1`, and reloads the unfiltered user list.

### Empty states
- When the unfiltered result has no users, the screen shows `No users registered yet`.
- When an applied search returns no users, the screen shows `No users matching search`.

### Populated user table
- The table columns are `Email`, `Verified`, `Created`, and `Actions`.
- Each user row shows email, user id, email verification status, localized creation date, and a delete action.
- Verified users show a green check icon; unverified users show a muted x icon.
- Seeded users must render by exact email so acceptance tests can prove the table is backed by real data.

### Pagination
- The request page size is the `PER_PAGE` value `20`.
- The footer shows the total user count with correct singular or plural text.
- The page indicator shows `<current page> / <total pages or 1>`.
- `Previous page` is disabled on page `1`; `Next page` is disabled on the last page.
- Pagination actions update the page within bounds and reload the list with the current applied search.

### Delete confirmation
- Clicking a row delete action opens a dialog titled `Delete User`.
- The dialog states `This will permanently delete the user and all their sessions.` and displays the exact target email.
- `Cancel` closes the dialog without deleting the user.
- Confirming `Delete` calls `deleteUser(id)`, disables the button while deleting, shows `Deleting...`, closes the dialog on success, shows a success toast containing the target email, and reloads the current list.

## Navigation

- Route: `/admin/` with the `Users` sidebar item selected.
- Entry: Select `Users` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Delete: stays on `Users` and refreshes the current list after successful deletion.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Users`, then the `Users` heading is visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/users-list.spec.ts` and `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given a user has been seeded, when the user opens `Users`, then the seeded email appears in the user table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/users-list.spec.ts`.
- Given the users screen is loaded, when the user inspects the search controls, then the search input is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/users-list.spec.ts`.
- Given seeded users exist, when the user searches for a seeded email, then the matching row is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given the user has searched for one email, when the user clears and searches for a second seeded email, then the second matching row is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given a deletable user row is visible, when the user clicks the row delete action, then the delete confirmation opens before deletion. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given the delete confirmation is open for a user, when the user confirms deletion, then a success toast containing that target email is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given the users screen has seeded data, when the user inspects the table, then `Email`, `Verified`, `Created`, and `Actions` columns, the seeded user id, and the pagination footer are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/users-list.spec.ts`.
- Given a search term is present, when the user inspects search controls, then the clear-search action is visible and clearing it restores the unfiltered search input state. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given the delete confirmation is open, when the user inspects the dialog, then the exact target email is displayed before confirmation. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
- Given the users screen is loaded with fewer than or equal to `20` users, when the user inspects pagination, then `Previous page` and `Next page` are visible and disabled at the single-page boundaries. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/users-list.spec.ts`.

## Edge cases

- Initial service unavailable: unmocked browser tests may skip only for 503, 404, or 501 service probes.
- Empty tenant: show `No users registered yet`.
- Search with no matches: show `No users matching search`.
- Search apply: reset to page `1` before requesting results.
- Retry: preserve the current page and applied search while retrying the failed request.
- Delete failure: keep the user context recoverable and show an error toast.
- Last page with zero `totalPages`: display `1` as the denominator.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove loading, retry/error, empty-list, search-empty, deleting button state, failed-delete toast, or multi-page navigation because current owners do not provide stable unmocked service-failure or high-volume data setup for those states.
- Target: Acceptance evidence should cover these target states when existing unmocked fixtures can exercise them without mocked routes, a new harness, or ad hoc service-failure setup.
- Evidence: `ui/src/components/Users.tsx`; `ui/browser-tests-unmocked/smoke/users-list.spec.ts`; `ui/browser-tests-unmocked/full/users-lifecycle.spec.ts`.
