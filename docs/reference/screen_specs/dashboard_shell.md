# Dashboard Shell

## Task

Navigate the authenticated dashboard shell by durable console URLs, choose table or admin views, and use Back, Forward, and refresh without losing shell state.

## Layout

1. Full-height authenticated shell with a fixed left sidebar and a flexible main pane.
2. Sidebar brand header: `Allyourbase`.
3. Command palette hint below the brand: `Search...` with a Cmd/Ctrl+K keyboard cue.
4. Tables section: `New Table` action, then either the sorted table list or the zero-table onboarding block.
5. Sidebar navigation sections in order: `Database`, `Services`, `Messaging`, `Admin`, `AI`, and `Auth`.
6. Sidebar action row: `Refresh schema`, theme toggle (`Switch to dark mode` or `Switch to light mode`), and `Log out`.
7. Main pane: selected-table tab shell when a table is active, selected admin-view content when sidebar navigation is active, or the no-selected-table empty state when no table exists.
8. Invalid or unavailable deep links replace the main pane with the visible route result defined under Navigation while keeping the shell usable.

## State contract

### Loading
- Before authentication and schema loading finish, the top-level admin shell shows centered `Loading...` and no sidebar controls.
- When a selected admin view is waiting for its lazy screen chunk, the authenticated shell keeps the sidebar and main pane mounted and the selected view area shows a status region named `Loading screen` with `Loading...`.

### Error
- If dashboard boot fails, the top-level admin shell shows `Connection Error`, the error message, and a `Retry` button that restarts boot.
- A malformed or unresolved shell pathname is not a boot error and must use the non-crashing visible route results under Navigation.
- The boot `Retry` button clears the current error, returns the top-level shell to `Loading...`, and runs a fresh admin-status/capabilities/schema boot request rather than reusing the failed request.
- If a lazy screen chunk fails after the authenticated shell is mounted, the dashboard shows the visible render-failure state `Something went wrong` with `Retry` and `Reload` actions.
- Lazy-screen `Retry` remounts the dashboard and issues a fresh-request Retry for a failed same-origin JavaScript chunk URL by appending `ayb_retry=<timestamp>` before re-importing it; retries for failures without a sanitized chunk URL rerun the original lazy loader.

### Table-selected shell
- When schema loading returns at least one table and the base URL has no deep link, `Layout` sorts tables by `schema.name` and selects the first sorted table by default.
- A valid table deep link selects the exact `schema` and `table` pair, even when it is not first in sort order.
- The active table appears in the sidebar table list and in the main-pane header.
- The selected-table header exposes `Data`, `Schema`, `SQL`, `Synonyms`, and `Search Settings` tabs; these tabs are local content state and do not change the pathname.
- Choosing any table, including from a deep link or `popstate`, opens its `Data` tab.

### Admin-view shell
- Choosing any sidebar admin navigation item clears the selected table and renders that admin view in the main pane.
- The selected admin navigation item is visually active.
- Choosing a table from the sidebar returns the main pane to the selected-table `Data` view.
- When runtime capabilities are known, any admin screen whose required capability is false is absent from the sidebar and command-palette `Navigation`; a direct URL for it uses the capability-hidden result under Navigation.
- When runtime capabilities are unknown because the capability endpoint is missing, unauthorized, malformed, or unreachable, the full registry remains visible and its screen URLs remain available.

### Zero-table shell
- At the base URL, the Tables section shows `No tables yet`, `Create your first table to get started.`, and an `Open SQL Editor` CTA.
- At the base URL, the main pane shows `Select a table from the sidebar` and `Use SQL Editor from the sidebar to create one.`
- Choosing `Open SQL Editor` navigates to the SQL Editor screen URL.

### Command palette
- The sidebar `Search...` hint opens a dialog named `Command palette`.
- Cmd/Ctrl+K toggles the same dialog.
- The dialog lists table commands first when tables exist and navigation commands under `Navigation`.
- Choosing a table command navigates to that table URL; choosing a navigation command navigates to that admin-screen URL.

## Navigation

### Current ownership and evidence

