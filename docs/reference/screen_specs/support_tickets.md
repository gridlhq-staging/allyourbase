# Support Tickets

## Task

Filter support tickets, inspect ticket threads, update ticket status or priority, and send support replies.

## Layout

1. Header with `Support Tickets` title.
2. Filter bar with status, priority, apply, and reset controls.
3. Tickets table with `Subject`, `Status`, `Priority`, `Tenant`, `Created`, and actions.
4. Ticket detail panel for the expanded ticket, including thread messages, status and priority selects, reply box, and `Send`.

## State contract

### Loading
- Before ticket data is available, keep heading and filters visible and show `Loading...`.

### Error
- List failure shows the `Support Tickets` title, the returned error message, and `Retry`; retry calls the existing ticket-list refresh owner.
- Detail, update, and reply failures keep the list and expanded detail context mounted while surfacing the returned error.

### Ticket list and filters
- `Status` supports all, open, in progress, waiting on customer, resolved, and closed.
- `Priority` supports all, low, normal, high, and urgent.
- `Apply Filters` refreshes the list with draft filter values.
- `Reset` clears filters and refreshes the list.
- Empty results show `No support tickets`.
- Rows show subject, status badge, priority badge, tenant id, created date, and `Details`.

### Ticket detail
- `Details` toggles the selected ticket detail panel.
- Expanding a ticket loads its messages.
- Detail panel shows the subject, status select, priority select, message thread, reply textarea, and `Send`.
- Message rows show sender type, body, and created timestamp.
- Status and priority changes call the ticket update endpoint and refresh the detail when the expanded ticket is affected.
- `Send` is disabled until reply body has non-whitespace content.
- Successful reply clears the reply draft and refreshes the thread.

## Navigation

- Route: `/admin/` with admin view `support-tickets`.
- Entry: Select `Support Tickets` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Filter, detail, update, and reply actions: stay on `Support Tickets`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Support Tickets`, then the `Support Tickets` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/support-tickets.spec.ts`, `ui/browser-tests-unmocked/full/support-tickets-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given tickets exist, when the table renders, then subject, status, priority, tenant, created date, and details action are visible. Evidence: `ui/src/components/__tests__/SupportTickets.test.tsx`; `ui/browser-tests-unmocked/smoke/support-tickets.spec.ts`.
- Given a seeded ticket has messages, when the user opens details, then the message thread is visible. Evidence: `ui/src/components/__tests__/SupportTickets.test.tsx`; `ui/browser-tests-unmocked/smoke/support-tickets.spec.ts`.
- Given an expanded ticket, when status and priority are changed, then the row reflects the new values. Evidence: `ui/browser-tests-unmocked/full/support-tickets-lifecycle.spec.ts`.
- Given an expanded ticket, when a reply is sent, then `adminAddMessage` receives the ticket id and body and the reply appears. Evidence: `ui/src/components/__tests__/SupportTickets.test.tsx`; `ui/browser-tests-unmocked/full/support-tickets-lifecycle.spec.ts`.
- Given list loading fails, then the `Support Tickets` heading and returned error text are visible. Evidence: `ui/src/components/__tests__/SupportTickets.test.tsx`; `ui/browser-tests-mocked/support-tickets-error-flows.spec.ts`.
- Given there are no tickets, when the list renders, then `No support tickets` is visible. Evidence: `ui/browser-tests-mocked/support-tickets-error-flows.spec.ts`.
- Given the Support Tickets page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Support tickets endpoint unavailable in live environments: browser tests skip only explicit 404, 501, or 503 probes.
- Target detail-load failure recovery keeps the list visible and surfaces the returned error.
- Reply body must not submit if blank or whitespace.

## Current implementation gaps

- Current: No reviewed browser test proves filter apply/reset changes the list query or result set.
- Target: Browser proof should cover status and priority filter serialization with deterministic tickets.
- Evidence: `ui/src/components/SupportTickets.tsx`; `ui/browser-tests-unmocked/full/support-tickets-lifecycle.spec.ts`; `ui/browser-tests-mocked/support-tickets-error-flows.spec.ts`.
