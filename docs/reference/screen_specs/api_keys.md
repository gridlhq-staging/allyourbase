# API Keys

## Task

Create, inspect, page through, copy, and revoke admin API keys for service-to-service authentication.

## Layout

1. Screen header with `API Keys` title, service-to-service authentication subtitle, and `Create Key` action.
2. Main content area showing loading, error, empty, or populated API-key table state.
3. Pagination footer when API-key data is available.
4. Create API key dialog.
5. One-time created-key reveal dialog.
6. Revoke confirmation dialog for the selected active key.

## State contract

### Loading
- When `fetchKeys` is loading before any data has been rendered, the screen shows a centered spinner with `Loading API keys...`.
- The list request uses the current page and `PER_PAGE` value `20`.
- User emails and app metadata load independently; failures in those supporting lookups do not replace the API-key list.

### Error
- When `fetchKeys` fails before data is available, the screen shows a centered error state with the returned error message, or `Failed to load API keys` when the thrown value is not an `Error`.
- The error state includes a `Retry` action.
- Clicking `Retry` sets loading true and reruns `fetchKeys` with the current page.
- When `fetchKeys` fails, existing API-key data is cleared so stale rows are not shown with the error state.

### Empty state
- When the API-key list is empty, the screen shows `No API keys created yet`.
- The empty state explains that API keys are for service-to-service authentication between backend systems.
- `Create your first API key` opens the same create dialog as the header `Create Key` action.

### Populated API-key table
- The table columns are `Name`, `Key`, `Scope`, `User`, `App`, `Last Used`, `Created`, `Status`, and `Actions`.
- Each row shows the key name, key id, stored key prefix with ellipsis, scope badge, optional allowed-table list, user email or user id, app name or app id, app rate-limit summary, last-used date or `Never`, created date, and status badge.
- Full-access keys show `full access`; readonly keys use a readonly badge; readwrite keys use a readwrite badge.
- User-scoped keys show `User-scoped` in the app column.
- Active keys show `Active` and a `Revoke key` row action.
- Revoked keys show `Revoked` and do not show the revoke action.

### Pagination
- The footer shows the total key count with correct singular or plural text.
- The page indicator shows `<current page> / <total pages or 1>`.
- `Previous page` is disabled on page `1`; `Next page` is disabled on the last page.
- Pagination actions keep the page within bounds and reload the list.

### Create dialog
- The dialog is titled `Create API Key`.
- The form includes `Key name`, `User` or `User ID`, `Scope`, `App Scope`, and `Allowed tables`.
- `Scope` offers `Full access (*)`, `Read only`, and `Read & write`.
- `App Scope` defaults to `User-scoped (no app)` and lists available apps; app lookup errors are shown inline without blocking user-scoped key creation.
- `Allowed tables` accepts comma-separated table names and treats blank input as all tables.
- `Create` is disabled while creating, when the name is blank, or when no user id is selected or entered.
- `Cancel` closes the dialog and resets unsaved create form values.
- Successful creation shows a success toast, refreshes the list, resets the create form, and opens the one-time reveal dialog.
- Create failure leaves the dialog context recoverable and shows a toast error.

### One-time key reveal
- The created-key dialog is titled `API Key Created`.
- The dialog warns `Copy this key now. It will not be shown again.`
- The full plaintext key is shown exactly once in a monospace code block.
- `Copy to clipboard` copies the full plaintext key, changes the copy icon to a success icon for two seconds, and leaves the dialog open.
- Copy failure shows `Failed to copy to clipboard` as a toast error.
- The dialog shows the created key metadata: name, user, scope, optional app, optional rate, and optional allowed tables.
- `Done` dismisses the dialog; once dismissed, only the stored prefix is available in the table.

### Revoke confirmation
- Clicking an active row `Revoke key` action opens a dialog titled `Revoke API Key`.
- The dialog states that revocation is permanent and applications using the key will lose access.
- The dialog displays the exact target key name and stored key prefix in the form `<name> (<prefix>...)`.
- `Cancel` closes the dialog without revoking the key, and the row remains active.
- Confirming `Revoke` disables the button while revoking, shows `Revoking...`, calls revoke for the selected key id, closes the dialog on success, shows a success toast naming the key, refreshes the list, and leaves the row visible with `Revoked` status.
- Revoke failure leaves the selected-key context recoverable and shows a toast error.

## Navigation

- Route: `/admin/` with the `API Keys` sidebar item selected.
- Entry: Select `API Keys` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Revoke: stays on `API Keys` and refreshes the current page after successful revocation.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `API Keys`, then the `API Keys` heading is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/api-keys-list.spec.ts`.
- Given an API key has been seeded, when the user opens `API Keys`, then the seeded key row appears with `full access`, `Active`, and `Revoke key`. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/api-keys-list.spec.ts`.
- Given a seeded API key exists, when the user opens `API Keys`, then the seeded key name appears in the list view. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given the create dialog is open, when the user provides a key name, user, and app scope, then key creation submits and the created modal opens. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given the created modal is open, when the user inspects it, then the one-time reveal warning, full plaintext key, created key name, app name, and rate limit are visible. Evidence owner: existing and Stage 2-added assertions in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given the created modal is open, when the user clicks `Copy to clipboard`, then the full plaintext key is sent to the clipboard writer. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given the created modal is open, when the user clicks `Done`, then the modal is dismissed and the key appears in the table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given an active key row is visible, when the user opens revoke confirmation, then the dialog shows the exact target key name and stored prefix before any destructive action. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given revoke confirmation is open, when the user clicks `Cancel`, then the dialog closes and the same key row still shows `Active`. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
- Given revoke confirmation is open for an active key, when the user confirms revocation, then the matching row shows `Revoked`. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.

## Edge cases

- API-key service unavailable: unmocked browser tests may skip only for 503, 404, 501, or 500 service probes.
- Empty tenant: show `No API keys created yet` and both create entry points remain available.
- No user lookup results: render a `User ID` text field instead of the `User` select.
- App lookup failure: keep the create form usable for user-scoped keys and show the app error inline.
- Clipboard denied: leave the one-time reveal dialog open and show `Failed to copy to clipboard`.
- Create failure: keep unsaved context in the create dialog and show a toast error.
- Revoke cancel: leave the selected key active and visible.
- Revoke failure: keep the revoke dialog context recoverable and show a toast error.
- Revoked key: omit the revoke action so a revoked key cannot be revoked again from the table.
- Last page with zero `totalPages`: display `1` as the denominator.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove initial loading, retry/error, empty-list, create disabled state, create cancel reset, copy failure toast, create failure toast, revoke failure toast, multi-page navigation, or revoked rows omitting the revoke action because the current unmocked fixtures do not provide stable service-failure or high-volume setup for those states.
- Target: Acceptance evidence should cover these target states when existing unmocked fixtures can exercise them without mocked routes, a new harness, or ad hoc service-failure setup.
- Evidence: `ui/src/components/ApiKeys.tsx`; `ui/src/components/ApiKeysModals.tsx`; `ui/src/components/ApiKeysTable.tsx`; `ui/browser-tests-unmocked/smoke/api-keys-list.spec.ts`; `ui/browser-tests-unmocked/full/api-keys-lifecycle.spec.ts`.
