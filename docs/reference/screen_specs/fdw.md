# FDW Management

## Task

Create and drop foreign servers, import foreign tables, and drop imported foreign tables from the admin database tools.

## Layout

1. Screen heading `FDW Management`.
2. Shared error state when either the server or table resource fails.
3. `Foreign Servers` section with `Add Server`, create form, and servers table.
4. `Foreign Tables` section with `Import Tables`, import form, and tables table.
5. Drop server confirmation dialog.
6. Drop table confirmation dialog.

## State contract

### Loading
- Server loading shows `Loading...` in the `Foreign Servers` section while table state remains independently owned.
- Table loading shows `Loading...` in the `Foreign Tables` section while server state remains independently owned.
- Create, import, and drop actions use the relevant `useAdminResource` action-loading state to disable their active confirmation or submit control.

### Error
- Server-list failure appears only in the `Foreign Servers` table with the exact message and `Retry`; retry calls the server refresh owner.
- Table-list failure appears only in the `Foreign Tables` table with the exact message and `Retry`; retry calls the table refresh owner.
- The unaffected section remains mounted because server and table loading, error, and retry state stay independently owned.

### Foreign servers
- `Foreign Servers` shows an `Add Server` action.
- The servers table columns are `Name`, `Type`, `Created`, and actions.
- Empty server data shows `No foreign servers`.
- Each server row has a `Drop <server>` action.

### Create server form
- `Add Server` opens the inline create form.
- Type defaults to `postgres_fdw` and offers `postgres_fdw` and `file_fdw`.
- For `postgres_fdw`, the form shows server name, host, port, database name, mapping user, and mapping password.
- For `file_fdw`, the form shows server name and filename.
- `Create` is enabled only when the required fields for the selected type are present, calls `handleCreateServer`, closes and resets the form on success, refreshes foreign tables, and leaves errors in the shared resource owner.
- `Cancel` closes and resets the form.

### Foreign tables
- `Foreign Tables` shows an `Import Tables` action.
- The tables table columns are `Schema`, `Table`, `Server`, `Columns`, and actions.
- Empty table data shows `No foreign tables`.
- Each table row has a `Drop <schema>.<table>` action.

### Import tables form
- `Import Tables` opens the inline import form.
- The form includes server selector, remote schema, and local schema.
- Local schema defaults to `public`.
- `Import` is enabled only when server and remote schema are present, calls `handleImportTables`, closes and resets the form on success, and leaves errors in the table resource owner.
- `Cancel` closes and resets the form.

### Destructive confirmations
- Dropping a server opens `Drop Server`, names the selected server, offers a `CASCADE (drop dependent objects)` checkbox, and calls `handleDropServer` only after confirmation.
- Dropping a table opens `Drop Table`, names the selected foreign table, and calls `handleDropTable` only after confirmation.
- Canceling either dialog clears the selected target without changing database objects.

## Navigation

- Route: `/admin/` with the `FDW Management` admin sidebar item selected.
- Entry: Select `FDW Management` from the `Database` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Create, import, and drop actions: stay on `FDW Management`.

## Acceptance criteria

- Given FDW is available and a file FDW server is seeded, when the user selects `FDW Management`, then `FDW Management`, `Foreign Servers`, `Foreign Tables`, and the seeded server row are visible. Evidence: `ui/browser-tests-unmocked/smoke/fdw.spec.ts`.
- Given the screen is loaded, when the user opens `Add Server`, then the create form shows fields appropriate to the selected FDW type.
- Given valid server input, when the user clicks `Create`, then the server form closes and the server list refreshes.
- Given a server row, when the user clicks `Drop`, then `Drop Server` requires confirmation before the server is removed.
- Given the screen is loaded, when the user opens `Import Tables`, then the import form shows server, remote schema, and local schema controls.
- Given valid import input, when the user clicks `Import`, then the import form closes and the foreign table list refreshes.
- Given a foreign table row, when the user clicks `Drop`, then `Drop Table` requires confirmation before the table is removed.

## Edge cases

- FDW API unavailable: the existing smoke proof skips environments where the endpoint reports `404` or `501`.
- `file_fdw` extension unavailable: the existing smoke proof skips when server seeding reports unsupported file FDW behavior.
- Server drop with dependencies: the user must opt into `CASCADE`.
- Server and table resources can load at different speeds and show independent loading states until a shared error occurs.

## Current implementation gaps

- Current: The smoke proof covers seeded server rendering and primary section/action visibility, but does not exercise create, import, or destructive confirmation flows.
- Target: Existing FDW proof should cover at least one inline form and one destructive confirmation through visible UI.
- Evidence: `ui/src/components/FDWManagement.tsx:54-105,282-318,397-413` and `ui/browser-tests-unmocked/smoke/fdw.spec.ts:45-61`.
