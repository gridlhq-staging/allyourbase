#!/usr/bin/env bash
set -euo pipefail

readonly CONTRACT_PATH="tests/contract/fixtures/sdk_contract/list_search_seed_contract.json"
readonly ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-${HOME}/.ayb/admin-token}"
readonly SEED_COLLECTION="${AYB_SDK_LIVE_PROOF_COLLECTION:-sdk_kotlin_search_posts}"
readonly SUPPORTED_SEED_COLLECTION="sdk_kotlin_search_posts"

run_admin_sql() {
  local query="$1"
  local payload
  payload="$(python3 -c 'import json,sys; print(json.dumps({"query": sys.argv[1]}))' "$query")"
  curl -fsS -w '\nHTTP %{http_code}\n' \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$ADMIN_SQL_URL"
}

build_insert_sql() {
  python3 -c '
import json
import sys

contract_path = sys.argv[1]
seed_collection = sys.argv[2]
with open(contract_path, encoding="utf-8") as f:
    contract = json.load(f)

facet_column = str(contract["facetColumn"])
expected_counts = contract["expectedFacetCounts"]
if seed_collection != "sdk_kotlin_search_posts":
    raise SystemExit(f"unsupported SDK live seed collection: {seed_collection}")
if facet_column != "category":
    raise SystemExit(f"unsupported facet column for posts seed: {facet_column}")
docs_categories = [name for name, count in expected_counts.items() if count == 2]
if len(docs_categories) != 1:
    raise SystemExit(f"expected one 2-row bucket and one 1-row bucket, got {expected_counts!r}")

rows = [
    ("one", str(contract["highlightedTitle"]), docs_categories[0]),
    ("two", str(contract["fuzzyMatchTitle"]), next(name for name, count in expected_counts.items() if count == 1)),
    ("three", "AllYourBase Go SDK reference", docs_categories[0]),
]
values = ", ".join(
    "(" + ", ".join(chr(39) + str(value).replace(chr(39), chr(39) * 2) + chr(39) for value in row) + ")"
    for row in rows
)
print(f"INSERT INTO {seed_collection} (id, title, category) VALUES {values}")
' "$CONTRACT_PATH" "$SEED_COLLECTION"
}

build_access_sql() {
  local table_name="$1"
  cat <<SQL
DO \$\$ BEGIN
  CREATE ROLE ayb_authenticated NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL;
END \$\$;
GRANT USAGE ON SCHEMA public TO ayb_authenticated;
ALTER TABLE ${table_name} ENABLE ROW LEVEL SECURITY;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE ${table_name} TO ayb_authenticated;
DROP POLICY IF EXISTS sdk_live_proof_select ON ${table_name};
CREATE POLICY sdk_live_proof_select ON ${table_name} FOR SELECT
  TO ayb_authenticated
  USING (true);
DROP POLICY IF EXISTS sdk_live_proof_insert ON ${table_name};
CREATE POLICY sdk_live_proof_insert ON ${table_name} FOR INSERT
  TO ayb_authenticated
  WITH CHECK (true);
DROP POLICY IF EXISTS sdk_live_proof_update ON ${table_name};
CREATE POLICY sdk_live_proof_update ON ${table_name} FOR UPDATE
  TO ayb_authenticated
  USING (true)
  WITH CHECK (true);
DROP POLICY IF EXISTS sdk_live_proof_delete ON ${table_name};
CREATE POLICY sdk_live_proof_delete ON ${table_name} FOR DELETE
  TO ayb_authenticated
  USING (true)
SQL
}

# build_rpc_function_sql emits the CREATE-first statement for the deterministic
# SDK contract RPC specimen. isDDL() (internal/server/sql_handler.go) only reloads
# the schema cache when CREATE|ALTER|DROP|... is the FIRST token, so this must stay
# CREATE-first: no leading comment, no DO $$ wrapper, or /api/rpc/{fn} 404s stale.
build_rpc_function_sql() {
  cat <<'SQL'
CREATE OR REPLACE FUNCTION sdk_contract_add(a int, b int, OUT sum int, OUT specimen text) AS $$
BEGIN
  sum := a + b;
  specimen := 'sdk_contract_add';
END;
$$ LANGUAGE plpgsql
SQL
}

