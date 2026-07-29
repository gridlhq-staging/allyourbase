# OAuth Clients

## Task

Register OAuth clients, copy newly issued secrets, rotate client secrets, and revoke clients from the admin tools.

## Layout

1. Header with `OAuth Clients` title, access-management subtitle, and `Register Client` action.
2. Main content area showing loading, error, empty, or populated client-list state.
3. OAuth clients table with client identity, app, scope, redirect, token-stat, status, and action columns.
4. Pagination footer with total count and page controls.
5. `Register OAuth Client` modal.
6. `OAuth Client Registered` created-secret modal.
7. `Rotate Client Secret` confirmation modal.
8. `New Client Secret` result modal.
9. `Revoke OAuth Client` confirmation modal.

## State contract

### Loading
- While `fetchClients` is waiting for the first `listOAuthClients` response, the screen shows a centered spinner and `Loading OAuth clients...`.
- Register, rotate, and revoke actions keep the current modal visible while their active button is disabled and shows `Registering...`, `Rotating...`, or `Revoking...`.

### Error
- When `listOAuthClients` fails before client data is available, the screen shows the error message or `Failed to load OAuth clients`.
- The error state includes `Retry`; clicking it sets loading true and reruns `fetchClients`.
- Register, rotate, revoke, and copy failures stay recoverable in the current context and report failure through a toast where applicable.

### Empty state
- When `data.items` is empty, the screen shows `No OAuth clients registered yet`.
- `Register your first client` opens the same register modal as the header `Register Client` action.

### Populated table
- The table columns are `Name`, `Client ID`, `Type`, `App`, `Scopes`, `Redirect URIs`, `Created`, `Token Stats`, `Status`, and `Actions`.
- Each row shows client name, client ID, client type, app name when known or app ID fallback, comma-separated scopes, redirect URIs, creation date, active access-token count, active refresh-token count, total grants, last-issued timestamp or `never`, and status.
- Active clients show `Active`; revoked clients show `Revoked`.
- Active confidential clients show `Rotate secret`.
- Active clients show `Revoke client`.

### Pagination
- The footer shows the total client count and the current page as `<page> / <totalPages>`.
- Previous page is disabled on page 1.
- Next page is disabled on the last page.
- Changing pages reloads the table with the selected page.

### Register modal
- `Register Client` opens `Register OAuth Client`.
- The modal includes client name, app selector, client type, redirect URIs, and scopes.
- Client type offers `confidential` and `public`.
- Scopes offer `readonly`, `readwrite`, and `*`.
- `Register` is disabled until client name and app are present.
- Successful registration opens `OAuth Client Registered`, reloads the client list, and reports success through a toast.
- `Cancel` closes the modal and clears the draft fields.

### Created-secret modal
- `OAuth Client Registered` shows the new client ID, the one-time client secret when returned, name, type, scopes, and redirect URIs.
- The secret can be copied with `Copy to clipboard`.
- `Done` closes the modal and leaves the client list visible.

### Rotate-secret flow
- `Rotate secret` opens `Rotate Client Secret`.
- `Rotate` calls `handleRotate`, opens `New Client Secret` on success, reloads the client list, and reports success through a toast.
- `New Client Secret` shows the one-time replacement secret with a copy action.
- `Done` closes the result modal.
- `Cancel` closes the confirmation without rotating the secret.

### Revoke confirmation
- `Revoke client` opens `Revoke OAuth Client`.
- The confirmation explains token invalidation and names the client.
- `Revoke` calls `handleRevoke`, closes the modal on success, reloads the client list, and reports success through a toast.
- Revoked clients no longer show rotate or revoke actions.
- `Cancel` closes the confirmation without revoking the client.

## Navigation

- Route: `/admin/` with the `OAuth Clients` admin sidebar item selected.
- Entry: Select `OAuth Clients` from the `Admin` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Register, rotate, and revoke actions: stay on `OAuth Clients`.

## Acceptance criteria

- Given the admin app is loaded and an OAuth client is seeded, when the user selects `OAuth Clients`, then the `OAuth Clients` heading and seeded row are visible. Evidence: `ui/browser-tests-unmocked/smoke/oauth-clients-list.spec.ts`.
- Given a seeded confidential client is active, when the table renders, then its app, `confidential` type, `readonly` scope, redirect URI, zero token stats, `Active` status, `Rotate secret`, and `Revoke client` are visible. Evidence: `ui/browser-tests-unmocked/smoke/oauth-clients-list.spec.ts`.
- Given the table renders, when the user inspects the header row, then identity, app, scope, redirect, token-stat, status, and action columns are visible. Evidence: `ui/browser-tests-unmocked/smoke/oauth-clients-list.spec.ts`.
- Given an active confidential client, when the user rotates its secret, then `New Client Secret` appears with a new `ayb_cs_` secret different from the original seeded secret. Evidence: `ui/browser-tests-unmocked/full/oauth-clients-lifecycle.spec.ts`.
- Given a valid app exists, when the user registers a client through `Register OAuth Client`, then `OAuth Client Registered` appears and the new active client row is visible. Evidence: `ui/browser-tests-unmocked/full/oauth-clients-lifecycle.spec.ts`.
- Given an active client row, when the user confirms `Revoke OAuth Client`, then the row shows `Revoked` and revoke action is removed. Evidence: `ui/browser-tests-unmocked/full/oauth-clients-lifecycle.spec.ts`.
- Given client data is loading, when the screen renders, then `Loading OAuth clients...` is visible. Evidence: `ui/src/components/__tests__/OAuthClients.test.tsx`.
- Given the list request fails before data is available, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/OAuthClients.test.tsx`.
- Given no OAuth clients are registered, when loading completes, then `No OAuth clients registered yet` and `Register your first client` are visible. Evidence: `ui/src/components/__tests__/OAuthClients.test.tsx`.

## Edge cases

- Apps lookup unavailable: the app cell falls back to the app ID.
- Public clients do not expose `Rotate secret`.
- Revoked clients do not expose rotate or revoke actions.
- Client secret values are shown only in the created-secret or rotated-secret result modal.
- OAuth clients endpoint unavailable in a test environment: existing browser proof skips only for explicit `404` or `501` service-unavailable probes.

## Current implementation gaps

None verified.
