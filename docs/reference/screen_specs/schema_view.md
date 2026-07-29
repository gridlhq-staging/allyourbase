# Selected-Table Schema

## Task

Inspect columns, constraints, indexes, relationships, and table comments for the currently selected table.

## Layout

1. Selected-table shell: `ui/src/components/ContentRouter.tsx` renders the selected table name, non-public schema prefix, table kind badge, and the `Data`, `Schema`, `SQL`, `Synonyms`, and `Search Settings` view toggle buttons.
2. Schema entry: `ContentRouter.renderSelectedContent` renders `<SchemaView table={selected} />` when `view === "schema"`.
3. Columns section: `ui/src/components/SchemaView.tsx` always renders a `Columns` table with `Name`, `Type`, `Nullable`, and `Default` headers. Primary-key columns show a key icon beside the column name.
4. Foreign Keys section: when the selected table has foreign-key metadata, render one card per constraint with the constraint name, local columns, referenced `schema.table(columns)`, and any `ON UPDATE` or `ON DELETE` actions.
5. Indexes section: when the selected table has index metadata, render an `Indexes` table with `Name`, `Method`, `Unique`, and `Definition` headers.
6. Relationships section: when the selected table has relationship metadata, render one card per relationship with field name, relationship type, and source-to-target table/column mapping.
7. Comment section: when the selected table has a table comment, render a `Comment` heading and the comment body.

## State contract

### Loading
- The selected-table shell is owned by the surrounding dashboard schema cache load; `SchemaView` renders only after a table is selected and table metadata is available.

### Error
- The selected-table shell owns schema-cache failure display and retry behavior; `SchemaView` has no screen-local error state or retry control.

### Populated schema
- `Table.columns` from `ui/src/types/schema.ts` is required and always rendered in the `Columns` table.
- Column names render as text, column types render as code text, nullable values render as `yes` or `no`, and missing defaults render as an em dash.
- Primary-key columns are marked in the `Name` cell using the shipped key icon next to the column name.
- `foreignKeys`, `indexes`, `relationships`, and `comment` are optional metadata fields on `Table`.

### Optional metadata hidden
- `Foreign Keys` is absent when `table.foreignKeys` is missing or empty.
- `Indexes` is absent when `table.indexes` is missing or empty.
- `Relationships` is absent when `table.relationships` is missing or empty.
- `Comment` is absent when `table.comment` is missing or empty.

## Navigation

- Route: selected-table `Schema` view in the dashboard selected-table shell.
- Entry: select a table from the dashboard table sidebar and click the `Schema` toggle, or remain on `Schema` while selecting another table.
- Back: there is no screen-local back button; browser history and sidebar/table selection remain owned by the surrounding dashboard shell.
- `Data`: switches the selected-table content to `TableBrowser`.
- `SQL`: switches the selected-table content to `SqlEditor`.
- `Synonyms`: switches the selected-table content to `SynonymsEditor`.
- `Search Settings`: switches the selected-table content to `SearchSettingsEditor`.

## Acceptance criteria

- Given a selected table, when the user opens `Schema`, then `ContentRouter.renderSelectedContent` renders `SchemaView` for that selected table. Evidence: `ui/src/components/__tests__/ContentRouter.test.tsx`; `ui/browser-tests-unmocked/smoke/schema-view.spec.ts`.
- Given the selected table has columns, when `Schema` renders, then the `Columns` table shows `Name`, `Type`, `Nullable`, and `Default` headers. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`; `ui/browser-tests-unmocked/smoke/schema-view.spec.ts`.
- Given a primary-key column is present, when the columns table renders, then the primary-key column name is visually marked with the key icon. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given nullable and non-nullable columns are present, when the columns table renders, then nullable cells show `yes` and non-nullable cells show `no`. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given a column has a default expression, when the columns table renders, then the default expression text is visible in that column row. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given a column has no default expression, when the columns table renders, then its default cell shows an em dash. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given index metadata is present, when `Schema` renders, then the `Indexes` table shows each index name, method, unique value as `yes` or `no`, and definition text. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`; `ui/browser-tests-unmocked/smoke/schema-view.spec.ts`.
- Given foreign-key metadata is present, when `Schema` renders, then each foreign-key card shows the constraint name, local columns, referenced `schema.table(columns)`, and update/delete actions exposed by the metadata. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given relationship metadata is present, when `Schema` renders, then each relationship card shows the field name, relationship type, and source-to-target mapping. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given a table comment is present, when `Schema` renders, then the `Comment` heading and comment body are visible. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.
- Given a table lacks foreign keys, relationships, or comments, when `Schema` renders, then those optional section headings are hidden while `Columns` remains visible. Evidence: `ui/src/components/__tests__/SchemaView.test.tsx`.

## Edge cases

- Tables with no optional metadata still render the `Columns` section.
- Empty or missing optional arrays hide their sections rather than rendering empty tables or cards.
- Missing column defaults render as an em dash, not an empty string.
- Non-public referenced tables include the referenced schema in foreign-key cards because the shipped renderer formats references as `referencedSchema.referencedTable(referencedColumns)`.
- Relationship field and type display only reflects metadata exposed through `Table.relationships`; the screen does not derive a second relationship model from foreign keys.

## Current implementation gaps

None verified.
