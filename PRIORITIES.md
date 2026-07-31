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
   `main` for the next tag. **Updated 2026-07-30:** the merged-but-unreleased
   window has since grown to six batches, and the all-green automation gate that
   made the last verdict a NO-GO is now **GREEN** at `d0fad4ae8` (`make
   test-everything` raw exit `0`, all nine `TEST SUMMARY` owners passing). The
   next release is therefore no longer gate-blocked — it is blocked on two small,
   autonomous, in-repo fixes plus the human cut. Both are named rows under
   `ROADMAP.md` Planned: `changelog-inventory-guard-anchor-jul30` (release-prep
   empties `[Unreleased]`, which turns a guard keyed off immutable historical
   closeouts permanently red — reproduced, not predicted) and
   `changelog-unreleased-sha-tokens-jul30`. Clearing those and cutting
   `v0.0.21-beta` is the shortest path from "mature product" to "released
   product", and it is the highest-value work available. Face the deeper reality: a live traffic
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
   failure was in what it was allowed to skip. **A second, distinct failure mode
   surfaced 2026-07-29 and now ranks alongside composition: a gate can run, be
   composed correctly, and still produce an unsound receipt.** The jul28_1pm L6
   dossier certified `fb6f291e7` on a real `make test-everything` exit `0`, but
   the repairs earned that verdict were never merged into the tree that was
   measured, and the one intermittent defect still present had already survived
   three consecutive green runs. **One control landed; the other is not yet a
   control.** Deterministic release-on-assertion gates did replace the
   fixed-delay sleeps in the movies and kanban suites, so those specific tests
   no longer race a wall-clock window — that is real and must stay, though it
   does not make the whole union a proof of absence. The repair-ancestry guard
   (`scripts/check-closeout-repair-ancestry.sh`,
   `internal/codehealth/closeout_repair_ancestry_test.go`) is **not** in force:
   it has no `Makefile` target, no CI job, and no aggregate; its tests exercise
   only synthetic `t.TempDir()` corpora; and run against the exact artifact
   that carried the withdrawn GO it returns `VACUOUS` with exit 0. Treat this
   failure mode as **open**, not closed —
   `repair-ancestry-guard-unwired-jul30` under `ROADMAP.md` Planned owns it.
   Keep the
   current allowlisted oversized Go-file baseline, and the
   function-size guardrail has no allowlisted oversized functions at HEAD.
4. Finish SDK and documentation parity gaps only when they are backed by tests,
   public docs, and implementation evidence — **and only after they can actually ship.**
5. Continue focused cleanup in active, high-churn areas without creating
   duplicate ownership or parallel docs.
