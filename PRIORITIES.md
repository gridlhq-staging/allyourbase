# Priorities

This file owns Allyourbase's strategic priority order. `ROADMAP.md` owns open
work, sequencing, and execution status; `PROJECT_OVERVIEW.md` owns durable
mission and scope.

## Current Order

1. **Ship one honest, trustworthy release — then launch.** **v0.0.20-beta shipped 2026-07-24** as the first release with verifiable
   full-SHA provenance (SHA `a66d0aa5b4f633da2d94e17469fd41903f44ec64`;
   `.goreleaser.yaml` `main.commit={{.FullCommit}}`, `fdaf30792`), verified from
   public artifacts — completing the honest-release half of this priority. The
   jul23_pm batch landed the pre-ship **claim⇄reality honesty pass** (the marketed
   surface no longer overclaims PostGIS/SAML/Firebase parity), and the follow-on
   jul24_9pm prelaunch-hardening batch closed three day-one-probe gaps (SAML
   assertion signature verification, CSP `Report-Only`, four-demo CI guard) now on
   `main` for the next tag. Face the deeper reality: a live traffic
   probe shows **0 stars / 0 forks / ~2 human views in 14 days** — AYB has never
   been launched. The product is mature; the bottleneck is distribution, and the
   launch itself (`_dev/launch/launch_runbook.md`) is a human act no agent can
   perform. Autonomous work does not wait on it — it prepares the artifact the
   human launches.
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
