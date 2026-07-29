# Custom Domains

## Task

Register custom hostnames, inspect DNS and certificate state, verify pending domains, and delete domain bindings.

## Layout

1. Header with `Custom Domains` title and `Add Domain` action.
2. Inline `New Domain` form when adding a domain.
3. Domains table with `Hostname`, `Status`, `Environment`, `DNS TXT Record`, `Cert Expiry`, and actions.
4. `Delete Domain` confirmation dialog.

## State contract

### Loading
- Before the list is available, keep the header visible and show `Loading...`.

### Error
- List failure shows the `Custom Domains` title, the returned error message, and `Retry`; retry calls the existing domain-list refresh owner.
- Action failures keep the table, form, or confirmation context mounted while surfacing the returned error.

### Domain table
- Rows show hostname, status badge, environment, TXT verification record or `-`, certificate expiry date or `-`, and actions.
- Pending verification rows show `Verify`.
- Every row shows `Delete`.
- Empty results show `No custom domains configured`.

### Add domain
- `Add Domain` opens `New Domain`.
- The form includes `Hostname`, `Environment`, and `Redirect Mode`.
- `Environment` supports production, staging, and development.
- `Redirect Mode` supports none and HTTPS.
- `Add` is disabled until hostname is present.
- `Cancel` closes the form and resets draft values.

### Verify and delete
- `Verify` calls verification for the selected domain.
- `Delete <hostname>` opens `Delete Domain`, names the selected hostname, and requires `Delete` confirmation.

## Navigation

- Route: `/admin/` with admin view `custom-domains`.
- Entry: Select `Custom Domains` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Add, verify, and delete actions: stay on `Custom Domains`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Custom Domains`, then the `Custom Domains` heading is visible. Evidence: `ui/src/components/Sidebar.tsx`; `ui/src/components/ContentRouter.tsx`; `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given domain rows exist, when the table renders, then hostname, status, environment, TXT record, certificate expiry, and actions are represented. Evidence: `ui/src/components/CustomDomains.tsx`; `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given a pending domain row is visible, when the user clicks `Verify`, then the verify API receives that domain id. Evidence: `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given `Add Domain` is opened without a hostname, then `Add` is disabled. Evidence: `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given a user deletes a domain, when `Delete Domain` is confirmed, then the delete API receives the selected domain id. Evidence: `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given list loading is in progress, then `Loading...` remains readable. Evidence: `ui/src/components/CustomDomains.tsx`; `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given list loading fails, then the returned error message is visible below the heading. Evidence: `ui/src/components/__tests__/CustomDomains.test.tsx`.
- Given the Custom Domains page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- No domains: show `No custom domains configured`.
- Missing TXT record or certificate expiry: show `-`.
- Only `pending_verification` rows expose `Verify`.
- Delete is destructive and must remain behind confirmation.

## Current implementation gaps

- Current: Browser proof covers navigation, isolated seeded data, empty state, exact backend error, retry recovery, create-domain submission, verify button actionability, and delete confirmation, but does not prove an actual pending verification status transition.
- Target: Browser proof should cover the post-verification status state when deterministic domain fixtures are available.
- Evidence: `ui/browser-tests-unmocked/full/custom-domains-lifecycle.spec.ts`; `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`; `ui/src/components/__tests__/CustomDomains.test.tsx`.
