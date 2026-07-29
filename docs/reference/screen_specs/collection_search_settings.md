# Collection Search Settings

## Task

Configure per-collection search relevance weights and ordered custom ranking from the dashboard table shell without changing backend, SDK, examples, OpenAPI, docs-site, or search-consumer surfaces.

## Layout

1. Selected collection shell: keep the existing selected-table header with the collection title and kind badge, owned by `ui/src/components/ContentRouter.tsx`.
2. Selected collection tab strip: render `Search Settings` beside the existing `Data`, `Schema`, `SQL`, and `Synonyms` table buttons. The view inventory owner is `ui/src/components/layout-types.ts`; selected-table rendering and tab button ownership stay in `ui/src/components/ContentRouter.tsx`.
3. Search settings panel header: show `Search settings for {label}` using the same label resolver as the shell. Public collections render the collection name; non-public collections render `schema.table`.
4. Duplicate-name guardrail banner: when the selected collection is non-public and another exposed collection has the same `name`, render the Search Settings tab as read-only with a warning that the current admin route is unqualified and cannot safely target this collection. Do not render save controls in this state. The guard owner is `ui/src/components/selected_collection_helpers.ts`.
5. Weighted attributes list: show each searchable text-column attribute as an editable row with a column selector, weight selector, and remove control. The supported weights are `high`, `medium`, `low`, and `lowest`.
6. Custom ranking list: show ordered custom-ranking rows from `ui/src/components/SearchCustomRankingEditor.tsx`; each row uses `{ column, order: "asc" | "desc" }` and preserves order as the saved secondary ranking chain.
7. Editor controls: `Add attribute` creates a new draft weighted attribute. `Add ranking` creates a new draft custom-ranking row. Add/remove/edit actions update draft state only until `Save` succeeds.
8. Save bar: show `Save` when editable drafts differ from the fetched settings. `Save` sends the complete replacement payload owned by `ui/src/api_admin.ts` and successful saves replace local draft state with the normalized response plus `Saved search settings.`.

## State contract

### Loading
- Keep the selected collection shell and `Search Settings` tab visible.
- The panel body shows a loading state while `GET /api/collections/{table}/search-settings` is in flight.
- Draft controls are not interactive until the first load succeeds.

### Error
- Fetch errors show an inline error banner with a `Retry` action.
- `Retry` repeats the same collection search-settings fetch without changing the selected collection, current dashboard route, or selected-table tab.

### Editing
- Users can add attributes, remove attributes, change attribute columns, and change attribute weights.
- Users can add custom-ranking rows, remove rows, change ranking columns, and change ranking order.
- Attribute selectors include searchable text columns only.
- Custom-ranking selectors include rankable columns only.
- Draft state lives in `ui/src/components/SearchSettingsEditor.tsx`, while custom-ranking row rendering lives in `ui/src/components/SearchCustomRankingEditor.tsx`.

### Validation error
- Client validation runs before save and blocks the request when no searchable attribute exists, attribute columns repeat, custom-ranking columns repeat, or a selected column is no longer eligible.
- Validation errors are shown near the save bar and preserve the invalid draft.

### Save in flight
- While `PUT /api/collections/{table}/search-settings` is in flight, disable add/remove/edit controls and show the save button as busy.
- No second save can start until the first request resolves.

### Save error
- Save errors preserve the current draft and show an inline error banner with the backend error text when available.
- A follow-up `Save` retries the full replacement payload currently visible in the editor.

### Duplicate collection-name read-only
- `GET /api/collections/{table}/search-settings` and `PUT /api/collections/{table}/search-settings` accept only an unqualified `{table}`.
- Because that route cannot represent `schema.table`, the Search Settings tab is read-only for a selected non-public collection when any other exposed collection shares its table name.
- In this state, render the warning and skip GET/PUT so the UI does not read or overwrite the wrong collection.

## Navigation

- Route: dashboard selected-collection shell; no new docs-site route is part of this stage.
- Entry: select a collection from the dashboard sidebar, then choose `Search Settings` in the selected-table tab strip.
- Back: browser and shell back behavior remain unchanged; the screen is not URL-state driven in this spec.
- Data: returns to the selected collection record browser through the existing `data` view.
- Schema: returns to the selected collection schema view through the existing `schema` view.
- SQL: returns to the selected collection SQL view through the existing `sql` view.
- Synonyms: returns to `Collection Synonyms` through the existing `synonyms` view.
- Search Settings: stays on `Collection Search Settings` and refreshes only the search-settings editor state for the currently selected collection.

## Acceptance criteria

- Given a selected collection, when the selected-table shell renders, then the tab strip includes `Search Settings` next to `Data`, `Schema`, `SQL`, and `Synonyms`.
- Given the user opens `Search Settings`, when settings load successfully, then the editor displays `Search settings for {label}` and the returned `attributes` plus ordered `customRanking` arrays from `GET /api/collections/{table}/search-settings`.
- Given seeded weighted attributes use all supported weights, when the screen loads, then the exact `high`, `medium`, `low`, and `lowest` combobox values are rendered.
- Given seeded custom ranking has multiple rows, when the screen loads, then row order, ranking columns, and `asc` or `desc` orders match the returned payload.
- Given the user edits weighted attributes and custom ranking, when `Save` succeeds, then the UI sends a full replacement payload to `PUT /api/collections/{table}/search-settings` and replaces draft state with the normalized response.
- Given the user reloads or reopens the collection after saving, when `Search Settings` loads again, then the persisted values still match the full replacement payload.
- Given a selected non-public collection shares its table name with another exposed collection, when the user opens `Search Settings`, then the tab is read-only because the admin route cannot target the schema-qualified collection.
- Given screen-spec validation runs from the repo root, then this spec passes: `test -f docs/reference/screen_specs/collection_search_settings.md && rg -q "Search settings" docs/reference/screen_specs/collection_search_settings.md && rg -q "custom ranking" docs/reference/screen_specs/collection_search_settings.md && rg -q "PLAYWRIGHT_BASE_URL=http://localhost:8094" docs/reference/screen_specs/collection_search_settings.md && rg -q "AYB_SERVER_PORT=8094" docs/reference/screen_specs/collection_search_settings.md`.
- Given browser validation runs against the dashboard, then use the fixed local-server anchors `PLAYWRIGHT_BASE_URL=http://localhost:8094` and `AYB_SERVER_PORT=8094`.

## Edge cases

- No selected collection: keep the current selected-table empty state owned by `ui/src/components/ContentRouter.tsx`; no Search Settings editor is rendered.
- Missing schema cache: fetch failure is displayed through the error state with `Retry`.
- Unknown collection response: show the fetch error banner and keep the selected shell intact.
- Public/non-public duplicate names: public collections remain editable because the unqualified route resolves public first; non-public duplicates are read-only.
- Duplicate searchable attributes: trim before comparison so repeated column selections are rejected before save.
- Duplicate custom-ranking columns: reject repeated ranking columns before save.
- No searchable attributes: do not send an empty replacement from the editor.
- Full replacement semantics: omitted draft rows are removed from the persisted collection settings after save.

## Open questions

- None. The selected-table view union includes `search-settings` in `ui/src/components/layout-types.ts`; `ui/src/components/ContentRouter.tsx` routes `view === "search-settings"` to `SearchSettingsEditor` and renders the `Search Settings` tab button; `ui/src/components/SearchSettingsEditor.tsx` owns the editor surface; `ui/src/components/SearchCustomRankingEditor.tsx` owns ordered custom-ranking rows; `ui/src/components/selected_collection_helpers.ts` owns the duplicate-name read-only guard.
