# Fly demo deployment

`deploy/fly/fly.toml` is the Fly app configuration owner for `allyourbase-demo`.
Every deploy and rollback command for this app must pass `--config deploy/fly/fly.toml`
so the `http_service`, `/health` check, and `allyourbase_demo_data` mount stay tied
to the repo-owned config.

`deploy/fly/ayb.toml` is the AYB runtime config baked into the container by the
root `Dockerfile` at `/home/ayb/ayb.toml`. The Stage 4 upgrade verified that
`ghcr.io/allyourbasehq/allyourbase:0.0.16-beta` carries the same config content.

## Release-image upgrade

From the repo root:

```bash
set -a
source /Users/stuart/.matt/secrets/ayb_demo.env
set +a
flyctl deploy -a allyourbase-demo --config deploy/fly/fly.toml --image ghcr.io/allyourbasehq/allyourbase:0.0.16-beta
```

After an image update, run the idempotent schema bootstrap. The Fly volume
currently preserves `/data/storage`; demo database tables must be verified and
recreated through the admin SQL API when the embedded database starts fresh.

```bash
set -a
source /Users/stuart/.matt/secrets/ayb_demo.env
set +a
bash deploy/fly/bootstrap_demo_schema.sh
```

## Rollback

Stage 4 captured this pre-upgrade rollback image:

```bash
set -a
source /Users/stuart/.matt/secrets/ayb_demo.env
set +a
flyctl deploy -a allyourbase-demo --config deploy/fly/fly.toml --image registry.fly.io/allyourbase-demo:deployment-01KS9QCBRKPWCEPCEM1T4NKXPY
```

Always refresh the rollback image with `flyctl image show -a allyourbase-demo`
immediately before a future deploy rather than reusing this historical value.

## Verification

```bash
set -a
source /Users/stuart/.matt/secrets/ayb_demo.env
set +a
flyctl image show -a allyourbase-demo
flyctl status -a allyourbase-demo
curl -fsS https://api.allyourbase.io/health
for url in https://api.allyourbase.io/health https://demo.allyourbase.io https://kanban.demo.allyourbase.io https://polls.demo.allyourbase.io https://movies.demo.allyourbase.io; do
  printf '%s ' "$url"
  curl -sS -o /dev/null -w '%{http_code} %{time_total}s\n' --max-time 20 "$url"
done
gh workflow run cross_demo_live.yml -R AllyourbaseHQ/allyourbase
# Dispatch is not acceptance — watch the dispatched run to terminal success:
gh run watch "$(gh run list -R AllyourbaseHQ/allyourbase --workflow cross_demo_live.yml --limit 1 --json databaseId --jq '.[0].databaseId')" -R AllyourbaseHQ/allyourbase --exit-status
```

Stage 4 evidence: `docs/live-state/20260713T035923Z_stage4_fly_upgrade.md`.
