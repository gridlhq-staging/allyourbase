# Allyourbase v0.0.7-beta

Release date: 2026-05-23

This beta packages shipped work landed since 2026-02-22, with emphasis on demo usability, SDK parity, release distribution reliability, and validation depth across high-churn server paths.

## Highlights

- Public demo surface is now live and verifiable end-to-end across API, landing, and app URLs.
- OAuth return routing supports per-request redirect targets with strict server-side validation.
- Auth helper parity expanded across non-JS SDKs, including anonymous sign-in and magic-link flows.
- Cross-demo Playwright coverage is wired into release validation paths and proven against deployed URLs.
- Distribution surfaces now consistently point to the canonical `griddlehq/allyourbase` identity.

## Reliability and Quality

- Added targeted thin-seam test hardening in realtime, server auth-session admin paths, and storage quota warning behavior.
- Extended auth refresh-session skew coverage to assert boundary behavior around expiration timestamps.
- Tightened storage fault and recovery confidence with focused coverage for transient failure and retry paths.
- Added managed Postgres outage recovery proof in pgmanager test harness flows.
- Improved schedule-boundary correctness validation for jobs endpoints and server scheduling handlers.
- Reduced code-quality debt in oversized files/functions while keeping existing runtime behavior intact.
- Preserved CI trust with root-cause fixes across toolchain/runtime seams and deterministic test pathways.

## Deployment and Distribution

- Completed the full demo deployment cycle through runtime alignment, hosting rollout, DNS routing, seed application, and live verification.
- Verified public endpoints for API and demo apps with green health/status signals.
- MAY20 GHCR closure remains the historical context. `MAY24-GHCR-REPUBLISH` on 2026-05-24T19:05Z republished `0.0.7-beta` + `latest` to anonymously pullable state (digest `sha256:514a9736…3a0d`); the `v0.0.7-beta` (v-prefixed) and `dev-c1facd7` tags remain 404 (never canonical / dev tag not republished by Strategy F).
- Kept release and installer surfaces aligned with canonical org/repo identifiers.
- Expanded workflow trigger coverage so docs/installer regressions surface automatically during normal release activity.

## SDK and Developer Experience

- JavaScript SDK: `signInWithOAuth` supports optional per-request `redirectTo` passthrough.
- Dart SDK: OAuth flow gained matching per-request redirect option.
- React SDK: auth hook typing and passthrough behavior aligned with JS SDK OAuth options.
- React/SSR/Dart/Python/Swift/Kotlin/Go SDKs: added parity for anonymous sign-in and magic-link helper coverage where supported by each SDK surface.
- Added and validated committed contract fixtures for key auth endpoints used by SDK helpers.
- Improved local and live demo E2E orchestration via env-driven URL fixtures for predictable cross-surface tests.

## Compatibility Notes

- Canonical project identity is now `griddlehq/allyourbase`.
- Docker references should use `ghcr.io/griddlehq/allyourbase`.
- Installer references should use `https://install.allyourbase.io/install.sh`.
- For migration compatibility only: legacy references like `gridlhq/allyourbase` should be treated as historical and replaced with canonical identifiers in active automation, docs, and scripts.
- Existing projects should audit pinned release scripts, package metadata, and mirror configuration for stale org/repo strings.

## Known Follow-up

- A deployed-only live-polls post-register transition regression remains open and is tracked in the active roadmap as current priority work.
- Local per-demo and cross-demo suites remain green; the open issue is isolated to the deployed live-polls signed-in transition.

## Upgrade Guidance

- If you are using OAuth flows, prefer per-request redirect targets instead of static callback assumptions.
- If you consume GHCR images, pull `ghcr.io/griddlehq/allyourbase:0.0.7-beta` (no v-prefix) or `:latest` — both are anonymously pullable as of 2026-05-24T19:05Z per `MAY24-GHCR-REPUBLISH` (digest `sha256:514a9736…3a0d`). Note the unprefixed tag form: `v0.0.7-beta` is 404.
- If you rely on installer bootstrap flows, confirm your environment still pulls from `install.allyourbase.io` and not cached legacy endpoints.

## Verification Snapshot

- Demo deployment validation includes public URL health checks and live E2E coverage.
- SDK parity updates were validated in their owner test suites and contract checks.
- Core reliability work was validated with focused owner tests and broader integration/CI passes.

## References

- Exhaustive shipped-work ledger: [roadmap/implemented.md](roadmap/implemented.md)
- Active roadmap context: [ROADMAP.md](ROADMAP.md)
- Current priorities: [PRIORITIES.md](PRIORITIES.md)

Full Changelog: [v0.0.6-beta...v0.0.7-beta](https://github.com/griddlehq/allyourbase/compare/v0.0.6-beta...v0.0.7-beta)
