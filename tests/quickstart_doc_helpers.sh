# Shared helper functions for quickstart doc snippet execution and assertions.

run_prepared_script() {
  local script_file="$1"
  local stdout_file="$2"
  local stderr_file="$3"
  local failure_message="$4"

  if ! bash "$script_file" >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2 || true
    fail "$failure_message"
  fi
}

run_extracted_bash_block() {
  local label="$1"
  local doc_file="$2"
  local heading="$3"
  local ordinal="$4"
  local stdout_file="$5"
  local stderr_file="$6"
  local command_file="$TMP_DIR/${label}.sh"

  extract_doc_block "$doc_file" "$heading" bash "$ordinal" | rewrite_doc_command >"$command_file"
  run_prepared_script "$command_file" "$stdout_file" "$stderr_file" \
    "documented ${label} command failed"
}

run_extracted_bash_block_with_curl_status() {
  local label="$1"
  local doc_file="$2"
  local heading="$3"
  local ordinal="$4"
  local stdout_file="$5"
  local stderr_file="$6"
  local command_file="$TMP_DIR/${label}.sh"
  local wrapper_file="$TMP_DIR/${label}_with_status.sh"

  extract_doc_block "$doc_file" "$heading" bash "$ordinal" | rewrite_doc_command >"$command_file"
  {
    printf '%s\n' 'set -euo pipefail'
    printf '%s\n' 'curl() { command curl -w '"'"'\n__AYB_HTTP_CODE:%{http_code}\n'"'"' "$@"; }'
    printf '. %q\n' "$command_file"
  } >"$wrapper_file"
  run_prepared_script "$wrapper_file" "$stdout_file" "$stderr_file" \
    "documented ${label} command failed"
}

# Single owner of how this harness invokes `ayb sql`; callers differ only in where
# the SQL text comes from and which failure message identifies the caller.
invoke_ayb_sql() {
  local sql="$1"
  local stdout_file="$2"
  local stderr_file="$3"
  local failure_message="$4"
  shift 4

  if ! AYB_SERVER_PORT="$API_PORT" "$AYB_BIN" sql "$sql" "$@" >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2 || true
    fail "$failure_message"
  fi
}

run_extracted_sql_block() {
  local label="$1"
  local doc_file="$2"
  local heading="$3"
  local ordinal="$4"
  local stdout_file="$5"
  local stderr_file="$6"
  local sql_file="$TMP_DIR/${label}.sql"

  extract_doc_block "$doc_file" "$heading" sql "$ordinal" >"$sql_file"
  invoke_ayb_sql "$(cat "$sql_file")" "$stdout_file" "$stderr_file" \
    "documented ${label} SQL block failed"
}

run_harness_sql() {
  local label="$1"
  local sql="$2"

  invoke_ayb_sql "$sql" "$TMP_DIR/${label}.stdout" "$TMP_DIR/${label}.stderr" \
    "harness SQL ${label} failed"
}

extract_http_json_response() {
  local response_file="$1"
  local expected_http_code="$2"
  local label="$3"

  python3 - "$response_file" "$expected_http_code" "$label" <<'PY'
import json
import sys

raw = open(sys.argv[1], encoding="utf-8").read()
marker = "\n__AYB_HTTP_CODE:"
if marker not in raw:
    raise SystemExit(
        f"{sys.argv[3]} response is missing the HTTP status marker; "
        "run the snippet through run_extracted_bash_block_with_curl_status"
    )
body_text, code_text = raw.rsplit(marker, 1)
actual_code = code_text.strip()
expected_code = sys.argv[2]
if actual_code != expected_code:
    raise SystemExit(f"{sys.argv[3]} HTTP mismatch: got {actual_code!r}, want {expected_code!r}")
json.dump(json.loads(body_text), sys.stdout)
PY
}

fetch_and_assert_collection_empty() {
  local collection="$1"
  local body_file="$2"

  curl -sS -m 5 -w '\n__AYB_HTTP_CODE:%{http_code}\n' \
    "$AYB_BASE_URL/api/collections/$collection" >"$body_file" || true
  assert_empty_collection_response "$body_file" "$collection"
}

