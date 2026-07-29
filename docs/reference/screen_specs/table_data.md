# Selected-Table Data

## Task

Browse, search, filter, inspect, create, edit, delete, export, and paginate rows for the currently selected table.

## Layout

1. Selected-table shell: `ui/src/components/ContentRouter.tsx` renders the selected table name, non-public schema prefix, table kind badge, and the `Data`, `Schema`, `SQL`, `Synonyms`, and `Search Settings` view toggle buttons.
2. Toolbar: `ui/src/components/TableBrowserToolbar.tsx` starts with a full-text search box labelled `Full-text search`, then a SQL-like filter input with placeholder `Filter... e.g. status='active' && age>25`, a shared `Apply` button, optional relation `Expand` menu, optional `Export` menu, selected-row batch delete control, and writable-table `New Row` button.
3. Grid header: `ui/src/components/TableBrowserGrid.tsx` shows a select-all checkbox for writable tables with a primary key, one sortable column header per table column, `PK` badges on primary-key columns, expanded relation column headers when enabled, and a row-action column.
4. Grid body: loading and empty states occupy the body; populated rows show one cell per column, `null` values in muted italic text, boolean values with status coloring, object values as JSON text, expanded relation cells when enabled, and per-row edit/delete icon actions.
5. Row detail drawer: clicking a row opens `Row Detail` from `ui/src/components/TableBrowserDialogs.tsx`, listing every column value with its type plus any expanded relation payloads, and writable primary-key rows expose edit and delete icon actions.
6. Create and edit drawers: `ui/src/components/RecordForm.tsx` opens from `New Row`, row actions, or row detail edit, with fields for table columns and primary-key fields skipped or disabled according to create/edit mode.
7. Delete dialogs: single-row delete opens `Delete record?`; selected rows open `Delete N records?`. Both dialogs require an explicit destructive confirmation.
8. Pagination footer: when data is loaded, the footer shows total row count, previous and next page controls, and the current page out of total pages.

## State contract

### Loading
- Initial data fetch keeps the toolbar visible and shows `Loading...` in the grid body until rows are available.
- Subsequent fetches preserve the last loaded grid while the next request is in flight.

### Error
- Fetch failure keeps the toolbar visible, shows an error banner below the toolbar, and provides a retry action that re-runs the current table request without changing table, page, sort, search, filter, or relation expansion state.
- Rate-limit responses may retry automatically within the bounded retry budget before showing the final error.

### Empty table
- A successful response with no items shows `No rows in this table yet` and the guidance `Insert data using the SQL editor, REST API, or SDK.`
- Current: the primary no-table-selected dashboard state showed only `Select a table from the sidebar` and `Use SQL Editor from the sidebar to create one.`, and the secondary empty-row state showed only `No rows in this table yet` plus insertion guidance, leaving migration help as a dead end.
- Target: both states retain their existing affordances and owner layouts while adding the shared CLI/docs CTA: `Migrating from another source?`, copy-pasteable `ayb migrate <source> --help`, and links to the migration, Supabase migration, and Algolia migration guides.
- Evidence: `ui/src/components/__tests__/ContentRouter.test.tsx`; `ui/src/components/__tests__/TableBrowser.test.tsx`; `ui/browser-tests-unmocked/smoke/create-table-nav.spec.ts`.
- Writable tables still show `New Row`; export and batch delete controls remain hidden because there are no rows to export or delete.

### Populated default view
- Entry route is selected-table `Data`; `ContentRouter.renderSelectedContent` renders `TableBrowser` for `view === "data"` and by default for selected-table views.
- The default request uses page 1, `PER_PAGE = 20`, no search, no filter, no expanded relations, and the standards-required default sort target.
- Seeded rows render before any create, update, delete, filter, or sort workflow is considered valid.
- Writable tables with a primary key show row selection, per-row edit/delete actions, batch delete after selection, and `New Row`; views or tables without a primary key hide write-only controls that require stable row identity.

### Search and filter
- Pressing Enter in the full-text search box applies search and resets to page 1.
- Pressing Enter in the filter input applies filter and resets to page 1.
- Clicking `Apply` submits both current search and filter text and resets to page 1.
- Clearing the search control removes the applied search and resets to page 1.
- Clearing the filter text and clicking `Apply` removes the applied filter and restores matching unfiltered rows.

### Sorting and pagination
- Clicking a sortable column header toggles that column from ascending to descending order and resets to page 1.
- Sort indicators show the active direction on the selected column.
- Pagination footer previous/next controls clamp to the first and last available pages and preserve the active search, filter, sort, and expansion state.

