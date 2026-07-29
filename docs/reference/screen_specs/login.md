# Login Auth Shell

## Task

Enter the admin password at `/admin/` so the existing dashboard shell can either hand off to the ready dashboard or redirect to a safe `return_to` path after login.

## Layout

1. Full-screen boot shell: `AdminDashboard` in `ui/src/App.tsx` first renders the centered `Loading...` state while `boot` checks `/api/admin/status`.
2. Connection error shell: when `boot` catches a non-401 error, `AdminDashboard` renders the centered `Connection Error` panel with the error message and `Retry` button.
3. Login form shell: when admin auth is required and no `ayb_admin_token` exists, or when a 401 clears stored auth, `AdminDashboard` renders `Login` from `ui/src/components/Login.tsx`.
4. Login form content: `Login` shows the `Allyourbase` heading, helper text `Enter the admin password to continue.`, `Password` field, and `Sign in` submit button.
5. Inline auth failure: `Login.handleSubmit` renders the API error text and `View guide` link to `https://allyourbase.io/guide/authentication` above the password field and keeps the same form visible.
6. Ready dashboard: successful login without a safe `return_to` returns control to `AdminDashboard.handleLogin`, which sets loading and reruns `boot`; successful schema load renders `Layout`.

## State contract

### Loading
- `AdminDashboard` shows only centered `Loading...` while `/api/admin/status` and, when allowed, `/api/schema` are in flight.
- Loading is the only transition state between login success and the ready dashboard when no safe `return_to` is present.

### Error
- `AdminDashboard` shows a centered `Connection Error` heading, the thrown error message, and a `Retry` button after a non-401 boot failure.
- A 401 is not shown as a connection error; `AdminDashboard.boot` clears stored auth tokens and returns to the login form.

### Retry recovery
- `Retry` clears the connection error, returns the shell to `Loading...`, and calls the same `boot` owner again.
- If the retry status check reports that auth is required and no admin token is stored, the shell recovers to the login form.

### Login
- `Login` owns password entry and submission.
- The `Password` field is required and the `Sign in` button submits the form through `Login.handleSubmit`.
- While the submit promise is in flight, the button is disabled and its label changes to `Signing in...`.

### Invalid password
- `Login.handleSubmit` catches `ApiError` from `/api/admin/auth` and renders the API message inline above the field.
- The password form remains visible so the user can correct the password and submit again.
- `Sign in` remains the recovery action, and the shared error notice links to `https://allyourbase.io/guide/authentication`.

### Post-login handoff
- `Login.handleSubmit` calls `onSuccess` only after `adminLogin` stores the returned admin token.
- `AdminDashboard.handleLogin` first checks `return_to`; a same-origin, slash-prefixed target is assigned through `window.location.assign`.
- Without a safe `return_to`, `handleLogin` sets `Loading...` and reruns `boot`; the post-login success target is the dashboard `Layout`.

## Navigation

- Route: `/admin/`.
- Entry: direct browser entry at `/admin/`, unauthenticated redirects from admin API 401 handling, and OAuth consent redirects that include a `return_to` query string.
- Back: browser back remains normal browser history; the auth shell does not add its own navigation stack.
- Sign in: submits the password in place; successful login either assigns a safe `return_to` path or returns through loading to the dashboard `Layout`.
- Retry: stays on `/admin/` and reruns the existing `AdminDashboard.boot` path.

## Acceptance criteria

- Given `/admin/` reports admin auth is required and no admin token is stored, when the shell renders, then the `Allyourbase` heading, `Password` field, and `Sign in` button are visible. Evidence: `ui/src/components/__tests__/Login.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
- Given a valid admin password is submitted through the visible form, when login succeeds, then `waitForDashboard(page)` observes the ready dashboard handoff. Evidence: `ui/src/components/__tests__/Login.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
- Given `/api/admin/auth` rejects the submitted password, when the user clicks `Sign in`, then the API error message, enabled `Sign in` recovery action, and `https://allyourbase.io/guide/authentication` guide link are visible with the password form. Evidence: `ui/src/components/__tests__/Login.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
- Given `/api/admin/status` fails during boot, when `/admin/` renders, then `Connection Error`, the failure message, and `Retry` are visible. Evidence: `ui/src/components/__tests__/App.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
- Given `/api/admin/status` fails once and then reports auth required, when the user clicks `Retry`, then the login form becomes visible through the same `AdminDashboard.boot` path. Evidence: `ui/src/components/__tests__/App.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
- Given login succeeds from `/admin/?return_to=/admin/`, when `handleLogin` processes the safe return target, then the page lands on `/admin/` and reaches the dashboard ready state. Evidence: `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.

## Edge cases

- No auth required: `/api/admin/status` can report `auth: false`; `AdminDashboard.boot` skips `Login` and loads `/api/schema` directly.
- Expired token: a 401 from `/api/schema` clears admin and user auth tokens, then shows the login form.
- Unsafe `return_to`: cross-origin or protocol-relative values are ignored by `normalizeReturnTo`; login falls back to loading and dashboard boot.
- Network failure on password submit: `Login.handleSubmit` shows `Failed to connect to server` for non-API errors and keeps the form visible.

## Current implementation gaps

None verified.
