# OAuth Consent

## Task

Review and approve or deny a third-party OAuth client request for account access.

## Layout

1. Standalone centered authorization panel on `/oauth/authorize`, outside the admin dashboard chrome.
2. Loading or redirecting indicator while the page checks authorization or hands off to another URL.
3. Error panel with alert icon, `Authorization Error` heading, and the backend or validation message.
4. Consent prompt with shield icon, `Authorization Request` heading, client name, and account-access copy.
5. Permission summary section labelled `Permissions requested`, with human-readable scope text and optional allowed-table list.
6. Action row with secondary `Deny` and primary `Approve` controls.

## State contract

### Loading
- `OAuthConsent` parses the current URL parameters and calls `checkOAuthAuthorize`.
- The page shows a centered spinner with `Checking authorization...`; no consent controls are visible.

### Error
- `OAuthConsent` shows a centered `Authorization Error` panel with the error message from URL validation or the authorize API.
- No retry, approve, deny, or dashboard navigation controls are visible.

### Missing required parameters
- If any required query parameter is absent (`response_type`, `client_id`, `redirect_uri`, `scope`, `state`, `code_challenge`, or `code_challenge_method`), `OAuthConsent` stays on `/oauth/authorize` and shows `Authorization Error` with `Missing required parameters`.
- The authorize API is not called from this state.

### Unauthenticated redirect
- If `checkOAuthAuthorize` returns 401, `OAuthConsent` redirects to `/?return_to=<encoded current /oauth/authorize URL>`.
- The login success return path is a boundary owned by the admin login shell in `App.tsx`, not by this consent screen.

### Permission summary
- The consent panel shows `Authorization Request`, the OAuth client name, `Permissions requested`, the human-readable scope (`Read your data`, `Read and modify your data`, `Full access to your account`, or the raw scope), and optional `Tables: ...` text when table restrictions are present.
- `Deny` and `Approve` are visible and enabled unless a decision submission is in progress.

### Deny
- Clicking `Deny` calls `submitOAuthConsent` with `decision: "deny"` and the checked prompt values.
- On success, the page redirects to the callback URL returned by the API, which includes `error=access_denied` and the original OAuth `state`.

### Approve
- Clicking `Approve` calls `submitOAuthConsent` with `decision: "approve"` and the checked prompt values.
- While the request is in flight, the approve control changes to `Approving...` and both decision controls are disabled.
- On success, the page redirects to the callback URL returned by the API, which includes a non-empty authorization `code` and the original OAuth `state`.

### Redirecting
- If authorization is already granted or a decision succeeds, `OAuthConsent` enters `redirecting` and calls `window.location.assign`.
- The page shows a centered spinner with `Redirecting...`; no decision controls remain visible.

## Navigation

- Route: `/oauth/authorize`
- Entry: direct OAuth authorization URL with required query parameters.
- Back: browser-managed; the standalone page does not add an in-app back control.
- Missing parameters or authorize API error: remain on `/oauth/authorize` and show `Authorization Error`.
- Unauthenticated: redirects to `/?return_to=<encoded current /oauth/authorize URL>` for the admin login shell to own login and return continuation.
- Deny: redirects to the OAuth callback URL returned by the consent API with `error=access_denied`.
- Approve: redirects to the OAuth callback URL returned by the consent API with an authorization code.

## Acceptance criteria

- Given `/oauth/authorize` is opened without required parameters, when the page renders, then it shows `Authorization Error` and `Missing required parameters`. Evidence: `ui/browser-tests-unmocked/full/oauth-consent.spec.ts`.
- Given a valid OAuth authorize URL but no authenticated auth session, when the page checks authorization, then it redirects to `/?return_to=<encoded /oauth/authorize?...>`. Evidence: `ui/browser-tests-unmocked/full/oauth-consent.spec.ts`.
- Given a valid authenticated OAuth authorize URL, when consent is required, then the page shows `Authorization Request`, the client name, `Permissions requested`, human-readable scope text, optional allowed-table summary, and `Deny`/`Approve` controls. Evidence: `ui/browser-tests-unmocked/full/oauth-consent.spec.ts`.
- Given the user denies consent, when the callback redirect is reached, then the callback URL contains `error=access_denied` and the original `state`. Evidence: `ui/browser-tests-unmocked/full/oauth-consent.spec.ts`.
- Given the user approves consent, when the callback redirect is reached, then the callback URL contains a non-empty `code` and the original `state`. Evidence: `ui/browser-tests-unmocked/full/oauth-consent.spec.ts`.

## Edge cases

- Already-consented authorization requests may skip the prompt and redirect immediately through the authorize API result.
- Network or backend failures during authorize or consent submission use the same `Authorization Error` panel with the returned message.
- Unknown scopes render as raw scope text so the user still sees what the client requested.
- Allowed tables are shown only when the authorize prompt includes one or more table names.

## Current implementation gaps

None verified.
