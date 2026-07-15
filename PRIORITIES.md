# Priorities

This file owns Allyourbase's strategic priority order. `ROADMAP.md` owns open
work, sequencing, and execution status; `PROJECT_OVERVIEW.md` owns durable
mission and scope.

## Current Order

1. **Ship what is already built.** Two full batches of user-facing work
   (passkeys dashboard UI, discoverable login across all six SDKs, credential-change
   notifications, HA/cell topology, demo deploy workflows) are merged to `main` and
   in **zero** users' hands. The last public release is v0.0.16-beta (2026-07-12);
   the pending beta release has failed twice. Merged is not shipped. Nothing else in this list
   creates value until the release pipeline works — see the release blocker in
   `ROADMAP.md` under Active. Building more while the shipping lane is broken just
   grows the unshipped pile.
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
