# Cross-demo E2E

This package holds the top-level Playwright smoke suite for the demo apps. It
proves one real user roundtrip per demo:

- `kanban`: sign in, create a board flow, and persist a card.
- `live-polls`: sign in, create a poll, vote, and assert the exact count.
- `movies`: sign in, search for `inception`, and assert the first result is
  `Inception`.

Run it locally from the repo root with `make test-demo-e2e-all`, or directly
with `cd tests/e2e && AYB_BIN=$(cd ../.. && pwd)/ayb npx playwright test
--reporter=line cross_demo.spec.ts`.

This suite is additive to the per-demo Playwright packages under `examples/`.
Those stay responsible for demo-specific coverage; this package is the shared
regression net across the demo surface.
