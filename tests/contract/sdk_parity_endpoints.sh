#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF' >&2
Usage: tests/contract/sdk_parity_endpoints.sh [--verify-against-committed]
EOF
  exit 1
}

MODE="capture"
if [[ $# -gt 1 ]]; then
  usage
fi
if [[ $# -eq 1 ]]; then
  case "$1" in
    --verify-against-committed)
      MODE="verify"
      ;;
    *)
      usage
      ;;
  esac
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE_DIR="$ROOT_DIR/tests/contract/fixtures/sdk_parity"
TMP_DIR="$(mktemp -d)"
TMP_HOME="$TMP_DIR/home"
CAPTURE_DIR="$TMP_DIR/capture"
POST_HEALTH_SCRIPT="$TMP_DIR/post_health.sh"
mkdir -p "$TMP_HOME" "$CAPTURE_DIR"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [[ ! -x "$ROOT_DIR/ayb" ]]; then
  (
    cd "$ROOT_DIR"
    go build -o ayb ./cmd/ayb
  )
fi

cat >"$POST_HEALTH_SCRIPT" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

require_json_field() {
  local file_path="$1"
  local expression="$2"
  python3 - "$file_path" "$expression" <<'PY'
import json
import sys

path = sys.argv[1]
expression = sys.argv[2]
payload = json.load(open(path, "r", encoding="utf-8"))
parts = [part for part in expression.split(".") if part]
value = payload
for part in parts:
    if not isinstance(value, dict) or part not in value:
        raise SystemExit(f"missing JSON field: {expression}")
    value = value[part]
if value in ("", None):
    raise SystemExit(f"empty JSON field: {expression}")
if isinstance(value, bool):
    print("true" if value else "false")
else:
    print(value)
PY
}

assert_feature_flags() {
  local settings_file="$1"
  python3 - "$settings_file" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], "r", encoding="utf-8"))
if payload.get("anonymous_auth_enabled") is not True:
    raise SystemExit("anonymous_auth_enabled is false in /api/admin/auth-settings/")
if payload.get("magic_link_enabled") is not True:
    raise SystemExit("magic_link_enabled is false in /api/admin/auth-settings/")
PY
}

assert_fixture_semantics() {
  local file_path="$1"
  local expected_flag="$2"
  python3 - "$file_path" "$expected_flag" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], "r", encoding="utf-8"))
expected = sys.argv[2] == "true"
response = payload["response"]
if "token" not in response or "refreshToken" not in response:
    raise SystemExit("response is missing auth tokens")
user = response.get("user") or {}
actual = user.get("is_anonymous")
if expected:
    if actual is not True:
        raise SystemExit(f"expected user.is_anonymous=true, got {actual!r}")
else:
    if actual not in (False, None):
        raise SystemExit(f"expected non-anonymous response, got is_anonymous={actual!r}")
PY
}

admin_login() {
  local password="$1"
  local body
  body="$(python3 - "$password" <<'PY'
import json
import sys

print(json.dumps({"password": sys.argv[1]}))
PY
)"
  curl -fsS \
    -H "Content-Type: application/json" \
    --data "$body" \
    "${AYB_BASE_URL%/}/api/admin/auth" \
    >"$TMP_WORK_DIR/admin_auth.json"
  require_json_field "$TMP_WORK_DIR/admin_auth.json" "token"
}

admin_sql() {
  local token="$1"
  local query="$2"
  local body
  body="$(python3 - "$query" <<'PY'
import json
import sys

print(json.dumps({"query": sys.argv[1]}))
PY
)"
  curl -fsS \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$body" \
    "${AYB_BASE_URL%/}/api/admin/sql/" \
    >"$TMP_WORK_DIR/admin_sql.json"
}

json_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local auth_header="${4:-}"
  local output_file="$5"
  local status_file="$6"
  local -a args
  args=(
    -sS
    -o "$output_file"
    -w '%{http_code}'
    -X "$method"
    -H "Content-Type: application/json"
  )
  if [[ -n "$auth_header" ]]; then
    args+=(-H "Authorization: Bearer $auth_header")
  fi
  if [[ -n "$body" ]]; then
    args+=(--data "$body")
  fi
  curl "${args[@]}" "${AYB_BASE_URL%/}${path}" >"$status_file"
}

expect_status() {
  local status_file="$1"
  local expected="$2"
  local actual
  actual="$(tr -d '\n' <"$status_file")"
  if [[ "$actual" != "$expected" ]]; then
    echo "expected HTTP $expected, got $actual" >&2
    exit 1
  fi
}

