# Browser Test Coverage Matrix

> Single source of truth for **view-level** browser-test coverage across all admin UI views.
> Consumed by Workstream 3 Stages 2-7. Do not maintain parallel coverage lists.
>
> Generated from Stage 1 audit. Source: `ui/src/components/layout-types.ts` (53 View literals),
> cross-checked against `ContentRouter.tsx` routes and `Sidebar.tsx` nav buttons.
>
> Last updated: 2026-03-24 (after MAR24 PM-1 reconciliation; deploy subflow row status lives in `_dev/AUDIT_LEDGER.md` R25-R31)

## How to read this matrix

| Column | Values | Meaning |
|---|---|---|
| **Smoke** | `none` / `heading-only` / `content-verified` | Quality of smoke spec in `browser-tests-unmocked/smoke/` |
| **Full Lifecycle** | `exists` / `not` | Whether a lifecycle/CRUD spec exists in `browser-tests-unmocked/full/` |
| **CRUD-capable** | `yes` / `N/A` | Whether the view has create/update/delete affordances |
| **Mocked** | `exists` / `not` | Whether a mocked spec exists in `browser-tests-mocked/` |

**Load-and-verify compliance** is derivable: a view is compliant if Smoke=`content-verified` OR Full Lifecycle=`exists`.

**Boundary note:** this matrix tracks view-level browser coverage only. Row-level audit status, evidence refs, pardons, and burndown ordering for `R01`-`R31` are owned by `_dev/AUDIT_LEDGER.md`.

**Cross-cutting specs** (`admin-login.spec.ts`, `dark-mode-persistence.spec.ts`, `theme-persistence.spec.ts`) are formally exempt from the one-row-per-view mapping — they test auth/theme flows that span multiple views.

**Multi-view specs** use a primary-view convention for counting. Secondary coverage is noted in the Evidence Specs column but doesn't add a separate matrix row.

## Coverage Matrix