assert_empty_collection_response() {
  local body_file="$1"
  local label="$2"
  local json_file="${body_file}.body.json"

  extract_http_json_response "$body_file" "200" "$label" >"$json_file"
  python3 - "$json_file" "$label" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))

expected = {
    "items": [],
    "page": 1,
    "perPage": 20,
    "totalItems": 0,
    "totalPages": 0,
}
if body != expected:
    raise SystemExit(f"{sys.argv[2]} empty list mismatch: got {body!r}, want {expected!r}")
PY
}

# Proves a documented "## Create a table" fence really built the documented posts
# table by reading the live catalog, so the check is independent of the REST read
# path and fails for a renamed table, a dropped column, or a reordered fence.
assert_documented_posts_columns() {
  local label="$1"
  local stdout_file="$TMP_DIR/${label}_columns.json"

  invoke_ayb_sql \
    "SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'posts' ORDER BY ordinal_position" \
    "$stdout_file" \
    "$TMP_DIR/${label}_columns.stderr" \
    "${label} posts column introspection failed" \
    --json

  python3 - "$stdout_file" "$label" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    result = json.load(handle)

actual = [row[0] for row in result["rows"]]
expected = ["id", "title", "body", "published", "created_at"]
if actual != expected:
    raise SystemExit(f"{sys.argv[2]} posts column mismatch: got {actual!r}, want {expected!r}")
PY
}

assert_readme_api_crud_response() {
  local body_file="$1"
  python3 - "$body_file" <<'PY'
import json
import sys

raw = open(sys.argv[1], encoding="utf-8").read()
decoder = json.JSONDecoder()
values = []
index = 0
while index < len(raw):
    while index < len(raw) and raw[index].isspace():
        index += 1
    if index >= len(raw):
        break
    value, index = decoder.raw_decode(raw, index)
    values.append(value)
if len(values) != 2:
    raise SystemExit(f"readme_api_open_crud expected two JSON responses, got {len(values)}: {raw!r}")
created, listed = values
expected_row = {"title": "Hello world", "body": "First post"}
for key, expected in expected_row.items():
    actual = created.get(key)
    if actual != expected:
        raise SystemExit(f"readme_api_open_crud created row {key} mismatch: got {actual!r}, want {expected!r}")
if not created.get("id"):
    raise SystemExit(f"readme_api_open_crud created row missing generated id: {created!r}")
expected_list = {
    "items": [created],
    "page": 1,
    "perPage": 10,
    "totalItems": 1,
    "totalPages": 1,
}
if listed != expected_list:
    raise SystemExit(f"readme_api_open_crud sorted list mismatch: got {listed!r}, want {expected_list!r}")
PY
}

assert_getting_started_post_response() {
  local body_file="$1"
  local expected_http_code="$2"
  local label="$3"
  local json_file="${body_file}.body.json"

  extract_http_json_response "$body_file" "$expected_http_code" "$label" >"$json_file"
  python3 - "$json_file" "$label" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
expected = {
    "id": 1,
    "title": "Hello World",
    "body": "My first post",
    "published": True,
}
for key, want in expected.items():
    got = body.get(key)
    if got != want:
        raise SystemExit(f"{sys.argv[2]} field {key} mismatch: got {got!r}, want {want!r}")
if not body.get("created_at"):
    raise SystemExit(f"{sys.argv[2]} missing non-empty created_at: {body!r}")
PY
}

