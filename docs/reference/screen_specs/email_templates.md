# Email Templates

## Task

Browse email template keys, edit the subject and HTML for the selected template, preview the rendered output, send a test email, and reset or delete custom overrides.

## Layout

1. Header with `Email Templates` heading and the `Customize built-in auth emails and manage app-specific templates` subtitle.
2. Two-column layout: a left `Template Keys` list panel and a right editor panel.
3. Left panel: `Template Keys` label followed by a selectable list; each row shows the template key, a `builtin` or `custom` source badge, an `enabled`/`disabled` label, and the `updated` date.
4. Right panel: selected key heading, an `Editing <source> template` line, action buttons, the `Subject Template` and `HTML Template` fields, a `Preview Variables (JSON)` field, a `Test Recipient` field with `Send Test Email`, and a `Preview` block showing rendered `Subject`, `HTML`, and `Plaintext`.

## State contract

### Loading
- While the initial `listEmailTemplates` request is in flight and before any list has loaded, the screen shows a centered spinner with `Loading email templates...`.
- When a template key is selected and `getEmailTemplate` is in flight, the editor panel shows an inline spinner with `Loading template...`.

### Error
- When the initial list load fails and no list is present, the screen shows a centered error state with the thrown message, or `Failed to load email templates` when the thrown value is not an `Error`, plus a `Retry` action.
- Clicking `Retry` sets loading true and reruns `listEmailTemplates`.
- When loading a specific template fails, the editor remains mounted for the selected key; the failure is surfaced through the existing preview error text in the `Preview` block while the editable fields stay visible.

### Empty list
- When the template list has no items, the left panel shows `No templates found.` and the editor panel shows `Select a template key to edit.`

### Populated list and editor
- The list highlights the selected key; selecting a different key loads its effective subject, HTML, and default preview variables into the editor.
- The editor shows `Save Template`, which saves the current subject and HTML for the selected key and refreshes the list and effective template.
- Custom overrides additionally show an `Enable Override`/`Disable Override` toggle reflecting the current enabled state.
- Custom overrides show `Reset to Default` for system keys (keys prefixed `auth.`) and `Delete Template` for non-system keys.
- Builtin templates with no custom override show only `Save Template`.

### Preview
- The preview re-renders after a debounce when the subject, HTML, or preview variables change, provided the subject and HTML are non-empty.
- While a render is in flight the block shows `Rendering preview...`.
- Invalid preview-variable JSON shows a client-side parse error and does not send a preview request.
- A backend render failure shows the returned error text in the preview block.
- A successful render shows the rendered `Subject`, `HTML`, and `Plaintext`; stale responses from an earlier render are ignored in favor of the latest.
- Before any successful render the block shows `Preview will appear after template or variables change.`

### Send test email
- `Send Test Email` requires a non-empty `Test Recipient`; a blank recipient shows a `Test recipient is required` toast error.
- Invalid preview-variable JSON blocks sending and shows the parse error as a toast.
- A successful send shows a success toast naming the recipient; a failure shows the send error as a toast.

## Navigation

- Route: `/admin/` with the `Email Templates` sidebar item selected (view id `email-templates`).
- Entry: Select `Email Templates` from the admin sidebar messaging group.
- Back: Browser back follows the admin app history.
- Reset to Default / Delete Template: stays on `Email Templates` and reloads the list and effective template after the change.

## Acceptance criteria

- Given a seeded template key and subject, when the user opens `Email Templates` and selects that key, then the `Email Templates` heading is visible and the `Subject Template` field shows the seeded subject. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/email-templates-list.spec.ts`.
- Given a seeded custom template, when the user opens `Email Templates`, then the seeded key appears in the list and its subject loads into the editor. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts`.
- Given the `auth.password_reset` system template, when the user edits the subject and HTML and clicks `Save Template`, then a `Saved auth.password_reset` toast appears and a `Reset to Default` action becomes available. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts`.
- Given a customized template, when the user changes the preview variables, then the rendered subject and HTML output reflect the substituted values. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts` and `ui/browser-tests-mocked/email-templates-preview.spec.ts`.
- Given a customized system template, when the user clicks `Reset to Default`, then a reset toast appears, the subject reverts to the builtin value, and `Reset to Default` disappears. Evidence owner: existing assertions in `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts`.
- Given invalid preview-variable JSON, when the user edits the variables, then a client-side parse error is shown and no preview request is sent. Evidence owner: existing assertion in `ui/browser-tests-mocked/email-templates-preview.spec.ts`.
- Given missing template variables, when the backend rejects the preview, then the backend validation error is shown in the preview block. Evidence owner: existing assertion in `ui/browser-tests-mocked/email-templates-preview.spec.ts`.
- Given a selected template, when the user enters a recipient and clicks `Send Test Email`, then a test email is sent for that template key. Evidence owner: existing assertion in `ui/src/components/__tests__/EmailTemplates.test.tsx`.

## Edge cases

- Empty template set: show `No templates found.` in the list and `Select a template key to edit.` in the editor.
- Email templates service unavailable: the smoke test skips only for `503`, `404`, or `501` probe responses.
- Deleted template mid-edit: after delete the selection moves to another key and a stale effective-load failure for the removed key is ignored without a toast.
- Invalid preview JSON: block preview and send, and show the parse error inline or as a toast.
- Stale preview response: ignore an older in-flight render once a newer render completes.
- Blank test recipient: block sending and show `Test recipient is required`.

## Current implementation gaps

- Current: The initial list `Loading email templates...` spinner and the list-load error/`Retry` flow have no dedicated unmocked browser assertions; they are covered only by `ui/src/components/__tests__/EmailTemplates.test.tsx`.
- Target: Unmocked probes could assert the list loading and error/retry states when a stable slow- or failing-response fixture is available without mocked routes.
- Evidence: `ui/src/components/EmailTemplates.tsx`; `ui/src/components/__tests__/EmailTemplates.test.tsx`; `ui/browser-tests-unmocked/smoke/email-templates-list.spec.ts`; `ui/browser-tests-unmocked/full/email-templates-lifecycle.spec.ts`.