- The merge-target prerequisite is satisfied: `ui/src/screens/registry.ts` exists on `origin/main`. Its `ADMIN_VIEWS` and derived `AdminScreenId` define 49 stable IDs, `SCREEN_REGISTRY` owns metadata and rendering, `filterScreenRegistry` owns capability visibility, and `findAdminScreen` is the reusable lookup seam (`ui/src/screens/registry.ts:102-286`). The uniqueness, inventory equality, metadata, and filtering contracts are pinned in `ui/src/components/__tests__/screen_registry.test.ts:18-101`. No router may introduce another admin-screen list.
- `Layout` reads the server-emitted admin base, parses the current pathname through `parseDashboardRoute`, and sends sidebar and command-palette choices through one `navigate` callback. That callback formats the target through `formatDashboardRoute`, pushes exactly one history entry while preserving query/hash, and reapplies the route. A single `popstate` subscription restores Back/Forward state (`ui/src/components/Layout.tsx`, `ui/src/components/dashboard_url_routing.ts`). `Sidebar`, `CommandPalette`, and `ContentRouter` remain consumers of this owner rather than maintaining parallel route lists.
- `/oauth/authorize` remains a standalone short-circuit before `AdminDashboard` (`ui/src/App.tsx:45-50`) and is not a shell route.
- Query/hash ownership remains with each screen. `SchemaDesigner` owns `schemaTable` and composes pathname, query, and hash (`ui/src/components/SchemaDesigner.tsx:42-57`); `SecurityAdvisor` owns `secSeverity`, `secCategory`, and `secStatus` (`ui/src/components/SecurityAdvisor.tsx:10-26`); `PerformanceAdvisor` owns `perfRange` (`ui/src/components/PerformanceAdvisor.tsx:10-28`). Their `history.replaceState` updates must neither push entries nor be absorbed into shell routing.
- The server already supports hard-refresh deep links. `registerAdminSPA` mounts `/` and `/*` beneath normalized `cfg.Admin.Path`; `normalizedAdminPath` maps empty to `/admin`, strips trailing slashes, and preserves legal root `/` (`internal/server/helpers.go:107-141`). `staticSPAHandler` serves a real embedded asset or falls back to `serveEmbeddedIndexHTML`; that function applies `rewriteAdminIndexHTML` for the runtime base (`internal/server/middleware_spa.go:14-103`). `TestAdminSPAFallback` proves `/admin/some/deep/route` returns index HTML (`internal/server/middleware_test.go:253-268`). No additional server fallback route is part of this work.

### Routing decision

Shipped 2026-07-15: use the browser History API in `Layout`, with pure parse/format helpers at the URL-to-shell-state boundary.

| Option | Pathname parse/format and history | React fit and complexity | Incremental production JS |
| --- | --- | --- | --- |
| Direct History API (shipped) | Native `location.pathname`, `history.pushState`, and `popstate`; refresh is served by the SPA fallback. AYB supplies the small closed grammar below. | `Layout` owns one subscription; pure helpers map registry IDs and schema tables. No framework concepts beyond this seam. | Zero dependency bytes. |
| wouter `3.10.0` | Supplies React location/navigation and pattern matching but AYB must still define this grammar, validate registry IDs/tables, handle the runtime base, and define failure states. | Lightweight integration, but an extra abstraction and three runtime dependencies for only two flat route families. | 5,434 B minified / 2,598 B gzip. |
| react-router-dom `7.18.1` | Rich browser routing and navigation; its route tree can express the grammar, but runtime-base injection and AYB domain validation remain custom. | Mature React integration, but its route/data-router surface greatly exceeds this shell-state seam. | 194,229 B minified / 61,324 B gzip. |

This comparison covers only pathname parsing/formatting, `pushState`, `popstate`, refresh, React integration, and complexity. Nested routes, route guards, route-level data loaders, lazy loading/code splitting, and a search-parameter abstraction are explicitly excluded. Screen-owned query state remains unchanged.