| View | Smoke | Full Lifecycle | CRUD-capable | Mocked | Evidence Specs |
|---|---|---|---|---|---|
| `data` | content-verified | exists | yes | not | collections-create, table-browser-crud, collections-crud, table-browser-advanced, blog-platform-journey |
| `schema` | content-verified | not | N/A | not | schema-view |
| `sql` | content-verified | exists | yes | not | sql-view, sql-lifecycle |
| `synonyms` | content-verified | exists | yes | not | collection_synonyms_editor |
| `search-settings` | content-verified | exists | yes | not | collection_search_settings_editor |
| `webhooks` | content-verified | exists | yes | exists | webhooks-crud, webhooks-lifecycle, dashboard-webhook-delivery-journey (secondary), webhooks-error-flows (mocked) |
| `storage` | content-verified | exists | yes | exists | storage-upload, storage-lifecycle, storage-error-flows (mocked) |
| `sites` | content-verified | exists | yes | not | sites-hosting, sites-lifecycle (view-level Sites coverage; deploy subflow row status lives in `_dev/AUDIT_LEDGER.md` `R25`-`R31`) |
| `users` | content-verified | exists | yes | not | users-list, users-lifecycle |
| `functions` | content-verified | exists | N/A | not | functions-list, functions-browser |
| `edge-functions` | content-verified | exists | yes | exists | edge-functions-crud, edge-function-triggers, edge-functions (mocked) |
| `apps` | content-verified | exists | yes | exists | apps-list, apps-lifecycle, apps-toast-outcomes (mocked) |
| `api-keys` | content-verified | exists | yes | exists | api-keys-list, api-keys-lifecycle, api-keys-error-flows (mocked) |
| `oauth-clients` | content-verified | exists | yes | exists | oauth-clients-list, oauth-clients-lifecycle, oauth-clients-error-flows (mocked); row-level OAuth subflow status for `R02`/`R03` remains in `_dev/AUDIT_LEDGER.md` |
| `api-explorer` | content-verified | exists | N/A | not | api-explorer-view, api-explorer |
| `rls` | content-verified | exists | yes | not | rls-policies-view, rls-policies |
| `sql-editor` | content-verified | exists | yes | exists | admin-sql-query, create-table-nav, admin-dashboard-setup, sql-editor-lifecycle, sql-editor-error-flows (mocked) |
| `graphql` | content-verified | not | N/A | not | graphql-explorer |
| `schema-designer` | content-verified | exists | N/A | exists | schema-designer-table, schema-designer-lifecycle, schema-designer (mocked) |
| `sms-health` | content-verified | exists | N/A | not | sms-health, sms-dashboard (secondary) |
| `sms-messages` | content-verified | exists | yes | not | sms-messages, sms-dashboard |
| `email-templates` | content-verified | exists | yes | exists | email-templates-list, email-templates-lifecycle, email-templates-preview (mocked) |
| `push` | content-verified | exists | yes | exists | push-devices, push-notifications-lifecycle, push-notifications (mocked) |
| `jobs` | content-verified | exists | yes | not | jobs-list, jobs-management |
| `schedules` | content-verified | exists | yes | not | schedules-list, schedules-lifecycle |
| `matviews` | content-verified | exists | yes | not | matviews-list, matviews-lifecycle |
| `auth-settings` | content-verified | exists | yes | exists | auth-settings-view, auth-provider-management, auth-mfa-lifecycle; auth-provider-management (mocked) |
| `mfa-management` | content-verified | exists | yes | exists | mfa-management-view, auth-mfa-lifecycle; auth-mfa-error-flows (mocked) |
| `account-linking` | content-verified | exists | yes | exists | account-linking-view, auth-mfa-lifecycle; auth-mfa-error-flows (mocked, secondary) |
| `branches` | content-verified | exists | yes | not | branches-crud, branches-lifecycle |
| `realtime-inspector` | content-verified | exists | N/A | exists | realtime-inspector-view, realtime-inspector-lifecycle, realtime-inspector (mocked) |
| `security-advisor` | content-verified | exists | N/A | exists | security-advisor-view, advisors-lifecycle, security-advisor (mocked) |
| `performance-advisor` | content-verified | exists | N/A | exists | performance-advisor-view, advisors-lifecycle, performance-advisor (mocked) |
| `backups` | content-verified | exists | yes | not | backups, backups-lifecycle |
| `analytics` | content-verified | exists | N/A | not | analytics, analytics-lifecycle |
| `usage` | content-verified | not | N/A | exists | usage-metering, usage-metering (mocked) |
| `replicas` | content-verified | exists | yes | not | replicas, replicas-lifecycle |
| `ai-assistant` | content-verified | exists | yes | not | ai-assistant, ai-assistant-lifecycle |
| `audit-logs` | content-verified | exists | N/A | not | audit-logs, audit-logs-lifecycle |
| `admin-logs` | content-verified | not | N/A | exists | admin-logs, admin-logs (mocked) |
| `secrets` | content-verified | exists | yes | exists | secrets, secrets-lifecycle, secrets-error-flows (mocked) |
| `saml` | content-verified | exists | yes | not | saml, saml-lifecycle |
| `custom-domains` | content-verified | exists | yes | not | custom-domains, custom-domains-lifecycle |
| `extensions` | content-verified | exists | yes | not | extensions, extensions-lifecycle |
| `search` | content-verified | exists | N/A | not | search-playground-journey |
| `vector-indexes` | content-verified | exists | yes | not | vector-indexes, vector-indexes-lifecycle |
| `log-drains` | content-verified | exists | yes | not | log-drains, log-drains-lifecycle |
| `stats` | content-verified | not | N/A | not | stats |
| `auth-hooks` | content-verified | exists | yes | not | auth-hooks, auth-hooks-lifecycle |
| `notifications` | content-verified | exists | yes | exists | notifications, notifications-lifecycle, notifications-error-flows (mocked) |
| `fdw` | content-verified | exists | yes | exists | fdw, fdw-lifecycle, fdw-error-flows (mocked) |
| `incidents` | content-verified | exists | yes | exists | incidents, incidents-lifecycle, incidents-error-flows (mocked) |
| `support-tickets` | content-verified | exists | yes | exists | support-tickets, support-tickets-lifecycle, support-tickets-error-flows (mocked) |
| `tenants` | content-verified | exists | yes | exists | tenants, tenants-lifecycle, tenants (mocked) |
| `organizations` | content-verified | exists | yes | exists | organizations, organizations-lifecycle, organizations (mocked) |

## Gap Summary

| Metric | Count |
|---|---|
| Total views | 55 |
| Smoke = none | 0 |
| Smoke = heading-only | 0 |
| Smoke = content-verified | 55 |
| All views with smoke coverage | 55/55 (100%) |
| Views with full lifecycle specs | 50 |
| CRUD-capable views missing full lifecycle | 0 |
| Views missing mocked coverage | 30 |

## Admin degraded-state inventory

