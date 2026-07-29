# Auth Settings

## Task

Configure project-wide auth method toggles, built-in OAuth providers, and custom OIDC providers from one auth administration screen.

## Layout

1. Screen heading `Auth Settings`.
2. Inline error banner for settings update failures and inline success banner for successful toggle saves.
3. Auth toggles region with one checkbox row each for magic link, SMS, email MFA, anonymous auth, TOTP, and WebAuthn.
4. `OAuth Providers` region with provider-level warning/success banners.
5. Provider list rows showing provider name, type (`Built-in` or `OIDC`), enabled/disabled status, client ID configured/missing status, and row actions.
6. Inline provider edit form, shown under the selected provider row.
7. Delete confirmation dialog for custom OIDC providers only.
8. Custom OIDC add form region, collapsed behind `Add OIDC Provider` until opened.

## State contract

### Loading
- Before settings resolve, show a centered spinner and `Loading...`.
- No auth toggles, provider rows, provider forms, or OIDC add form are visible while the composed screen is in initial loading.

### Error
- If auth settings fail before any settings object exists, show a centered error state with the returned message and a `Retry` action.
- `Retry` reruns the settings and provider fetch.
- If provider loading fails while settings load successfully, keep the toggles visible and show the provider error inside the `OAuth Providers` region.
- If a toggle save fails, keep the current screen visible and show the error banner above the toggles.

### Auth toggles
- Each toggle reflects its current boolean value and calls the auth-settings update API with the full updated settings payload when changed.
- Successful toggle updates replace local settings state and show `Settings updated`.
- Toggle controls remain on the screen with provider management; this is one navigable screen, not separate settings screens.

### OAuth provider list
- Empty provider list shows `No OAuth providers configured.` when no provider error is present.
- Populated provider rows show provider name, provider type, enabled/disabled status, client ID status, and `Edit` or `Configure` based on whether client ID is configured.
- Built-in provider edit rows can show setup instructions with provider console URL and redirect URI.
- Built-in provider forms include `Enable provider`, client ID, client secret except Apple, and provider-specific fields for Microsoft tenant ID or Apple team/key/private key.
- OIDC provider forms include `Enable provider`, client ID, client secret, issuer URL, optional display name, and space-separated scopes.
- `Test Connection` shows a provider-specific success or error result without leaving the form.
- `Save Provider` is disabled while saving, then closes the form and shows a provider success message on success.
- `Cancel` closes the provider edit form and clears unsaved edit/test state.

### OIDC create and delete
- `Add OIDC Provider` opens an inline `Add Custom OIDC Provider` form.
- The create form requires provider name before save; blank provider name shows `Provider name is required`.
- The create form captures provider name, issuer URL, client ID, client secret, display name, and scopes.
- Successful create reloads providers, closes the form, resets fields, and shows `OIDC provider "<name>" added.`.
- OIDC provider rows show `Delete`; built-in provider rows do not.
- `Delete` opens a `Delete Provider` confirmation dialog naming the provider and warning that deletion cannot be undone.
- Confirming delete disables the dialog action while deleting, reloads providers, closes any open edit form for that provider, and shows `Provider "<name>" deleted.`.

## Navigation

- Route: `/admin/` with admin view `auth-settings`.
- Entry: Select `Auth Settings` from the `Auth` sidebar section.
- Back: Browser back follows the admin shell history; in-screen cancel actions close inline forms without leaving `Auth Settings`.
- Retry: stays on `Auth Settings` and reruns the screen fetch.

## Acceptance criteria

- Given live auth settings exist, when the user opens `Auth Settings`, then all six method toggles render with checked states matching the settings in component coverage, and the five smoke-asserted toggles (`totp_enabled`, `anonymous_auth_enabled`, `email_mfa_enabled`, `sms_enabled`, `magic_link_enabled`) match the live API booleans in the unmocked smoke run. Evidence owner: `ui/src/components/__tests__/AuthSettings.test.tsx` (all six toggles render with correct initial state); `ui/browser-tests-unmocked/smoke/auth-settings-view.spec.ts` (five live-API toggle states).
- Given provider data exists, when the user opens `Auth Settings`, then the `OAuth Providers` region renders provider rows with type and configuration status. Evidence owner: `ui/src/components/__tests__/AuthSettings.test.tsx`.
- Given a built-in provider is edited, when credentials are saved, then `updateAuthProvider` receives enabled state and the provider-specific credential payload. Evidence owner: `ui/src/components/__tests__/AuthSettings.test.tsx`.
- Given a custom OIDC provider is added, when the user saves valid fields, then the provider appears as `OIDC` and enabled. Evidence owner: `ui/browser-tests-unmocked/full/auth-provider-management.spec.ts`.
- Given a custom OIDC provider exists, when the user deletes it and confirms the dialog, then the row is removed and the provider is absent from the API list. Evidence owner: `ui/browser-tests-unmocked/full/auth-provider-management.spec.ts`.
- Given provider form helpers receive whitespace-padded OIDC input, when building payloads, then issuer/display name are trimmed and scopes are split into an array. Evidence owner: `ui/src/components/__tests__/auth-settings-helpers.test.ts`.

## Edge cases

- Settings API unavailable: show full-screen error with retry.
- Provider API unavailable: keep auth toggles usable and show provider-region warning.
- No providers configured: show the provider empty message without a dead action.
- Provider test failure: show the test error inline in the open provider form.
- OIDC provider name blank: block save and show an inline validation error.
- OIDC reload failure after create/delete: keep the provider-region error visible so the user knows the list may be stale.
- Delete cancel: close the confirmation dialog and leave the provider row unchanged.

## Current implementation gaps

- Current: Automated browser unmocked coverage proves five live toggle states, seeded built-in provider rendering, built-in edit instructions, OIDC create/edit-field visibility, and OIDC delete. Component coverage proves the WebAuthn toggle renders from settings and covers full-screen settings fetch failure, provider-only load failure, toggle save failure, provider test failure, OIDC blank-name validation, and OIDC reload failure.
- Target: Extend unmocked browser or seeded coverage to the WebAuthn toggle and deterministic error/validation branches when the harness can produce them without mocks.
- Evidence: `ui/src/components/AuthSettings.tsx`; `ui/src/components/AuthSettingsProviders.tsx`; `ui/src/components/AuthSettingsToggles.tsx`; `ui/src/components/AuthSettingsOIDCForm.tsx`; `ui/src/components/__tests__/AuthSettings.test.tsx`; `ui/src/components/__tests__/AuthSettings.oidc-delete.test.tsx`; `ui/browser-tests-unmocked/smoke/auth-settings-view.spec.ts`; `ui/browser-tests-unmocked/full/auth-provider-management.spec.ts`.