assert_getting_started_filter_response() {
  local body_file="$1"
  python3 - "$body_file" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
items = body.get("items")
if not isinstance(items, list) or len(items) != 1:
    raise SystemExit(f"getting_started_filter_posts expected one item, got {items!r}")
row = items[0]
expected = {
    "id": 1,
    "title": "Hello World",
    "body": "My first post",
    "published": True,
}
for key, want in expected.items():
    got = row.get(key)
    if got != want:
        raise SystemExit(f"getting_started_filter_posts field {key} mismatch: got {got!r}, want {want!r}")
if not row.get("created_at"):
    raise SystemExit(f"getting_started_filter_posts missing non-empty created_at: {row!r}")
if any(item.get("title") == "Excluded Post" for item in items):
    raise SystemExit(f"getting_started_filter_posts included excluded row: {items!r}")
expected_page = {"page": 1, "perPage": 20, "totalItems": 1, "totalPages": 1}
for key, want in expected_page.items():
    got = body.get(key)
    if got != want:
        raise SystemExit(f"getting_started_filter_posts {key} mismatch: got {got!r}, want {want!r}")
PY
}

wait_for_realtime_log_entry() {
  local first_fragment="$1"
  local second_fragment="$2"
  local phase="$3"
  local attempt=1

  while [ "$attempt" -le "$REALTIME_MAX_RETRIES" ]; do
    if ! kill -0 "$REALTIME_PID" 2>/dev/null; then
      cat "$TMP_DIR/realtime.stderr" >&2 || true
      fail "quickstart_realtime: listener exited during ${phase}"
    fi
    if awk -v first="$first_fragment" -v second="$second_fragment" '
      second == "" && index($0, first) { found = 1 }
      index($0, first) { in_entry = 1 }
      in_entry && index($0, second) { found = 1 }
      in_entry && $0 == "}" { in_entry = 0 }
      END { exit found ? 0 : 1 }
    ' "$TMP_DIR/realtime.stdout"; then
      return 0
    fi
    sleep "$REALTIME_RETRY_SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done

  cat "$TMP_DIR/realtime.stderr" >&2 || true
  fail "quickstart_realtime: timed out waiting for ${phase}"
}

assert_auth_me_disabled() {
  local body_file="$1"
  local http_code=""

  http_code="$(curl -sS -m 5 -o "$body_file" -w "%{http_code}" "$AUTH_ME_URL" || true)"
  if [ "$http_code" != "404" ]; then
    echo "expected auth-disabled /api/auth/me to return HTTP 404, got ${http_code:-<none>}" >&2
    [ -f "$body_file" ] && cat "$body_file" >&2
    fail "pre-demo auth-disabled check failed"
  fi
}

run_readme_api_authenticated_crud() {
  local stdout_file="$1"
  local stderr_file="$2"
  local command_file="$TMP_DIR/readme_api_authenticated_crud.sh"
  local wrapper_file="$TMP_DIR/readme_api_authenticated_crud_wrapper.sh"

  extract_doc_block "README.md" "## Working with the API" bash 3 \
    | rewrite_doc_command >"$command_file"
  {
    printf '%s\n' 'set -euo pipefail'
    printf 'AYB_LOGIN_CAPTURE=%q\n' "$TMP_DIR/readme_api_authenticated_crud_login.json"
    printf '%s\n' 'curl() {'
    printf '%s\n' '  if printf "%s\n" "$*" | grep -Fq "/api/auth/login"; then'
    printf '%s\n' '    command curl "$@" | tee "$AYB_LOGIN_CAPTURE"'
    printf '%s\n' '    return ${PIPESTATUS[0]}'
    printf '%s\n' '  fi'
    printf '%s\n' '  command curl -w '"'"'\n__AYB_HTTP_CODE:%{http_code}\n'"'"' "$@"'
    printf '%s\n' '}'
    printf '. %q\n' "$command_file"
  } >"$wrapper_file"
  run_prepared_script "$wrapper_file" "$stdout_file" "$stderr_file" \
    "documented readme_api_authenticated_crud command failed"
}

readme_api_authenticated_token() {
  python3 - "$TMP_DIR/readme_api_authenticated_crud_login.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    print(json.load(handle)["token"])
PY
}

readme_api_authenticated_created_id() {
  extract_http_json_response \
    "$TMP_DIR/readme_authenticated_crud.stdout" \
    "201" \
    "readme_api_authenticated_crud" \
    | python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])'
}