> Single source of truth for the **shipped** loading / empty / error / retry state
> of every admin screen. Exactly one row per ID in `ADMIN_VIEWS`
> (`ui/src/screens/registry.ts`); `scripts/check-coverage-matrix.sh` fails when the
> row set drifts from the registry, when a status leaves the controlled vocabulary,
> when the evidence entries disagree with the statuses, or when any referenced file
> does not exist. Every total below the table is derived from these rows — none of
> them are hand-entered.
>
> Path roots (omitted from cells): `Component` and `Evidence` resolve under
> `ui/src/components/`, `Screen spec` under `docs/reference/screen_specs/`, and
> `Unmocked proof` under `ui/browser-tests-unmocked/`.

Status vocabulary — exactly one of `present`, `missing`, `not-applicable`:

| State | `present` | `missing` | `not-applicable` |
|---|---|---|---|
| **Loading** | a distinct loading affordance renders while the primary request is in flight | the screen issues a primary request but renders no affordance | the screen issues no data request of its own |
| **Empty** | a dedicated zero-result message renders for the primary collection | the screen has a primary collection but no zero-result message | the primary surface is not a collection |
| **Error** | a primary-request failure renders a user-visible message | a primary-request failure renders nothing | the screen issues no data request of its own |
| **Retry** | the error surface offers a re-attempt control | no error-scoped re-attempt control | required whenever **Error** is `not-applicable` |

`Evidence` carries one `<state>=<path>:L<line>` entry for each state whose status is
`present`, and nothing else — a `missing` or `not-applicable` state must not carry
evidence. `Unmocked proof` records real-server browser proof of a degraded state and
counts only when the spec seeds or verifies real state and asserts a unique page-body
value; conditional `A.or(B)` fallbacks do not count. `Requires` mirrors the registry's
capability gate so capability-gated screens stay in the denominator.

**This section records shipped truth, not target behavior.** Target behavior is owned
by each screen spec's `## State contract`; verified deltas are owned by that spec's
`## Current implementation gaps` records. A `missing` row is a Stage 3 migration
candidate, not a gate failure.

