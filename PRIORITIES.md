# Priorities

This file owns Allyourbase's strategic priority order. `ROADMAP.md` owns open
work, sequencing, and execution status; `PROJECT_OVERVIEW.md` owns durable
mission and scope.

## Current Order

1. **Make the release process trustworthy, then launch.** v0.0.17-beta **SHIPPED
   2026-07-15** (third attempt) — the two batches of merged-but-unshipped user-facing
   work (passkeys dashboard UI, discoverable login across all six SDKs, credential-change
   notifications, HA/cell topology, demo deploy workflows) are finally in users' hands.
   But the release *event* succeeded by hand-working around root causes the release
   *process* still hasn't fixed: the release-runbook + gate-composition lane
   (`jul14_pm_4`, Wave 4) is **not even dispatched**, so the next release will re-derive
   its rules from this one's scars. Land the four process-debt rows in `ROADMAP.md` →
   Active first (`release-runbook-ssot`, `release-gate-composition-owned`,
   `release-lane-salvage-path`, `gate-retro-check`). Keep the discipline — the new
   console-seam work is again merged-but-unreleased; merged is not shipped. And face the
   deeper reality: a live traffic probe shows **0 stars / 0 forks / ~2 human views in 14
   days** — AYB has never been launched. The product is mature; the bottleneck is
   distribution, and the launch itself is a human act no agent can perform.
2. Keep core AYB server correctness stable across auth, API, realtime, storage,
   search, and operational runtime paths.
3. Preserve automated validation as the release gate, and make sure the gate's
   *composition* is owned: the local gate omitted `test-sdk-integration`, so an
   SDK/migration drift reached staging before anything caught it, and a red `build`
   job silently masked the same suite in CI. The gate did its job when it ran — the
   failure was in what it was allowed to skip. Keep the
   current allowlisted oversized Go-file baseline, and the
   function-size guardrail has no allowlisted oversized functions at HEAD.
4. Finish SDK and documentation parity gaps only when they are backed by tests,
   public docs, and implementation evidence — **and only after they can actually ship.**
5. Continue focused cleanup in active, high-churn areas without creating
   duplicate ownership or parallel docs.
