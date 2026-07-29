# AI Assistant

## Task

Inspect AI call logs and usage analytics, run interactive assistant queries, and manage reusable prompt templates.

## Layout

1. Header with `AI Assistant` heading and the `AI logs, usage analytics, interactive assistant, and prompt management` subtitle.
2. Tab bar with `Logs`, `Usage`, `Assistant`, and `Prompts`; `Logs` is selected by default.
3. Logs tab: a filter bar (`Provider`, `Status`, `From`, `To`, `Apply`, `Reset`) above a call-log table.
4. Usage tab: summary stat cards, an optional by-provider table, and a daily-usage table.
5. Assistant tab: mode buttons, a query input with a send action, and streaming/response output.
6. Prompts tab: a `Create Prompt` action, an inline editor, a prompts table, a template renderer, and version history.
7. Delete-prompt confirmation dialog.

## State contract

### Loading
- On first mount, while the active tab's data is loading and no logs, usage, or prompts have been fetched yet, the whole screen shows a centered spinner with `Loading AI data...`.

### Error
- When the active tab's fetch fails, the screen shows a centered error state with the returned message, or the tab-specific fallback (`Failed to load AI logs`, `Failed to load usage`, `Failed to load prompts`), plus a `Retry` action.
- `Retry` reloads only the active tab's data.

### Logs tab
- The call-log table columns are `Provider`, `Model`, `Tokens (in/out)`, `Cost`, `Duration`, `Status`, and `Time`.
- The filter bar offers `Provider` (`All providers`/`OpenAI`/`Anthropic`), `Status` (`All statuses`/`Success`/`Error`), and `From`/`To` date inputs.
- Editing a filter updates the draft only; the logs reload when `Apply` is clicked, and `Reset` restores the initial filters and reloads.
- Date filters are converted to local-day start/end RFC3339 boundaries before the request.
- When there are no logs, the table shows `No AI logs found`.

### Usage tab
- The summary shows stat cards for `Total Calls`, `Total Tokens`, `Input Tokens`, `Output Tokens`, `Total Cost`, and `Errors`.
- A `By Provider` table appears when per-provider usage exists.
- The `Daily Usage` table lists day/provider/model/calls/tokens/cost rows, or shows `No daily usage records` when empty.

### Assistant tab
- Mode buttons `SQL`, `RLS`, `Migration`, and `General` select the assistant mode; `SQL` is the default.
- The query input plus send action submit the trimmed query; send is disabled while a request is in flight or the query is empty.
- While streaming, a `Streaming` panel shows the accumulated streamed text.
- On completion, a response panel shows an optional `SQL` block and an `Explanation` (falling back to the response text or `No explanation provided.`).
- A request failure shows the error message in a red panel.

### Prompts tab
- `Create Prompt` opens an inline editor with `Prompt Name` and `Template` fields; `Save Prompt` is disabled until both are non-empty.
- `Edit` opens the same editor with the name disabled and `Update Prompt` disabled until the template is non-empty.
- The prompts table columns are `Name`, `Version`, `Model`, `Provider`, `Variables`, `Updated`, and `Actions` (`Edit`, `Versions`, `Render`, and a delete action).
- When no prompts exist, the table shows `No prompts configured`.
- The `Template Renderer` renders the selected prompt with the entered `Render Variables` JSON; invalid JSON shows a toast error and no render request is sent.
- `Version History` shows `Loading versions...`, a version list, or `No versions loaded`.

### Delete prompt confirmation
- The delete action opens a `Delete Prompt` dialog with the message `Delete prompt "<name>"? This action cannot be undone.` and a destructive `Delete` confirm plus `Cancel`.
- Confirming deletes the prompt, shows a success toast naming it, closes the dialog, and reloads the prompts.

## Navigation

- Route: `/admin/` with the `AI Assistant` sidebar item selected (view id `ai-assistant`).
- Entry: Select `AI Assistant` from the admin sidebar AI group.
- Back: Browser back follows the admin app history.
- Logs / Usage / Assistant / Prompts: switch tabs in place; switching a tab reloads that tab's data.

## Acceptance criteria

- Given AI call logs, when the user opens `AI Assistant`, then the logs table shows provider, model, tokens, cost, and status. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given the logs tab, when the user edits a filter, then the logs do not reload until `Apply` is clicked. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given the logs tab, when a date filter is applied, then it is converted to a local-day RFC3339 boundary in the request. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given the user switches to the usage tab, when it loads, then the usage summary is shown. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given the assistant tab, when the user submits a query, then the streamed assistant response is displayed. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given a seeded prompt, when the user opens the `Prompts` tab, then the seeded prompt row is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/ai-assistant.spec.ts`.
- Given the prompts tab, when the user creates, updates, views versions, and renders a prompt, then each action succeeds. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given a prompt row, when the user clicks the delete action, then the `Delete Prompt` confirmation dialog appears before deletion. Evidence owner: existing assertion in `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Given the active tab's request fails, when the error state renders, then the error message and a `Retry` action are visible and `Retry` reloads the active tab. Evidence owner: existing assertions in `ui/src/components/__tests__/AIAssistant.test.tsx`.

## Edge cases

- No logs: show `No AI logs found`.
- No prompts: show `No prompts configured`.
- No daily usage: show `No daily usage records`.
- No usage summary: the usage tab renders nothing until a summary is available.
- Invalid render-variables JSON: block the render and show a toast error.
- AI prompts service unavailable: the smoke and full prompt tests skip only for `501`, `404`, or `503` probe responses.
- Assistant request failure: show the error panel and keep the query editable.
- Prompt delete cancel: close the dialog and leave the prompt in the list.

## Current implementation gaps

- Current: The initial `Loading AI data...` state has no dedicated unmocked browser assertion; it is covered only by `ui/src/components/__tests__/AIAssistant.test.tsx`.
- Target: An unmocked probe could assert the loading state when a stable slow-response fixture is available without mocked routes.
- Evidence: `ui/src/components/AIAssistant.tsx`; `ui/src/components/__tests__/AIAssistant.test.tsx`.