write_fixture() {
  local request_json="$1"
  local response_file="$2"
  local destination="$3"
  python3 - "$request_json" "$response_file" "$destination" <<'PY'
import json
import sys

request_json, response_path, destination = sys.argv[1:4]
request_payload = json.loads(request_json)
response_payload = json.load(open(response_path, "r", encoding="utf-8"))
with open(destination, "w", encoding="utf-8") as fh:
    json.dump({"request": request_payload, "response": response_payload}, fh, indent=2, sort_keys=True)
    fh.write("\n")
PY
}

ADMIN_TOKEN="$(admin_login "$AYB_ADMIN_PASSWORD")"

curl -fsS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "${AYB_BASE_URL%/}/api/admin/auth-settings/" \
  >"$TMP_WORK_DIR/auth_settings.json"
assert_feature_flags "$TMP_WORK_DIR/auth_settings.json"

REGISTER_REQUEST='{"email":"fixture@example.com","password":"FixturePass123!"}'
json_request "POST" "/api/auth/register" "$REGISTER_REQUEST" "" "$TMP_WORK_DIR/register.json" "$TMP_WORK_DIR/register.status"
expect_status "$TMP_WORK_DIR/register.status" "201"

ANONYMOUS_REQUEST='{}'
json_request "POST" "/api/auth/anonymous" "$ANONYMOUS_REQUEST" "" "$TMP_WORK_DIR/anonymous_response.json" "$TMP_WORK_DIR/anonymous.status"
expect_status "$TMP_WORK_DIR/anonymous.status" "201"
write_fixture "$ANONYMOUS_REQUEST" "$TMP_WORK_DIR/anonymous_response.json" "$FIXTURE_OUTPUT_DIR/anonymous.json"
assert_fixture_semantics "$FIXTURE_OUTPUT_DIR/anonymous.json" "true"
ANONYMOUS_TOKEN="$(require_json_field "$TMP_WORK_DIR/anonymous_response.json" "token")"

MAGIC_LINK_REQUEST='{"email":"fixture@example.com"}'
json_request "POST" "/api/auth/magic-link" "$MAGIC_LINK_REQUEST" "" "$TMP_WORK_DIR/magic_link_request_response.json" "$TMP_WORK_DIR/magic_link_request.status"
expect_status "$TMP_WORK_DIR/magic_link_request.status" "200"

MAGIC_LINK_TOKEN="sdk-parity-magic-token"
MAGIC_LINK_HASH="$(printf '%s' "$MAGIC_LINK_TOKEN" | shasum -a 256 | awk '{print $1}')"
admin_sql "$ADMIN_TOKEN" "INSERT INTO _ayb_magic_links (email, token_hash, expires_at) VALUES ('fixture@example.com', '$MAGIC_LINK_HASH', NOW() + INTERVAL '10 minutes')"

MAGIC_LINK_CONFIRM_REQUEST='{"token":"sdk-parity-magic-token"}'
json_request "POST" "/api/auth/magic-link/confirm" "$MAGIC_LINK_CONFIRM_REQUEST" "" "$TMP_WORK_DIR/magic_link_confirm_response.json" "$TMP_WORK_DIR/magic_link_confirm.status"
expect_status "$TMP_WORK_DIR/magic_link_confirm.status" "200"
require_json_field "$TMP_WORK_DIR/magic_link_confirm_response.json" "token" >/dev/null
require_json_field "$TMP_WORK_DIR/magic_link_confirm_response.json" "refreshToken" >/dev/null

LINK_EMAIL_REQUEST='{"email":"upgraded@example.com","password":"LinkedPass123!"}'
json_request "POST" "/api/auth/link/email" "$LINK_EMAIL_REQUEST" "$ANONYMOUS_TOKEN" "$TMP_WORK_DIR/link_email_response.json" "$TMP_WORK_DIR/link_email.status"
expect_status "$TMP_WORK_DIR/link_email.status" "200"
write_fixture "$LINK_EMAIL_REQUEST" "$TMP_WORK_DIR/link_email_response.json" "$FIXTURE_OUTPUT_DIR/link_email.json"
assert_fixture_semantics "$FIXTURE_OUTPUT_DIR/link_email.json" "false"
EOF
chmod +x "$POST_HEALTH_SCRIPT"

