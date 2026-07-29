# Branches

## Task

Create, inspect, and delete database branches from the admin console.

## Layout

1. Centered `Branches` screen with header, `Branches` title, and `Add Branch`.
2. Error banner with retry when branch loading fails.
3. Empty-state panel when no branches exist.
4. Branch table with `Name`, `Status`, `Source`, `Created`, and row delete action.
5. `Create Branch` modal with branch name and optional source database URL.
6. `Delete Branch` modal naming the selected branch.

## State contract

### Loading
- Before branch data is available, show a centered spinner.

### Error
- List failure keeps the screen visible and shows the returned error with `Retry`.
- Create and delete failures remain on `Branches` and surface toast errors.

### Empty state
- When no branches exist, show `No branches yet` and explanatory copy.
- `Add Branch` remains available from the header.

### Branch list
- Rows show branch name, lifecycle status, source database, created date, and a row delete action.
- Status badges render known branch states as `Ready`, `Creating`, `Failed`, or `Deleting`.

### Create branch
- `Add Branch` opens `Create Branch`.
- Branch name is required; source database URL is optional and defaults server-side when empty.
- `Create` is disabled until a non-empty branch name is present.
- `Cancel` closes the modal without submitting and preserves the existing branch list.
- Successful create closes the modal, shows success feedback, and refreshes the list.

### Delete branch
- Row delete opens `Delete Branch`.
- `Delete Branch` names the selected branch and explains that the action cannot be undone.
- `Cancel` closes `Delete Branch`, leaves the row visible, and sends no `DELETE /api/admin/branches/{name}` request.
- Confirming delete sends `DELETE /api/admin/branches/{name}`, closes after success, shows success feedback, and refreshes the list.

## Navigation

- Route: `/admin/` with admin view `branches`.
- Entry: Select `Branches` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create and delete actions: stay on `Branches`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Branches`, then the `Branches` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/branches.spec.ts`; `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.
- Given a branch exists, when the user opens `Branches`, then the branch row is visible and does not report `Failed`. Evidence: `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.
- Given the create dialog is open with an empty name, then `Create` is disabled. Evidence: `ui/src/components/__tests__/Branches.test.tsx`.
- Given a valid branch name, when the user creates the branch, then the created row appears and success feedback is visible. Evidence: `ui/src/components/__tests__/Branches.test.tsx`; `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.
- Given a branch row is visible, when the user opens `Delete Branch`, then the dialog names that branch. Evidence: `ui/src/components/__tests__/Branches.test.tsx`; `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.
- Given `Delete Branch` is open, when the user selects `Cancel`, then the dialog closes, the row remains, and no delete request is sent. Evidence: `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.
- Given `Delete Branch` is confirmed, then the delete endpoint succeeds and success feedback is visible. Evidence: `ui/src/components/__tests__/Branches.test.tsx`; `ui/browser-tests-unmocked/full/branches-lifecycle.spec.ts`.

## Edge cases

- Branch service may be unavailable in local live runs; full lifecycle skips only an explicit 503 list probe or a seed failure that identifies a missing `pg_dump` dependency before UI-created mutation work begins.
- A branch reporting `Failed` after seed or create is a product defect in the full lifecycle guard.

## Current implementation gaps

None verified.
