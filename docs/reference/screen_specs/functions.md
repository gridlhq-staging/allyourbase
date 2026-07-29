# Functions

## Task

Browse the PostgreSQL functions exposed by the database, inspect each function's parameters and return type, and execute a callable function with supplied arguments to see its result.

## Layout

1. Section heading `Functions (<count>)` showing the number of discovered functions.
2. Vertical list of function rows, each a toggle button showing the qualified name, parameter summary, and return type.
3. Expanded panel below the active row containing the function comment, parameter form or unnamed-parameter notice, `Execute` action, and result or error display.

## State contract

### Loading
- `FunctionBrowser` renders from the already-fetched admin schema cache (`schema.functions`); it performs no fetch of its own, so it has no independent loading spinner.
- While the schema cache is still loading, the Functions view is not yet mounted by the admin content router, and the row list appears only once the cache resolves.

### Error
- `FunctionBrowser` has no independent error state; schema-load failures are surfaced by the schema cache layer that owns the fetch, not by this view.
- Per-execution failures are shown inline in the expanded row's result area rather than replacing the list (see Execution result).

### Empty
- When the schema cache contains no functions, the view shows the single centered message `No functions found in the database.` and renders no list, heading, or execute controls.

### Populated list
- Functions are sorted by `<schema>.<name>` and each row is a button showing the name, a parenthesized parameter summary, and the return type.
- Non-`public` schemas are prefixed as `<schema>.` before the function name; `public` functions show the bare name.
- The parameter summary lists each parameter name, or `$<position>` for unnamed parameters.
- The return type reads `SETOF <returnType>` for set-returning functions and `<returnType>` otherwise.
- A collapsed row shows a right chevron; the active row shows a down chevron.

### Row expansion
- Clicking a row toggles it open; clicking the open row (or another row) closes it and resets any entered arguments and prior result.
- Only one row is expanded at a time.
- An expanded row shows the function comment when one exists.

### Parameter entry
- For a callable function with named parameters, the expanded row shows a `Parameters` group with one labeled text input per parameter, each label showing the parameter name and type.
- Each input placeholder is `NULL`; leaving an input blank omits that argument from the call.
- Pressing `Enter` inside a parameter input executes the function.
- A callable function with no parameters shows the `Execute` action and no `Parameters` group.

### Unnamed parameters
- When any parameter is unnamed, the expanded row shows the notice `This function has unnamed parameters and cannot be called via the REST API.` and renders no parameter inputs and no `Execute` action.

### Execution
- The `Execute` action calls the function through the REST RPC endpoint with the coerced argument values.
- While a call is in flight the action label reads `Executing...` and is disabled.

### Execution result
- A successful call shows a `Result` panel with the call duration in milliseconds and the JSON-formatted return value.
- A `204` response is labeled `Result (void)` and displays `(no return value)`.
- A failed call shows an `Error` panel containing the thrown error message.
- The result or error is scoped to the row that was executed and is cleared when a different row is toggled.

## Navigation

- Route: `/admin/` with the `Functions` sidebar item selected.
- Entry: Select `Functions` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Refresh schema: Re-running the schema refresh reloads the function list from the current database schema.

## Acceptance criteria

- Given a function is seeded into the `public` schema, when the user opens `Functions`, then the `Functions (<count>)` heading and the seeded function row are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/functions-list.spec.ts`.
- Given the seeded two-argument function is listed, when the user expands its row, then a parameter input (placeholder `NULL`) is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/functions-browser.spec.ts`.
- Given the expanded function has parameter inputs, when the user fills the arguments `3` and `5` and clicks `Execute`, then a `Result` panel is shown and the returned value `8` is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/functions-browser.spec.ts`.
- Given a function with an unnamed parameter is seeded, when the user expands its row, then the unnamed-parameter non-callable notice is shown and no `Execute` action is rendered. Evidence owner: assertion added in this stage in `ui/browser-tests-unmocked/full/functions-browser.spec.ts`.

## Edge cases

- Empty database: show `No functions found in the database.` with no list or controls.
- Unnamed parameters: show the non-callable notice and suppress the parameter form and execute action.
- No-argument function: show `Execute` with no `Parameters` group.
- Blank argument: omit the empty input from the call rather than sending an empty string.
- Void return: label the result `Result (void)` and show `(no return value)`.
- Execution failure: show the inline `Error` panel and keep the function list intact.

## Current implementation gaps

- Current: The unmocked browser specs do not exercise the empty-database message, schema-load error handling, void-return result labeling, or execution-failure display; those states are only covered by the mocked unit test `ui/src/components/__tests__/FunctionBrowser.test.tsx`.
- Target: Acceptance evidence should cover these visible states in an unmocked fixture when one can deterministically produce an empty schema, a schema-load failure, and a failing execution without adding mocked routes or a parallel harness.
- Evidence: `ui/src/components/FunctionBrowser.tsx`; `ui/src/components/__tests__/FunctionBrowser.test.tsx`.
