# Secrets

## Task

Manage admin secrets, reveal stored values when needed, update or delete individual secrets, and rotate the JWT signing secret deliberately.

## Layout

1. Header with `Secrets` title, `Create Secret`, and destructive `Rotate JWT Secret` action.
2. Inline `New Secret` or `Update <name>` form when creating or updating a secret.
3. Secrets table with `Name`, `Created`, `Updated`, and row actions.
4. Empty-state panel with guidance and `Create your first secret`.
5. `Delete Secret` confirmation dialog.
6. `Rotate JWT Secret` confirmation dialog.

## State contract

### Loading
- Before the list is available, keep the header visible and show `Loading...` below it.

### Error
- List failure shows the `Secrets` title, the returned error message, and `Retry`; retry calls the existing list refresh owner.
- Action failures keep the list, form, or confirmation context mounted while surfacing the returned error above the table.

### Empty state
- When no secrets exist, show `No secrets configured yet`, explanatory copy, and `Create your first secret`.
- The empty-state action opens the same create form as `Create Secret`.

### List and reveal
- Rows show secret name, created date, updated date, and `Reveal`, `Update`, and `Delete` actions.
- `Reveal <name>` fetches the secret value and replaces that row's reveal action with the value.

### Create and update
- `Create Secret` opens `New Secret` with required `Name` and `Value` fields.
- `Create` is disabled until both fields have values, creates the secret, closes the form, and refreshes the list.
- `Update <name>` opens the same form with the name prefilled and disabled.
- `Update` is disabled until a replacement value is present, updates the secret value, closes the form, and refreshes the list.
- `Cancel` closes the form and clears draft values.

### Delete and rotate confirmation
- `Delete <name>` opens `Delete Secret`, names the selected secret, and requires `Delete` confirmation.
- `Rotate JWT Secret` opens a destructive confirmation explaining that existing JWT tokens are invalidated.
- `Cancel` from `Delete Secret` or `Rotate JWT Secret` closes the shared confirmation dialog and sends no delete or rotation request.
- Confirming rotation posts the rotation request and closes the dialog after success.

## Navigation

- Route: `/admin/` with admin view `secrets`.
- Entry: Select `Secrets` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create, update, delete, reveal, and rotate actions: stay on `Secrets`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Secrets`, then the `Secrets` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/secrets.spec.ts`, `ui/browser-tests-unmocked/full/secrets-lifecycle.spec.ts`, `ui/browser-tests-unmocked/full/secrets-jwt-rotation.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a secret is seeded, when the user opens `Secrets`, then `Name`, `Created`, and `Updated` columns and the seeded row are visible. Evidence: `ui/browser-tests-unmocked/smoke/secrets.spec.ts`.
- Given a secret row is visible, when the user reveals it, then the fetched secret value appears in that row. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-unmocked/smoke/secrets.spec.ts`; `ui/browser-tests-unmocked/full/secrets-lifecycle.spec.ts`.
- Given the create form is opened with missing fields, then `Create` is disabled. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`.
- Given valid create values, when the user submits, then the create API receives the name and value and the new row can appear. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-unmocked/full/secrets-lifecycle.spec.ts`.
- Given an existing secret is updated, when the replacement value is submitted and revealed, then the updated value is shown. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-unmocked/full/secrets-lifecycle.spec.ts`.
- Given the user deletes a secret, when `Delete Secret` is confirmed, then the delete API is called and the row is removed. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-unmocked/full/secrets-lifecycle.spec.ts`.
- Given `Rotate JWT Secret` is open, when the user selects `Cancel`, then the dialog closes, no rotation request is sent, and the pre-rotation auth token remains valid. Evidence: `ui/browser-tests-unmocked/full/secrets-jwt-rotation.spec.ts`.
- Given the user rotates the JWT secret, when `Rotate JWT Secret` is confirmed, then the rotation endpoint succeeds and old auth tokens are rejected. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-unmocked/full/secrets-jwt-rotation.spec.ts`.
- Given no secrets exist, when loading completes, then the empty-state guidance and create entry point are visible. Evidence: `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-mocked/secrets-error-flows.spec.ts`.
- Given the list API fails, when the screen handles the failure, then the `Secrets` heading and returned error text are visible. Evidence: `ui/src/components/Secrets.tsx`; `ui/src/components/__tests__/Secrets.test.tsx`; `ui/browser-tests-mocked/secrets-error-flows.spec.ts`.

## Edge cases

- Secrets endpoint unavailable in live environments: browser tests skip only explicit 404, 501, or 503 probes.
- Secret values are hidden until a per-row reveal action succeeds.
- JWT rotation is destructive and must remain behind confirmation.
- Create and update actions must not submit empty names or values.

## Current implementation gaps
