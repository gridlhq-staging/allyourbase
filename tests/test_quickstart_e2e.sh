#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=tests/bash_assert_helpers.sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
source "$SCRIPT_DIR/bash_assert_helpers.sh"
source "$SCRIPT_DIR/port_helpers.sh"

API_PORT="$(pick_free_port 45290 46290 47290 48290 49290)" || fail "no free port available for quickstart API server"
DEMO_PORT="$(pick_free_port 45295 46295 47295 48295 49295)" || fail "no free port available for quickstart demo app"
PG_PORT="$(pick_free_port 45432 46432 47432 48432 49432)" || fail "no free port available for quickstart managed Postgres"

AYB_BASE_URL="http://localhost:${API_PORT}"
AYB_HEALTH_URL="${AYB_BASE_URL}/health"
AUTH_ME_URL="${AYB_BASE_URL}/api/auth/me"
DEMO_URL="http://localhost:${DEMO_PORT}/"
LOGIN_URL="${AYB_BASE_URL}/api/auth/login"
DEMO_JWT_SECRET="quickstart-e2e-demo-jwt-secret-0123456789abcdef"
MAX_RETRIES=90
RETRY_SLEEP_SECONDS=1
QUICKSTART_BUILD_VERSION="quickstart-e2e"

TMP_DIR="$(mktemp -d)"
RUNTIME_HOME="$TMP_DIR/home"
RUNTIME_WORKDIR="$TMP_DIR/workdir"
LOCAL_SDK_DIR="$TMP_DIR/sdk"
DOC_ROOT="${AYB_QUICKSTART_DOC_ROOT:-$REPO_ROOT}"
EXTRACT_DOC_BLOCK="$REPO_ROOT/scripts/extract_doc_block.sh"
LOCAL_AYB_BIN="$RUNTIME_HOME/.ayb/bin/ayb"
AYB_BIN="${AYB_QUICKSTART_BIN:-$LOCAL_AYB_BIN}"
DEMO_PID=""

cleanup() {
  HOME="$RUNTIME_HOME" \
    AYB_SERVER_PORT="$API_PORT" \
    AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
    "$AYB_BIN" stop >/dev/null 2>&1 || true

  if [ -n "$DEMO_PID" ]; then
    if kill -0 "$DEMO_PID" 2>/dev/null; then
      kill "$DEMO_PID" 2>/dev/null || true
    fi
    wait "$DEMO_PID" 2>/dev/null || true
  fi

  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

resolve_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$(pwd)" "$1" ;;
  esac
}

prepare_ayb_binary() {
  if [ -n "${AYB_QUICKSTART_BIN:-}" ]; then
    AYB_BIN="$(resolve_path "$AYB_QUICKSTART_BIN")"
    if [ ! -x "$AYB_BIN" ]; then
      fail "AYB_QUICKSTART_BIN does not resolve to an executable file: $AYB_BIN"
    fi
    return 0
  fi

  AYB_BIN="$LOCAL_AYB_BIN"
  mkdir -p "$(dirname "$AYB_BIN")"
  (cd "$REPO_ROOT" && go build -ldflags "-X main.version=$QUICKSTART_BUILD_VERSION" -o "$AYB_BIN" ./cmd/ayb)
}

wait_for_ready_health() {
  local body_file="$1"
  local http_code=""
  local attempt=1

  while [ "$attempt" -le "$MAX_RETRIES" ]; do
    http_code="$(curl -s -m 2 -o "$body_file" -w "%{http_code}" "$AYB_HEALTH_URL" || true)"
    if [ "$http_code" = "200" ] \
      && grep -Fq '"status":"ok"' "$body_file" \
      && grep -Fq '"database":"ok"' "$body_file"; then
      return 0
    fi
    sleep "$RETRY_SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done

  echo "Health endpoint did not reach ready state after ${MAX_RETRIES} attempts." >&2
  echo "Last HTTP status: ${http_code:-<none>}" >&2
  [ -f "$body_file" ] && cat "$body_file" >&2
  return 1
}

