# Organizations

## Task

Create organizations, update organization metadata, manage members, teams, assigned tenants, usage, audit history, and deletion.

## Layout

1. Split layout with `organizations-view` root.
2. Left `org-list-panel` with `Organizations` title, create-org form, organization rows, slug, and plan badge.
3. Right detail pane with empty prompt, loading detail state, error banner, or selected organization detail.
4. Detail header with organization name, child/team/tenant counts, `Delete`, and tabs: Info, Members, Teams, Tenants, Usage, Audit.
5. Tab panels for info, members, teams and team members, tenant assignment, usage, and audit.

## State contract

### Loading
- Initial list loading shows a centered spinner and loading text for organizations in `organizations-view`.
- Detail loading after selecting an organization shows organization-detail loading text.

### Error
- Initial list failure shows `Failed to load organizations: <error>` with an error-scoped `Retry` that reruns the existing list loader.
- Detail failure shows `Failed to load organization: <error>` with an error-scoped `Retry` that reruns the selected-detail loader while the surrounding list remains visible.
- Create, update, member, team, tenant assignment, usage, audit, and delete failures show scoped errors without navigating away.

### Organization list and create
- Organization rows show name, slug, and plan tier.
- Empty list shows `No organizations found` in the detail pane.
- The list panel create form includes organization name, slug, plan tier, and optional parent org id.
- Creating requires name and slug; backend validation errors are shown inline.
- Successful create selects the new organization and refreshes the list.

### Info and deletion
- Info tab shows id, slug, plan, parent org, created, and updated values.
- `Update Organization` edits name, slug, and parent org id.
- `Save Info` persists edits; `Reset` restores current detail values.
- `Delete` deletes the selected organization and returns the detail pane to an unselected state after success.

### Members and teams
- Members tab supports adding members, updating roles, removing members, and preserving last-owner errors.
- Teams tab shows teams, supports creating teams, selecting a team, editing/deleting selected teams, and adding, updating, or removing team members.
- Team member additions require the user to be an org member when the backend enforces that prerequisite.

### Tenants, usage, and audit
- Tenants tab shows assigned tenants, supports assigning by tenant id, and unassigning.
- Usage tab shows period and date filters, totals, tenant count, and daily usage table when data exists.
- Audit tab shows filters for from, to, action, result, and actor id plus audit event rows.

## Navigation

- Route: `/admin/` with admin view `organizations`.
- Entry: Select `Organizations` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create, detail, tab, member, team, tenant assignment, usage, audit, and delete actions: stay on `Organizations`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Organizations`, then `organizations-view` and `org-list-panel` are visible. Evidence: `ui/browser-tests-unmocked/smoke/organizations.spec.ts`; `ui/browser-tests-mocked/organizations.spec.ts`; `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given list loading is pending, then organization loading text is visible with compliant contrast. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/src/components/__tests__/Organizations.contrast.test.tsx`.
- Given list loading fails, then the exact `Failed to load organizations: <error>` copy and an error-scoped `Retry` are visible; selecting Retry reruns the list loader and restores the uniquely seeded organization. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-unmocked/smoke/organizations.spec.ts`.
- Given selected-organization detail loading fails, then the exact `Failed to load organization: <error>` copy and an error-scoped `Retry` are visible while the organization list remains mounted; selecting Retry reruns the detail loader. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`.
- Given organizations exist, when the list renders, then organization names, slugs, and plan tiers are visible. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given no organizations exist, when loading completes, then `No organizations found` is visible. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-unmocked/smoke/organizations.spec.ts`.
- Given valid create fields, when the user creates an organization, then the create API receives name, slug, plan tier, and optional parent org id and the new organization detail opens. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-unmocked/full/organizations-lifecycle.spec.ts`.
- Given invalid create fields or backend create conflicts, then inline validation or backend error text is shown. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`.
- Given an organization is selected, when the user updates info, then the update API receives edited name, slug, and parent org id. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-unmocked/full/organizations-lifecycle.spec.ts`.
- Given the Members tab is opened, then members render and add, role update, remove, and last-owner protection flows are visible. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given the Teams tab is opened, then teams render and create, select, update, delete, add team member, update team member role, and remove team member flows are available. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given the Tenants tab is opened, then assigned tenants render and assign/unassign flows update the visible panel. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given the Usage tab is opened, then usage totals render and period/date filters serialize into usage queries. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given the Audit tab is opened, then audit events render and filters serialize into audit queries. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given an organization is deleted, then the delete API is called and the selected organization is cleared or removed. Evidence: `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-unmocked/full/organizations-lifecycle.spec.ts`; `ui/browser-tests-mocked/organizations.spec.ts`.
- Given the Organizations page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- The unmocked degraded-state proof isolates `_ayb_organizations`, restores it in `finally`, and deletes only its uniquely seeded organization.
- Removing or demoting the last owner must show the backend protection error and keep the member visible.
- Team-member add can fail when the user is not already an organization member.
- Delete failure keeps the selected organization visible and shows the returned error.
- Usage with no summary shows `No usage data available`; audit with no events shows `No audit events found`.

## Current implementation gaps

- Current: Browser proof covers a uniquely seeded list value, deterministic empty-list output, and exact real-server list failure with Retry recovery; detail-load retry is covered by the focused component contract.
- Target: Add browser proof for detail-load failure, create validation conflicts, all team-member edge failures, usage reset, audit reset, and delete failure when those flows enter scope.
- Evidence: `ui/src/components/Organizations.tsx`; `ui/src/components/OrganizationsSections.tsx`; `ui/src/components/OrganizationManagementSections.tsx`; `ui/src/components/OrganizationTeamSections.tsx`; `ui/src/components/__tests__/Organizations.test.tsx`; `ui/browser-tests-mocked/organizations.spec.ts`; `ui/browser-tests-unmocked/full/organizations-lifecycle.spec.ts`.
