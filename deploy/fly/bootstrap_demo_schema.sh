#!/bin/bash
set -euo pipefail

API_URL="${AYB_API_URL:-https://api.allyourbase.io}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# TODO: Document admin_token.
admin_token() {
  if [[ -z "${AYB_ADMIN_PASSWORD:-}" ]]; then
    if [[ -n "${AYB_ADMIN_TOKEN:-}" ]]; then
      printf '%s' "$AYB_ADMIN_TOKEN"
      return
    fi
    echo "AYB_ADMIN_TOKEN or AYB_ADMIN_PASSWORD is required" >&2
    return 1
  fi

  local auth_body auth_response auth_code
  auth_body="$(mktemp)"
  auth_response="$(mktemp)"

  python3 -c 'import json, os; print(json.dumps({"password": os.environ["AYB_ADMIN_PASSWORD"]}))' > "$auth_body"
  auth_code="$(
    curl -sS -o "$auth_response" -w '%{http_code}' --max-time 20 \
      -X POST "$API_URL/api/admin/auth" \
      -H 'content-type: application/json' \
      --data-binary "@$auth_body"
  )"
  if [[ "$auth_code" != "200" ]]; then
    echo "admin auth failed with HTTP $auth_code" >&2
    rm -f "$auth_body" "$auth_response"
    return 1
  fi
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("token", ""))' "$auth_response"
  rm -f "$auth_body" "$auth_response"
}

json_body_for_schema() {
  local schema_path="${1:?}"
  python3 -c 'import json, pathlib, sys; print(json.dumps({"query": pathlib.Path(sys.argv[1]).read_text()}))' "$schema_path"
}

# TODO: Document apply_schema.
apply_schema() {
  local token="${1:?}"
  local schema_path="${2:?}"
  local body response code
  body="$(mktemp)"
  response="$(mktemp)"

  json_body_for_schema "$schema_path" > "$body"
  code="$(
    curl -sS -o "$response" -w '%{http_code}' --max-time 40 \
      -X POST "$API_URL/api/admin/sql/" \
      -H 'content-type: application/json' \
      -H "authorization: Bearer $token" \
      --data-binary "@$body"
  )"
  if [[ "$code" != "200" ]]; then
    echo "$schema_path failed with HTTP $code" >&2
    python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("message", "unknown error"))' "$response" >&2 || true
    rm -f "$body" "$response"
    return 1
  fi
  printf 'applied %s\n' "${schema_path#"$REPO_ROOT"/}"
  rm -f "$body" "$response"
}

main() {
  local token
  token="$(admin_token)"
  if [[ -z "$token" ]]; then
    echo "admin auth returned an empty token" >&2
    return 1
  fi

  apply_schema "$token" "$REPO_ROOT/examples/kanban/schema.sql"
  apply_schema "$token" "$REPO_ROOT/examples/live-polls/schema.sql"
}

main "$@"
