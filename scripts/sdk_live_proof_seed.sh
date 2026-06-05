#!/usr/bin/env bash
set -euo pipefail

readonly CONTRACT_PATH="tests/contract/fixtures/sdk_contract/list_search_seed_contract.json"
readonly ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-${HOME}/.ayb/admin-token}"

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
with open(contract_path, encoding="utf-8") as f:
    contract = json.load(f)

facet_column = str(contract["facetColumn"])
expected_counts = contract["expectedFacetCounts"]
if facet_column != "category":
    raise SystemExit(f"unsupported facet column for posts seed: {facet_column}")
if len(expected_counts) != 2:
    raise SystemExit(f"expected exactly two facet buckets, got {len(expected_counts)}")

docs_categories = [name for name, count in expected_counts.items() if count == 2]
fruit_categories = [name for name, count in expected_counts.items() if count == 1]
if len(docs_categories) != 1 or len(fruit_categories) != 1:
    raise SystemExit(f"expected one 2-row bucket and one 1-row bucket, got {expected_counts!r}")

rows = [
    (contract["highlightedTitle"], docs_categories[0]),
    (f"{docs_categories[0]} filler", docs_categories[0]),
    (contract["fuzzyMatchTitle"], fruit_categories[0]),
]
values = ", ".join(
    "(" + ", ".join(chr(39) + str(value).replace(chr(39), chr(39) * 2) + chr(39) for value in row) + ")"
    for row in rows
)
print(f"INSERT INTO posts (title, category) VALUES {values}")
' "$CONTRACT_PATH"
}

run_admin_sql "DROP TABLE IF EXISTS posts CASCADE"
run_admin_sql "CREATE TABLE posts (id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL, category TEXT NOT NULL)"
run_admin_sql "$(build_insert_sql)"
