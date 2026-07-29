# Push Notifications

## Task

Manage mobile push device tokens and audit push delivery history, register a device, revoke a device, and send a test push.

## Layout

1. Header with `Push Notifications` heading and the `Manage mobile push device tokens and delivery audit history` subtitle.
2. Tab bar with `Devices` and `Deliveries` toggle buttons; `Devices` is selected by default.
3. Devices tab: a filter form (`Filter App ID`, `Filter User ID`, `Include inactive`, `Apply Filters`), a `Register Device` action, and a device table.
4. Deliveries tab: a filter form (`Filter App ID`, `Filter User ID`, `Status`, `Apply Filters`), a `Send Test Push` action, and a delivery table with expandable detail rows.
5. `Register Device` modal and `Send Test Push` modal.

## State contract

### Loading
- While the initial `listAdminPushDevices` request is in flight and before any devices load, the whole screen shows a centered spinner with `Loading push notifications...`.
- On the `Deliveries` tab, while `listAdminPushDeliveries` is loading and no deliveries are present, the tab body shows an inline spinner with `Loading deliveries...`.

### Error
- When the initial device load fails and no devices are present, the screen shows a centered error state with the thrown message, or `Failed to load devices` when the thrown value is not an `Error`, plus a `Retry` action that reloads devices with the applied filters.
- On the `Deliveries` tab, when the delivery load fails and no deliveries are present, the tab body shows the delivery error message in a red panel.

### Devices tab
- The device table columns are `Token`, `Provider`, `Platform`, `User`, `Device Name`, `Active`, `Last Refreshed`, `Last Used`, and `Actions`.
- Each row shows a truncated token preview, a provider badge, platform, user id, device name or `-`, active `yes`/`no`, refreshed and used dates, and a `Revoke` action.
- When the device list is empty, the tab shows `No push devices found`.
- Applying filters submits the trimmed `Filter App ID`, `Filter User ID`, and `Include inactive` values and reloads the list; the applied filters are reused for later refreshes.
- `Revoke` revokes the device immediately, shows a success toast, and reloads the list with the applied filters.

### Deliveries tab
- The delivery table columns are `Title`, `User`, `Provider`, `Status`, `Error`, `Sent At`, `Created At`, and `Details`.
- Each row shows the title, user id, provider, a status badge, error code or `-`, sent and created dates, and a `View`/`Hide` toggle.
- When the delivery list is empty, the tab shows `No deliveries found`.
- Applying filters submits the trimmed app/user ids and the selected `Status` and reloads the list.
- `View` loads the delivery detail on demand while the row remains collapsed; after the detail request succeeds, the row expands with an inline panel containing `Body`, `Data Payload`, `Error Details`, and `Job ID`. `Hide` collapses it. A detail-load failure shows a toast error and leaves the row collapsed.

### Register Device modal
- The `Register Device` action opens a modal titled `Register Device` with `App ID`, `User ID`, `Provider` (`fcm`/`apns`), `Platform` (`android`/`ios`), `Token`, and `Device Name` fields.
- Submitting requires non-empty `App ID`, `User ID`, and `Token`; a missing required field shows `App ID, User ID, and Token are required.`
- A successful register shows a `Device registered` toast, closes and resets the modal, and reloads the device list; a failure shows the error toast and keeps the modal open.
- `Save Device` is disabled while the register request is in flight; `Cancel` closes the modal.

### Send Test Push modal
- The `Send Test Push` action opens a modal titled `Send Test Push` with `App ID`, `User ID`, `Title`, `Body`, and `Data (JSON)` fields.
- Submitting requires non-empty `App ID`, `User ID`, `Title`, and `Body`; a missing required field shows `App ID, User ID, Title, and Body are required.`
- Invalid `Data (JSON)` blocks the send and shows the JSON parse error as a toast.
- A successful send shows a `Push delivery queued` toast, closes and resets the modal, and reloads the deliveries; a failure shows the error toast.
- `Send Push` is disabled while the send request is in flight; `Cancel` closes the modal.

## Navigation

- Route: `/admin/` with the `Push Notifications` sidebar item selected (view id `push`).
- Entry: Select `Push Notifications` from the admin sidebar messaging group.
- Back: Browser back follows the admin app history.
- Devices / Deliveries: switch tabs in place; deliveries load lazily the first time the `Deliveries` tab is shown.

## Acceptance criteria

- Given a seeded push device token, when the user opens `Push Notifications`, then the `Push Notifications` heading and the seeded device appear in the `Devices` tab. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/push-devices.spec.ts`.
- Given seeded devices and deliveries, when the user opens `Push Notifications` and switches tabs, then seeded device and delivery data render in the respective views. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/push-notifications-lifecycle.spec.ts`.
- Given the register modal, when the user fills the required fields and submits, then a device-registered toast appears and the device list refreshes. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/push-notifications-lifecycle.spec.ts` and `ui/browser-tests-mocked/push-notifications.spec.ts`.
- Given a device row, when the user clicks `Revoke`, then a revoke toast appears and the list refreshes. Evidence owner: existing assertions in `ui/src/components/__tests__/PushNotifications.test.tsx` and `ui/browser-tests-mocked/push-notifications.spec.ts`.
- Given the deliveries tab, when the user applies a status filter, then the deliveries reload with that status. Evidence owner: existing assertions in `ui/src/components/__tests__/PushNotifications.test.tsx` and `ui/browser-tests-mocked/push-notifications.spec.ts`.
- Given a delivery row, when the user clicks `View`, then the delivery detail panel expands with body, data payload, error details, and job id. Evidence owner: existing assertions in `ui/src/components/__tests__/PushNotifications.test.tsx` and `ui/browser-tests-mocked/push-notifications.spec.ts`.
- Given the send modal, when the user fills the required fields and submits, then a push-queued toast appears. Evidence owner: existing assertions in `ui/src/components/__tests__/PushNotifications.test.tsx` and `ui/browser-tests-mocked/push-notifications.spec.ts`.
- Given a server failure, when the user registers a device or sends a push, then an error toast is shown. Evidence owner: existing assertions in `ui/browser-tests-mocked/push-notifications.spec.ts`.

## Edge cases

- No registered devices: show `No push devices found` while keeping filters and `Register Device` available.
- No deliveries: show `No deliveries found` while keeping filters and `Send Test Push` available.
- Include inactive: the devices filter can include inactive tokens via the checkbox.
- Missing required fields: block the register or send submit and show the specific required-fields message.
- Invalid push data JSON: block the send and show the parse error.
- Delivery detail fetch failure: keep the row collapsed and show a toast error.
- Register or send failure: keep the modal open and show the error toast.

## Current implementation gaps

- Current: The initial `Loading push notifications...` state and the device-load error/`Retry` flow have no dedicated unmocked browser assertions; they are covered by `ui/src/components/__tests__/PushNotifications.test.tsx` and mocked coverage rather than unmocked probes.
- Target: Unmocked probes could assert the screen loading and device error/retry states when a stable slow- or failing-response fixture is available without mocked routes.
- Evidence: `ui/src/components/PushNotifications.tsx`; `ui/src/components/push-notifications/PushNotificationsDevicesTab.tsx`; `ui/src/components/push-notifications/PushNotificationsDeliveriesTab.tsx`; `ui/browser-tests-unmocked/smoke/push-devices.spec.ts`; `ui/browser-tests-mocked/push-notifications.spec.ts`.
