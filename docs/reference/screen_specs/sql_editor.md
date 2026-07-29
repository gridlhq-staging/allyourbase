# SQL Editor

## Task

Write, execute, inspect, copy, and persist SQL queries from either the selected-table SQL tab or the standalone admin SQL Editor.

## Layout

1. Selected-table shell entry: `ui/src/components/ContentRouter.tsx` renders the selected table name, non-public schema prefix, table kind badge, and the `Data`, `Schema`, `SQL`, `Synonyms`, and `Search Settings` view toggle buttons; `SQL` renders `SqlEditor`.
2. Standalone admin entry: `ContentRouter.renderAdminContent` renders `SqlEditor` for the `sql-editor` admin view opened from the sidebar.
3. Editor panel: `ui/src/components/SqlEditor.tsx` renders a CodeMirror SQL editor labelled `SQL query`, initialized from `localStorage` key `ayb_sql_query` or `SELECT 1 AS hello;`.
4. Run controls: below the editor, the `Execute` button runs the current query, changes to `Running...` while executing, and is disabled while loading or when the query is blank; the keyboard hint shows `Cmd+Enter` on Mac or `Ctrl+Enter` elsewhere.
5. Empty results area: before a query runs, the results area shows `Run a query to see results`.
6. Error result: failed execution renders the shared recoverable error notice with the backend error text and `View guide` linking to `https://allyourbase.io/guide/patterns`.
7. SELECT result table: queries returning columns render a result table with one column header per returned column, one row per result row, `null` values shown as italic `null`, object values stringified as JSON, and a duration/count status such as `N rows in Xms`.
8. Copy controls: SELECT-style result tables expose icon buttons titled `Copy as CSV` and `Copy as JSON`; clicking them writes the current result in that format and shows `CSV copied!` or `JSON copied!`.
9. DDL/DML result feedback: statements with no returned columns show a green success panel; DDL shows `Statement executed successfully in Xms`, DML shows `N rows affected in Xms`, and other no-column statements show `Query OK`.

## State contract

### Loading
- Clicking `Execute` or pressing the editor keybinding trims and submits the current query.
- While the request is in flight, `Execute` is disabled and labelled `Running...`; previous result and error output are cleared.
- Blank or whitespace-only query text keeps `Execute` disabled and does not call the SQL API.

### Error
- Query failure shows the backend error message in the results area without clearing the editor contents.
- The error notice links to `https://allyourbase.io/guide/patterns` and includes a `Retry` action that reruns the current editor query through the existing execute path.
- After an error, editing the query and executing again replaces the error with the new result or new error.

### Empty editor result
- Before any query result or error exists, the screen shows `Run a query to see results`.
- The default editor text is `SELECT 1 AS hello;` unless a previously successful query was persisted.

### SELECT results
- Returned column names render as table headers in server order.
- Returned row values render as cells in server order.
- `null` renders distinctly as italic `null`; objects and arrays render as compact JSON text.
- The status text reports exact row count and duration.
- CSV and JSON copy buttons are visible only when a result table exists.

### DDL, DML, and schema refresh
- DML statements such as `INSERT`, `UPDATE`, `DELETE`, and `MERGE` show affected-row feedback when the API returns no columns.
- DDL statements such as `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `GRANT`, `REVOKE`, and `COMMENT` show statement-success feedback when the API returns no columns.
- When `SqlEditor` receives `onSchemaChange`, successful DDL awaits the schema refresh before returning the execute button to the idle state.
- In the dashboard, both selected-table `SQL` and standalone `SQL Editor` pass `onSchemaChange`, so successful DDL should refresh the sidebar/schema cache.

### Query persistence
- Successful execution stores the trimmed query in `localStorage` under `ayb_sql_query`.
- A newly mounted SQL editor restores that stored query.
- Failed execution does not replace the stored successful query.

### Destructive statements
- Destructive statements are SQL text entered in the editor, including `DELETE`, `DROP`, and `TRUNCATE`.
- The target behavior requires an explicit destructive confirmation before executing those statements.

## Navigation

- Route: selected-table `SQL` view in the dashboard selected-table shell.
- Route: standalone admin `sql-editor` view.
- Entry: select a table and click the selected-table `SQL` toggle, or click `SQL Editor` / `Open SQL Editor` from the admin sidebar.
- Back: there is no screen-local back button; browser history, sidebar navigation, and selected-table view toggles remain owned by the surrounding dashboard shell.
- `Data`: from selected-table context, switches the selected-table content to `TableBrowser`.
- `Schema`: from selected-table context, switches the selected-table content to `SchemaView`.
- `Synonyms`: from selected-table context, switches the selected-table content to `SynonymsEditor`.
- `Search Settings`: from selected-table context, switches the selected-table content to `SearchSettingsEditor`.

## Acceptance criteria

- Given a selected table is open, when the user clicks `SQL`, then the selected-table content renders the SQL editor labelled `SQL query`.
- Given the standalone admin sidebar is visible, when the user clicks `SQL Editor`, then the same SQL editor renders outside selected-table context.
- Given no query has run, when the SQL editor first renders, then `Run a query to see results` is visible. Evidence: `ui/src/components/__tests__/SqlEditor.test.tsx`; `ui/browser-tests-unmocked/smoke/admin-sql-query.spec.ts`; `ui/browser-tests-unmocked/smoke/sql-view.spec.ts`.
- Given the editor contains a SELECT query, when the user clicks `Execute`, then returned column headers, row cells, row count, duration, and CSV/JSON copy controls are visible.
- Given a SELECT result table is visible, when the user clicks `Copy as CSV`, then `CSV copied!` appears.
- Given a SELECT result table is visible, when the user clicks `Copy as JSON`, then `JSON copied!` appears.
- Given the editor contains a DML statement, when the user executes it, then affected-row feedback is visible.
- Given the editor contains a DDL statement, when the user executes it, then statement-success feedback is visible and the schema cache refreshes.
- Given the editor contains invalid SQL or SQL that violates a unique constraint, when the user executes it, then the backend error text, `Retry` recovery action, and `https://allyourbase.io/guide/patterns` guide link are visible and the editor remains usable. Evidence: `ui/src/components/__tests__/SqlEditor.test.tsx`; `ui/browser-tests-unmocked/full/sql-lifecycle.spec.ts`; `ui/browser-tests-unmocked/full/sql-editor-lifecycle.spec.ts`.
- Given a query executes successfully, when the SQL editor is remounted, then that query is restored from local storage.
- Given the editor is blank, when the user views the run controls, then `Execute` is disabled. Evidence: `ui/src/components/__tests__/SqlEditor.test.tsx`; `ui/browser-tests-unmocked/smoke/sql-view.spec.ts`.
- Given the editor contains a destructive statement, when the user requests execution, then an explicit confirmation is required before the statement runs. Evidence: `ui/src/components/__tests__/SqlEditor.test.tsx`; `ui/browser-tests-unmocked/full/sql-lifecycle.spec.ts`; `ui/browser-tests-unmocked/full/sql-editor-lifecycle.spec.ts`.

## Edge cases

- Multi-row SELECT results preserve server row order and column order.
- Single-row SELECT results use singular `row` in the status text.
- Zero-row SELECT results still render headers and copy controls when columns are returned.
- DML row counts use singular or plural `row` according to the API-reported count.
- Failed clipboard writes still leave the result table visible; the shipped fallback is silent.
- Schema-refresh failure after successful DDL is surfaced as the execution error path because `execute` awaits `onSchemaChange`.
- Both selected-table and standalone contexts share one persisted query value.
