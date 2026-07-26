# Demo landing and instant trial

This Cloudflare Pages project serves `demo.allyourbase.io`. Its Pages Functions
provision private, temporary Allyourbase sandboxes through Daytona.

## Runtime configuration

`wrangler.toml` owns non-secret settings and the `TRY_STATE` KV binding. The
Cloudflare Pages project must have these production secrets:

- `DAYTONA_API_KEY`
- `TURNSTILE_SECRET_KEY`
- `TRY_RATE_LIMIT_SECRET`

The committed Turnstile sitekey is public by design. Never commit the three
runtime secret values.

Every trial uses one vCPU, 1 GiB memory, 5 GiB disk, a private signed preview,
and a 30-minute destruction TTL. The launcher also independently applies the
TTL after creation, limits the account to three active trial sandboxes, and
enforces a one-hour hashed client cooldown.

## Validation

Run unit and mocked browser contracts from the repository root:

```bash
node --test examples/apex_landing/tests/*.test.mjs
npm --prefix tests/e2e exec -- playwright test --config tests/e2e/try_allyourbase.config.ts
go test ./internal/codehealth -run '^TestApexLanding' -count=1
```

`tests/e2e/try_allyourbase_live.spec.ts` owns the costly live lifecycle check.
Run it only against a verification environment configured with Cloudflare's
official Turnstile testing keys; production intentionally rejects automated
browsers. The Daytona TTL remains the cleanup backstop.