| Screen | Component | Requires | Loading | Empty | Error | Retry | Evidence | Screen spec | Unmocked proof |
|---|---|---|---|---|---|---|---|---|---|
| `webhooks` | `Webhooks.tsx` | none | present | present | present | present | loading=Webhooks.tsx:L107; empty=Webhooks.tsx:L154; error=Webhooks.tsx:L121; retry=Webhooks.tsx:L129 | `webhooks.md` | none |
| `storage` | `StorageBrowser.tsx` | none | present | present | present | present | loading=StorageBrowser.tsx:L247; empty=StorageBrowser.tsx:L260; error=StorageBrowser.tsx:L225; retry=StorageBrowser.tsx:L228 | `storage.md` | empty=smoke/storage-upload.spec.ts; error=smoke/storage-upload.spec.ts |
| `sites` | `Sites.tsx` | none | present | present | present | present | loading=Sites.tsx:L521; empty=Sites.tsx:L520; error=Sites.tsx:L523; retry=Sites.tsx:L525 | `sites.md` | empty=smoke/sites-hosting.spec.ts; error=smoke/sites-hosting.spec.ts |
| `users` | `Users.tsx` | none | present | present | present | present | loading=Users.tsx:L75; empty=Users.tsx:L150; error=Users.tsx:L89; retry=Users.tsx:L97 | `users.md` | none |
| `functions` | `FunctionBrowser.tsx` | none | not-applicable | present | not-applicable | not-applicable | empty=FunctionBrowser.tsx:L71 | `functions.md` | none |
| `edge-functions` | `EdgeFunctions.tsx` | none | present | present | present | present | loading=edge-functions/FunctionList.tsx:L30; empty=edge-functions/FunctionList.tsx:L44; error=edge-functions/FunctionList.tsx:L38; retry=edge-functions/FunctionList.tsx:L38 | `edge_functions.md` | empty=smoke/edge-functions-crud.spec.ts; error=smoke/edge-functions-crud.spec.ts |
| `apps` | `Apps.tsx` | none | present | present | present | present | loading=Apps.tsx:L105; empty=Apps.tsx:L152; error=Apps.tsx:L119; retry=Apps.tsx:L127 | `apps.md` | none |
| `api-keys` | `ApiKeys.tsx` | none | present | present | present | present | loading=ApiKeys.tsx:L122; empty=ApiKeys.tsx:L169; error=ApiKeys.tsx:L136; retry=ApiKeys.tsx:L144 | `api_keys.md` | none |
| `oauth-clients` | `OAuthClients.tsx` | none | present | present | present | present | loading=OAuthClients.tsx:L132; empty=OAuthClients.tsx:L179; error=OAuthClients.tsx:L146; retry=OAuthClients.tsx:L154 | `oauth_clients.md` | none |
| `api-explorer` | `ApiExplorer.tsx` | none | present | not-applicable | present | present | loading=ApiExplorerRequest.tsx:L125; error=ApiExplorerResponse.tsx:L44; retry=ApiExplorerResponse.tsx:L47 | `api_explorer.md` | error=smoke/api-explorer-view.spec.ts |
| `rls` | `RlsPolicies.tsx` | none | present | present | present | present | loading=RlsPolicies.tsx:L256; empty=RlsPolicies.tsx:L328; error=RlsPolicies.tsx:L263; retry=RlsPolicies.tsx:L267 | `rls_policies.md` | none |
| `sql-editor` | `SqlEditor.tsx` | none | present | present | present | present | loading=SqlEditor.tsx:L333; empty=SqlEditor.tsx:L429; error=SqlEditor.tsx:L351; retry=SqlEditor.tsx:L354 | `sql_editor.md` | error=full/sql-editor-lifecycle.spec.ts |
| `graphql` | `GraphqlExplorer.tsx` | none | present | not-applicable | present | present | loading=GraphqlExplorerRequest.tsx:L100; error=GraphqlExplorerResponse.tsx:L22; retry=GraphqlExplorerResponse.tsx:L25 | none | error=smoke/graphql-explorer.spec.ts |
| `schema-designer` | `SchemaDesigner.tsx` | none | present | present | present | present | loading=SchemaDesigner.tsx:L76; empty=SchemaDesigner.tsx:L96; error=SchemaDesigner.tsx:L82; retry=SchemaDesigner.tsx:L85 | `schema_designer.md` | none |
| `sms-health` | `SMSHealth.tsx` | none | present | not-applicable | present | present | loading=SMSHealth.tsx:L60; error=SMSHealth.tsx:L74; retry=SMSHealth.tsx:L79 | `sms_health.md` | none |
| `sms-messages` | `SMSMessages.tsx` | none | present | present | present | present | loading=SMSMessages.tsx:L49; empty=SMSMessages.tsx:L75; error=SMSMessages.tsx:L63; retry=SMSMessages.tsx:L68 | `sms_messages.md` | none |
| `email-templates` | `EmailTemplates.tsx` | none | present | present | present | present | loading=EmailTemplates.tsx:L299; empty=EmailTemplates.tsx:L371; error=EmailTemplates.tsx:L313; retry=EmailTemplates.tsx:L321 | `email_templates.md` | none |
| `push` | `PushNotifications.tsx` | none | present | present | present | present | loading=PushNotifications.tsx:L245; empty=push-notifications/PushNotificationsDevicesTab.tsx:L79; error=PushNotifications.tsx:L259; retry=PushNotifications.tsx:L266 | `push_notifications.md` | none |
| `jobs` | `Jobs.tsx` | none | present | present | present | present | loading=Jobs.tsx:L160; empty=Jobs.tsx:L275; error=Jobs.tsx:L174; retry=Jobs.tsx:L182 | `jobs.md` | none |
| `schedules` | `Schedules.tsx` | none | present | present | present | present | loading=Schedules.tsx:L202; empty=Schedules.tsx:L249; error=Schedules.tsx:L216; retry=Schedules.tsx:L224 | `schedules.md` | none |
| `matviews` | `MatviewsAdmin.tsx` | none | present | present | present | present | loading=MatviewsAdmin.tsx:L163; empty=MatviewsAdmin.tsx:L210; error=MatviewsAdmin.tsx:L177; retry=MatviewsAdmin.tsx:L185 | `matviews.md` | none |
| `auth-settings` | `AuthSettings.tsx` | none | present | present | present | present | loading=AuthSettings.tsx:L246; empty=AuthSettingsProviders.tsx:L59; error=AuthSettings.tsx:L260; retry=AuthSettings.tsx:L262 | `auth_settings.md` | none |
| `mfa-management` | `MFAEnrollment.tsx` | none | present | present | present | present | loading=MFAEnrollment.tsx:L264; empty=MFAEnrollment.tsx:L272; error=MFAEnrollment.tsx:L240; retry=MFAEnrollment.tsx:L243 | `mfa_enrollment.md` | empty=smoke/mfa-management-view.spec.ts; error=smoke/mfa-management-view.spec.ts |
| `account-linking` | `AccountLinking.tsx` | none | not-applicable | not-applicable | not-applicable | not-applicable | none | `account_linking.md` | none |
| `branches` | `Branches.tsx` | none | present | present | present | present | loading=Branches.tsx:L91; empty=Branches.tsx:L125; error=Branches.tsx:L112; retry=Branches.tsx:L120 | `branches.md` | none |
| `realtime-inspector` | `RealtimeInspector.tsx` | none | present | present | present | present | loading=RealtimeInspector.tsx:L45; empty=RealtimeInspector.tsx:L71; error=RealtimeInspector.tsx:L39; retry=RealtimeInspector.tsx:L42 | `realtime_inspector.md` | empty=smoke/realtime-inspector-view.spec.ts; error=smoke/realtime-inspector-view.spec.ts |
| `security-advisor` | `SecurityAdvisor.tsx` | none | present | present | present | present | loading=SecurityAdvisor.tsx:L48; empty=SecurityAdvisor.tsx:L82; error=SecurityAdvisor.tsx:L50; retry=SecurityAdvisor.tsx:L53 | `security_advisor.md` | empty=smoke/security-advisor-view.spec.ts; error=smoke/security-advisor-view.spec.ts |
| `performance-advisor` | `PerformanceAdvisor.tsx` | none | present | present | present | present | loading=PerformanceAdvisor.tsx:L58; empty=PerformanceAdvisor.tsx:L69; error=PerformanceAdvisor.tsx:L60; retry=PerformanceAdvisor.tsx:L63 | `performance_advisor.md` | empty=smoke/performance-advisor-view.spec.ts; error=smoke/performance-advisor-view.spec.ts |
| `backups` | `Backups.tsx` | none | present | present | present | present | loading=Backups.tsx:L281; empty=Backups.tsx:L344; error=Backups.tsx:L295; retry=Backups.tsx:L300 | `backups.md` | none |
| `analytics` | `Analytics.tsx` | none | present | present | present | present | loading=Analytics.tsx:L412; empty=Analytics.tsx:L574; error=Analytics.tsx:L426; retry=Analytics.tsx:L438 | `analytics.md` | empty=smoke/analytics.spec.ts |
| `usage` | `UsageMetering.tsx` | none | present | present | present | present | loading=UsageMetering.tsx:L435; empty=UsageMeteringSections.tsx:L340; error=UsageMetering.tsx:L449; retry=UsageMetering.tsx:L454 | `usage_metering.md` | none |
| `replicas` | `Replicas.tsx` | none | present | present | present | present | loading=Replicas.tsx:L200; empty=Replicas.tsx:L389; error=Replicas.tsx:L214; retry=Replicas.tsx:L219 | `replicas.md` | none |
| `ai-assistant` | `AIAssistant.tsx` | none | present | present | present | present | loading=AIAssistant.tsx:L166; empty=ai/AILogsTab.tsx:L91; error=AIAssistant.tsx:L181; retry=AIAssistant.tsx:L186 | `ai_assistant.md` | none |
| `audit-logs` | `AuditLogs.tsx` | none | present | present | present | present | loading=AuditLogs.tsx:L119; empty=AuditLogs.tsx:L118; error=AuditLogs.tsx:L121; retry=AuditLogs.tsx:L123 | `audit_logs.md` | empty=smoke/audit-logs.spec.ts; error=smoke/audit-logs.spec.ts |
| `admin-logs` | `AdminLogs.tsx` | none | present | present | present | present | loading=AdminLogs.tsx:L407; empty=AdminLogs.tsx:L420; error=AdminLogs.tsx:L401; retry=AdminLogs.tsx:L404 | `admin_logs.md` | empty=smoke/admin-logs.spec.ts; error=smoke/admin-logs.spec.ts |
| `secrets` | `Secrets.tsx` | none | present | present | present | present | loading=Secrets.tsx:L227; empty=Secrets.tsx:L226; error=Secrets.tsx:L229; retry=Secrets.tsx:L231 | `secrets.md` | empty=smoke/secrets.spec.ts; error=smoke/secrets.spec.ts |
| `saml` | `SAMLConfig.tsx` | none | present | present | present | present | loading=SAMLConfig.tsx:L231; empty=SAMLConfig.tsx:L230; error=SAMLConfig.tsx:L233; retry=SAMLConfig.tsx:L235 | `saml_config.md` | empty=smoke/saml.spec.ts; error=smoke/saml.spec.ts |
| `custom-domains` | `CustomDomains.tsx` | none | present | present | present | present | loading=CustomDomains.tsx:L185; empty=CustomDomains.tsx:L184; error=CustomDomains.tsx:L187; retry=CustomDomains.tsx:L189 | `custom_domains.md` | empty=smoke/custom-domains.spec.ts; error=smoke/custom-domains.spec.ts |
| `extensions` | `Extensions.tsx` | none | present | present | present | present | loading=Extensions.tsx:L92; empty=Extensions.tsx:L91; error=Extensions.tsx:L94; retry=Extensions.tsx:L96 | `extensions.md` | empty=smoke/extensions.spec.ts; error=smoke/extensions.spec.ts |
| `search` | `Search.tsx` | none | present | present | present | present | loading=TableBrowserGrid.tsx:L130; empty=Search.tsx:L362; error=Search.tsx:L349; retry=Search.tsx:L350 | `search_playground.md` | empty=full/search-playground-journey.spec.ts |
| `vector-indexes` | `VectorIndexes.tsx` | none | present | present | present | present | loading=VectorIndexes.tsx:L161; empty=VectorIndexes.tsx:L160; error=VectorIndexes.tsx:L163; retry=VectorIndexes.tsx:L165 | `vector_indexes.md` | empty=smoke/vector-indexes.spec.ts; error=smoke/vector-indexes.spec.ts |
| `log-drains` | `LogDrains.tsx` | none | present | present | present | present | loading=LogDrains.tsx:L227; empty=LogDrains.tsx:L226; error=LogDrains.tsx:L229; retry=LogDrains.tsx:L231 | `log_drains.md` | empty=smoke/log-drains.spec.ts; error=smoke/log-drains.spec.ts |
| `stats` | `StatsOverview.tsx` | none | present | not-applicable | present | present | loading=StatsOverview.tsx:L68; error=StatsOverview.tsx:L53; retry=StatsOverview.tsx:L56 | `stats_overview.md` | error=smoke/stats.spec.ts |
| `auth-hooks` | `AuthHooks.tsx` | none | present | not-applicable | present | present | loading=AuthHooks.tsx:L51; error=AuthHooks.tsx:L38; retry=AuthHooks.tsx:L41 | `auth_hooks.md` | error=smoke/auth-hooks.spec.ts |
| `notifications` | `Notifications.tsx` | none | not-applicable | not-applicable | not-applicable | not-applicable | none | `notifications.md` | none |
| `fdw` | `FDWManagement.tsx` | none | present | present | present | present | loading=FDWManagement.tsx:L279; empty=FDWManagement.tsx:L278; error=FDWManagement.tsx:L281; retry=FDWManagement.tsx:L283 | `fdw.md` | empty=smoke/fdw.spec.ts; error=smoke/fdw.spec.ts |
| `incidents` | `Incidents.tsx` | status | present | present | present | present | loading=Incidents.tsx:L206; empty=Incidents.tsx:L205; error=Incidents.tsx:L208; retry=Incidents.tsx:L210 | `incidents.md` | empty=smoke/incidents.spec.ts; error=smoke/incidents.spec.ts |
| `support-tickets` | `SupportTickets.tsx` | support | present | present | present | present | loading=SupportTickets.tsx:L175; empty=SupportTickets.tsx:L174; error=SupportTickets.tsx:L177; retry=SupportTickets.tsx:L179 | `support_tickets.md` | empty=smoke/support-tickets.spec.ts; error=smoke/support-tickets.spec.ts |
| `tenants` | `Tenants.tsx` | none | present | present | present | present | loading=Tenants.tsx:L245; empty=Tenants.tsx:L369; error=Tenants.tsx:L256; retry=Tenants.tsx:L259 | `tenants.md` | empty=smoke/tenants.spec.ts; error=smoke/tenants.spec.ts |
| `organizations` | `Organizations.tsx` | none | present | present | present | present | loading=Organizations.tsx:L433; empty=Organizations.tsx:L355; error=Organizations.tsx:L444; retry=Organizations.tsx:L447 | `organizations.md` | empty=smoke/organizations.spec.ts; error=smoke/organizations.spec.ts |

