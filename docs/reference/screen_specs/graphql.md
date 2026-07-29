# GraphQL

## Task

Compose and execute GraphQL queries, inspect response envelopes, browse the live
schema, and recall recent queries.

## Layout

1. `GraphQL` heading with a `History (<count>)` action.
2. GraphQL query editor labelled `GraphQL query`.
3. `Variables (JSON)` editor labelled `GraphQL variables`.
4. `Send` action and `Cmd/Ctrl+Enter to send` keyboard hint.
5. `Schema` region with a `Load schema` action.
6. Optional recent-query history panel.
7. Response area showing the initial prompt, a recoverable error, or response
   status, duration, and formatted body.

## State contract

### Loading
- The screen renders the default example query immediately and does not require
  an initial data load.
- While a query is running, `Send` is disabled and changes to `Sending...`;
  stale response and error output are cleared.
- While introspection is running, `Load schema` is disabled and changes to
  `Loading schema...`.

### Error
- Malformed variables show `Variables must be valid JSON object text.` beside
  the variables editor without sending a request.
- A rejected query transport renders the shared recoverable error notice with
  the transport message, GraphQL guide link, and `Retry` action.
- Query retry preserves the current query and variables and sends them through
  the same execution path.
- Schema HTTP 404 renders `GraphQL is not enabled on this server`.
- Schema HTTP 403 or an introspection error envelope renders `Schema browsing
  requires admin access or is disabled`.
- Other schema transport failures render `Unable to load the GraphQL schema`;
  normal query controls remain usable.

### Initial state
- The query editor contains `query Example { __typename }`.
- Variables are blank, `History (0)` is visible, and the response area shows
  `Send a query to see the response`.
- `Send` is disabled only when the query is blank.

### Response
- Completed requests show `<status> <statusText>`, rounded duration in
  milliseconds, and the exact GraphQL response envelope as formatted JSON.
- GraphQL data and GraphQL errors remain together in the response envelope;
  application-level GraphQL errors are not misrepresented as transport alerts.
- Completed responses enter recent history; rejected transports do not.

### Schema
- `Load schema` executes the canonical introspection query.
- Successful introspection lists non-internal schema types, fields, argument
  types, default values, descriptions, and deprecation details.
- An introspection request never enters query history.
- An empty successful schema shows `No schema types available`.

### History
- `History (<count>)` toggles recent completed queries when at least one exists.
- Selecting a history entry restores its query and current-session variables,
  closes history, and does not execute automatically.
- Persisted history keeps query metadata but omits variables from local storage.
- `Clear` empties history.

## Navigation

- Route: `/admin/` with the `GraphQL` admin screen selected.
- Entry: Select `GraphQL` from the `Database` section of the admin sidebar.
- Back: Browser back follows the admin app history.
- Sending queries, loading schema, and selecting or clearing history: stay on
  `GraphQL`.

## Acceptance criteria

- Given the admin app is loaded, when the user selects `GraphQL`, then the
  heading, query editor, variables editor, `Send`, `History (0)`, schema region,
  and initial response prompt are visible. Evidence:
  `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`;
  `ui/src/components/__tests__/GraphqlExplorer.test.tsx`.
- Given GraphQL is enabled and a seeded table exists, when the user sends a
  query with variables, then the HTTP response is 200 and the exact seeded row
  is visible in the formatted response envelope. Evidence:
  `ui/browser-tests-unmocked/smoke/graphql-explorer.spec.ts`.
- Given a completed query, when the user opens history, then its query, status,
  and duration are visible and selecting it restores the editors. Evidence:
  `ui/browser-tests-unmocked/smoke/graphql-explorer.spec.ts`;
  `ui/src/components/__tests__/GraphqlExplorer.test.tsx`.
- Given introspection is available, when the user loads schema, then live query
  fields, arguments, and seeded table field types are visible. Evidence:
  `ui/browser-tests-unmocked/smoke/graphql-explorer.spec.ts`.
- Given invalid variables, when the user sends, then inline validation appears
  and no request is made. Evidence:
  `ui/src/components/__tests__/GraphqlExplorer.test.tsx`.
- Given the query transport is unreachable, when the user retries from the
  error notice after connectivity returns, then the current query succeeds and
  its editor contents remain intact. Evidence:
  `ui/browser-tests-unmocked/smoke/graphql-explorer.spec.ts`;
  `ui/src/components/__tests__/GraphqlExplorer.test.tsx`.
- Given schema browsing is disabled, forbidden, or unavailable, when the user
  loads schema, then the matching non-destructive degradation message is
  visible and normal query controls remain usable. Evidence:
  `ui/src/components/__tests__/GraphqlExplorer.test.tsx`.

## Edge cases

- The GraphQL endpoint is optional; a runtime without it still exposes the
  explorer and reports the explicit disabled-server schema state.
- Blank or whitespace-only queries cannot be sent.
- Variables must parse to a JSON object; arrays, scalars, null, and malformed
  JSON are rejected inline.
- Duplicate in-flight send or schema requests are ignored.
- A new request clears a stale response before rendering its own result.
- History retains at most the helper-owned limit and de-duplicates matching
  entries according to `insertGraphqlHistoryEntry`.

## Current implementation gaps

None verified.
