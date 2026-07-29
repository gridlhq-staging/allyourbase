# Log Drains

## Task

Configure external log drain destinations, validate optional header JSON, inspect delivery stats, and delete drains.

## Layout

1. Header with `Log Drains` title and `Create Drain` action.
2. Inline `New Log Drain` form when creating a drain.
3. Log drains table with `Name`, `Sent`, `Failed`, `Dropped`, and actions.
4. `Delete Drain` confirmation dialog.

## State contract

### Loading
- Before drain data is available, keep the header visible and show `Loading...`.

### Error
- List failure shows the `Log Drains` title, the returned error message, and `Retry`; retry calls the existing drain-list refresh owner.
- Create and delete failures keep the table, form, or confirmation context mounted while surfacing the returned error.

### Drain table
- Rows show drain name and delivery stats for sent, failed, and dropped events.
- Every row shows `Delete`.
- Empty results show `No log drains configured`.

### Create drain
- `Create Drain` opens `New Log Drain`.
- The form includes `Type`, `URL`, optional `Headers (JSON, optional)`, `Batch Size`, and `Flush Interval (seconds)`.
- `Type` supports HTTP, Datadog, and Loki.
- `Create` is disabled until URL is present.
- Header JSON must parse to an object whose values are strings.
- Invalid header JSON shows an inline validation error and does not call create.
- Successful create closes the form, resets draft values, and refreshes the list.

### Delete confirmation
- `Delete <name>` opens `Delete Drain`, names the selected drain, and requires `Delete` confirmation.

## Navigation

- Route: `/admin/` with admin view `log-drains`.
- Entry: Select `Log Drains` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Create and delete actions: stay on `Log Drains`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Log Drains`, then the `Log Drains` heading is visible. Evidence: `ui/src/components/Sidebar.tsx`; `ui/src/components/ContentRouter.tsx`; `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given drain rows exist, when the table renders, then drain names and sent, failed, and dropped stats are visible. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given the create form has a URL and no optional headers, when submitted, then the create API receives type and URL. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given optional headers are valid JSON object string values, when submitted, then the create API receives the parsed headers object. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given optional headers are invalid JSON, when the user submits, then `Headers must be valid JSON` is visible and create is not called. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given the user deletes a drain, when `Delete Drain` is confirmed, then the delete API receives the selected drain id. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given list loading fails, then the returned error message is visible below the heading. Evidence: `ui/src/components/__tests__/LogDrains.test.tsx`.
- Given the Log Drains page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- No drains: show `No log drains configured`.
- Empty headers field means no headers are sent.
- Headers must be a JSON object and every value must be a string.
- Delete is destructive and must remain behind confirmation.

## Current implementation gaps

- Current: Browser proof covers navigation, isolated seeded data, empty state, exact unreachable-API error, retry recovery, create flow, and delete confirmation, but does not prove header validation in a browser.
- Target: Browser proof should cover header validation when that form-specific behavior needs browser-level evidence.
- Evidence: `ui/browser-tests-unmocked/smoke/log-drains.spec.ts`; `ui/browser-tests-unmocked/full/log-drains-lifecycle.spec.ts`; `ui/src/components/__tests__/LogDrains.test.tsx`.