assert_readme_api_authenticated_crud_response() {
  local stdout_file="$1"
  local login_file="$TMP_DIR/readme_api_authenticated_crud_login.json"
  local created_file="$TMP_DIR/readme_api_authenticated_crud_created.json"

  extract_http_json_response "$stdout_file" "201" "readme_api_authenticated_crud" >"$created_file"
  python3 - "$login_file" "$created_file" <<'PY'
import json
import re
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    login = json.load(handle)
token = login.get("token")
if not isinstance(token, str) or token in {"", "null"}:
    raise SystemExit("readme_api_authenticated_crud: login response missing non-empty token")
if not re.fullmatch(r"[^.]+[.][^.]+[.][^.]+", token):
    raise SystemExit("readme_api_authenticated_crud: token is not a three-segment JWT")

with open(sys.argv[2], encoding="utf-8") as handle:
    created = json.load(handle)
expected = {"title": "Hello world", "body": "First post"}
for field, want in expected.items():
    got = created.get(field)
    if got != want:
        raise SystemExit(
            f"readme_api_authenticated_crud: created {field} mismatch: got {got!r}, want {want!r}"
        )
if not created.get("id"):
    raise SystemExit(f"readme_api_authenticated_crud: created row missing id: {created!r}")
PY
}

register_quickstart_auth_account() {
  local email="$1"
  local password="$2"
  local label="$3"
  local body_file="$TMP_DIR/${label}_register.json"
  local http_code=""

  http_code="$(curl -sS -m 5 -o "$body_file" -w "%{http_code}" \
    -X POST "$AYB_BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$password\"}" || true)"
  if [ "$http_code" != "201" ]; then
    echo "${label}: register returned HTTP ${http_code:-<none>}, want 201" >&2
    [ -f "$body_file" ] && cat "$body_file" >&2
    fail "${label}: auth fixture registration failed"
  fi
  python3 - "$body_file" "$email" "$label" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    body = json.load(handle)
user = body.get("user") if isinstance(body.get("user"), dict) else {}
if user.get("email") != sys.argv[2]:
    raise SystemExit(f"{sys.argv[3]}: registered email mismatch: {body!r}")
if not user.get("id"):
    raise SystemExit(f"{sys.argv[3]}: registration response missing user id: {body!r}")
PY
}

register_quickstart_auth_fixtures() {
  register_quickstart_auth_account "you@example.com" "yourpassword" "readme_api_authenticated_crud"
  register_quickstart_auth_account "user@example.com" "password" "readme_sdk_program"
}

stop_realtime_listener() {
  [ -n "$REALTIME_PID" ] || return 0
  if kill -0 "$REALTIME_PID" 2>/dev/null; then
    kill "$REALTIME_PID" 2>/dev/null || true
  fi
  wait "$REALTIME_PID" 2>/dev/null || true
  REALTIME_PID=""
}

restart_ayb_with_auth_enabled() {
  stop_realtime_listener
  HOME="$RUNTIME_HOME" \
    AYB_SERVER_PORT="$API_PORT" \
    AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
    "$AYB_BIN" stop >/dev/null 2>&1 || true

  AYB_SERVER_PORT="$API_PORT" \
  AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
  AYB_AUTH_ENABLED=true \
  AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
  "$AYB_BIN" start >"$TMP_DIR/ayb_auth_start.stdout" 2>"$TMP_DIR/ayb_auth_start.stderr" || {
    cat "$TMP_DIR/ayb_auth_start.stdout" >&2 || true
    cat "$TMP_DIR/ayb_auth_start.stderr" >&2 || true
    fail "auth-enabled ayb restart failed"
  }
  wait_for_ready_health "$TMP_DIR/auth_health.json" || {
    cat "$TMP_DIR/ayb_auth_start.stdout" >&2 || true
    cat "$TMP_DIR/ayb_auth_start.stderr" >&2 || true
    fail "auth-enabled health check readiness failed"
  }
}