Bundle evidence is reproducible without manifest changes. At HEAD `34a8147`, `cd ui && pnpm build` produced one JS asset of 1,395.00 kB raw / 377.93 kB gzip (plus CSS 45.97 kB / 8.02 kB gzip). On 2026-07-15, `npm view wouter version peerDependencies dependencies --json` returned `3.10.0` with runtime `mitt`, `regexparam`, and `use-sync-external-store`, while `npm view react-router-dom version peerDependencies dependencies --json` returned `7.18.1` with runtime `react-router@7.18.1`. Version-pinned Bundlephobia responses were [wouter 3.10.0](https://bundlephobia.com/api/size?package=wouter@3.10.0) and [react-router-dom 7.18.1](https://bundlephobia.com/api/size?package=react-router-dom@7.18.1). Their top-level `size`/`gzip` figures include the reported runtime dependency graph; peer React (and React DOM for react-router-dom) is listed separately and excluded. Against the current JS asset, wouter adds about 0.39% raw / 0.69% gzip and react-router-dom about 13.92% raw / 16.23% gzip. The browser primitives are specified by MDN's [`pushState`](https://developer.mozilla.org/en-US/docs/Web/API/History/pushState) and [`popstate`](https://developer.mozilla.org/en-US/docs/Web/API/Window/popstate_event) references; library integration references are the [wouter README](https://github.com/molefrog/wouter#readme) and [React Router declarative installation](https://reactrouter.com/start/declarative/installation).

### Runtime admin base decision

- Choose a server-emitted `<meta name="ayb-admin-base" content="...">` whose content is `adminPathWithTrailingSlash(adminPath)`, injected by `rewriteAdminIndexHTML`. The route seam reads this value before parsing the pathname. It must not derive the base from `window.location.pathname`, which is ambiguous (`/dashboard/users` can mean base `/dashboard` or legal base `/`).
- Rejected alternative: deriving from entry-module `import.meta.url` works today because `rewriteAdminIndexHTML` rewrites absolute Vite entry URLs under the configured base (`internal/server/middleware_spa.go:90-103`), but depends on the asset URL containing that base. Sibling lane `batman/jul14_pm_8_console_code_split_and_size_guards` is at baseline commit `bf295bf1` with no delta from `origin/main` as of 2026-07-15 and may adopt relative assets. Server metadata is independent of absolute-versus-relative asset output, so it is the compatibility contract for both lanes. If both lanes later touch `rewriteAdminIndexHTML`, they must preserve the meta injection and its tests during merge.
- `internal/server/middleware_spa.go` injects the normalized base and rewrites embedded asset paths; `internal/server/middleware_test.go` pins default, custom, and root emission. `readDashboardAdminBase` consumes that metadata, and the pure route helpers plus `Layout` own client parsing, formatting, navigation, and history restoration. The shipped-binary evidence below proves the same contract at default `/admin`, custom `/dashboard`, and legal root `/` without a second SPA fallback.
- A future cloud owner may prepend its own path before the emitted runtime admin base. The grammar below is relative to that emitted base and defines no organization/project routes now.

### Canonical pathname grammar

Let `B` be the emitted runtime base with exactly one trailing slash (`/admin/`, `/dashboard/`, or `/`). The grammar relative to `B` is closed:

```text
default       := B
admin screen  := B + "screens/" + encodeURIComponent(registry_screen_id)
selected table:= B + "tables/" + encodeURIComponent(schema) + "/" + encodeURIComponent(table)
```

- Examples: `/admin/screens/sql-editor`; public table `/admin/tables/public/users`; non-public table `/dashboard/tables/audit/events`; names containing delimiters remain one component, for example schema `tenant/east`, table `order history` becomes `/dashboard/tables/tenant%2Feast/order%20history`. Decode each component exactly once with `decodeURIComponent`; never decode the whole pathname before splitting.
- The `screens` and `tables` namespaces and exact segment counts prevent screen/table collisions. Admin-screen validity comes only from `findAdminScreen` over the registry; table validity comes from an exact decoded `schema` plus `name` match in the loaded schema.
- `B` is the canonical default and ends in `/` except root. Deep URLs never end in `/`. A successfully resolved equivalent with a trailing slash or non-canonical percent spelling is normalized with `replaceState`, preserving search and hash and adding no history entry. Invalid or unavailable targets are preserved as entered under the failure rules below.
- The base URL has no encoded selected-table identity by design: it preserves today's sorted-first-table default, or today's zero-table state. A later explicit table selection uses the table URL.

### Transitions, ownership, and closed failure rules

- Pathname is the source of truth for a deep link. Sidebar admin/table choices, command-palette choices, `New Table`, and `Open SQL Editor` format the target and call `pushState` exactly once, preserving the current search string and hash, then apply the same resolved shell state. Table selection always opens `Data`.
- After any in-shell push, the first Back stays in the console and restores the immediately previous base, screen, or exact `schema.table`; Forward restores the pushed target. `popstate` never pushes another entry. Refresh parses the same pathname and restores the same valid state after schema/capability boot.
- A base load performs no synthetic push or replace and retains current behavior: sorted-first table in `Data`, or the existing zero-table shell. Thus a first navigation from the base and its first Back are contained in the console.
- The routing seam owns only `window.location.pathname`. Every push, successful canonicalizing replace, and screen-owned replace preserves the current `window.location.search` and `window.location.hash`. Screen-owned `replaceState` calls create no entry. `/oauth/authorize` is checked before the shell/router mounts.
- Malformed percent encoding: preserve pathname/search/hash exactly and show `Invalid console link` with `Return to console` and `View guide` linking to `https://allyourbase.io/guide/getting-started`; keep sidebar and shell actions usable.
- Unknown registry screen ID: preserve the URL and show `Screen not found` with `Return to console` and `View guide` linking to `https://allyourbase.io/guide/getting-started`. Do not fall back silently to `users` and do not add the ID to another list.
- Missing decoded `schema.table`: preserve the URL and show `Table not found` with `Return to console` and `View guide` linking to `https://allyourbase.io/guide/getting-started`. Schema refresh re-resolves the current URL, so a subsequently available table opens without a history mutation.
- Capability-hidden screen: preserve the URL and show `Screen unavailable` with `Return to console` and `View guide` linking to `https://allyourbase.io/guide/getting-started`; do not render a different screen and do not reveal the hidden screen's content. Unknown capability state continues to use the full registry contract.
- Any other path shape (including extra or missing segments) is an unknown target: preserve it and show `Page not found` with `Return to console` and `View guide` linking to `https://allyourbase.io/guide/getting-started`. The return action is an ordinary in-shell push.

### Shipped binary acceptance evidence

Evidence captured 2026-07-15 from a fresh `cd ui && pnpm build` followed by `go build -o /tmp/ayb-stage4-routing ./cmd/ayb`; the focused runs used `scripts/run-with-ayb.sh` and the existing `ui/browser-tests-unmocked/smoke/dashboard-routing.spec.ts`, never Vite:

- **Yes — direct screen/table deep links, refresh, and assets:** the default `/admin` run passed 8/8 tests, including SQL Editor refresh and the encoded `tenant%2Feast.../order%20history...` table load and refresh.
- **Yes — Back/Forward and command-palette restoration:** the same run passed first-Back/Forward containment and repeated base → screen → table traversal with exact state restoration.
- **Yes — query/hash preservation:** `schemaTable`, `secSeverity`, `secCategory`, `secStatus`, and `perfRange`, each with a hash, survived shell navigation to SQL Editor unchanged.
- **Yes — OAuth isolation:** `/oauth/authorize` rendered standalone `Authorization Error` / `Missing required parameters` content with no dashboard sidebar and retained the OAuth pathname.
- **Yes — runtime admin bases:** the isolated `[admin] path = "/dashboard"` run passed 8/8; the isolated legal `[admin] path = "/"` run passed all 7 applicable tests with only the non-root-only case skipped. Both runs loaded embedded assets and refreshed the deepest encoded table URL.
- **Yes — base without trailing slash:** `/admin` and `/dashboard` loaded the shell and survived a hard refresh; root `/` has no distinct no-trailing-slash spelling.
- **No duplicate product routing was needed:** the binary matrix exposed only a test-data ownership race, fixed by making the query-preservation case create and clean up its own table. No Go server or Stage 3 route-seam product code changed.
- Repository entry-point confirmation: `make test-smoke` on isolated ports completed with 119 passed, 17 intentionally skipped, 1 retry that passed, and 0 unexpected failures.

## Acceptance criteria

- Given an authenticated base visit at `/admin/`, `/dashboard/`, or `/`, when schema boot completes, then the existing sorted-first-table `Data` default is selected; with zero tables, the existing onboarding and no-selected-table states remain visible, and no history entry is synthesized.
- Given any ID currently in `SCREEN_REGISTRY`, when its `B/screens/<encoded-id>` URL loads directly or refreshes, then exactly that visible, capability-allowed admin screen renders and the URL survives the round trip.
- Given schema `tenant/east` and table `order history`, when `B/tables/tenant%2Feast/order%20history` loads directly or refreshes, then that exact table opens on `Data`; this is the deepest required path proof.
- Given base, screen, or table state, when a user chooses sidebar navigation, a sidebar table, `New Table`, `Open SQL Editor`, or a command-palette item, then exactly one pathname-preserving-query/hash history entry is pushed and the correct screen/table renders.
- Given an in-shell navigation, when Back is pressed first, then the prior console state is restored without leaving the console; when Forward is pressed, then the exact pushed state returns. Repeated `popstate` traversal restores every exact screen or `schema.table` and adds no entries.
- Given an active route containing `?schemaTable=...`, `?secSeverity=...&secCategory=...&secStatus=...`, or `?perfRange=...` plus a hash, when shell navigation and screen-owned filter changes occur, then pathname routing preserves the query/hash and the screen's `replaceState` creates no navigation entry.
- Given `/oauth/authorize`, when `App` renders, then standalone OAuth consent renders before any console route parsing and the URL is not canonicalized by the shell.
- Given malformed encoding, an unknown screen ID, a missing table, a capability-hidden screen, or another unknown path shape, when boot or `popstate` resolves it, then the exact visible result and preserve behavior defined above occurs without a crash or silent fallback.
- Given any closed route failure renders, when the user inspects its recovery controls, then the existing failure label, `Return to console`, and `https://allyourbase.io/guide/getting-started` guide link are visible. Evidence owner: `ui/src/components/__tests__/ContentRouter.test.tsx`.
- Given a valid deep path with a trailing slash or non-canonical percent spelling, when it resolves, then `replaceState` produces the canonical path without changing query/hash or history length.
- Given the embedded production binary configured successively with `/admin`, `/dashboard`, and `/`, when the direct admin-screen and deepest table paths hard-refresh, then the server returns the SPA, assets load, and the emitted base drives the same parse/format results.

## Edge cases

- Auth required and no valid admin token: handled before this shell by the login screen; after login, the original same-origin pathname/search/hash must remain available for shell resolution.
- Empty user schema: the base keeps both existing zero-table surfaces; a table deep link shows `Table not found` rather than substituting the zero-table default.
- Schema refresh after DDL: newly created tables appear without a full page reload and the current pathname is re-resolved.
- Public and non-public tables use the same two-component table grammar; `public` is explicit in URLs even though the sidebar omits its visual prefix.
- Schema/table names may contain spaces, slashes, percent signs, Unicode, or strings equal to `screens`/`tables`; per-component percent encoding keeps them unambiguous. Empty schema or table components and decode failures are invalid links.
- Capability state may become more restrictive after initial render; re-resolving the current route produces `Screen unavailable` without changing its URL.
- Query parameters whose owners are not mounted remain preserved; shell routing does not interpret or discard them.
- Open questions: none. Nested routing, guards, loaders, lazy loading, search abstraction, and cloud prefixes are explicit non-goals rather than unresolved Stage 1 decisions.

## Current implementation gaps

- Current: Boot loading and boot error recovery are owned by `AdminDashboard`; lazy-screen loading and retryable imports are owned by the registry render seam; visible lazy-screen failures are owned by the top-level error boundary.
- Target: Keep those shell states explicit whenever dashboard routing, lazy chunk boundaries, or shell boot behavior change.
- Evidence: `ui/src/App.tsx`; `ui/src/screens/registry.ts`; `ui/src/components/ErrorBoundary.tsx`; `ui/src/components/__tests__/App.test.tsx`; `ui/src/components/__tests__/screen_registry.test.ts`; `ui/browser-tests-unmocked/smoke/admin-login.spec.ts`.
