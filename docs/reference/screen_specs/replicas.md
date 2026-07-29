# Replicas

## Task

Inspect read replicas, add replica targets, run health checks, promote or remove replicas, and execute confirmed failover.

## Layout

1. Header with `Replicas` title, topology subtitle, `Check Health`, `Add Replica`, and `Failover` actions.
2. Optional `Add New Replica` form with name, host, port, database, SSL mode, weight, max-lag, cancel, and add controls.
3. Replica table with `URL`, `State`, `Lag`, `Connections`, `Last Checked`, and `Actions` columns.
4. Promote/remove confirmation dialog, optionally including `Replica name` input for unnamed rows.
5. Failover confirmation dialog requiring typed `failover` before execution.

## State contract

### Loading
- Before replica data is available, the screen shows a centered spinner with `Loading replicas...`.
- Check-health, add, promote, remove, and failover actions show button-level loading state while preserving the current table.

### Error
- Initial replica-list failure shows a centered error state containing the returned error message, or `Failed to load replicas` when the thrown value is not an `Error`.
- The initial error state includes a `Retry` action that reloads the replica list.
- Check-health, add, promote, remove, and failover failures surface as toast errors and keep the current screen context recoverable.

### Replica table
- The header title is `Replicas` and the subtitle is `Manage read replicas and replication topology`.
- Empty replica results show `No replicas configured`.
- Rows show replica URL, state badge, lag bytes, in-use/total connections, and last-checked timestamp.
- Each row exposes `Promote` and `Remove` actions with accessible labels that include the replica name when present or URL fallback.

### Add replica
- `Add Replica` toggles the add form.
- Name, host, and database are required before `Add` is enabled.
- Port defaults to `5432`, SSL mode defaults to `verify-full`, weight defaults to `100`, and max lag defaults to `0`.
- `Cancel` closes the form and resets all fields to defaults.
- Successful add closes the form, resets fields, replaces the table with the returned replica list, and shows `Replica <name> added`. Reopening the form after a successful add shows defaults, not the submitted values.
- Add failure with status `503` shows `Replica lifecycle support is not enabled on this server. Enable replica lifecycle support, then retry.`
- Add failure with status `502` shows `Replica target is not reachable. Provide a reachable standby endpoint, then retry.`
- These two states are classified by `ApiError.status`, never by matching backend message text; all other statuses surface the backend message.
- After a `502` or `503` add failure the add button stops loading, the form stays open with its entered values, and no success state is applied.

### Promote, remove, and failover
- `Promote` opens `Promote Replica` and warns that the current primary will be demoted.
- `Remove` opens `Remove Replica` and warns that the replica will be disconnected from the pool.
- Rows without a stored replica name require `Replica name` input before promote/remove confirmation is enabled.
- Successful promote refreshes the replica list and shows `Replica promoted`.
- Successful remove reloads the replica list and shows `Replica removed`.
- `Failover` opens a destructive confirmation explaining downtime risk and requires typing `failover` before `Execute Failover` is enabled.

## Navigation

- Route: `/admin/` with admin view `replicas`.
- Entry: Select `Replicas` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Replica actions: Stay on `Replicas` and refresh the table after successful topology changes.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Replicas`, then the `Replicas` heading is visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/replicas.spec.ts`, `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a reachable standby is configured and a replica has been seeded, when the user opens `Replicas`, then the seeded replica row appears with host and healthy state. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/replicas.spec.ts`.
- Given the replicas screen is loaded with data, when the user inspects the table, then `URL`, `State`, `Lag`, and `Connections` columns are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/replicas.spec.ts`.
- Given a seeded replica row exists, when the user clicks `Check Health`, then a health-completed toast or health state result is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`.
- Given the add form is open and a reachable standby is configured, when the user submits name, host, port, and database, then the add returns `201`, `Replica <name> added` appears, and the created row renders with the target host. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`.
- Given the add form is open, when the user submits an unreachable target, then the add returns `502` or `503`, actionable operator guidance is visible, the add button stops loading, and the entered values remain in the open form. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`.
- Given `addReplica` resolves with the created replica, when the add succeeds, then the form closes, reopening it shows defaults, the returned row renders, and the toast names the replica. Evidence owner: existing assertion in `ui/src/components/__tests__/Replicas.test.tsx`.
- Given `addReplica` rejects with `ApiError` status `502` or `503`, when the add fails, then status-keyed operator guidance is shown and the form stays recoverable. Evidence owner: existing assertions in `ui/src/components/__tests__/Replicas.test.tsx` and `TestHandleAddReplicaConnectivityError` / `TestLifecycleHandlersReturnServiceUnavailableWhenNil` in `internal/server/admin_replicas_test.go`.
- Given a created replica row exists, when the user clicks its remove action, then `Remove Replica` opens before deletion and `Replica removed` appears after confirmation. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`.
- Given the Replicas page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Replica service not configured: the smoke test may skip for 501 or 404 service probes.
- No `AYB_DATABASE_REPLICA_URLS`: the smoke test and the full add-success test skip because neither can prove a reachable standby. Both resolve the target through the single `resolveReplicaSeedTarget` fixture in `ui/browser-tests-unmocked/fixtures/infra.ts`.
- Target is not a standby or connectivity check fails: the full add-success test may skip only on a `502`/`503` returned before any `201`. A missing row after a `201` is a failure, never a skip.
- The full unreachable-target test never skips; it always submits to `127.0.0.1:1` and asserts the recovery contract.
- No replicas: show `No replicas configured`.
- Missing replica name for existing row: require a name in the confirmation dialog before promote or remove.
- Failover confirmation typo: keep `Execute Failover` disabled until the input exactly equals `failover`.

## Current implementation gaps

- Current: Unmocked browser coverage does not prove initial retry/error state, empty state, promote confirmation, failover confirmation, unnamed-row replica-name input, add-form cancel/reset, or failed action toasts.
- Target: Acceptance evidence should cover these visible states when a stable replica fixture can exercise them without fake topology services.
- Evidence: `ui/src/components/Replicas.tsx`; `ui/browser-tests-unmocked/smoke/replicas.spec.ts`; `ui/browser-tests-unmocked/full/replicas-lifecycle.spec.ts`.
