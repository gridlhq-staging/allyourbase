# Passkeys

## Task

Manage dashboard WebAuthn passkeys used for MFA step-up.

## Layout

1. `MFA Management` screen header with the `Multi-Factor Authentication` title.
2. `Passkeys` section explaining that passkeys register device-backed WebAuthn credentials for MFA step-up challenges.
3. `Passkey name` input for the display name of the credential being enrolled.
4. `Register Passkey` primary action.
5. Success and error message area above the passkey form.
6. `Registered passkeys` area showing loading, empty, or row states.
7. One row per registered passkey with display name, created date, optional last-used date, rename input, rename/save control, and delete control.
8. Delete confirmation dialog titled `Delete passkey`.

## State contract

### Loading
- Before passkey metadata loads, the registered-passkeys area shows `Loading passkeys...`.
- The enrollment controls remain visible while metadata loads so users can see the available task.

### Error
- If passkey metadata fails to load, the screen shows the returned error message or `Failed to load passkeys`.
- No dedicated retry control is shipped today; users can retry by re-entering or refreshing the screen.
- If a future retry control is added, it must rerun the `ui/src/api_passkeys.ts` list dependency without clearing any visible success or error context until the new result arrives.

### Empty
- When no registered credentials are returned, the registered-passkeys area shows `No passkeys registered`.
- The `Passkey name` input and `Register Passkey` action remain available.

### Populated passkey rows
- Each credential row shows the display name, `Created <date>`, and `Last used <date>` only when last-used metadata exists.
- Each row has its own rename input and save control scoped to that credential.
- Each row has its own delete control scoped to that credential.
- Multi-credential lists must keep every visible row independently actionable so renaming or deleting one passkey does not change the controls for another row.

### Registering
- Clicking `Register Passkey` with an empty trimmed name shows `Passkey name is required`.
- While registration is in progress, the register action is disabled and displays `Registering...`.
- On success, the input clears, `Passkey "<name>" registered` is visible, and the new credential appears in the registered-passkeys rows.
- Registration depends on `ui/src/api_passkeys.ts` and browser WebAuthn attestation helpers; this spec does not redefine those API contracts.

### Renaming
- A row rename input starts with that credential's current display name.
- Clicking the row save control with an empty trimmed name shows `Passkey name is required`.
- While a rename is in progress for one row, that row's rename input/save control are disabled and the save control displays `Saving...`.
- On success, `Passkey "<new name>" renamed` is visible, the renamed value appears in that row, and the previous value is no longer shown for that row.

### Deleting
- Clicking a row delete control opens `Delete passkey` confirmation naming that passkey.
- Clicking `Cancel` closes the confirmation and leaves the row unchanged.
- Confirming deletion disables confirmation controls while the delete request is in flight.
- On success, `Passkey deleted` is visible and the deleted credential row is removed.

### Final credential rejection
- When the backend rejects deletion of the final remaining WebAuthn credential, the exact backend-owned message `cannot delete final WebAuthn credential` remains visible.
- The final remaining passkey row remains visible and actionable after the rejection.
- Backend-owned error strings are dependencies of this screen contract, not API contracts redefined here.

## Navigation

- Route: `/admin/` with `MFA Management` selected.
- Entry: Enable WebAuthn in `Auth Settings`, then select `MFA Management` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Register, rename, delete, and final-delete rejection: stay on `MFA Management`.

## Acceptance criteria

- Given WebAuthn is enabled, when the user opens the admin dashboard, then `MFA Management` is available from the sidebar. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given the user opens `MFA Management`, when they enroll the first named passkey through `Passkey name` and `Register Passkey`, then the first passkey name is visible in the registered rows. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given the first passkey is visible, when the user enrolls a second distinct passkey through the same controls, then both passkey names are visible in registered rows. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given two passkey rows are visible, when the user renames one row, then the renamed value is visible and the original value is no longer visible for that row. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given two passkey rows are visible, when the user deletes the other row and confirms `Delete passkey`, then the deleted name is gone and the renamed passkey remains visible. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given one final passkey row remains, when the user attempts to delete it and confirms `Delete passkey`, then `cannot delete final WebAuthn credential` is visible and the remaining row/name stays visible. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.

## Edge cases

- WebAuthn disabled: `MFA Management` is not an available passkey-management entry point.
- Metadata load failure: show the load error instead of stale rows when no current rows are available.
- Empty credential list: show `No passkeys registered`.
- Duplicate or backend-rejected names: show the backend-owned error message returned by the passkey API dependency.
- Browser WebAuthn unavailable or canceled: show the registration error returned by the WebAuthn/API dependency.
- Final credential deletion: preserve the final row and show `cannot delete final WebAuthn credential`.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove loading, metadata load-error, empty-list, registration cancellation, duplicate/backend-rejected names, canceling delete confirmation, in-flight button labels, or last-used-date rendering because the current real-server lifecycle does not create those conditions without mocked routes or browser-level WebAuthn cancellation.
- Target: Those states should receive acceptance evidence when existing unmocked fixtures can exercise them without adding mocked browser coverage or a parallel WebAuthn harness.
- Evidence: `ui/src/components/Passkeys.tsx`; `ui/src/api_passkeys.ts`; `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
