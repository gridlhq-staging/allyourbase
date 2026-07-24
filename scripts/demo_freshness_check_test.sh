#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/demo_freshness_check.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"$TMP_DIR/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

expected=(run list -R AllyourbaseHQ/allyourbase --workflow=cross_demo_live.yml --status completed --limit 1 --json conclusion,updatedAt)
if [[ "$*" != "${expected[*]}" ]]; then
  echo "unexpected gh args: $*" >&2
  exit 90
fi

case "${GH_STUB_MODE:-}" in
  fresh|edge_fresh)
    printf '[{"conclusion":"success","updatedAt":"%s"}]\n' "$GH_STUB_UPDATED_AT"
    ;;
  stale)
    printf '[{"conclusion":"success","updatedAt":"%s"}]\n' "$GH_STUB_UPDATED_AT"
    ;;
  non_success)
    printf '[{"conclusion":"failure","updatedAt":"%s"}]\n' "$GH_STUB_UPDATED_AT"
    ;;
  empty)
    printf '[]\n'
    ;;
  missing_updated_at)
    printf '[{"conclusion":"success"}]\n'
    ;;
  malformed_updated_at)
    printf '[{"conclusion":"success","updatedAt":"not-a-time"}]\n'
    ;;
  gh_failure)
    echo "stub gh failure" >&2
    exit 42
    ;;
  *)
    echo "unknown GH_STUB_MODE: ${GH_STUB_MODE:-}" >&2
    exit 91
    ;;
esac
SH
chmod +x "$TMP_DIR/gh"

read -r FRESH_AT EDGE_FRESH_AT STALE_AT < <(
  python3 - <<'PY'
from datetime import datetime, timedelta, timezone

now = datetime.now(timezone.utc).replace(microsecond=0)
print(
    (now - timedelta(hours=1)).isoformat().replace("+00:00", "Z"),
    (now - timedelta(hours=25, minutes=59)).isoformat().replace("+00:00", "Z"),
    (now - timedelta(hours=26, minutes=1)).isoformat().replace("+00:00", "Z"),
)
PY
)

run_case() {
  local name="$1"
  local mode="$2"
  local updated_at="$3"
  local want_status="$4"
  local want_text="$5"
  local output status

  set +e
  output="$(PATH="$TMP_DIR:$PATH" GH_STUB_MODE="$mode" GH_STUB_UPDATED_AT="$updated_at" bash "$SCRIPT" 2>&1)"
  status=$?
  set -e

  if [[ "$status" -ne "$want_status" ]]; then
    printf 'case %s: got status %s, want %s\noutput:\n%s\n' "$name" "$status" "$want_status" "$output" >&2
    exit 1
  fi
  if [[ "$output" != *"$want_text"* ]]; then
    printf 'case %s: output missing %q\noutput:\n%s\n' "$name" "$want_text" "$output" >&2
    exit 1
  fi
  printf 'demo freshness branch %s: PASS\n' "$name"
}

run_case "fresh_success" "fresh" "$FRESH_AT" 0 "fresh successful cross_demo_live.yml run"
run_case "edge_fresh_success" "edge_fresh" "$EDGE_FRESH_AT" 0 "fresh successful cross_demo_live.yml run"
run_case "stale_success" "stale" "$STALE_AT" 1 "stale successful cross_demo_live.yml run"
run_case "non_success" "non_success" "$FRESH_AT" 1 "latest completed cross_demo_live.yml run conclusion was failure"
run_case "empty_run_list" "empty" "$FRESH_AT" 1 "no completed cross_demo_live.yml runs found"
run_case "missing_updated_at" "missing_updated_at" "$FRESH_AT" 1 "missing updatedAt"
run_case "malformed_updated_at" "malformed_updated_at" "$FRESH_AT" 1 "malformed updatedAt"
run_case "gh_failure" "gh_failure" "$FRESH_AT" 1 "gh run list failed"
