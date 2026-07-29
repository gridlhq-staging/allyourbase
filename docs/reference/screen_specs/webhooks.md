# Webhooks

## Task

Create, inspect, enable or disable, test, edit, delete, and review delivery history for database event webhooks.

## Layout

1. Header with `Webhooks` title, event-notification subtitle, and `Add Webhook` action.
2. Main content area showing loading, error, empty, or populated list state.
3. Webhooks table with URL, event, table, enabled, and action columns.
4. Create or edit webhook modal.
5. Delete webhook confirmation modal.
6. Delivery history modal for a selected webhook.

## State contract

### Loading
- While `fetchWebhooks` is waiting for the first `listWebhooks` response, the screen shows a centered spinner and `Loading webhooks...`.
- Testing a webhook disables only the active row test action and shows a spinner in that row action.

### Error
- When `fetchWebhooks` fails, the screen shows the error message or `Failed to load webhooks`.
- The error state includes `Retry`; clicking it sets loading true and reruns `fetchWebhooks`.
- Toggle, delete, copy, and test failures report through toasts without replacing the list.

### Empty state
- When no webhooks exist, the screen shows `No webhooks configured yet`.
- The empty state explains that create, update, and delete events can be delivered to external URLs in real time.
- `Create your first webhook` opens the same create modal as `Add Webhook`.

### Webhook list
- The table columns are `URL`, `Events`, `Tables`, `Enabled`, and `Actions`.
- Each row shows the webhook URL, HMAC-secret indicator when present, copy URL action, event badges, table badges or `all tables`, enabled switch, and row action bar.
- The enabled switch calls `handleToggleEnabled`, updates the row in place on success, and reports success or failure through a toast.
- Copy URL uses the browser clipboard and reports success or failure through a toast.

### Create and edit modal
- `Add Webhook` opens create mode.
- Editing a row opens edit mode with the selected webhook as initial data.
- Saving a new webhook appends it to the list and reports `Webhook created`.
- Saving an existing webhook replaces the matching row and reports `Webhook updated`.
- Closing the modal leaves the list unchanged.

### Row actions
- `Delivery History` opens delivery history for the selected webhook.
- `Test` calls `handleTest`, reports success with status and duration, or reports the returned error/failure status.
- `Edit` opens the edit modal.
- `Delete` opens delete confirmation.

### Delete confirmation
- `Delete Webhook` warns that deletion cannot be undone and shows the selected URL.
- `Delete` calls `handleDelete`, removes the row from local state on success, closes the modal, and reports `Webhook deleted`.
- `Cancel` closes the modal without deleting.

## Navigation

- Route: `/admin/` with the `Webhooks` admin sidebar item selected.
- Entry: Select `Webhooks` from the `Services` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Delivery History: stays on `Webhooks` and opens the delivery history modal.
- Create, edit, test, enable/disable, and delete: stay on `Webhooks`.

## Acceptance criteria

- Given the admin app is loaded, when the user selects `Webhooks`, then the `Webhooks` heading and `Add Webhook` action are visible. Evidence: `ui/browser-tests-unmocked/smoke/webhooks-crud.spec.ts`.
- Given a user creates a webhook through the modal, when creation succeeds, then the webhook URL appears in the list. Evidence: `ui/browser-tests-unmocked/smoke/webhooks-crud.spec.ts`.
- Given a webhook row exists, when the user deletes it and confirms, then the row is removed from the list. Evidence: `ui/browser-tests-unmocked/smoke/webhooks-crud.spec.ts`.
- Given a webhook delivery is generated, when the user opens `Delivery History`, then each expected delivery attempt is visible and expandable. Evidence: `ui/browser-tests-unmocked/full/dashboard-webhook-delivery-journey.spec.ts`.
- Given webhook loading is in progress, when the screen renders, then `Loading webhooks...` is visible. Evidence: `ui/src/components/__tests__/Webhooks.test.tsx`.
- Given webhook loading fails, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/Webhooks.test.tsx`.
- Given no webhooks exist, when loading completes, then `No webhooks configured yet` and `Create your first webhook` are visible. Evidence: `ui/src/components/__tests__/Webhooks.test.tsx`.
- Given a webhook row exists, when the user toggles the enabled switch, then the row updates to the opposite enabled state and a success toast is shown.
- Given a webhook row exists, when the user clicks `Test`, then the active row shows test progress and a success or error toast reports the result. Evidence: `ui/src/components/__tests__/Webhooks.test.tsx`.

## Edge cases

- Webhook without scoped tables: the table column shows `all tables`.
- Webhook with HMAC secret: the URL cell shows the secret-configured indicator without exposing the secret value.
- Test endpoint failure: the failure message is shown as a toast and the row remains available.
- Clipboard failure: the copy failure is shown as a toast and the URL remains visible.

## Current implementation gaps

- Current: Existing proofs cover create/delete and delivery-history behavior, but do not assert enable/disable, copy URL, edit modal, test action, loading, error, or empty states.
- Target: Existing webhook proofs should cover at least enable/disable and test action behavior when the unmocked environment can make those deterministic.
- Evidence: `ui/src/components/Webhooks.tsx:45-91,207-278` and `ui/browser-tests-unmocked/smoke/webhooks-crud.spec.ts:31-79`.
