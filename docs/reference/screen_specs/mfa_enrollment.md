# Multi-Factor Authentication

## Task

Enroll and inspect multi-factor authentication methods for the current user session.

## Layout

1. Centered page container with `Multi-Factor Authentication` heading.
2. Session AAL badge showing the current token assurance level.
3. Inline error and success banners.
4. Loading body, or enrolled-methods and enrollment controls once data resolves.
5. `Enrolled Methods` region listing existing MFA factors or `No MFA methods enrolled`.
6. Backup-code count when at least one MFA method is enrolled.
7. Idle enrollment actions for authenticator app and email MFA, with backup-code actions when MFA exists.
8. Inline `Passkeys` region while idle, with passkey name input, register action, and registered passkey list.
9. One active enrollment subform at a time for TOTP, email MFA confirmation, or backup-code display.

## State contract

### Loading
- Keep the page heading and AAL badge visible.
- Show centered spinner and `Loading...` while the screen bootstraps a linkable auth session, fetches MFA factors, and fetches backup-code count.

### Error
- Show the returned error message in an inline red banner.
- Error banners do not remove the heading, AAL badge, or currently visible recoverable controls after initial load.
- Initial factor-load failures use the shared recoverable error notice with a `Retry` action that reruns the existing factor/profile load callback.
- TOTP start/confirm, email start/confirm, backup-code generation/regeneration, passkey register/delete, and initial factor fetch each report their own returned error or fallback message.

### Idle enrollment
- Show enrolled factors with labels `Authenticator App`, `Email`, `SMS`, a custom factor label, or the raw method.
- If no factors exist, show `No MFA methods enrolled`.
- Always show `Set Up Authenticator` and `Set Up Email MFA`.
- Show `Generate Backup Codes` only after at least one MFA factor exists.
- Show `Regenerate` only after MFA exists and backup-code remaining count is greater than zero.
- Show the inline `Passkeys` section for WebAuthn enrollment and passkey deletion; passkey credential management is composed here by `Passkeys`.
- `Passkeys` management has no separate screen spec in this stage. If the L12 passkey management spec lands later, cross-reference it from this section.

### TOTP enrollment
- `Set Up Authenticator` calls TOTP enrollment and switches to `Set Up Authenticator App`.
- Show instructions, the TOTP secret, the TOTP URI, a six-digit code input, `Verify Code`, and `Cancel`.
- `Verify Code` is disabled until a code is entered.
- Successful verification shows `TOTP MFA enrolled successfully`, clears the code, returns to idle, and refreshes factors.
- `Cancel` returns to idle and clears the TOTP code.

### Email MFA enrollment
- `Set Up Email MFA` starts email enrollment, shows `Verification code sent to your email`, and switches to `Confirm Email MFA`.
- Show a six-digit code input, `Confirm Email MFA`, and `Cancel`.
- `Confirm Email MFA` is disabled until a code is entered.
- Successful confirmation shows `Email MFA enrolled successfully`, clears the code, returns to idle, and refreshes factors.
- `Cancel` returns to idle and clears the email code.

### WebAuthn passkeys
- The passkey region asks for a passkey name and shows `Register Passkey`.
- Blank passkey names show `Passkey name is required`.
- During registration, the button shows `Registering...`; successful registration shows `Passkey "<name>" registered`, clears the input, refreshes factors, and updates AAL to `AAL2`.
- Registered passkeys render by display name, label, or `Passkey`, with a `Delete` action.
- During deletion, the delete button shows `Deleting...`; successful deletion shows `Passkey deleted` and refreshes factors.

### Backup codes
- `Generate Backup Codes` and `Regenerate` switch to `Save Your Backup Codes` when successful.
- The backup-code display warns that codes should be stored safely and renders every returned code.
- `Done` returns to idle while preserving the updated backup-code count.

## Navigation

- Route: `/admin/` with admin view `mfa-management`.
- Entry: Select `Multi-Factor Authentication` from the `Auth` sidebar section.
- Back: Browser back follows the admin shell history.
- Cancel: returns from the active enrollment subform to the idle `MFA Enrollment` state without leaving the screen.
- Done: returns from backup-code display to the idle state.

## Acceptance criteria

- Given the user opens `Multi-Factor Authentication`, when data loads, then the heading, AAL badge, enrolled-methods region, and TOTP/email/passkey entry points are visible. Evidence owner: `ui/src/components/__tests__/MFAEnrollment.test.tsx` and `ui/browser-tests-unmocked/smoke/mfa-management-view.spec.ts`.
- Given TOTP enrollment starts, when the enrollment API returns a secret and URI, then the secret and URI are visible and a code can be submitted. Evidence owner: `ui/src/components/__tests__/MFAEnrollment.test.tsx`.
- Given TOTP confirmation fails, when the user verifies an invalid code, then the returned error is visible. Evidence owner: `ui/src/components/__tests__/MFAEnrollment.test.tsx`.
- Given email enrollment starts, when the user enters the email code and confirms, then the confirm API receives that code and success is shown. Evidence owner: `ui/src/components/__tests__/MFAEnrollment.test.tsx`.
- Given passkey registration is available in the browser, when the user registers a passkey, then a browser WebAuthn prompt path is exercised and the passkey appears in the list. Evidence owner: `ui/browser-tests-unmocked/full/auth-passkey-lifecycle.spec.ts`.
- Given MFA exists, when backup codes are generated or regenerated, then every returned code is visible until `Done`. Evidence owner: `ui/src/components/__tests__/MFAEnrollment.test.tsx`.

## Edge cases

- No existing auth token or only anonymous token: the screen bootstraps an anonymous session and links generated email credentials before fetching MFA data.
- Factor payload is null: treat it as an empty factor list rather than crashing.
- Backup-code count fetch fails: treat remaining count as zero and keep MFA enrollment usable.
- Slow background refresh after backup-code regeneration: keep the newly returned codes visible.
- Browser WebAuthn unsupported or denied: show the returned passkey registration error in the passkey region.

## Current implementation gaps

- Current: The UI text mentions scanning a QR code, but `MFAEnrollment.tsx` renders the secret and TOTP URI text, not an actual QR image element.
- Target: If QR scanning is required as target behavior, render a scannable QR image and add browser/component coverage for it.
- Evidence: `ui/src/components/MFAEnrollment.tsx`; `ui/src/components/__tests__/MFAEnrollment.test.tsx`.
