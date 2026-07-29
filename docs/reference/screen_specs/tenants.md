# Tenants

## Task

Create tenants, inspect and update tenant details, manage members, maintenance, lifecycle state, and audit history.

## Layout

1. Split layout with `tenants-view` root.
2. Left `tenant-list-panel` with `Tenants` title, `Create Tenant`, tenant rows, state badges, and pagination.
3. Right detail pane with empty prompt, loading detail state, error banner, or selected tenant detail.
4. Detail header with tenant name, lifecycle actions, and tabs: Info, Members, Maintenance, Audit.
5. `Create Tenant` dialog.
6. `Delete Tenant` destructive confirmation dialog.
7. Tab panels for tenant info, members, maintenance and circuit breaker, and audit events.

## State contract

### Loading
- Initial list loading shows a centered spinner and loading text for tenants in `tenants-view`.
- Detail loading after selecting a tenant shows tenant-detail loading text and clears stale detail controls.

### Error
- Initial list failure shows `Failed to load tenants: <error>` with an error-scoped `Retry` that reruns the existing list loader.
- Detail failure shows `Failed to load tenant: <error>` with an error-scoped `Retry` that reruns the selected-detail loader while the surrounding list remains visible.
- Create, info save, member, lifecycle, maintenance, and audit failures show scoped errors without navigating away.

### Tenant list
- Tenant rows show name, slug, and lifecycle state.
- Empty list shows `No tenants found`.
- Pagination shows `Page <n> of <total>` with previous and next controls when total pages are positive.
- Selecting a tenant resets the active tab to Info.

### Create tenant
- `Create Tenant` opens a dialog with tenant name, slug, optional owner user id, isolation mode, plan tier, and region.
- Tenant name and slug are required, and slug must match lowercase letters, numbers, and hyphens.
- Owner user id can be empty, searched, or pasted as a raw id.
- Successful create closes the dialog, selects the created tenant, resets defaults, and refreshes the list.
- Duplicate slug errors show `A tenant with this slug already exists.`
- Unexpected create errors are redacted to generic guidance.

### Info tab
- Shows id, slug, isolation, plan, region, state, created, and updated values.
- `Update Tenant` allows editing tenant name and org metadata JSON.
- `Save Info` persists name and parsed org metadata; `Reset` restores the current detail values.

### Members tab
- Shows current members with user id, role, role update control, joined timestamp, and remove action.
- `Add Member` requires user id and role.
- Updating role, adding members, and removing members update the visible member table.

### Maintenance and Audit tabs
- Maintenance shows maintenance status, reason when present, enable or disable action, circuit breaker state, failures, probes, and reset action when the breaker is not closed.
- Audit shows filter controls for from, to, action, result, and actor id plus audit event rows.

### Lifecycle actions
- Active tenants show `Suspend` and `Delete`.
- Suspended tenants show `Resume` and `Delete`.
- Provisioning tenants show `Delete`.
- Deleting and deleted tenants do not show suspend, resume, or delete.
- `Delete` opens `Delete Tenant`, names the selected tenant, and requires typing the exact tenant slug before confirmation is enabled.
- `Cancel` closes `Delete Tenant`, clears the typed slug, leaves the detail state unchanged, and sends no delete request.
- Incorrect typed text keeps confirmation disabled and sends no delete request.
- Successful confirmation transitions the selected tenant to `deleting`, keeps that tenant selected, refreshes the list, and removes the Delete action from the detail header.

## Navigation

- Route: `/admin/` with admin view `tenants`.
- Entry: Select `Tenants` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create, detail, tab, member, maintenance, audit, suspend, resume, and delete actions: stay on `Tenants`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Tenants`, then `tenants-view` and `tenant-list-panel` are visible. Evidence: `ui/browser-tests-unmocked/smoke/tenants.spec.ts`; `ui/browser-tests-mocked/tenants.spec.ts`; `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given list loading is pending, then tenant loading text is visible with compliant contrast. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`.
- Given list loading fails, then the exact `Failed to load tenants: <error>` copy and an error-scoped `Retry` are visible; selecting Retry reruns the list loader and restores the uniquely seeded tenant. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/smoke/tenants.spec.ts`.
- Given selected-tenant detail loading fails, then the exact `Failed to load tenant: <error>` copy and an error-scoped `Retry` are visible while the tenant list remains mounted; selecting Retry reruns the detail loader. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`.
- Given tenant rows exist, when the list renders, then tenant names, slugs, and state badges are visible. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`.
- Given no tenants exist, when loading completes, then `No tenants found` is visible. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/smoke/tenants.spec.ts`.
- Given pagination has multiple pages, then `Page 1 of 3` style page info is visible. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`.
- Given the user creates a tenant with valid fields, then the tenant is selected and persisted, with ownerless creation allowed. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
- Given invalid create values or duplicate slug, then validation or duplicate-slug errors are shown and create is not called for invalid local input. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`.
- Given a tenant is selected, when the user updates info, then the update API receives the edited name and org metadata. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
- Given a tenant is selected, when lifecycle actions are used, then suspend, resume, and delete visibility and state transitions follow the tenant state. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
- Given a provisioning, active, or suspended tenant is selected, when the user opens Delete, then `Delete Tenant` requires the exact slug before confirmation and Cancel or incorrect text sends no request. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
- Given a provisioning tenant deletion is confirmed with the exact slug, then the live API transitions the tenant to `deleting`, keeps it selected, and no Delete action remains visible. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
- Given the Members tab is opened, then members render and add, role update, and remove flows update the visible table. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`.
- Given the Maintenance tab is opened, then maintenance and breaker state render and maintenance can toggle. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`.
- Given the Audit tab is opened, then audit events render and filters serialize into the audit query. Evidence: `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`.
- Given the Tenants page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- The unmocked degraded-state proof isolates `_ayb_tenants`, restores it in `finally`, and deletes only its uniquely seeded tenant.
- Switching selected tenant while detail is loading must clear stale controls.
- Delete transitions the tenant into deleting lifecycle state rather than immediately removing every server record.
- Deleting and deleted tenants must not expose a Delete action.
- Unexpected create errors must not expose raw database messages.

## Current implementation gaps

- Current: Browser proof covers a uniquely seeded list value, deterministic empty-list output, and exact real-server list failure with Retry recovery; detail-load retry is covered by the focused component contract.
- Target: Add browser proof for create-dialog validation, member add/remove, audit filter serialization, and detail-load failure when those flows enter scope.
- Evidence: `ui/src/components/Tenants.tsx`; `ui/src/components/TenantsSections.tsx`; `ui/src/components/TenantManagementSections.tsx`; `ui/src/components/__tests__/Tenants.test.tsx`; `ui/browser-tests-mocked/tenants.spec.ts`; `ui/browser-tests-unmocked/full/tenants-lifecycle.spec.ts`.