wait_for_demo_http_200_with_body() {
  local body_file="$1"
  local http_code=""
  local attempt=1

  while [ "$attempt" -le "$MAX_RETRIES" ]; do
    http_code="$(curl -s -m 2 -o "$body_file" -w "%{http_code}" "$DEMO_URL" || true)"
    if [ "$http_code" = "200" ] \
      && [ -s "$body_file" ] \
      && grep -Fq '<title>Live Polls</title>' "$body_file"; then
      return 0
    fi
    sleep "$RETRY_SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done

  echo "Demo app did not become ready after ${MAX_RETRIES} attempts." >&2
  echo "Last HTTP status: ${http_code:-<none>}" >&2
  [ -f "$body_file" ] && cat "$body_file" >&2
  return 1
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

run_extracted_bash_block() {
  local label="$1"
  local doc_file="$2"
  local heading="$3"
  local ordinal="$4"
  local stdout_file="$5"
  local stderr_file="$6"
  local command_file="$TMP_DIR/${label}.sh"

  extract_doc_block "$doc_file" "$heading" bash "$ordinal" | rewrite_doc_command >"$command_file"
  if ! bash "$command_file" >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2 || true
    fail "documented ${label} command failed"
  fi
}

assert_collection_empty() {
  local collection="$1"
  local body_file="$2"
  local http_code=""

  http_code="$(curl -sS -m 5 -o "$body_file" -w "%{http_code}" "$AYB_BASE_URL/api/collections/$collection" || true)"
  if [ "$http_code" != "200" ]; then
    echo "expected ${collection} list endpoint to return HTTP 200, got ${http_code:-<none>}" >&2
    [ -f "$body_file" ] && cat "$body_file" >&2
    fail "${collection} collection reachability check failed"
  fi

  python3 - "$body_file" "$collection" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    body = json.load(handle)

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

prepare_local_sdk_package() {
  cp -R "$REPO_ROOT/sdk" "$LOCAL_SDK_DIR"
  if ! (cd "$LOCAL_SDK_DIR" && npm ci >"$TMP_DIR/sdk_npm_ci.stdout" 2>"$TMP_DIR/sdk_npm_ci.stderr"); then
    cat "$TMP_DIR/sdk_npm_ci.stderr" >&2 || true
    fail "local SDK npm ci failed"
  fi
  if ! (cd "$LOCAL_SDK_DIR" && npm run build >"$TMP_DIR/sdk_build.stdout" 2>"$TMP_DIR/sdk_build.stderr"); then
    cat "$TMP_DIR/sdk_build.stderr" >&2 || true
    fail "local SDK build failed"
  fi
}

run_quickstart_sdk_program() {
  local stdout_file="$1"
  local stderr_file="$2"
  local setup_script="$TMP_DIR/quickstart_sdk_setup.sh"
  local run_script="$TMP_DIR/quickstart_sdk_run.sh"
  local app_dir="$RUNTIME_WORKDIR/todo-app"

  prepare_local_sdk_package

  # Substitute only the package source so registry publication lag cannot
  # invalidate the CRUD contract or write SDK build artifacts into the checkout.
  extract_doc_block "docs-site/guide/quickstart.md" "## 3. Set up the project" bash 1 \
    | rewrite_doc_command \
    | sed "s#npm install @allyourbase/js#npm install \"$LOCAL_SDK_DIR\"#" \
    >"$setup_script"
  if ! (cd "$RUNTIME_WORKDIR" && bash "$setup_script" >"$TMP_DIR/sdk_setup.stdout" 2>"$TMP_DIR/sdk_setup.stderr"); then
    cat "$TMP_DIR/sdk_setup.stderr" >&2 || true
    fail "documented SDK setup command failed"
  fi

  extract_doc_block "docs-site/guide/quickstart.md" "## 4. Write the app" js 1 \
    | rewrite_doc_command >"$app_dir/index.mjs"

  extract_doc_block "docs-site/guide/quickstart.md" "## 5. Run it" bash 1 \
    | rewrite_doc_command >"$run_script"
  if ! (cd "$app_dir" && bash "$run_script" >"$stdout_file" 2>"$stderr_file"); then
    cat "$stderr_file" >&2 || true
    fail "documented SDK run command failed"
  fi
}

assert_quickstart_sdk_output() {
  local stdout_file="$1"

  [ -f "$stdout_file" ] || fail "SDK CRUD program was not executed"
  python3 - "$stdout_file" <<'PY'
import json
import sys

lines = [line.rstrip("\n") for line in open(sys.argv[1], encoding="utf-8")]

expected_all = [
    {"id": 3, "title": "Ship v1", "completed": False},
    {"id": 2, "title": "Write docs", "completed": True},
    {"id": 1, "title": "Buy groceries", "completed": False},
]
expected_pending = [
    {"id": 3, "title": "Ship v1", "completed": False},
    {"id": 1, "title": "Buy groceries", "completed": False},
]
expected_remaining = [
    {"id": 2, "title": "Write docs", "completed": True},
    {"id": 1, "title": "Buy groceries", "completed": False},
]

def parse_json_line(label):
    prefix = f"{label}: "
    matches = [line[len(prefix):] for line in lines if line.startswith(prefix)]
    if len(matches) != 1:
        raise SystemExit(f"expected exactly one {label!r} line, got {len(matches)} in {lines!r}")
    return json.loads(matches[0])

checks = {
    "All todos": expected_all,
    "Pending": expected_pending,
    "Remaining": expected_remaining,
}
for label, expected in checks.items():
    actual = parse_json_line(label)
    if actual != expected:
        raise SystemExit(f"{label} mismatch: got {actual!r}, want {expected!r}")

for exact_line in ['Marked "Ship v1" as done', 'Deleted "Ship v1"']:
    if lines.count(exact_line) != 1:
        raise SystemExit(f"expected exact line {exact_line!r} once, got {lines!r}")
PY
}

run_generated_scaffold_project() {
  local project_dir="$RUNTIME_WORKDIR/todo-scaffold"
  local schema_step_script="$TMP_DIR/scaffold_schema_step.sh"

  if ! (cd "$RUNTIME_WORKDIR" && "$AYB_BIN" init todo-scaffold --template plain >"$TMP_DIR/scaffold_init.stdout" 2>"$TMP_DIR/scaffold_init.stderr"); then
    cat "$TMP_DIR/scaffold_init.stderr" >&2 || true
    fail "documented scaffold init command failed"
  fi

  printf '%s\n' "ayb sql < schema.sql" >"$schema_step_script"
  if ! (cd "$project_dir" && AYB_SERVER_PORT="$API_PORT" bash "$schema_step_script" >"$TMP_DIR/scaffold_schema.stdout" 2>"$TMP_DIR/scaffold_schema.stderr"); then
    cat "$TMP_DIR/scaffold_schema.stderr" >&2 || true
    fail "generated scaffold schema command failed"
  fi

  if ! (cd "$project_dir" && npm install "$LOCAL_SDK_DIR" >"$TMP_DIR/scaffold_npm_install.stdout" 2>"$TMP_DIR/scaffold_npm_install.stderr"); then
    cat "$TMP_DIR/scaffold_npm_install.stderr" >&2 || true
    fail "generated scaffold npm install failed"
  fi
  if ! (cd "$project_dir" && npm run build >"$TMP_DIR/scaffold_build.stdout" 2>"$TMP_DIR/scaffold_build.stderr"); then
    cat "$TMP_DIR/scaffold_build.stderr" >&2 || true
    fail "generated scaffold build failed"
  fi
  if ! (cd "$project_dir" && AYB_URL="$AYB_BASE_URL" npm run start >"$TMP_DIR/scaffold_run.stdout" 2>"$TMP_DIR/scaffold_run.stderr"); then
    cat "$TMP_DIR/scaffold_run.stderr" >&2 || true
    fail "generated scaffold start failed"
  fi
}

assert_generated_scaffold_project() {
  local project_dir="$1"
  local init_stdout="$2"
  local run_stdout="$3"

  [ -f "$init_stdout" ] || fail "scaffold init command was not executed"
  assert_contains "$init_stdout" "todo-scaffold" "scaffold init output missing project name"
  assert_contains "$init_stdout" "Done!" "scaffold init output missing success line"
  assert_contains "$init_stdout" "cd todo-scaffold" "scaffold init output missing cd next step"
  assert_contains "$init_stdout" "ayb sql < schema.sql" "scaffold init output missing schema next step"

  for generated_file in ayb.toml .env schema.sql package.json tsconfig.json src/index.ts src/lib/ayb.ts; do
    [ -f "$project_dir/$generated_file" ] || fail "scaffold missing generated file: $generated_file"
  done

  assert_contains "$project_dir/src/lib/ayb.ts" "process.env.AYB_URL" "scaffold client missing AYB_URL override"
  [ -f "$run_stdout" ] || fail "scaffold generated program was not executed"
  assert_contains "$run_stdout" "AYB server: ok" "scaffold generated program missing health output"
  assert_contains "$run_stdout" 'Search items for "demo": 0' "scaffold generated program missing search output"
}

mkdir -p "$RUNTIME_WORKDIR"
assert_admin_password_predicate_contract
prepare_ayb_binary
if ! AYB_EXPECTED_VERSION="$("$AYB_BIN" version --json | python3 -c '
import json, sys
version = json.load(sys.stdin).get("version", "")
if not isinstance(version, str) or not version:
    raise SystemExit("ayb version --json returned no usable version")
print(version)
')"; then
  fail "could not read the quickstart binary version"
fi
export PATH="$(dirname "$AYB_BIN"):$PATH"
export HOME="$RUNTIME_HOME"
cd "$RUNTIME_WORKDIR"

AYB_SERVER_PORT="$API_PORT" "$AYB_BIN" stop >/dev/null 2>&1 || true

AYB_SERVER_PORT="$API_PORT" \
AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
AYB_AUTH_ENABLED=false \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" start >"$TMP_DIR/ayb_start.stdout" 2>"$TMP_DIR/ayb_start.stderr" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "documented ayb start command failed"
}

wait_for_ready_health "$TMP_DIR/health.json" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "health check readiness failed"
}