authorized_posts_count() {
  local token="$1"
  local label="$2"
  local response_file="$TMP_DIR/${label}_posts_count.response"

  curl -sS -m 5 -w '\n__AYB_HTTP_CODE:%{http_code}\n' \
    "$AYB_BASE_URL/api/collections/posts" \
    -H "Authorization: Bearer $token" >"$response_file" || true
  extract_http_json_response "$response_file" "200" "$label" \
    | python3 -c 'import json, sys; print(json.load(sys.stdin)["totalItems"])'
}

assert_readme_api_authenticated_read_and_rejects_unauthorized_create() {
  local token created_id before_count after_count response_file
  token="$(readme_api_authenticated_token)"
  created_id="$(readme_api_authenticated_created_id)"
  response_file="$TMP_DIR/readme_api_authenticated_read.response"

  curl -sS -m 5 -w '\n__AYB_HTTP_CODE:%{http_code}\n' \
    "$AYB_BASE_URL/api/collections/posts/$created_id" \
    -H "Authorization: Bearer $token" >"$response_file" || true
  extract_http_json_response "$response_file" "200" "readme_api_authenticated_crud read-back" \
    >"$TMP_DIR/readme_api_authenticated_read.json"
  python3 - "$TMP_DIR/readme_api_authenticated_read.json" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
expected = {"title": "Hello world", "body": "First post"}
for field, want in expected.items():
    if body.get(field) != want:
        raise SystemExit(f"readme_api_authenticated_crud read-back {field} mismatch: {body!r}")
PY

  before_count="$(authorized_posts_count "$token" "readme_api_authenticated_before_reject")"
  curl -sS -m 5 -w '\n__AYB_HTTP_CODE:%{http_code}\n' \
    -X POST "$AYB_BASE_URL/api/collections/posts" \
    -H "Content-Type: application/json" \
    -d '{"title": "Hello world", "body": "First post"}' \
    >"$TMP_DIR/readme_api_authenticated_unauthorized_create.response" || true
  extract_http_json_response \
    "$TMP_DIR/readme_api_authenticated_unauthorized_create.response" \
    "401" \
    "readme_api_authenticated_crud unauthenticated create" \
    >"$TMP_DIR/readme_api_authenticated_unauthorized_create.json"
  after_count="$(authorized_posts_count "$token" "readme_api_authenticated_after_reject")"
  [ "$after_count" = "$before_count" ] \
    || fail "readme_api_authenticated_crud: unauthenticated rejected create changed count from $before_count to $after_count"
}

assert_health_contract() {
  local body_file="$1"
  local expected_version="$2"
  python3 - "$body_file" "$expected_version" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    body = json.load(handle)

expected_fields = {
    "status": "ok",
    "database": "ok",
    "version": sys.argv[2],
}
for field, expected in expected_fields.items():
    actual = body.get(field)
    if actual != expected:
        raise SystemExit(
            f"health response field {field!r} mismatch: got {actual!r}, want {expected!r}; body={body!r}"
        )
PY
}

