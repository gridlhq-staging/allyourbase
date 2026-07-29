# Collection Synonyms

## Task

Configure search synonym groups for the selected collection from the dashboard table shell without changing the backend, SDK, examples, OpenAPI, docs-site, or search-consumer surfaces.

## Layout

1. Selected collection shell: keep the existing selected-table header with the collection title and kind badge, owned by `ui/src/components/ContentRouter.tsx:242-280`.
2. Selected collection tab strip: render `Synonyms` beside the existing `Data`, `Schema`, and `SQL` table buttons. The view inventory owner is `ui/src/components/layout-types.ts:8`; selected-table rendering and tab button ownership stay in `ui/src/components/ContentRouter.tsx:207-280`.
3. Synonyms panel header: show the resolved collection label using the same title context as the shell. If the backend target is the unqualified `public` table, show the public collection name; if a non-public collection is safely unique, show `schema.table`.
4. Duplicate-name guardrail banner: when the selected collection is non-public and another exposed collection has the same `name`, render the Synonyms tab as read-only with a warning that the current admin route is unqualified and cannot safely target this collection. Do not render save controls in this state.
5. Group list: show each synonym group as an editable row with term inputs, an add-term icon button, remove-term controls, and a remove-group control. Keep the groups visually separate from table records; synonym UI state belongs in the dedicated `ui/src/components/SynonymsEditor.tsx` path, not in `ui/src/components/TableBrowser.tsx`.
6. Empty state: when the collection has no synonym groups, show an empty panel with an `Add group` action and no record-grid chrome.
7. Editor controls: `Add group` creates a new draft group with two empty term fields. Add/remove/edit actions update draft state only until `Save` succeeds.
8. Save bar: show `Save` when editable drafts differ from the fetched groups. `Save` sends the complete replacement payload described by `docs-site/guide/synonyms.md:21-45`; successful saves replace local draft state with the normalized response.

## State contract

### Loading
- Keep the selected collection shell and `Synonyms` tab visible.
- The panel body shows a loading state while `GET /api/collections/{table}/synonyms` is in flight.
- Draft controls are not interactive until the first load succeeds.

### Error
- Fetch errors show an inline error banner with a `Retry` action.
- `Retry` repeats the same collection synonym fetch without changing the selected collection, current dashboard route, or selected-table tab.

### Empty
- A successful response with no groups renders the empty panel and `Add group`.
- Saving remains disabled until the user creates a valid draft group.

### Editing
- Users can add groups, remove groups, add terms, remove terms, and edit term text.
- Each group must keep at least two term fields before it can be saved.
- The editor trims whitespace for validation and mirrors the backend expectation that terms are normalized before storage.
- Draft synonym state lives in `ui/src/components/SynonymsEditor.tsx`, not in `ui/src/components/TableBrowser.tsx`, whose owner remains record browsing state.

### Validation error
- Client validation runs before save and blocks the request when a draft group has fewer than two non-empty terms, a duplicate normalized term, or a term over 128 characters.
- Validation errors are shown near the affected group or term and summarized near the save bar.

### Save in flight
- While `PUT /api/collections/{table}/synonyms` is in flight, disable add/remove/edit controls and show the save button as busy.
- The selected-table shell and tab strip stay interactive only for navigation away from the screen; no second save can start until the first request resolves.

### Save error
- Save errors preserve the current draft and show an inline error banner with the backend error text when available.
- A follow-up `Save` retries the full replacement payload currently visible in the editor.

### Duplicate term rejection
- Reject duplicate normalized terms across all groups, not only within one group.
- Normalization for duplicate checks is trim plus lowercase, matching `internal/server/search_synonyms_handler.go:119-131` and the integration cases at `internal/server/search_synonyms_handler_integration_test.go:255-256`.
- The UI error text must include the word `duplicate` so validation can assert the guardrail.

### Very-long term rejection
- Reject terms longer than 128 characters before save.
- This mirrors `maxSearchSynonymTermLength` in `internal/server/search_synonyms_handler.go:19-21` and the backend validation branch at `internal/server/search_synonyms_handler.go:127-129`.

### Duplicate collection-name read-only
- `docs-site/guide/synonyms.md:11` defines the admin route as accepting only an unqualified `{table}`. `internal/schema/schema.go:26-34` resolves `public.<table>` first and otherwise scans by unqualified name.
- Because that route cannot represent `schema.table`, the Synonyms tab is read-only for a selected non-public collection when any other exposed collection shares its table name.
- In this state, render the fetched state only if the selected collection is the same collection the unqualified route resolves to; otherwise render the warning and skip GET/PUT so the UI does not read or overwrite the wrong collection.

