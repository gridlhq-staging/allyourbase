#!/usr/bin/env bash
set -euo pipefail

readonly CONTRACT_PATH="tests/contract/fixtures/sdk_contract/list_search_seed_contract.json"
readonly ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-${HOME}/.ayb/admin-token}"
readonly SEED_COLLECTION="${AYB_SDK_LIVE_PROOF_COLLECTION:-sdk_kotlin_search_posts}"
readonly SUPPORTED_SEED_COLLECTION="sdk_kotlin_search_posts"

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

readonly ADMIN_TOKEN="$(<"$ADMIN_TOKEN_PATH")"
readonly ADMIN_SQL_URL="${AYB_BASE_URL%/}/api/admin/sql"

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

run_admin_sql "DROP TABLE IF EXISTS ${SEED_COLLECTION} CASCADE"
run_admin_sql "CREATE TABLE ${SEED_COLLECTION} (id TEXT PRIMARY KEY, title TEXT NOT NULL, category TEXT NOT NULL)"
run_admin_sql "$(build_access_sql "$SEED_COLLECTION")"
run_admin_sql "$(build_insert_sql)"
