#!/usr/bin/env bash
set -euo pipefail

# Focused unit test for scripts/sdk_live_proof_seed.sh.
#
# The seed script is the single owner of the two deterministic SDK live specimens
# (the sdk_contract_add RPC function + the sdk-contract-echo edge function). This
# test asserts the two frozen payload builders emit the exact bytes Stages 3-6
# consume, and that the script fails closed when its admin credentials are absent
# or an admin deployment request fails.
#
# Payload builders are sourced (pure, side-effect-free thanks to the main-guard).
# Fail-closed behavior is top-level, so it is exercised via a subprocess run with
# stubbed env — mirroring scripts/demo_freshness_check_test.sh.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/sdk_live_proof_seed.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

# Source the script to expose its pure builder functions. The main-guard means
# this fires no admin calls and no env preconditions.
# shellcheck source=scripts/sdk_live_proof_seed.sh
source "$SCRIPT"

# (a) The RPC specimen SQL must be CREATE OR REPLACE FUNCTION-first so isDDL()
# (internal/server/sql_handler.go) reloads the schema cache: no leading comment,
# no DO $$ wrapper, CREATE as the very first token.
rpc_sql="$(build_rpc_function_sql)"
if [[ ! "$rpc_sql" =~ ^CREATE[[:space:]]+OR[[:space:]]+REPLACE[[:space:]]+FUNCTION[[:space:]]+sdk_contract_add\( ]]; then
  fail "RPC SQL is not CREATE OR REPLACE FUNCTION sdk_contract_add(-first:
$rpc_sql"
fi
if [[ "$rpc_sql" == *"DO \$\$"* || "$rpc_sql" == *"DO \$"* ]]; then
  fail "RPC SQL must not wrap the specimen in a DO \$\$ block:
$rpc_sql"
fi
first_token="$(printf '%s' "$rpc_sql" | awk 'NR==1{print $1}')"
if [[ "$first_token" != "CREATE" ]]; then
  fail "RPC SQL first token is '$first_token', want CREATE (isDDL requires it)"
fi
# The OUT-param specimen must name both output columns so /api/rpc returns a
# named-field record, and freeze the specimen marker string.
if [[ "$rpc_sql" != *"OUT sum int"* || "$rpc_sql" != *"OUT specimen text"* ]]; then
  fail "RPC SQL must declare OUT sum int and OUT specimen text:
$rpc_sql"
fi
if [[ "$rpc_sql" != *"specimen := 'sdk_contract_add'"* ]]; then
  fail "RPC SQL must set specimen := 'sdk_contract_add':
$rpc_sql"
fi
printf 'seed builder rpc_create_first: PASS\n'

# (b) The edge deploy JSON must name the specimen, be public, target the shared
# entry point, and carry a source that echoes request method + specimen marker.
edge_json="$(build_edge_echo_deploy_json)"
python3 - "$edge_json" <<'PY'
import json
import sys

body = json.loads(sys.argv[1])
assert body["name"] == "sdk-contract-echo", body.get("name")
assert body["public"] is True, body.get("public")
assert body["entry_point"] == "handler", body.get("entry_point")
source = body["source"]
assert "request.method" in source, "source must echo request.method"
assert '"sdk-contract-echo"' in source, "source must stamp the specimen marker"
assert "parsed.message" in source, "source must echo the caller's message"
print("seed builder edge_deploy_json: PASS")
PY

# (c) The script must fail closed (non-zero) when the admin-token file is missing
# or empty, or an admin call fails. Exercised as a subprocess because this is
# top-level main() behavior.
run_expect_failclosed() {
  local name="$1" want_text="$2"
  shift 2
  local output status
  set +e
  output="$(env "$@" bash "$SCRIPT" 2>&1)"
  status=$?
  set -e
  if (( status == 0 )); then
    fail "fail-closed case $name exited 0; expected non-zero
output:
$output"
  fi
  if [[ "$output" != *"$want_text"* ]]; then
    fail "fail-closed case $name output missing '$want_text'
output:
$output"
  fi
  printf 'seed fail-closed %s: PASS\n' "$name"
}

MISSING_TOKEN="$ROOT_DIR/scripts/.sdk_live_proof_seed_test_absent_token"
rm -f "$MISSING_TOKEN"
run_expect_failclosed "missing_token" "Admin token file is missing or empty" \
  "AYB_BASE_URL=http://127.0.0.1:1" "AYB_ADMIN_TOKEN_PATH=$MISSING_TOKEN"

EMPTY_TOKEN="$(mktemp)"
ADMIN_ERROR_TOKEN="$(mktemp)"
CURL_STUB_DIR="$(mktemp -d)"
trap 'rm -f "$EMPTY_TOKEN" "$ADMIN_ERROR_TOKEN" "$CURL_STUB_DIR/curl"; rmdir "$CURL_STUB_DIR"' EXIT
run_expect_failclosed "empty_token" "Admin token file is missing or empty" \
  "AYB_BASE_URL=http://127.0.0.1:1" "AYB_ADMIN_TOKEN_PATH=$EMPTY_TOKEN"

run_expect_failclosed "missing_base_url" "AYB_BASE_URL is required" \
  "AYB_BASE_URL=" "AYB_ADMIN_TOKEN_PATH=$MISSING_TOKEN"

printf 'test-admin-token' >"$ADMIN_ERROR_TOKEN"
cat >"$CURL_STUB_DIR/curl" <<'SH'
#!/usr/bin/env bash
last_arg=""
for arg in "$@"; do
  last_arg="$arg"
done

case "$last_arg" in
  */api/admin/sql)
    printf '{"ok":true}\nHTTP 200\n'
    ;;
  */api/admin/functions/by-name/*)
    exit 22
    ;;
  */api/admin/functions)
    printf 'simulated edge deployment failure\n' >&2
    exit 22
    ;;
  *)
    printf 'unexpected curl URL in test stub: %s\n' "$last_arg" >&2
    exit 2
    ;;
esac
SH
chmod +x "$CURL_STUB_DIR/curl"
run_expect_failclosed "admin_deployment_error" "simulated edge deployment failure" \
  "PATH=$CURL_STUB_DIR:$PATH" "AYB_BASE_URL=http://seed.test" \
  "AYB_ADMIN_TOKEN_PATH=$ADMIN_ERROR_TOKEN"

printf 'ALL sdk_live_proof_seed builder + fail-closed cases PASS\n'