# build_edge_echo_deploy_json emits the POST /api/admin/functions deploy body for
# the deterministic SDK contract edge specimen. The handler echoes the request
# method and a fixed specimen marker alongside the caller's message so the frozen
# invoke response is {"message":<in>,"method":"POST","specimen":"sdk-contract-echo"}.
build_edge_echo_deploy_json() {
  local source
  source='function handler(request) {
  var parsed = request.body ? JSON.parse(request.body) : {};
  return {
    statusCode: 200,
    headers: {"Content-Type": ["application/json"]},
    body: JSON.stringify({message: parsed.message, method: request.method, specimen: "sdk-contract-echo"})
  };
}'
  python3 -c 'import json,sys; print(json.dumps({"name": "sdk-contract-echo", "source": sys.argv[1], "entry_point": "handler", "public": True}))' "$source"
}

# deploy_edge_function idempotently creates or updates an edge function from a
# deploy JSON body: it looks the function up by name, PUT-updating when it already
# exists and POST-creating otherwise, so reseeding never trips the name conflict.
deploy_edge_function() {
  local deploy_json="$1"
  local name existing_id
  name="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["name"])' "$deploy_json")"
  existing_id="$(
    curl -fsS \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      "${ADMIN_FUNCTIONS_URL}/by-name/${name}" 2>/dev/null |
      python3 -c 'import json,sys
try:
    print(json.load(sys.stdin).get("id", ""))
except Exception:
    print("")' || true
  )"

  if [[ -n "$existing_id" ]]; then
    local update_json
    update_json="$(python3 -c 'import json,sys; body=json.loads(sys.argv[1]); body.pop("name", None); print(json.dumps(body))' "$deploy_json")"
    curl -fsS -w '\nHTTP %{http_code}\n' -X PUT \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "$update_json" \
      "${ADMIN_FUNCTIONS_URL}/${existing_id}"
  else
    curl -fsS -w '\nHTTP %{http_code}\n' \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "$deploy_json" \
      "$ADMIN_FUNCTIONS_URL"
  fi
}

# main runs the live seed: it validates env preconditions (fail-closed), resolves
# the admin bearer + endpoint URLs, then seeds the search collection plus the two
# deterministic SDK contract specimens. Guarded so `source`-ing this script exposes
# only the pure builder functions without firing any admin calls.
main() {
  if ! [[ "$SEED_COLLECTION" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "Invalid AYB_SDK_LIVE_PROOF_COLLECTION: $SEED_COLLECTION" >&2
    exit 1
  fi

  if [[ "$SEED_COLLECTION" != "$SUPPORTED_SEED_COLLECTION" ]]; then
    echo "Unsupported AYB_SDK_LIVE_PROOF_COLLECTION: $SEED_COLLECTION" >&2
    exit 1
  fi

  if [[ -z "${AYB_BASE_URL:-}" ]]; then
    echo "AYB_BASE_URL is required" >&2
    exit 1
  fi

  if [[ ! -s "$ADMIN_TOKEN_PATH" ]]; then
    echo "Admin token file is missing or empty: $ADMIN_TOKEN_PATH" >&2
    exit 1
  fi

  ADMIN_TOKEN="$(<"$ADMIN_TOKEN_PATH")"
  ADMIN_SQL_URL="${AYB_BASE_URL%/}/api/admin/sql"
  ADMIN_FUNCTIONS_URL="${AYB_BASE_URL%/}/api/admin/functions"
  readonly ADMIN_TOKEN ADMIN_SQL_URL ADMIN_FUNCTIONS_URL

  run_admin_sql "DROP TABLE IF EXISTS ${SEED_COLLECTION} CASCADE"
  run_admin_sql "CREATE TABLE ${SEED_COLLECTION} (id TEXT PRIMARY KEY, title TEXT NOT NULL, category TEXT NOT NULL)"
  run_admin_sql "$(build_access_sql "$SEED_COLLECTION")"
  run_admin_sql "$(build_insert_sql)"
  run_admin_sql "$(build_rpc_function_sql)"
  deploy_edge_function "$(build_edge_echo_deploy_json)"

  echo "SDK live-proof seed complete: collection=${SEED_COLLECTION} rpc=sdk_contract_add edge=sdk-contract-echo"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