export AYB_BASE_URL="${AYB_BASE_URL:-http://127.0.0.1:8090}"
export AYB_ADMIN_PASSWORD="${AYB_ADMIN_PASSWORD:-SdkParityAdminPass123!}"
export AYB_AUTH_ENABLED=true
export AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true
export AYB_AUTH_MAGIC_LINK_ENABLED=true
export AYB_AUTH_JWT_SECRET="${AYB_AUTH_JWT_SECRET:-sdk-parity-jwt-secret-that-is-at-least-32-bytes}"
export FIXTURE_OUTPUT_DIR="$CAPTURE_DIR"
export TMP_WORK_DIR="$TMP_DIR/work"
export AYB_START_LOG="$TMP_DIR/ayb-start.log"
mkdir -p "$TMP_WORK_DIR"

(
  cd "$ROOT_DIR"
  HOME="$TMP_HOME" bash scripts/run-with-ayb.sh "HOME=\"$TMP_HOME\" bash \"$POST_HEALTH_SCRIPT\""
)

for fixture_name in anonymous link_email; do
  if [[ ! -s "$CAPTURE_DIR/${fixture_name}.json" ]]; then
    echo "missing captured fixture: ${fixture_name}.json" >&2
    exit 1
  fi
done

normalize_fixture() {
  local path="$1"
  python3 - "$path" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path, "r", encoding="utf-8"))

timestamp_keys = {
    "createdAt",
    "created_at",
    "updatedAt",
    "updated_at",
    "linkedAt",
    "linked_at",
}

def scrub(value, path_parts):
    if isinstance(value, dict):
        result = {}
        for key, child in value.items():
            child_path = path_parts + [key]
            if child_path == ["response", "token"]:
                result[key] = "<token>"
                continue
            if child_path == ["response", "refreshToken"]:
                result[key] = "<refresh-token>"
                continue
            if child_path == ["response", "user", "id"]:
                result[key] = "<user-id>"
                continue
            if key in timestamp_keys:
                result[key] = "<timestamp>"
                continue
            result[key] = scrub(child, child_path)
        return result
    if isinstance(value, list):
        return [scrub(child, path_parts + ["[]"]) for child in value]
    return value

print(json.dumps(scrub(payload, []), indent=2, sort_keys=True))
PY
}

compare_fixture() {
  local committed="$1"
  local captured="$2"
  python3 - "$committed" "$captured" <<'PY'
import difflib
import json
import sys

committed = json.load(open(sys.argv[1], "r", encoding="utf-8"))
captured = json.load(open(sys.argv[2], "r", encoding="utf-8"))

timestamp_keys = {
    "createdAt",
    "created_at",
    "updatedAt",
    "updated_at",
    "linkedAt",
    "linked_at",
}

def scrub(value, path_parts):
    if isinstance(value, dict):
        result = {}
        for key, child in value.items():
            child_path = path_parts + [key]
            if child_path == ["response", "token"]:
                result[key] = "<token>"
                continue
            if child_path == ["response", "refreshToken"]:
                result[key] = "<refresh-token>"
                continue
            if child_path == ["response", "user", "id"]:
                result[key] = "<user-id>"
                continue
            if key in timestamp_keys:
                result[key] = "<timestamp>"
                continue
            result[key] = scrub(child, child_path)
        return result
    if isinstance(value, list):
        return [scrub(child, path_parts + ["[]"]) for child in value]
    return value

left = json.dumps(scrub(committed, []), indent=2, sort_keys=True).splitlines()
right = json.dumps(scrub(captured, []), indent=2, sort_keys=True).splitlines()
if left != right:
    for line in difflib.unified_diff(left, right, fromfile=sys.argv[1], tofile=sys.argv[2], lineterm=""):
        print(line)
    raise SystemExit(1)
PY
}

if [[ "$MODE" == "capture" ]]; then
  mkdir -p "$FIXTURE_DIR"
  cp "$CAPTURE_DIR"/*.json "$FIXTURE_DIR/"
  echo "Captured SDK parity fixtures into $FIXTURE_DIR"
  exit 0
fi

for fixture_name in anonymous link_email; do
  committed_path="$FIXTURE_DIR/${fixture_name}.json"
  captured_path="$CAPTURE_DIR/${fixture_name}.json"
  if [[ ! -f "$committed_path" ]]; then
    echo "missing committed fixture: $committed_path" >&2
    exit 1
  fi
  compare_fixture "$committed_path" "$captured_path"
done

echo "Verified SDK parity fixtures against committed contract files"
