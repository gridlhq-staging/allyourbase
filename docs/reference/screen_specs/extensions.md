# Extensions

## Task

Inspect Postgres extension availability and enable or disable extensions from the admin dashboard.

## Layout

1. Header with `Extensions` title.
2. Extensions table with `Name`, `Status`, `Version`, `Description`, and actions.
3. `Disable Extension` confirmation dialog.

## State contract

### Loading
- Before extension data is available, keep the heading visible and show `Loading...`.

### Error
- List failure shows the `Extensions` title, the returned error message, and `Retry`; retry calls the existing extension-list refresh owner.
- Enable and disable failures keep the table and any open confirmation dialog mounted while surfacing the returned error.

### Extension table
- Installed extensions show status `installed`, version, description, and `Disable`.
- Available but uninstalled extensions show status `available`, default version, description, and `Enable`.
- Missing version or description renders `-`.
- Empty results show `No extensions available`.

### Enable and disable
- `Enable` calls the enable endpoint for the selected extension and disables extension action buttons while pending.
- `Disable` opens `Disable Extension`, explains dependent object risk, and requires `Disable` confirmation.
- Confirming disable calls the disable endpoint and closes the dialog after success.

## Navigation

- Route: `/admin/` with admin view `extensions`.
- Entry: Select `Extensions` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Enable and disable actions: stay on `Extensions`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Extensions`, then the `Extensions` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/extensions.spec.ts`, `ui/browser-tests-unmocked/full/extensions-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given extension data is loaded, when the table renders, then `Name`, `Status`, version, description, and action content are visible. Evidence: `ui/src/components/__tests__/Extensions.test.tsx`; `ui/browser-tests-unmocked/full/extensions-lifecycle.spec.ts`.
- Given an installed extension is seeded, when the user opens `Extensions`, then its row shows `installed` and `Disable`. Evidence: `ui/browser-tests-unmocked/smoke/extensions.spec.ts`.
- Given an available extension is visible, when the user enables it, then the enable endpoint receives that extension name and action buttons are disabled while pending. Evidence: `ui/src/components/__tests__/Extensions.test.tsx`; `ui/browser-tests-unmocked/full/extensions-lifecycle.spec.ts`.
- Given an installed extension is disabled, when `Disable Extension` is confirmed, then the disable endpoint receives that extension name and the row returns to `available`. Evidence: `ui/src/components/__tests__/Extensions.test.tsx`; `ui/browser-tests-unmocked/full/extensions-lifecycle.spec.ts`.
- Given list loading is in progress, then `Loading...` remains readable. Evidence: `ui/src/components/__tests__/Extensions.test.tsx`.
- Given list loading fails, then the returned error message is visible below the heading. Evidence: `ui/src/components/__tests__/Extensions.test.tsx`.
- Given the Extensions page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Extensions endpoint unavailable in live environments: browser tests skip only explicit 404, 501, or 503 probes.
- Extension not present in a specific Postgres environment: lifecycle proof skips that extension only after checking availability.
- Disable is destructive enough to require confirmation because dependent database objects may break.

## Current implementation gaps
