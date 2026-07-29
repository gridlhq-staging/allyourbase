# Notifications

## Task

Send an admin notification to a user over a selected channel and recover cleanly from delivery failures.

## Layout

1. Header with `Notifications` title.
2. Notification form with `User ID`, `Title`, `Body`, and `Channel`.
3. `Send Notification` action.
4. Inline success status or error alert below the action.

## State contract

### Loading
- There is no initial data load; the screen renders the form immediately.
- While a send request is pending, `Send Notification` is disabled.

### Error
- Submit failure shows an accessible alert with `Failed to send notification.`
- Failure preserves all form values and re-enables submit.
- Starting a new submit clears stale error feedback.

### Compose
- `User ID`, `Title`, and `Channel` are required.
- `Body` is optional and omitted from the request when blank.
- `Send Notification` is disabled until required fields are present.

### Success
- Successful send shows `Notification sent successfully.`
- Successful send clears user id, title, body, and channel.
- Starting a new submit clears stale success feedback.

## Navigation

- Route: `/admin/` with admin view `notifications`.
- Entry: Select `Notifications` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Send action: stay on `Notifications`.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Notifications`, then the `Notifications` heading is visible. Evidence: `ui/browser-tests-unmocked/smoke/notifications.spec.ts`, `ui/browser-tests-unmocked/full/notifications-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given required fields are empty, then `Send Notification` is disabled. Evidence: `ui/src/components/__tests__/Notifications.test.tsx`.
- Given valid user id, title, body, and channel, when submitted, then `createNotification` receives those values. Evidence: `ui/src/components/__tests__/Notifications.test.tsx`.
- Given a notification is sent in a live browser, when the request succeeds, then success feedback appears and required fields reset. Evidence: `ui/browser-tests-unmocked/smoke/notifications.spec.ts`; `ui/browser-tests-unmocked/full/notifications-lifecycle.spec.ts`.
- Given submit fails, then an accessible alert appears, values are preserved, and submit is re-enabled. Evidence: `ui/src/components/__tests__/Notifications.test.tsx`; `ui/browser-tests-mocked/notifications-error-flows.spec.ts`.
- Given stale error or success feedback exists, when a new submit starts, then the stale feedback is cleared. Evidence: `ui/src/components/__tests__/Notifications.test.tsx`; `ui/browser-tests-mocked/notifications-error-flows.spec.ts`.
- Given the Notifications page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Notifications endpoint unavailable in live environments: browser tests skip only explicit 404, 501, or 503 probes.
- Blank body is allowed and should not block submission.
- Send failures do not clear user-entered values.

## Current implementation gaps

None verified.
