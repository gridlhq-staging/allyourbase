# Priorities

This file owns Allyourbase's strategic priority order. `ROADMAP.md` owns open
work, sequencing, and execution status; `PROJECT_OVERVIEW.md` owns durable
mission and scope.

## Current Order

1. Keep core AYB server correctness stable across auth, API, realtime, storage,
   search, and operational runtime paths.
2. Preserve automated validation as the release gate, including the completed
   v0.0.10-beta GHCR runtime proof, the current allowlisted oversized Go-file
   baseline, and the function-size guardrail with no allowlisted oversized
   functions at HEAD.
3. Finish SDK and documentation parity gaps only when they are backed by tests,
   public docs, and implementation evidence.
4. Continue focused cleanup in active, high-churn areas without creating
   duplicate ownership or parallel docs.
