# Incidents

## Task

Track service incidents, create incidents, expand timeline details, add updates, and resolve active incidents.

## Layout

1. Header with `Incidents` title, active/all filter toggle, and `Create Incident`.
2. Inline create form when creating an incident.
3. Incidents table with `Title`, `Status`, `Affected Services`, `Created`, and actions.
4. Timeline panel for the expanded incident.

## State contract

### Loading
- Before incident data is available, keep the header visible and show `Loading...`.

### Error
- List failure shows the `Incidents` title, the returned error message, and `Retry`; retry calls the existing incident-list refresh owner.
- Create, update, add-update, and resolve failures keep the list, form, or expanded timeline mounted while surfacing the returned error.

### Incident table
- Rows show title, status badge, affected services or `-`, created timestamp, `Details`, and `Resolve` when not resolved.
- Empty results show `No incidents`.
- `Show All` and `Active Only` toggle the active-only filter and refresh the list.

### Create incident
- `Create Incident` opens an inline form for title, status, and comma-separated affected services.
- `Create` is disabled until title has non-whitespace content.
- Submit creates the incident, closes the form, resets draft values, and refreshes the list.

### Timeline and updates
- `Details` toggles the selected incident timeline.
- Timeline identifies the selected incident by title.
- When no updates exist, show `No updates yet`.
- Updates show timestamp, status badge, and message.
- The add-update controls include update message, update status, and `Add Update`.
- `Add Update` is disabled until update message has non-whitespace content.
- `Resolve` changes the incident status to resolved and removes the row-level resolve action.

## Navigation

- Route: `/admin/` with admin view `incidents`.
- Entry: Select `Incidents` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create, update, detail, filter, and resolve actions: stay on `Incidents`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Incidents`, then the `Incidents` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/incidents.spec.ts`, `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given incidents exist, when the table renders, then title, status, affected services, created timestamp, and actions are visible. Evidence: `ui/src/components/__tests__/Incidents.test.tsx`; `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`.
- Given create form title is blank, then `Create` is disabled. Evidence: `ui/src/components/__tests__/Incidents.test.tsx`.
- Given valid incident values, when submitted, then create is called and the new incident appears in the list. Evidence: `ui/src/components/__tests__/Incidents.test.tsx`; `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`.
- Given an incident has updates, when the user opens `Details`, then the timeline shows the update message. Evidence: `ui/src/components/__tests__/Incidents.test.tsx`; `ui/browser-tests-unmocked/smoke/incidents.spec.ts`.
- Given an expanded incident, when the user adds an update, then the new update message appears in the timeline. Evidence: `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`.
- Given an unresolved incident, when the user clicks `Resolve`, then the row status becomes `resolved`. Evidence: `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`.
- Given list loading fails, then the `Incidents` heading and returned error text are visible. Evidence: `ui/src/components/__tests__/Incidents.test.tsx`; `ui/browser-tests-mocked/incidents-error-flows.spec.ts`.
- Given the Incidents page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Incidents endpoint unavailable in live environments: browser tests skip only explicit 404, 501, or 503 probes.
- Empty incident list: show `No incidents`.
- No timeline updates: show `No updates yet`.
- Affected services are trimmed and empty comma segments are discarded before create.

## Current implementation gaps

- Current: No reviewed browser test proves the active/all filter toggle changes the request or list contents.
- Target: Browser proof should cover both filter states with deterministic active and resolved incidents.
- Evidence: `ui/src/components/Incidents.tsx`; `ui/browser-tests-unmocked/full/incidents-lifecycle.spec.ts`; `ui/browser-tests-mocked/incidents-error-flows.spec.ts`.