### Reading the derived totals

`scripts/check-coverage-matrix.sh` prints `DEGRADED_STATE_INVENTORY:<rows>/<registry screens>`,
one `DEGRADED_STATE_<STATE>` line per degraded state, `DEGRADED_STATE_SCREEN_SPEC`, and
`DEGRADED_STATE_UNMOCKED_PROOF`. `internal/codehealth` owns the expected values, so a
silently flipped status cell fails `TestCheckCoverageMatrixScriptReportsDegradedStateInventoryTotals`.

Notable rows:

- `graphql` is the only screen with no paired screen spec — every other ID maps to one,
  including the deliberate renames `mfa-management` → `mfa_enrollment.md`,
  `saml` → `saml_config.md`, `stats` → `stats_overview.md`, `push` → `push_notifications.md`,
  `rls` → `rls_policies.md`, and `search` → `search_playground.md`.
- `account-linking` and `notifications` are pure mutation forms with no primary data
  request, so all four states are `not-applicable`. They are not Stage 3 migration targets.
- `incidents` (`requires: status`) and `support-tickets` (`requires: support`) are
  capability-gated but stay in the 50-screen denominator.
- Only two screens carry real-server degraded-state proof: `analytics` (filters the request
  log to a unique path and asserts `No request logs found`) and `search` (searches a
  misspelled term and asserts `No results matched this search`). The `tenants`,
  `organizations`, and `matviews` smoke specs assert a degraded state only inside an
  `.or()` fallback, so they do not count.