## Navigation

- Route: dashboard selected-collection shell; no new docs-site route is part of this stage.
- Entry: select a collection from the dashboard sidebar, then choose `Synonyms` in the selected-table tab strip.
- Back: browser and shell back behavior remain unchanged; the screen is not URL-state driven in this spec.
- Data: returns to the selected collection record browser through the existing `data` view.
- Schema: returns to the selected collection schema view through the existing `schema` view.
- SQL: returns to the selected collection SQL view through the existing `sql` view.
- Synonyms: stays on `Collection Synonyms` and refreshes only the synonym editor state for the currently selected collection.

## Acceptance criteria

- Given a selected collection, when the selected-table shell renders, then the tab strip includes `Synonyms` next to `Data`, `Schema`, and `SQL`. Evidence: `ui/src/components/__tests__/ContentRouter.test.tsx`; `ui/browser-tests-unmocked/full/collection_synonyms_editor.spec.ts`.
- Given the user opens `Synonyms`, when groups load successfully, then the editor displays the selected collection title context and the returned groups from `GET /api/collections/{table}/synonyms`. Evidence: `ui/src/components/__tests__/SynonymsEditor.test.tsx`; `ui/browser-tests-unmocked/full/collection_synonyms_editor.spec.ts`.
- Given a collection has no groups, when the screen loads, then the empty state shows `Add group` and no table-record grid. Evidence: `ui/src/components/__tests__/SynonymsEditor.test.tsx`.
- Given the user edits groups, when `Save` succeeds, then the UI sends a full replacement payload to `PUT /api/collections/{table}/synonyms` and replaces draft state with the normalized response. Evidence: `ui/src/components/__tests__/SynonymsEditor.test.tsx`; `ui/browser-tests-unmocked/full/collection_synonyms_editor.spec.ts`.
- Given duplicate normalized terms exist anywhere in the draft, when the user attempts to save, then no request is sent and the UI reports a duplicate validation error. Evidence: `ui/src/components/__tests__/SynonymsEditor.test.tsx`.
- Given a term longer than 128 characters exists in the draft, when the user attempts to save, then no request is sent and the UI reports the term length validation error. Evidence: `ui/src/components/__tests__/SynonymsEditor.test.tsx`.
- Given a selected non-public collection shares its table name with another exposed collection, when the user opens `Synonyms`, then the tab is read-only because the admin route cannot target the schema-qualified collection.
- Given screen-spec validation runs from the repo root, then this spec passes: `test -f docs/reference/screen_specs/collection_synonyms.md && rg -q "Synonyms" docs/reference/screen_specs/collection_synonyms.md && rg -q "duplicate" docs/reference/screen_specs/collection_synonyms.md && rg -q "PLAYWRIGHT_BASE_URL=http://localhost:8092" docs/reference/screen_specs/collection_synonyms.md && rg -q "AYB_SERVER_PORT=8092" docs/reference/screen_specs/collection_synonyms.md`.
- Given browser validation runs against the dashboard, then use the fixed local-server anchors `PLAYWRIGHT_BASE_URL=http://localhost:8092` and `AYB_SERVER_PORT=8092`.

## Edge cases

- No selected collection: keep the current selected-table empty state owned by `ui/src/components/ContentRouter.tsx:279-286`; no Synonyms editor is rendered.
- Missing schema cache: fetch failure is displayed through the error state with `Retry`.
- Unknown collection response: show the fetch error banner and keep the selected shell intact.
- Public/non-public duplicate names: public collections remain editable because the unqualified route resolves public first; non-public duplicates are read-only.
- Duplicate normalized terms: trim and lowercase before comparison so `SciFi`, ` scifi `, and `SCIFI` collide.
- Too few terms: a group with fewer than two non-empty terms cannot be saved.
- Too many terms: if the backend returns the existing max-term rejection, preserve the draft and show the save error.
- Very long terms: terms over 128 characters cannot be saved.
- Empty groups payload: do not send an empty replacement from the empty state.

## Open questions

- None. The selected-table view union includes `synonyms` alongside `data`, `schema`, and `sql` in `ui/src/components/layout-types.ts:8`; `ui/src/components/ContentRouter.tsx:207-280` routes `view === "synonyms"` to `SynonymsEditor` and renders the `Synonyms` tab button; `ui/src/components/SynonymsEditor.tsx` owns the dedicated editor surface.

The duplicate-name guardrail and validation rules above still describe the target contract this screen owns against the backend handler at `internal/server/search_synonyms_handler.go`.