assert_contains "$TMP_DIR/health.json" '"status":"ok"' "health response missing status ok"
admin_password_is_nonempty "$TMP_DIR/ayb_start.stderr" \
  || fail "startup banner missing non-empty admin password"
assert_contains "$TMP_DIR/ayb_start.stderr" "To reset: ayb admin reset-password" "startup banner missing reset hint"
assert_health_contract "$TMP_DIR/health.json" "$AYB_EXPECTED_VERSION"

run_extracted_bash_block \
  "quickstart_create_todos" \
  "docs-site/guide/quickstart.md" \
  "## 2. Create a todos table" \
  1 \
  "$TMP_DIR/todos_sql.stdout" \
  "$TMP_DIR/todos_sql.stderr"

run_extracted_bash_block \
  "getting_started_create_posts" \
  "docs-site/guide/getting-started.md" \
  "## Create a table" \
  1 \
  "$TMP_DIR/posts_sql.stdout" \
  "$TMP_DIR/posts_sql.stderr"

run_extracted_bash_block \
  "getting_started_list_posts" \
  "docs-site/guide/getting-started.md" \
  "### List records" \
  1 \
  "$TMP_DIR/posts_list.json" \
  "$TMP_DIR/posts_list.stderr"

assert_collection_empty "posts" "$TMP_DIR/posts_list.json"
assert_collection_empty "todos" "$TMP_DIR/todos_list.json"
assert_dashboard_served "$TMP_DIR/admin.html" "$TMP_DIR/admin_asset.js"
assert_npm_registry_package_available
run_quickstart_sdk_program "$TMP_DIR/sdk_run.stdout" "$TMP_DIR/sdk_run.stderr"
assert_quickstart_sdk_output "$TMP_DIR/sdk_run.stdout"
run_generated_scaffold_project
assert_generated_scaffold_project "$RUNTIME_WORKDIR/todo-scaffold" "$TMP_DIR/scaffold_init.stdout" "$TMP_DIR/scaffold_run.stdout"
assert_auth_me_disabled "$TMP_DIR/auth_me_disabled.json"

