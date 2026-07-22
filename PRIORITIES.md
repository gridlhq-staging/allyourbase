# Priorities

This file owns Allyourbase's strategic priority order. `ROADMAP.md` owns open
work, sequencing, and execution status; `PROJECT_OVERVIEW.md` owns durable
mission and scope.

## Current Order

1. **Make the release process trustworthy, then launch.** The release runbook
   and gate-composition work now ship beta releases through documented gates,
   but v0.0.19-beta has unresolved public-artifact verification gaps: GitHub
   release metadata reports `isPrerelease=false`, and the installed archive
   binary reports short commit `c23cc95` instead of the full public SHA. Keep
   the discipline — published is not verified until the release-owner decisions
   in `ROADMAP.md` are resolved and the runbook's artifact verification is
   rerun. And face the deeper reality: a live traffic probe shows **0 stars / 0
   forks / ~2 human views in 14 days** — AYB has never been launched. The
   product is mature; the bottleneck is distribution, and the launch itself is
   a human act no agent can perform.
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
