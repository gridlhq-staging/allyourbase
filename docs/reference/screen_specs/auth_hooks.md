# Auth Hooks

## Task

Inspect configured auth hook function references for each supported auth lifecycle hook slot.

## Layout

1. Padded page container with `Auth Hooks` heading.
2. Loading, error, or hook-card list content.
3. Hook-card list with six rows: `Before Sign Up`, `After Sign Up`, `Custom Access Token`, `Before Password Reset`, `Send Email`, and `Send SMS`.
4. Each hook card shows the human-readable hook label on the left and the configured function reference or `Not configured` on the right.

## State contract

### Loading
- Show the `Auth Hooks` heading and `Loading...` body text while hook configuration is being fetched.
- Do not show hook cards until the configuration object resolves.

### Error
- Show the `Auth Hooks` heading and the returned error message in red.
- The error notice includes a `Retry` action that reruns the existing hook-config load callback.

### Hook list
- Render one card for each supported hook key in the fixed order used by the screen.
- If a hook key has a configured value, show the value in monospace text.
- If a hook key is empty, show `Not configured` in muted italic text.
- Cards are read-only; there are no enable, disable, URL editing, save, or delete controls in the shipped screen.

## Navigation

- Route: `/admin/` with admin view `auth-hooks`.
- Entry: Select `Auth Hooks` from the `Auth` sidebar section.
- Back: Browser back follows the admin shell history.
- Hook card: no navigation; cards are read-only.

## Acceptance criteria

- Given the auth hooks API returns a configuration object, when the user opens `Auth Hooks`, then all six hook labels render. Evidence owner: `ui/browser-tests-unmocked/full/auth-hooks-lifecycle.spec.ts`.
- Given a hook key has a configured value, when the user opens `Auth Hooks`, then that exact value appears in the matching hook row. Evidence owner: `ui/browser-tests-unmocked/smoke/auth-hooks.spec.ts`.
- Given a hook key is empty, when the user opens `Auth Hooks`, then the matching hook row shows `Not configured`. Evidence owner: `ui/browser-tests-unmocked/smoke/auth-hooks.spec.ts`.
- Given the hook fetch fails, when the component renders, then the returned error message is visible. Evidence owner: `ui/src/components/__tests__/AuthHooks.test.tsx`.

## Edge cases

- Auth hooks endpoint unavailable: unmocked probes may skip only when the endpoint reports unavailable status.
- Empty hook config: every hook slot still renders, with `Not configured` values.
- Unknown hook keys from the API: ignore them in this screen until the fixed UI hook list is extended.

## Current implementation gaps

- Current: The shipped `AuthHooks.tsx` screen is read-only. It does not expose hook enable/disable controls, URL or function-reference editing controls, or save actions.
- Target: If hook configuration editing is required, add explicit create/edit/disable controls and update tests to prove mutation success and failure states.
- Evidence: `ui/src/components/AuthHooks.tsx`; `ui/src/components/__tests__/AuthHooks.test.tsx`; `ui/browser-tests-unmocked/smoke/auth-hooks.spec.ts`; `ui/browser-tests-unmocked/full/auth-hooks-lifecycle.spec.ts`.