## Stage Gap Lists

### Stage 3 — Smoke specs needing rewrite (heading-only)

**COMPLETED.** All 18 previously heading-only admin smoke specs have been upgraded to content-verified (commits `4a491e6`, `d99c621`, `330fd97`). The orphan `admin-login.spec.ts` remains heading-only by design.

### Stage 4 — Admin views with no smoke spec

**COMPLETED.** All 54 views (49 admin + 5 data) now have smoke coverage. Admin views closed in commits `330fd97`–`be00895`; data views `schema`, `sql`, and `synonyms` closed in `2a2c6aa` plus `collection_synonyms_editor`, `search-settings` is covered by `collection_search_settings_editor`, and `search` is covered by `search-playground-journey`.

#### Stage 4 upgrade pass — 14 weak smoke specs upgraded to content-verified quality

**COMPLETED.** 14 specs that were nominally `content-verified` but still at chrome-only quality (asserting headings/buttons/labels without seeding data or verifying API state) have been upgraded to fixture-backed, deterministic content verification. Static validation passes (lint 0 errors, typecheck clean, `--list` 121 tests, hygiene 8/8). Runtime Playwright execution deferred to CI (local environment lacks pg_cron for managed Postgres startup).

**Data-seeded specs (6)** — seed deterministic data via API/SQL, assert seeded content renders, clean up in `afterEach`:

