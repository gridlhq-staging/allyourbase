# Security Advisor

## Task

Review security findings, filter by severity/category/status, and expand a finding for remediation guidance.

## Layout

1. Header with `Security Advisor` title.
2. Loading, error, or stale telemetry banner below the title when applicable.
3. Filter row with `Severity`, `Category`, and `Status` selects.
4. Empty-state text when no findings match the current filters.
5. Severity sections for critical, high, medium, and low findings, shown only when matching findings exist.
6. Expandable finding list items within each severity section.
7. Footer text `Last evaluated: <evaluatedAt>`.

## State contract

### Loading
- The panel remains mounted with test id `security-advisor-panel`.
- Before the first report is available, show `Loading security telemetry...`.
- Filters remain visible while loading.

### Error
- Request failure shows red inline text from `toPanelError(error)`.
- The error notice includes a `Retry` action that reruns the existing polling refresh while preserving the advisor panel shell and filters.
- Existing report data remains visible if a later poll fails.

### Filters
- `Severity` defaults to `all`, supports `critical`, `high`, `medium`, and `low`, and persists non-all values to URL query param `secSeverity`.
- `Category` defaults to `all`, is populated from report findings, and persists non-all values to `secCategory`.
- `Status` defaults to `all`, supports `open`, `accepted`, and `resolved`, and persists non-all values to `secStatus`.
- All filters apply together to the loaded findings.

### Findings
- When `data.stale` is true, show `Telemetry may be stale`.
- When no findings match, show `No findings for current filters.`.
- Matching findings are grouped in severity order: critical, high, medium, low.
- Each finding renders as a list item with a button labeled by the finding title.
- Clicking a finding toggles expanded details with description and remediation.
- Footer always shows `Last evaluated: <evaluatedAt>` or `Last evaluated: n/a`.

## Navigation

- Route: `/admin/` with admin view `security-advisor`.
- Entry: Select `Security Advisor` from the Admin section of the sidebar.
- Back: Browser back follows the admin app history.
- Filters: Stay on `Security Advisor` and update URL query params for non-all filter values.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Security Advisor`, then the `Security Advisor` heading and `security-advisor-panel` are visible. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/security-advisor-view.spec.ts`, `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`, and `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.
- Given a live security advisor report is available, when the screen renders, then `Severity`, `Category`, and `Status` filters are visible and the screen shows either severity findings or `No findings for current filters.`. Evidence owner: existing assertions in `ui/browser-tests-unmocked/smoke/security-advisor-view.spec.ts`.
- Given the report has findings, when the user filters by a finding severity, then the URL includes `secSeverity=<severity>` and the rendered finding count matches the report-filtered count. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the report has findings, when the user expands a finding, then the description and remediation from the live report are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the report has no findings, when the screen renders, then `No findings for current filters.` is visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
- Given the Security Advisor page is included in the admin accessibility sweep, when axe scans the page, then there are no critical or serious accessibility violations. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/accessibility.spec.ts`.

## Edge cases

- Advisor endpoint unavailable with status 404, 501, or 503: unmocked coverage may skip because the runtime surface is not available in that environment.
- Empty report: show `No findings for current filters.` and keep filters visible.
- No finding matches active filters: show `No findings for current filters.`.
- Stale telemetry: show `Telemetry may be stale` without hiding the report.
- Missing evaluation timestamp: show `Last evaluated: n/a`.
- Category list empty: keep only the `all` category option.

## Current implementation gaps

- Evidence: `ui/src/components/SecurityAdvisor.tsx`.
- Current: Unmocked browser coverage proves live content, severity/status filtering, empty state, error retry recovery, and accessibility, but does not prove loading, stale banner, category/status URL persistence, detail collapse, or all severity section ordering.
- Target: Acceptance evidence should cover those visible states when deterministic unmocked advisor fixtures can exercise them without adding mocked-only coverage or a parallel test file.
- Evidence: `ui/browser-tests-unmocked/smoke/security-advisor-view.spec.ts`; `ui/browser-tests-unmocked/full/advisors-lifecycle.spec.ts`.
