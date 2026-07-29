# SAML Configuration

## Task

Create, edit, inspect, and delete SAML identity provider configurations.

## Layout

1. Padded page container with `SAML Configuration` heading.
2. Header action `Add Provider`.
3. Inline create/edit provider form when opened.
4. Loading, error, empty, or populated provider table.
5. Delete confirmation dialog for the selected provider.

## State contract

### Loading
- Show the `SAML Configuration` heading and `Add Provider` action.
- Show `Loading...` in the table area while provider data is loading.

### Error
- Show the `SAML Configuration` heading and the returned error message in the table error region.
- The error includes `Retry`, which calls the existing provider-list refresh owner while the heading and `Add Provider` action remain mounted.
- Invalid attribute mapping JSON in the form sets the screen error to `Attribute Mapping is not valid JSON` and does not submit.

### Empty provider list
- When loading is complete and no providers exist, the table area shows `No SAML providers configured`.
- `Add Provider` remains available.

### Populated provider list
- The table columns are `Name`, `Entity ID`, `Updated`, and row actions.
- Each row shows provider name, entity ID, formatted updated date, `Edit`, and `Delete`.
- Pagination is fixed to one page for this screen.

### Create provider
- `Add Provider` opens a `New Provider` form.
- The form captures name, entity ID, metadata URL, metadata XML, and attribute mapping JSON.
- `Create` is disabled while an action is loading, when name is blank, or when entity ID is blank.
- Metadata URL and metadata XML are optional inputs; blank metadata URL/XML submit as omitted values.
- Attribute mapping is optional; when present, it must parse as JSON and submits as an object.
- `Cancel` closes the form and resets unsaved values.
- Successful create closes and resets the form, then refreshes through the shared admin-resource action owner.

### Edit provider
- `Edit` opens the same form titled `Edit <name>`.
- Name and entity ID are populated from the selected provider.
- Metadata XML and attribute mapping are populated from the selected provider when available.
- Metadata URL is intentionally blank on edit because the provider row does not expose a stored metadata URL.
- `Update` follows the same validation and action-loading rules as create.

### Delete provider
- `Delete` opens a `Delete Provider` confirmation dialog naming the selected provider and warning that deletion cannot be undone.
- Confirming delete calls the delete action, disables the dialog action while loading, closes the dialog on success, and refreshes through the shared admin-resource action owner.
- Cancel closes the dialog without deleting.

## Navigation

- Route: `/admin/` with admin view `saml`.
- Entry: Select `SAML Configuration` from the `Auth` sidebar section.
- Back: Browser back follows the admin shell history.
- Create/Update/Delete: stay on `SAML Configuration` and refresh the provider list on success.
- Cancel: closes the open form or dialog without leaving the screen.

## Acceptance criteria

- Given the user opens `SAML Configuration`, when providers load, then the `SAML Configuration` heading and provider table or empty message are visible. Evidence owner: `ui/browser-tests-unmocked/smoke/saml.spec.ts`.
- Given the user opens the new-provider form, when name and entity ID are blank, then `Create` is disabled. Evidence owner: `ui/src/components/__tests__/SAMLConfig.test.tsx`.
- Given valid create input, when the user submits, then `createSAMLProvider` receives name, entity ID, and optional metadata. Evidence owner: `ui/src/components/__tests__/SAMLConfig.test.tsx`.
- Given invalid attribute mapping JSON, when the user submits the form, then `Attribute Mapping is not valid JSON` is shown and `createSAMLProvider` is not called. Evidence owner: `ui/src/components/__tests__/SAMLConfig.test.tsx`.
- Given a provider exists, when the user edits and updates it, then the update request targets that provider and the form closes on success. Evidence owner: `ui/browser-tests-unmocked/full/saml-lifecycle.spec.ts`.
- Given a provider exists, when the user confirms deletion, then the provider row is removed. Evidence owner: `ui/browser-tests-unmocked/full/saml-lifecycle.spec.ts`.

## Edge cases

- SAML endpoint unavailable: unmocked probes may skip only for unavailable endpoint status.
- Empty provider list: show the empty table message and keep creation available.
- Invalid JSON mapping: show an error and preserve entered form values for correction.
- Delete cancel: leave the provider row unchanged.
- Action failure: show the shared admin-resource error state from the owner hook.

## Current implementation gaps

- Current: `SAMLConfig.tsx` allows metadata URL entry and metadata XML paste, but it does not provide a file upload or URL-fetch preview flow for metadata import.
- Target: If richer metadata import is required, add an explicit import workflow and evidence for URL/XML parsing feedback.
- Evidence: `ui/src/components/SAMLConfig.tsx`; `ui/src/components/__tests__/SAMLConfig.test.tsx`; `ui/browser-tests-unmocked/full/saml-lifecycle.spec.ts`.