| Spec | Seed method | Assertion target | Cleanup |
|---|---|---|---|
| `functions-list` | `execSQL` (CREATE FUNCTION) | Function name in list | `execSQL` (DROP FUNCTION) |
| `email-templates-list` | `seedEmailTemplate` fixture | Template key button + subject input value | `cleanupEmailTemplate` fixture |
| `push-devices` | `seedPushDeviceToken` fixture | Token preview in Devices tab row | `cleanupPushTestData` fixture |
| `ai-assistant` | `seedAIPrompt` fixture | Prompt name in Prompts tab table | `cleanupAIPromptByName` fixture |
| `extensions` | `enableExtension` fixture | Extension row shows installed + Disable action | `disableExtension` fixture (conditional) |
| `vector-indexes` | `execSQL` (CREATE TABLE + INDEX) | Index name/schema/table/method in table row | `execSQL` (DROP TABLE) |

**Config-state specs (8)** — fetch live API state via fixture helper, assert rendered UI matches:

| Spec | Fixture read | Assertion target |
|---|---|---|
| `auth-hooks` | `fetchAuthHooksConfig` | 6 hook cards with per-key value text matching API state |
| `api-explorer-view` | `fetchAdminStatsSnapshot` | Response status `200 OK` + body keys (`uptime_seconds`, `go_version`) |
| `auth-settings-view` | `fetchAuthSettings` | 5 toggle checkboxes match API boolean state |
| `mfa-management-view` | `fetchAuthSettings` + `createLinkedEmailAuthSessionToken` | Enrollment buttons visible, TOTP flow transition/cancel cycle |
| `account-linking-view` | `fetchAuthSettings` | Anonymous session + email/password link form + link flow |
| `realtime-inspector-view` | `fetchRealtimeStats` | Metric card values match API snapshot totals |
| `security-advisor-view` | `fetchSecurityAdvisorReport` | Findings or empty-state consistent with API report |
| `performance-advisor-view` | `fetchPerformanceAdvisorReport` | Time-range selector + table-or-empty consistent with API report |

**New fixture helpers added** (all in `ui/browser-tests-unmocked/fixtures/`):

- `admin.ts`: `seedEmailTemplate`, `cleanupEmailTemplate`, `fetchAuthSettings`, `fetchRealtimeStats`, `fetchSecurityAdvisorReport`, `fetchPerformanceAdvisorReport`
- `auth.ts`: `createLinkedEmailAuthSessionToken` (linked email auth session for MFA spec)
- `core.ts`: `waitForDashboard(page)` (sidebar-based dashboard-ready gate replacing brittle brand-text checks)

**Component selector aids added**:

