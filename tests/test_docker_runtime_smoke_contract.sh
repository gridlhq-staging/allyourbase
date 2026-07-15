#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/bash_assert_helpers.sh"

smoke_script="scripts/docker-runtime-smoke.sh"
[[ -f "$smoke_script" ]] || fail "missing ${smoke_script}"

fetch_before_block="$(sed -n '/^fetch_code=/,/^require_http 200 .*storage fetch before restart/p' "$smoke_script")"
fetch_after_block="$(sed -n '/^fetch_after_code=/,/^require_http 200 .*storage fetch after restart/p' "$smoke_script")"

if ! grep -Fq -- '-H "Authorization: Bearer ${login_token}"' <<<"$fetch_before_block"; then
  fail "storage fetch before restart must authenticate with the login token"
fi

if ! grep -Fq -- '-H "Authorization: Bearer ${relogin_token}"' <<<"$fetch_after_block"; then
  fail "storage fetch after restart must authenticate with the refreshed login token"
fi

restart_block="$(sed -n '/^"\$DOCKER_BIN" restart/,/^echo "restart health: ok"/p' "$smoke_script")"
if ! grep -Fq -- 'resolve_base_url' <<<"$restart_block"; then
  fail "container restart must refresh the ephemeral host port before polling health"
fi

echo "PASS: Docker runtime smoke authenticates storage fetches and refreshes restart port"
