# Schema Designer

## Task

Inspect database schema structure as an interactive graph and select tables to review columns, foreign keys, and indexes.

## Layout

1. Header bar with `Schema Designer` title and graph controls.
2. Zoom controls: `Zoom Out`, zoom percentage, `Zoom In`, `Fit View`, and `Auto Arrange`.
3. Graph canvas with one button node per table and SVG relationship edges.
4. Details panel with `data-testid="schema-details-panel"`.
5. Loading, error, and empty states replace the graph when schema data is unavailable.

## State contract

### Loading
- While schema-designer data is loading, the screen shows `Loading schema designer...`.
- The screen may use passed `loading` props or the internal `useSchemaDesignerData` loading state.

### Error
- When schema-designer data fails to load, the screen shows the error message.
- When a retry callback is available, `Retry` is visible and invokes the passed `onRetry` or internal retry function.

### Empty state
- When no graph nodes are available, the screen shows `No tables available`.

### Graph view
- Each schema table renders as a graph node button with `data-testid="schema-node-<schema>.<table>"`.
- A node shows its table label, kind, column count, and a preview of columns.
- Clicking a node selects it, updates the details panel, and writes `schemaTable=<schema>.<table>` into the current URL query string.
- If `schemaTable` is present in the URL on mount and matches a table, that table is selected.

### Controls
- `Zoom Out` decreases zoom to a minimum of 50%.
- `Zoom In` increases zoom to a maximum of 200%.
- `Fit View` resets zoom to 100%.
- `Auto Arrange` arranges nodes in a three-column grid and calls `onAutoArrange` when supplied.
- The zoom percentage is shown in `data-testid="schema-zoom-level"`.

### Details panel
- The details panel always renders with `data-testid="schema-details-panel"` when the graph view is active.
- When a table is selected, the panel shows `<schema>.<table>`, table kind, columns, foreign keys, and indexes.
- Empty foreign-key and index lists show `None`.
- When no table is selected, the panel shows `Select a table node to inspect details.`

## Navigation

- Route: `/admin/` with the `Schema Designer` admin sidebar item selected.
- Entry: Select `Schema Designer` from the `Database` section of the admin sidebar using `data-testid="nav-schema-designer"`.
- Back: Browser back follows the admin app history.
- Selected node: stays on `Schema Designer` and updates the current URL query string.
- This spec follows the `schema-designer` admin view, not the data `schema` or `schema_view` path.

## Acceptance criteria

- Given the admin app has a seeded table, when the user selects `Schema Designer`, then the seeded table appears as `data-testid="schema-node-public.<table>"`. Evidence: `ui/browser-tests-unmocked/smoke/schema-designer-table.spec.ts`.
- Given a visible schema node, when the user selects it, then `data-testid="schema-details-panel"` shows that table heading and its columns. Evidence: `ui/browser-tests-unmocked/smoke/schema-designer-table.spec.ts`.
- Given schema data is loading, when the screen renders, then `Loading schema designer...` is visible. Evidence: `ui/src/components/__tests__/SchemaDesigner.test.tsx`.
- Given schema data fails and retry is available, when the screen renders, then the error message and `Retry` action are visible. Evidence: `ui/src/components/__tests__/SchemaDesigner.test.tsx`.
- Given no tables are available, when loading completes, then `No tables available` is visible. Evidence: `ui/src/components/__tests__/SchemaDesigner.test.tsx`.
- Given the graph is visible, when the user uses zoom controls, then the zoom percentage changes within 50% through 200%, and `Fit View` returns it to 100%. Evidence: `ui/src/components/__tests__/SchemaDesigner.test.tsx`.
- Given the graph is visible, when the user clicks `Auto Arrange`, then node positions switch to the arranged grid. Evidence: `ui/src/components/__tests__/SchemaDesigner.test.tsx`.

## Edge cases

- Initial schema prop absent: the internal data hook owns loading, error, and retry behavior.
- URL query names an unknown `schemaTable`: the first available node remains the selected table.
- Tables without foreign keys or indexes show `None` for those sections.
- Relationship edges render only when both source and target nodes are present.

## Current implementation gaps

- Current: The smoke proof verifies node selection and details, but does not exercise zoom or `Auto Arrange` controls.
- Target: Existing schema-designer proof should cover visible graph controls without using raw DOM shortcuts.
- Evidence: `ui/src/components/SchemaDesigner.tsx:88-128` and `ui/browser-tests-unmocked/smoke/schema-designer-table.spec.ts:35-52`.