### Relation expansion and export
- The `Expand` menu appears only when the table has many-to-one relationships.
- Selecting a relation adds an expanded relation column and includes that relation in subsequent data fetches.
- The `Export` menu appears only after a loaded response contains at least one item and exports the current page as CSV or JSON.

### Row detail and mutation
- Clicking a row opens the row-detail drawer without selecting the row checkbox.
- Row-detail edit opens the edit drawer for the same row; row-detail delete opens the single-row delete confirmation.
- `New Row` opens the create drawer, successful create refreshes the grid, and failure stays in the drawer with an error.
- Row action edit opens the edit drawer, successful save refreshes the grid, and failure stays in the drawer with an error.
- Row action delete and row-detail delete must show `Delete record?` before the record is removed.
- Selecting one or more row checkboxes shows `Delete (N)`; confirming `Delete N records?` removes the selected rows and clears selection after refresh.

## Navigation

- Route: selected-table `Data` view in the dashboard selected-table shell.
- Entry: select a table from the dashboard table sidebar while the current view is `Data`, or click the `Data` toggle while another selected-table view is active.
- Back: there is no screen-local back button; browser history and sidebar/table selection remain owned by the surrounding dashboard shell.
- `Schema`: switches the selected-table content to `SchemaView`.
- `SQL`: switches the selected-table content to `SqlEditor`.
- `Synonyms`: switches the selected-table content to `SynonymsEditor`.
- `Search Settings`: switches the selected-table content to `SearchSettingsEditor`.

## Acceptance criteria

- Given a seeded table is selected, when the `Data` view opens, then seeded row values are visible in the grid before any mutation workflow begins. Evidence: `ui/src/components/__tests__/TableBrowser.test.tsx`; `ui/browser-tests-unmocked/full/table-browser-advanced.spec.ts`.
- Given the table request is loading with no prior data, then the grid body shows `Loading...`. Evidence: `ui/src/components/__tests__/TableBrowser.test.tsx`.
- Given the table request fails, then an error banner and retry action are visible without hiding the toolbar. Evidence: `ui/src/components/__tests__/TableBrowser.test.tsx`.
- Given a table has no rows, then the empty-table message appears and export/batch delete controls are hidden. Evidence: `ui/src/components/__tests__/TableBrowser.test.tsx`.
- Given a user enters text in full-text search and presses Enter, then only matching rows are shown and clearing the search restores the unsearched rows.
- Given a user enters a filter expression and clicks `Apply`, then only matching rows are shown and clearing the filter text plus `Apply` restores unfiltered rows.
- Given a user clicks a column header twice, then the rendered row order changes from ascending to descending for that column.
- Given a user clicks a row, then the row-detail drawer opens with all column values and type labels.
- Given a user clicks a row edit action, then the edit drawer opens and saving refreshes the visible grid with the edited value.
- Given a user clicks a row delete action, then `Delete record?` appears and the row is removed only after explicit confirmation.
- Given a user selects multiple rows, then `Delete (N)` appears and `Delete N records?` is required before batch deletion. Evidence: `ui/src/components/__tests__/TableBrowser.test.tsx`; `ui/browser-tests-unmocked/full/table-browser-advanced.spec.ts`.
- Given loaded rows exist, then the export menu offers CSV and JSON for the current page.

## Edge cases

- Tables without primary keys: row detail remains available, but row selection, row edit/delete actions, and batch delete are hidden.
- Non-writable views: browsing, search, filter, sort, expansion, export, row detail, and pagination remain available; create/edit/delete controls are hidden.
- Nullable values: grid cells and row detail render `null` distinctly instead of an empty string.
- Object or array values: grid cells render compact JSON and row detail renders formatted JSON.
- Relationship expansion with missing related data: expanded cells show `null` while preserving the expanded column.
- Page boundaries: previous is disabled on page 1, next is disabled on the final page, and page count falls back to `1` when the server reports zero pages.
- Delete failure: the confirmation dialog remains open and shows the failure message.
- Save failure: the create or edit drawer remains open and shows the failure message.

## Current implementation gaps

- Current: the initial `TableBrowser` sort state is `null`, so the first fetch passes no `sort` parameter.
- Target: the populated default view uses the standards-required default sort target.
- Evidence: `ui/src/components/TableBrowser.tsx` initializes `sort` with `useState<string | null>(null)` and `fetchData` passes `sort: sort || undefined`.
