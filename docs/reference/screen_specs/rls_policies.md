# RLS Policies

## Task

Inspect tables, toggle row-level security, create policies, preview policy SQL, and delete policies for a selected table.

## Layout

1. Two-column screen with an internal `Tables` sidebar and selected-table policy workspace.
2. Table sidebar listing schema-qualified table names when needed.
3. Main workspace showing no-table, loading, error, empty-policy, or populated policy state.
4. Selected-table header with table name, RLS enabled/disabled badge, RLS toggle action, and `Add Policy` action.
5. Create RLS policy dialog.
6. SQL preview dialog.
7. Delete policy confirmation dialog.

## State contract

### Loading
- On first render, the first table sorted by schema-qualified name is selected by default.
- While `fetchData` is waiting for both `listRlsPolicies` and `getRlsStatus`, the selected-table workspace shows a spinner with `Loading policies...`.
- Changing the selected table reloads policies and RLS status for that table.

### Error
- When policy or status loading fails, the selected-table workspace shows an inline red error alert containing the returned error message, or the string form of the thrown value.
- The error state includes a `Retry` action.
- The shared error notice includes `View guide` linking to `https://allyourbase.io/guide/authentication#row-level-security-rls`.
- Clicking `Retry` reruns `fetchData` for the currently selected table.
- Toggle, create, and delete failures show toast errors rather than replacing the selected table context.

### Table sidebar
- The table list includes only schema entries whose kind is `table`, sorted by `<schema>.<name>`.
- The sidebar header is `Tables`.
- The selected table button has selected styling.
- Non-`public` tables render the schema prefix before the table name.
- When there are no tables, the sidebar shows `No tables found` and the main workspace shows `Select a table to manage RLS policies`.

### Selected table header
- The header displays the selected table name, prefixed by schema only when the schema is not `public`.
- When RLS status is available, the header shows either `RLS Enabled` with enabled styling or `RLS Disabled` with disabled styling.
- When RLS is disabled, the toggle action is `Enable RLS`.
- When RLS is enabled, the toggle action is `Disable RLS`.
- While a toggle request is in flight, the toggle action is disabled.
- Successful enable shows a success toast naming the selected table and refreshes policies/status.
- Successful disable shows a success toast naming the selected table and refreshes policies/status.

### Empty policy state
- When the selected table has no policies, the workspace shows `No policies on this table`.
- `Create your first policy` opens the same create dialog as `Add Policy`.

### Policy cards
- Each policy renders as a card keyed by policy name.
- The card header shows policy name, command badge, permissive/restrictive badge, `View SQL`, and `Delete policy`.
- Command badges distinguish `SELECT`, `INSERT`, `UPDATE`, `DELETE`, and `ALL`.
- If roles are present, the card shows `Roles: <roles>`.
- If a USING expression is present, the card shows `USING:` with the expression in monospace.
- If a WITH CHECK expression is present, the card shows `WITH CHECK:` with the expression in monospace.

### SQL preview
- Clicking a policy card `View SQL` action opens `SQL Preview`.
- The preview dialog displays the generated SQL for that policy in a focusable monospace block.
- `Close` dismisses the SQL preview without changing policy or RLS state.

### Create policy dialog
- The dialog is titled `Create RLS Policy` and names the selected table as `on <schema>.<table>`.
- Template buttons are shown for each reusable policy template and apply command, USING, and WITH CHECK values into the form.
- The form includes `Policy name`, `Command`, `Permissive`, `USING expression`, and `WITH CHECK expression`.
- `Command` offers the supported policy commands from the shared RLS helper.
- `Permissive` switches between `PERMISSIVE` and `RESTRICTIVE`.
- `Create Policy` is disabled while creating or while the trimmed policy name is blank.
- `Cancel` closes the dialog and resets unsaved form data to the initial defaults.
- Successful creation trims the policy name, submits selected table schema/name and form values, shows a success toast naming the policy, closes the dialog, resets the form, and refreshes the selected table data.
- Create failure keeps the dialog context recoverable and shows a toast error.

