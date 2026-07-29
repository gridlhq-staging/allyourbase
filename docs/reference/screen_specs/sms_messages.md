# SMS Messages

## Task

Browse the log of sent SMS messages with delivery status, page through history, and send a test SMS.

## Layout

1. Header row with `SMS Messages` heading and a `Send SMS` action.
2. Main content area showing loading, error, empty, or a populated messages table.
3. Messages table with columns `To`, `Body`, `Provider`, `Status`, `Sent At`, and `Error`.
4. Pagination footer with `Prev`, a `Page <n> of <total>` indicator, and `Next` when there is more than one page.
5. Send Test SMS modal.

## State contract

### Loading
- While `listAdminSMSMessages` is in flight and before any data is rendered, the screen shows a centered spinner with `Loading...`.
- The loading view replaces the whole screen; the header and table are not shown yet.

### Error
- When `listAdminSMSMessages` rejects, the screen shows a centered error state with an alert icon and the thrown error message, or `Failed to load messages` when the thrown value is not an `Error`.
- The error state includes a `Retry` action that reloads the current page.

### Empty state
- When the list is empty, the header with `SMS Messages` and the `Send SMS` action stay visible.
- The body shows a message icon and `No messages sent yet`.
- The `Send SMS` action still opens the send modal from the empty state.

### Populated table
- The table columns are `To`, `Body`, `Provider`, `Status`, `Sent At`, and `Error`.
- Each row shows the recipient phone, the body truncated to 60 characters with an ellipsis when longer, the provider, a status badge, the localized `created_at` timestamp, and the error message when present.
- The status badge is green for `delivered` and `sent`, red for `failed`, `undelivered`, and `canceled`, and yellow for any other status.

### Pagination
- Pagination controls render only when `totalPages` is greater than 1.
- The indicator shows `Page <current> of <total pages>`.
- `Prev` is disabled on page 1; `Next` is disabled on the last page.
- `Prev` and `Next` reload the list at the adjacent page.

### Send Test SMS modal
- The `Send SMS` action opens a modal titled `Send Test SMS` with `To (phone number)` and `Message body` fields.
- `Send` is disabled until both the phone and body are non-empty, and shows `Sending...` while the request is in flight.
- On success the modal shows a `Message sent` result block with the returned id, recipient, and status, clears the inputs, and refreshes the messages list.
- On failure the modal shows the send error text and leaves the form recoverable.
- `Cancel` and the close icon dismiss the modal.

## Navigation

- Route: `/admin/` with the `SMS Messages` sidebar item selected (view id `sms-messages`).
- Entry: Select `SMS Messages` from the admin sidebar messaging group.
- Back: Browser back follows the admin app history.
- Send SMS: opens the `Send Test SMS` modal in place and refreshes the list on a successful send.

## Acceptance criteria

- Given a seeded SMS message, when the user opens `SMS Messages`, then the `SMS Messages` heading, the seeded recipient phone, and the seeded body are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/sms-messages.spec.ts`.
- Given seeded messages with delivered, failed, and pending statuses, when the user views the table, then each row shows the matching status badge. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given a failed message with an error message, when the user views its row, then the error text and the `failed` badge are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given no messages exist, when the user opens `SMS Messages`, then `No messages sent yet` is shown. Evidence owner: existing assertion in `ui/src/components/__tests__/SMSMessages.test.tsx`.
- Given the user clicks `Send SMS`, when the modal opens, then the `Send Test SMS` heading, phone field, and body field are visible, and `Send` is disabled until both fields are filled. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given more than 50 seeded messages, when the user opens `SMS Messages`, then pagination controls appear, `Prev` is disabled on page 1, and `Next` advances to page 2. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
- Given the list request fails, when the error state renders, then the error message and a `Retry` action are visible and `Retry` re-fetches. Evidence owner: existing assertion in `ui/src/components/__tests__/SMSMessages.test.tsx`.

## Edge cases

- Empty log: show `No messages sent yet` while keeping the `Send SMS` action available.
- Long body: truncate to 60 characters with a trailing ellipsis.
- Single page of results: hide the pagination footer entirely.
- Provider not configured: the full E2E send test skips the live-send assertion when the provider probe reports unconfigured; the modal and validation flow still work.
- Send failure: keep the modal open with the entered values and show the send error.
- Load failure: replace the table with the error state and offer `Retry`.

## Current implementation gaps

- Current: The initial `Loading...` state and the list-load error/`Retry` flow have no dedicated unmocked browser assertions; they are covered only by `ui/src/components/__tests__/SMSMessages.test.tsx`.
- Target: Unmocked probes could assert loading and error/retry states when a stable slow- or failing-response fixture is available without mocked routes.
- Evidence: `ui/src/components/SMSMessages.tsx`; `ui/src/components/__tests__/SMSMessages.test.tsx`; `ui/browser-tests-unmocked/smoke/sms-messages.spec.ts`; `ui/browser-tests-unmocked/full/sms-dashboard.spec.ts`.