- `AuthHooks.tsx`: `data-testid="auth-hook-card-${key}"` and `data-testid="auth-hook-value-${key}"` per hook card
- `RealtimeInspector.tsx`: `data-testid` prop on `MetricCard` (`realtime-metric-total`, `-authenticated`, `-anonymous`, `-churn`) with `-value` suffix for value element

**Scope corrections documented for audit accuracy**:

- **`mfa-management`**: Enrollment flow verification only — `MFAEnrollment.tsx` renders TOTP/Email enrollment buttons unconditionally (does not read admin auth-settings), so spec asserts button visibility and enrollment step transitions, not config-coupled conditional rendering. Uses linked email auth token via `createLinkedEmailAuthSessionToken` because backend blocks MFA enrollment for anonymous users.
- **`account-linking`**: Email/password linking only — `AccountLinking.tsx` renders only anonymous session + email/password link form; `linkOAuth()` exists in `api_auth.ts` but is never imported by the component; no provider dropdown/selector exists. Spec skips when `anonymous_auth_enabled` is false.
- **Realtime inspector**: Frontend adapter `adaptRealtimeStatsToSnapshot` bridges backend `/api/admin/realtime/stats` payload shape (`connections/subscriptions/counters/version/timestamp`) to frontend `RealtimeInspectorSnapshot` shape (`summary/channels/throughput/degraded`). Endpoint URL aligned from stale `/realtime/inspector` to `/realtime/stats`.
- **Security/Performance advisors**: Backend route group `/api/admin/advisors/{security,performance}` added with admin-auth gating and deterministic report payload stubs (covered by `advisor_admin_handler_test.go`). These endpoints were previously missing from `internal/server` route wiring.
- **Vector indexes**: Frontend `listVectorIndexes()` in `api_vector.ts` now unwraps `{ indexes: [...] }` wrapped backend payload (covered by `api_vector_response_shape.test.ts`).

**Infrastructure improvements**:

- `waitForDashboard(page)` in `fixtures/core.ts` replaces inline `getByText("Allyourbase")` brand-text checks across all 14 specs — waits for sidebar `<aside>` element with 15s timeout, tolerating slow schema fetches under parallel test load.
- `auth.setup.ts` hardened to use token-based post-login readiness check (`localStorage.getItem("ayb_admin_token")`) instead of URL/sidebar navigation gating.
- 8 hygiene regression tests in `browserUnmockedHygiene.test.ts` enforce test patterns: no credential logging, MFA lifecycle cleanup, AAL2 step-up, token-based auth setup, `waitForDashboard` usage, fixtures barrel export, auth config seeding, and email template key format.

### Stage 5 — CRUD-capable views missing full-lifecycle coverage

**COMPLETED.** The remaining CRUD-capable gaps are now closed with dedicated unmocked full-lifecycle specs:
`sites` (`sites-lifecycle.spec.ts`), `tenants` (`tenants-lifecycle.spec.ts`), and
`organizations` (`organizations-lifecycle.spec.ts`).

This Stage 5 closeout is strictly view-level (`Full Lifecycle = exists`). It does not override narrower row-level subflow gaps tracked in `_dev/AUDIT_LEDGER.md` (for example `R02`, `R03`, and `R25`-`R31`).

The previously tracked Stage 5 closeout remains true for the original 22-row audit set: the final 5 closed there were `sql` (sql-lifecycle), `sql-editor` (sql-editor-lifecycle), `backups` (backups-lifecycle), `replicas` (replicas-lifecycle), and `auth-hooks` (auth-hooks-lifecycle).

### Stage 6 — Views missing mocked coverage

30 views remaining, matching the row-derived `Views missing mocked coverage` metric in
`## Gap Summary` (the previous "29" was hand-maintained and one behind the rows).
10 highest-priority mutation surfaces now covered:
`api-keys`, `oauth-clients`, `webhooks`, `storage`, `secrets`, `sql-editor`, `notifications`, `support-tickets`, `incidents`, `fdw`.

## Prioritization (product-surface-first)

1. ~~Close untested CRUD admin surfaces first~~ — **DONE**: all 51 views now have smoke coverage.
2. ~~Rewrite heading-only smoke on operationally critical pages~~ — **DONE**: all heading-only specs upgraded.
3. **Add mocked error-state tests** for high-risk mutation surfaces: `api-keys`, `oauth-clients`, `webhooks`, `storage`, `sql-editor`, `notifications`, `support-tickets`.
4. ~~Resolve mapping-policy for cross-cutting specs~~ — **DONE**: formal exemption documented.
5. ~~Add full-lifecycle coverage for remaining CRUD-capable views~~ — **DONE**: `sites`, `tenants`, and `organizations` now have dedicated full-lifecycle specs.
