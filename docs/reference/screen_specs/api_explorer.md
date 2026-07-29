# API Explorer

## Task

Compose and send authenticated API requests from the admin UI, then inspect responses, snippets, and recent request history.

## Layout

1. Request header with `API Explorer` title and `History (<count>)` action.
2. Request builder with HTTP method selector, request path input, and `Send` action.
3. Schema-derived collection shortcut buttons when schema tables are available.
4. Collapsible `Query Parameters` controls.
5. Conditional `Request Body (JSON)` editor for `POST` and `PATCH`.
6. Optional recent request history panel.
7. Response area showing initial placeholder, error panel, or response details.
8. Snippet panel with `cURL`, `JS SDK`, and `Copy` actions after a response.
9. Response body panel.

## State contract

### Loading
- `API Explorer` has no initial data-load spinner; it renders the request builder immediately from the current schema cache.
- While a request is in flight, `Send` is disabled and shows `Sending...`.
- The previous response and error are cleared when a new request starts.

### Error
- When `executeApiExplorer` rejects, the response area shows a red error panel containing the error message.
- The response error panel includes a `Retry` action that reuses the existing request submit callback without clearing method, path, headers, params, or body controls.
- The user remains on `API Explorer` with the current method, path, body, query-parameter controls, and history state intact so the request can be corrected and sent again.

### Initial state
- Method defaults to `GET`.
- Path defaults to `/api/collections/`.
- The response area shows `Send a request to see the response`.
- History is hidden when there are no saved request-history entries.

### Request builder
- The HTTP method selector offers the supported explorer methods from `METHODS`.
- `Request path` accepts the API path to send.
- `Send` is disabled when the path is blank.
- `Cmd+Enter` or `Ctrl+Enter` sends the request from anywhere inside the explorer.
- Selecting a collection shortcut sets the path to `/api/collections/<table>` for public tables or `/api/collections/<schema>.<table>` for non-public tables.
- When more than 12 schema tables are available, only the first 12 shortcuts render and a `+<count> more` indicator summarizes the remainder.

### Query parameters
- `Query Parameters` expands or collapses the parameter grid.
- The grid includes `filter`, `sort`, `page`, `perPage`, `fields`, `expand`, and `search`.
- Non-empty query fields are appended to the request path when sending.
- Selecting a history entry restores query fields and expands the query-parameter grid when the saved request contains a query string.

### Request body
- The JSON body editor is visible only for `POST` and `PATCH`.
- The body is sent when non-empty and omitted when empty.

### Response panel
- A successful response shows `<status> <statusText>`, duration, response byte size, snippets, and formatted response body.
- `cURL` and `JS SDK` switch the snippet preview.
- `Copy` copies the active snippet and changes to `Copied` briefly.
- Response bodies that parse as JSON are pretty-formatted.

### History
- Successful requests are added to local request history with method, path including query string, body, status, duration, and timestamp.
- History keeps the newest matching method/path entry first and stores at most 20 entries.
- `History (<count>)` toggles the recent request panel.
- Selecting a recent request restores method, path, body, and query-parameter fields.
- `Clear` empties request history.

## Navigation

- Route: `/admin/` with the `API Explorer` admin sidebar item selected.
- Entry: Select `API Explorer` from the `Admin` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Sending requests, selecting history, toggling query parameters, and copying snippets: stay on `API Explorer`.

## Acceptance criteria

- Given the admin app is loaded, when the user selects `API Explorer`, then the `API Explorer` heading, HTTP method selector, request path input, `Query Parameters`, and `Send` action are visible. Evidence: `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.
- Given the query-parameter panel is collapsed, when the user opens `Query Parameters`, then `filter`, `sort`, `page`, `perPage`, `fields`, `expand`, and `search` controls are visible. Evidence: `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.
- Given `/api/admin/stats` is available, when the user sends `GET /api/admin/stats`, then the response panel shows `200 OK`, runtime body fields, and the actual `go_version` value returned by the API. Evidence: `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.
- Given a response is visible, when the user inspects snippets, then `cURL`, `JS SDK`, and `Copy` actions are visible. Evidence: `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.
- Given a request succeeds, when the user opens `History (1)`, then `Recent Requests` includes the `GET /api/admin/stats` entry with status `200`. Evidence: `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.
- Given a request is in flight, when the request builder renders, then `Send` is disabled and shows `Sending...`. Evidence: `ui/src/components/__tests__/ApiExplorer.test.tsx`.
- Given a request fails, when the response area renders, then the error panel shows the failure message, a `Retry` action, and the request controls remain editable. Evidence: `ui/src/components/__tests__/ApiExplorer.test.tsx`; `ui/browser-tests-unmocked/smoke/api-explorer-view.spec.ts`.

## Edge cases

- Admin stats endpoint unavailable in a test environment: existing browser proof skips only for explicit `503`, `404`, or `501` service-unavailable probes.
- Empty path: `Send` is disabled and no request is sent.
- Non-JSON response body: the response body panel shows the raw body text.
- Empty history: the history panel does not render even if toggled.
- Duplicate method/path requests: the newest request replaces the older matching history entry.

## Current implementation gaps

None verified.