admin_password_is_nonempty() {
  local banner_file="$1"

  awk '
    {
      line = $0
      gsub(/\033\[[0-9;]*m/, "", line)
      marker = "Admin password:"
      marker_start = index(line, marker)
      if (marker_start == 0) {
        next
      }
      value = substr(line, marker_start + length(marker))
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (length(value) > 0) {
        found = 1
      }
    }
    END { exit found ? 0 : 1 }
  ' "$banner_file"
}

assert_admin_password_predicate_contract() {
  local empty_banner="$TMP_DIR/admin_password_empty.stderr"
  local whitespace_banner="$TMP_DIR/admin_password_whitespace.stderr"
  local populated_banner="$TMP_DIR/admin_password_populated.stderr"

  printf '%s\n' '  Admin password:' >"$empty_banner"
  printf '%s\n' '  Admin password:    ' >"$whitespace_banner"
  printf '%s\n' '  Admin password: generated-secret' >"$populated_banner"

  if admin_password_is_nonempty "$empty_banner"; then
    fail "startup predicate accepted an empty Admin password value"
  fi
  if admin_password_is_nonempty "$whitespace_banner"; then
    fail "startup predicate accepted a whitespace-only Admin password value"
  fi
  admin_password_is_nonempty "$populated_banner" \
    || fail "startup predicate rejected a non-empty Admin password value"
}

quickstart_node_minimum_from_doc() {
  local doc_root="$1"

  sed -nE 's/^This quickstart requires Node\.js ([0-9]+) or newer\.$/\1/p' \
    "$doc_root/docs-site/guide/quickstart.md"
}

assert_quickstart_documented_node_floor() {
  local minimum="$1"

  if ! [[ "$minimum" =~ ^[0-9]+$ ]]; then
    fail "quickstart_realtime: expected one documented Node.js minimum"
  fi
  [ "$minimum" -ge 22 ] \
    || fail "quickstart_realtime: documented Node.js minimum must be at least 22"
}

assert_quickstart_node_minimum_contract() {
  local doc_root="$1"
  local minimum

  minimum="$(quickstart_node_minimum_from_doc "$doc_root")"
  assert_quickstart_documented_node_floor "$minimum"
}

write_quickstart_node_minimum_fixture() {
  local doc_root="$1"
  local minimum="$2"

  mkdir -p "$doc_root/docs-site/guide"
  printf 'This quickstart requires Node.js %s or newer.\n' "$minimum" \
    >"$doc_root/docs-site/guide/quickstart.md"
}

assert_quickstart_node_floor_predicate_contract() {
  local node_22_doc="$TMP_DIR/node_22_doc"
  local node_24_doc="$TMP_DIR/node_24_doc"
  local node_21_doc="$TMP_DIR/node_21_doc"

  write_quickstart_node_minimum_fixture "$node_22_doc" "22"
  write_quickstart_node_minimum_fixture "$node_24_doc" "24"
  write_quickstart_node_minimum_fixture "$node_21_doc" "21"

  assert_quickstart_node_minimum_contract "$node_22_doc"
  assert_quickstart_node_minimum_contract "$node_24_doc"
  if (assert_quickstart_node_minimum_contract "$node_21_doc") \
    >"$TMP_DIR/node_21_floor.stdout" 2>"$TMP_DIR/node_21_floor.stderr"; then
    fail "quickstart_realtime: accepted a documented Node.js 21 minimum"
  fi
}

extract_doc_block() {
  local doc_file="$1"
  local heading="$2"
  local language="$3"
  local ordinal="$4"

  "$EXTRACT_DOC_BLOCK" "$DOC_ROOT/$doc_file" "$heading" "$language" "$ordinal"
}

rewrite_doc_command() {
  sed \
    -e "s#http://127.0.0.1:8090#$AYB_BASE_URL#g" \
    -e "s#http://localhost:8090#$AYB_BASE_URL#g"
}

# Single owner of the documented-package-name -> local-SDK-source substitution.
# Replaces the public `@allyourbase/js` install with the locally built package
# so source changes are exercised at HEAD without depending on registry lag.
substitute_local_sdk_source() {
  sed "s#npm install @allyourbase/js#npm install \"$LOCAL_SDK_DIR\"#"
}

run_installer_path_block() {
  local label="$1"
  local doc_file="$2"
  local heading="$3"
  local ordinal="$4"
  local command_file="$TMP_DIR/${label}_path.sh"
  local stdout_file="$TMP_DIR/${label}_path.stdout"
  local stderr_file="$TMP_DIR/${label}_path.stderr"

  extract_doc_block "$doc_file" "$heading" bash "$ordinal" \
    | grep -F 'export PATH="$HOME/.ayb/bin:$PATH"' >"$command_file"

  export HOME="$RUNTIME_HOME"
  if ! env HOME="$RUNTIME_HOME" PATH="/usr/bin:/bin:/usr/sbin:/sbin" bash -c ". '$command_file'; command -v ayb" >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2 || true
    fail "documented ${label} PATH command did not resolve ayb from isolated HOME"
  fi

  assert_contains "$stdout_file" "$RUNTIME_HOME/.ayb/bin/ayb" "documented ${label} PATH command resolved the wrong ayb binary"
}

assert_dashboard_served() {
  local html_file="$1"
  local asset_file="$2"
  local headers_file="$TMP_DIR/admin_asset.headers"
  local http_code=""
  local asset_path=""

  http_code="$(curl -sS -m 5 -o "$html_file" -w "%{http_code}" "$AYB_BASE_URL/admin/" || true)"
  if [ "$http_code" != "200" ]; then
    echo "expected dashboard shell to return HTTP 200, got ${http_code:-<none>}" >&2
    [ -f "$html_file" ] && cat "$html_file" >&2
    fail "dashboard shell check failed"
  fi
  assert_contains "$html_file" "<title>Allyourbase Admin</title>" "dashboard shell missing title"
  assert_contains "$html_file" 'id="root"' "dashboard shell missing root element"

  asset_path="$(python3 - "$html_file" <<'PY'
import re
import sys

html = open(sys.argv[1], encoding="utf-8").read()
match = re.search(r'<script[^>]+src="(/admin/assets/[^"]+\.js)"', html)
if not match:
    raise SystemExit("dashboard shell missing /admin/assets/*.js module")
print(match.group(1))
PY
)"

  http_code="$(curl -sS -m 5 -D "$headers_file" -o "$asset_file" -w "%{http_code}" "$AYB_BASE_URL$asset_path" || true)"
  if [ "$http_code" != "200" ]; then
    echo "expected dashboard asset ${asset_path} to return HTTP 200, got ${http_code:-<none>}" >&2
    [ -f "$asset_file" ] && cat "$asset_file" >&2
    fail "dashboard asset check failed"
  fi
  assert_contains "$headers_file" "javascript" "dashboard asset response missing JavaScript content type"
  [ -s "$asset_file" ] || fail "dashboard asset body was empty"
}

