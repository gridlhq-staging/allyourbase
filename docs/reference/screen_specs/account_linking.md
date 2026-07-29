# Link Your Account

## Task

Convert an anonymous auth session into an email/password account.

## Layout

1. Centered narrow panel with `Link Your Account` heading.
2. Short explanation that an email and password secure the anonymous account.
3. Inline error and success banners.
4. `Start Anonymous Session` action when no auth token is present.
5. Email input.
6. Password input.
7. Full-width `Link Account` submit action.

## State contract

### Loading
- There is no initial data-loading state because the screen derives readiness from the local auth token.
- While starting an anonymous session, the session button is disabled and shows a spinner with `Starting Session...`.
- While linking, the submit button is disabled and shows a spinner with `Linking...`.

### Error
- Anonymous session start failure shows the returned error message, or `Failed to start anonymous session`.
- Submitting without a ready session shows `Start an anonymous session before linking your account.`
- Link failure shows the returned error message, or `Failed to link account`.
- Errors stay inline above the form and keep entered email/password recoverable.

### No anonymous session
- `Start Anonymous Session` is visible and enabled unless a session request is in flight.
- Email and password inputs remain visible.
- `Link Account` stays disabled until a session is ready and both fields contain non-whitespace text.
- Starting a session successfully hides the start button and shows `Anonymous session started. You can now link your account.`

### Ready to link
- When an auth token already exists or the anonymous session start succeeds, the start-session button is hidden.
- Email and password inputs accept user text.
- `Link Account` is enabled only when both fields contain non-whitespace text and no link request is in flight.
- Successful link shows `Account linked successfully` and calls the parent `onLinked` callback with returned tokens.

## Navigation

- Route: `/admin/` with admin view `account-linking`.
- Entry: Select `Link Your Account` from the `Auth` sidebar section.
- Back: Browser back follows the admin shell history; successful linking stays on `Link Your Account`.
- Start Anonymous Session: stays on `Link Your Account` and changes the local form state.
- Link Account: stays on `Link Your Account` and reports success inline.

## Acceptance criteria

- Given anonymous auth is enabled in the environment, when the user opens `Link Your Account`, starts an anonymous session, enters email and password, and submits, then the screen shows `Account linked successfully`. Evidence owner: `ui/browser-tests-unmocked/smoke/account-linking-view.spec.ts`.
- Given no auth token exists, when the screen renders, then `Start Anonymous Session` is visible and `Link Account` is disabled until a session and non-empty credentials exist. Evidence owner: `ui/src/components/__tests__/AccountLinking.test.tsx`.
- Given session creation fails, when the user starts an anonymous session, then the returned error is shown, the start-session action remains available, credentials inputs remain visible, and no link request is sent. Evidence owner: `ui/src/components/__tests__/AccountLinking.test.tsx`.
- Given linking fails, when the user submits credentials, then the returned error is shown. Evidence owner: `ui/src/components/__tests__/AccountLinking.test.tsx`.

## Edge cases

- Existing auth token: hide `Start Anonymous Session` and let the user provide credentials immediately.
- Blank email or password: keep `Link Account` disabled.
- Anonymous auth disabled in a live environment: unmocked smoke coverage skips through its service probe rather than asserting a flow the server cannot accept.
- OAuth provider linking: not part of the shipped screen; no provider selector is rendered here.

## Current implementation gaps

- Current: `AccountLinking.tsx` only supports anonymous session plus email/password linking. Although `linkOAuth()` exists in API code, this screen does not import it or render provider prompts/selectors.
- Target: If OAuth account-linking becomes a product target, add a visible provider-selection flow and update this spec with tested provider prompts.
- Evidence: `ui/src/components/AccountLinking.tsx`; `ui/src/components/__tests__/AccountLinking.test.tsx`; `scripts/COVERAGE_MATRIX.md` row notes for `account-linking`; `ui/browser-tests-unmocked/smoke/account-linking-view.spec.ts`.