AYB_SERVER_PORT="$API_PORT" \
AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
AYB_DEMO_APP_PORT="$DEMO_PORT" \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" demo live-polls >"$TMP_DIR/demo.stdout" 2>"$TMP_DIR/demo.stderr" &
DEMO_PID="$!"

sleep 1
if ! kill -0 "$DEMO_PID" 2>/dev/null; then
  cat "$TMP_DIR/demo.stdout" >&2 || true
  cat "$TMP_DIR/demo.stderr" >&2 || true
  fail "demo process exited before readiness"
fi

wait_for_demo_http_200_with_body "$TMP_DIR/demo_index.html" || {
  cat "$TMP_DIR/demo.stdout" >&2 || true
  cat "$TMP_DIR/demo.stderr" >&2 || true
  fail "demo app serving check failed"
}

login_http_code="$(curl -sS -m 5 -o "$TMP_DIR/login.json" -w "%{http_code}" \
  -X POST "$LOGIN_URL" \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@demo.test","password":"password123"}' || true)"

if [ "$login_http_code" != "200" ]; then
  echo "login request failed with HTTP ${login_http_code:-<none>}" >&2
  [ -f "$TMP_DIR/login.json" ] && cat "$TMP_DIR/login.json" >&2
  fail "seeded auth login check failed"
fi

if ! grep -Eq '"token"[[:space:]]*:[[:space:]]*"[^"]+"' "$TMP_DIR/login.json"; then
  cat "$TMP_DIR/login.json" >&2 || true
  fail "login response missing non-empty token field"
fi

echo "PASS: quickstart e2e probe succeeded"