### Delete policy confirmation
- Clicking a policy card `Delete policy` action opens `Delete Policy`.
- The dialog states that it will permanently drop the exact policy name from the exact table name and that the action cannot be undone.
- `Cancel` closes the dialog without deleting the policy, and the policy card remains visible.
- Confirming `Delete` disables the button while deleting, shows `Deleting...`, deletes by schema-qualified table and policy name, closes the dialog on success, shows a success toast naming the policy, and refreshes the selected table data.
- Delete failure keeps the selected-policy context recoverable and shows a toast error.

## Navigation

- Route: `/admin/` with the `RLS Policies` sidebar item selected.
- Entry: Select `RLS Policies` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Table select: stays on `RLS Policies` and reloads the selected table workspace.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `RLS Policies`, then the `Tables` sidebar heading is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/rls-policies-view.spec.ts`.
- Given the RLS Policies screen has a default selected table, when the user inspects the workspace, then the RLS toggle and `Add Policy` actions are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/rls-policies-view.spec.ts`.
- Given the selected table has either no policies or existing policies, when the screen loads, then either `No policies on this table` or a policy action is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/rls-policies-view.spec.ts`.
- Given a table and policy have been seeded, when the user selects that table, then the seeded policy name appears in the policy list. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given a new table is selected with RLS disabled, when the user clicks `Enable RLS`, then the UI reports RLS enabled for that table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given the create policy dialog is open, when the user enters a policy name, command, and USING expression and submits, then the dialog closes, `RLS Enabled` is visible, and the new policy card shows the policy name, command, and USING expression. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given a policy card is visible, when the user opens SQL preview, then the generated SQL dialog contains the exact policy name and closes without removing the policy. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given a policy card is visible, when the user opens delete confirmation, then the dialog names the exact policy and table before any destructive action. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given delete confirmation is open, when the user clicks `Cancel`, then the dialog closes and the same policy card remains visible. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given delete confirmation is open for a policy, when the user confirms deletion, then the matching policy card is removed. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given RLS is enabled on the selected table, when the user clicks `Disable RLS`, then the UI reports RLS disabled for that table. Evidence owner: Stage 2-added assertion in `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
- Given policy or status loading fails, when the error state renders and the user clicks `Retry`, then the error and `https://allyourbase.io/guide/authentication#row-level-security-rls` guide link are visible and `fetchData` reruns for the selected table. Evidence owner: `ui/src/components/__tests__/RlsPolicies.failure.test.tsx`.

## Edge cases

- RLS service unavailable: unmocked browser tests may skip only for 503, 404, or 501 service probes.
- No schema tables: show `No tables found` and `Select a table to manage RLS policies`.
- Non-public schema: display the schema prefix in the sidebar and selected-table heading.
- Loading failure: keep the selected table context visible through the retryable error state.
- Toggle failure: keep the prior RLS status visible and show a toast error.
- Blank policy name: keep `Create Policy` disabled.
- Create cancel: reset unsaved form data so reopening starts from defaults.
- SQL preview close: leave policy and RLS state unchanged.
- Delete cancel: leave the selected policy and RLS state unchanged.
- Delete failure: keep the delete dialog context recoverable and show a toast error.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove no-table, initial loading, retry/error, create disabled state, create cancel reset, template application, create failure toast, toggle failure toast, delete failure toast, or non-public schema rendering because the current unmocked fixtures do not provide stable service-failure or schema-shape setup for those states.
- Target: Acceptance evidence should cover these target states when existing unmocked fixtures can exercise them without mocked routes, a new harness, or ad hoc service-failure setup.
- Evidence: `ui/src/components/RlsPolicies.tsx`; `ui/src/components/RlsPolicyCreateModal.tsx`; `ui/src/components/RlsPolicyActionModals.tsx`; `ui/browser-tests-unmocked/smoke/rls-policies-view.spec.ts`; `ui/browser-tests-unmocked/full/rls-policies.spec.ts`.