assert_npm_registry_package_available() {
  local stdout_file="$TMP_DIR/npm_view_allyourbase_js.stdout"
  local stderr_file="$TMP_DIR/npm_view_allyourbase_js.stderr"
  local version=""

  if ! npm view @allyourbase/js version >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2 || true
    fail "npm registry availability check failed for @allyourbase/js"
  fi
  version="$(tr -d '[:space:]' <"$stdout_file")"
  if ! printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$'; then
    cat "$stdout_file" >&2 || true
    fail "npm registry returned a non-semver @allyourbase/js version"
  fi
}

run_documented_sdk_install() {
  local inventory_id="$1"
  local doc_file="$2"
  local heading="$3"
  local scratch_dir=""
  local command_file=""
  local package_file=""

  scratch_dir="$(mktemp -d "$TMP_DIR/${inventory_id}.XXXXXX")"
  command_file="$scratch_dir/install.sh"
  package_file="$scratch_dir/node_modules/@allyourbase/js/package.json"
  printf '%s\n' '{"private":true}' >"$scratch_dir/package.json"
  extract_doc_block "$doc_file" "$heading" bash 1 >"$command_file"

  if ! (cd "$scratch_dir" && bash "$command_file" >install.stdout 2>install.stderr); then
    cat "$scratch_dir/install.stderr" >&2 || true
    fail "${inventory_id}: documented npm install command failed"
  fi

  python3 - "$package_file" "$inventory_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    package = json.load(handle)

expected_name = "@allyourbase/js"
if package.get("name") != expected_name:
    raise SystemExit(
        f"{sys.argv[2]}: installed package name mismatch: "
        f"got {package.get('name')!r}, want {expected_name!r}"
    )
PY
  printf 'PASS: %s installed package name @allyourbase/js\n' "$inventory_id"
}
