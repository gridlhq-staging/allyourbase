#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=tests/bash_assert_helpers.sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
source "$SCRIPT_DIR/bash_assert_helpers.sh"
source "$SCRIPT_DIR/port_helpers.sh"
source "$SCRIPT_DIR/quickstart_doc_helpers.sh"

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
REALTIME_MAX_RETRIES=100
REALTIME_RETRY_SLEEP_SECONDS=0.1
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
REALTIME_PID=""

cleanup() {
  if [ -n "$REALTIME_PID" ]; then
    if kill -0 "$REALTIME_PID" 2>/dev/null; then
      kill "$REALTIME_PID" 2>/dev/null || true
    fi
    wait "$REALTIME_PID" 2>/dev/null || true
  fi

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

prepare_local_sdk_package() {
  [ -f "$LOCAL_SDK_DIR/dist/index.js" ] && return 0
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

sdk_program_doc_file() {
  case "$1" in
    getting_started_sdk_crud) printf '%s\n' "docs-site/guide/getting-started.md" ;;
    readme_sdk_program*) printf '%s\n' "README.md" ;;
    *) fail "unknown SDK program label: $1" ;;
  esac
}

sdk_program_heading() {
  case "$1" in
    getting_started_sdk_crud) printf '%s\n' "## Use the JavaScript SDK" ;;
    readme_sdk_program*) printf '%s\n' "## SDK" ;;
    *) fail "unknown SDK program label: $1" ;;
  esac
}

sdk_program_language() {
  case "$1" in
    getting_started_sdk_crud) printf '%s\n' "ts" ;;
    readme_sdk_program*) printf '%s\n' "typescript" ;;
    *) fail "unknown SDK program label: $1" ;;
  esac
}

sdk_program_app_prefix() {
  case "$1" in
    getting_started_sdk_crud) printf '%s\n' "getting_started_sdk" ;;
    readme_sdk_program*) printf '%s\n' "readme_sdk" ;;
    *) fail "unknown SDK program label: $1" ;;
  esac
}

prepare_documented_sdk_program_app() {
  local label="$1"
  local app_dir="$2"
  local setup_script="$app_dir/install.sh"
  local doc_file=""
  local heading=""
  local language=""

  doc_file="$(sdk_program_doc_file "$label")"
  heading="$(sdk_program_heading "$label")"
  language="$(sdk_program_language "$label")"

  prepare_local_sdk_package
  printf '%s\n' '{"private":true,"type":"module"}' >"$app_dir/package.json"

  extract_doc_block "$doc_file" "$heading" bash 1 \
    | substitute_local_sdk_source >"$setup_script"
  if ! (cd "$app_dir" && bash "$setup_script" >install.stdout 2>install.stderr); then
    cat "$app_dir/install.stderr" >&2 || true
    fail "$label: local SDK install failed"
  fi

  extract_doc_block "$doc_file" "$heading" "$language" 1 \
    | rewrite_doc_command >"$app_dir/index.mjs"
}

run_documented_sdk_program() {
  local label="$1"
  local stdout_file="$2"
  local stderr_file="$3"
  local app_dir=""

  app_dir="$(mktemp -d "$RUNTIME_WORKDIR/$(sdk_program_app_prefix "$label").XXXXXX")"
  prepare_documented_sdk_program_app "$label" "$app_dir"
  if ! (cd "$app_dir" && node index.mjs >"$stdout_file" 2>"$stderr_file"); then
    cat "$stderr_file" >&2 || true
    fail "$label: documented SDK program failed"
  fi
}

run_getting_started_sdk_program() {
  run_documented_sdk_program "getting_started_sdk_crud" "$1" "$2"
}

run_readme_sdk_program() {
  run_documented_sdk_program "readme_sdk_program" "$1" "$2"
}

capture_prerepair_readme_sdk_baseline() {
  local git_ref="$1"
  local output_dir="$2"
  local previous_doc_root="$DOC_ROOT"
  local baseline_doc_root="$TMP_DIR/prerepair_readme_doc_root"
  local app_dir=""
  local status=0

  mkdir -p "$output_dir" "$baseline_doc_root"
  git -C "$REPO_ROOT" show "${git_ref}:README.md" >"$baseline_doc_root/README.md"

  DOC_ROOT="$baseline_doc_root"
  app_dir="$(mktemp -d "$RUNTIME_WORKDIR/readme_sdk_prerepair.XXXXXX")"
  prepare_documented_sdk_program_app "readme_sdk_program_prerepair" "$app_dir"
  DOC_ROOT="$previous_doc_root"

  set +e
  (cd "$app_dir" && node index.mjs >"$output_dir/stdout.txt" 2>"$output_dir/stderr.txt")
  status=$?
  set -e
  printf '%s\n' "$status" >"$output_dir/status.txt"
  cp "$app_dir/index.mjs" "$output_dir/index.mjs"
  cp "$app_dir/install.sh" "$output_dir/install.sh"

  if [ "$status" -eq 0 ]; then
    fail "readme_sdk_program_prerepair: expected the pre-repair SDK sample to fail"
  fi
}

assert_sdk_runner_single_lifecycle_owner() {
  python3 - "$0" <<'PY'
from pathlib import Path
import sys

text = Path(sys.argv[1]).read_text()
needle = "prepare_documented_sdk_program_app" + "() {"
if text.count(needle) != 1:
    raise SystemExit("SDK runner lifecycle helper must have exactly one owner")
for wrapper in ("run_getting_started_sdk_program", "run_readme_sdk_program"):
    start = text.index(f"{wrapper}() {{")
    end = text.index("\n}\n", start)
    body = text[start:end]
    forbidden = ("prepare_local_sdk_package", "mktemp -d", "extract_doc_block", "node index.mjs")
    leaked = [token for token in forbidden if token in body]
    if leaked:
        raise SystemExit(f"{wrapper} duplicates SDK runner lifecycle tokens: {leaked}")
PY
}

assert_readme_sdk_output() {
  local stdout_file="$1"

  [ -f "$stdout_file" ] || fail "readme_sdk_program: SDK program was not executed"
  python3 - "$stdout_file" <<'PY'
import ast
import sys

lines = [line.strip() for line in open(sys.argv[1], encoding="utf-8")]

def line_after(prefix):
    matches = [line[len(prefix):].strip() for line in lines if line.startswith(prefix)]
    if len(matches) != 1:
        raise SystemExit(f"readme_sdk_program: expected one {prefix!r} line, got {matches!r}; stdout={lines!r}")
    return matches[0]

published = ast.literal_eval(line_after("Published posts:"))
expected_published = ["Hello", "Hello World"]
if published != expected_published:
    raise SystemExit(
        f"readme_sdk_program: published title order mismatch: "
        f"got {published!r}, want {expected_published!r}; stdout={lines!r}"
    )
if "Excluded Post" in published or "Hello world" in published:
    raise SystemExit(f"readme_sdk_program: unpublished row leaked into published list: {published!r}")
if line_after("Created post:") != "New post":
    raise SystemExit(f"readme_sdk_program: created post line mismatch; stdout={lines!r}")
if line_after("Realtime event:") != "create New post":
    raise SystemExit(f"readme_sdk_program: realtime event mismatch; stdout={lines!r}")
PY
}

assert_getting_started_sdk_output() {
  local stdout_file="$1"

  [ -f "$stdout_file" ] || fail "getting_started_sdk_crud: SDK program was not executed"
  python3 - "$stdout_file" <<'PY'
import re
import sys

output = open(sys.argv[1], encoding="utf-8").read()
titles = [match[1] for match in re.findall(r"\btitle:\s*(['\"])(.*?)\1", output)]
expected_titles = ["Hello", "Hello World"]
if titles != expected_titles:
    raise SystemExit(
        f"getting_started_sdk_crud: title order mismatch: "
        f"got {titles!r}, want {expected_titles!r}; stdout={output!r}"
    )
if "Excluded Post" in output:
    raise SystemExit(
        f"getting_started_sdk_crud: unpublished excluded row was returned; stdout={output!r}"
    )
PY
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
# inventory-id: quickstart_project_setup
  extract_doc_block "docs-site/guide/quickstart.md" "## 3. Set up the project" bash 1 \
    | rewrite_doc_command \
    | substitute_local_sdk_source \
    >"$setup_script"
  if ! (cd "$RUNTIME_WORKDIR" && bash "$setup_script" >"$TMP_DIR/sdk_setup.stdout" 2>"$TMP_DIR/sdk_setup.stderr"); then
    cat "$TMP_DIR/sdk_setup.stderr" >&2 || true
    fail "documented SDK setup command failed"
  fi

# inventory-id: quickstart_app_crud
  extract_doc_block "docs-site/guide/quickstart.md" "## 4. Write the app" js 1 \
    | rewrite_doc_command >"$app_dir/index.mjs"

# inventory-id: quickstart_run_app
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

if [ "${AYB_QUICKSTART_NODE_FLOOR_SELFTEST:-}" = "1" ]; then
  assert_quickstart_node_floor_predicate_contract
  exit 0
fi

if [ "${AYB_QUICKSTART_SDK_RUNNER_SELFTEST:-}" = "1" ]; then
  assert_sdk_runner_single_lifecycle_owner
  exit 0
fi

mkdir -p "$RUNTIME_WORKDIR"
QUICKSTART_NODE_MINIMUM="$(quickstart_node_minimum_from_doc "$DOC_ROOT")"
assert_quickstart_documented_node_floor "$QUICKSTART_NODE_MINIMUM"
QUICKSTART_NODE_MAJOR="$(node -p 'Number(process.versions.node.split(".")[0])')"
[ "$QUICKSTART_NODE_MAJOR" -ge "$QUICKSTART_NODE_MINIMUM" ] || fail "quickstart_realtime: Node.js ${QUICKSTART_NODE_MINIMUM} or newer is required"
assert_admin_password_predicate_contract
# inventory-id: readme_quickstart_commands
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
export HOME="$RUNTIME_HOME"
export PATH="$HOME/.ayb/bin:$PATH"
[ "$(command -v ayb)" = "$LOCAL_AYB_BIN" ] || fail "documented installer PATH did not resolve the local ayb binary"
# inventory-id: getting_started_curl_install
run_installer_path_block "getting_started_install" "docs-site/guide/getting-started.md" "### curl (macOS / Linux)" 1
# inventory-id: quickstart_start_commands
run_installer_path_block "quickstart_install" "docs-site/guide/quickstart.md" "## 1. Start AYB" 1
cd "$RUNTIME_WORKDIR"

AYB_SERVER_PORT="$API_PORT" "$AYB_BIN" stop >/dev/null 2>&1 || true

# inventory-id: getting_started_managed_start
AYB_SERVER_PORT="$API_PORT" \
AYB_DATABASE_EMBEDDED_PORT="$PG_PORT" \
AYB_AUTH_ENABLED=false \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" start >"$TMP_DIR/ayb_start.stdout" 2>"$TMP_DIR/ayb_start.stderr" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "documented ayb start command failed"
}

# inventory-id: getting_started_health
wait_for_ready_health "$TMP_DIR/health.json" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "health check readiness failed"
}

assert_contains "$TMP_DIR/health.json" '"status":"ok"' "health response missing status ok"
admin_password_is_nonempty "$TMP_DIR/ayb_start.stderr" \
  || fail "startup banner missing non-empty admin password"
assert_contains "$TMP_DIR/ayb_start.stderr" "To reset: ayb admin reset-password" "startup banner missing reset hint"
# inventory-id: quickstart_health
assert_health_contract "$TMP_DIR/health.json" "$AYB_EXPECTED_VERSION"

# inventory-id: readme_api_create_table
run_extracted_bash_block \
  "readme_api_create_table" \
  "README.md" \
  "## Working with the API" \
  1 \
  "$TMP_DIR/readme_posts_sql.stdout" \
  "$TMP_DIR/readme_posts_sql.stderr"
fetch_and_assert_collection_empty "posts" "$TMP_DIR/readme_posts_empty.json"
# inventory-id: readme_api_open_crud
run_extracted_bash_block \
  "readme_api_open_crud" \
  "README.md" \
  "## Working with the API" \
  2 \
  "$TMP_DIR/readme_posts_crud.json" \
  "$TMP_DIR/readme_posts_crud.stderr"
assert_readme_api_crud_response "$TMP_DIR/readme_posts_crud.json"
run_harness_sql "drop_readme_posts" "DROP TABLE IF EXISTS posts CASCADE"
# inventory-id: quickstart_create_todos
run_extracted_bash_block \
  "quickstart_create_todos" \
  "docs-site/guide/quickstart.md" \
  "## 2. Create a todos table" \
  1 \
  "$TMP_DIR/todos_sql.stdout" \
  "$TMP_DIR/todos_sql.stderr"

# Both "## Create a table" fences create the same posts table, so each is executed
# and asserted in turn with a harness-local drop between them.
# inventory-id: getting_started_create_posts_ayb_sql
run_extracted_bash_block \
  "getting_started_create_posts_ayb_sql" \
  "docs-site/guide/getting-started.md" \
  "## Create a table" \
  1 \
  "$TMP_DIR/getting_started_create_posts_ayb_sql.stdout" \
  "$TMP_DIR/getting_started_create_posts_ayb_sql.stderr"
assert_documented_posts_columns "getting_started_create_posts_ayb_sql"
run_harness_sql "drop_getting_started_ayb_sql_posts" "DROP TABLE IF EXISTS posts CASCADE"

# inventory-id: getting_started_create_posts_sql
run_extracted_sql_block \
  "getting_started_create_posts_sql" \
  "docs-site/guide/getting-started.md" \
  "## Create a table" \
  1 \
  "$TMP_DIR/posts_sql_block.stdout" \
  "$TMP_DIR/posts_sql_block.stderr"

# inventory-id: getting_started_list_posts
run_extracted_bash_block_with_curl_status \
  "getting_started_list_posts" \
  "docs-site/guide/getting-started.md" \
  "### List records" \
  1 \
  "$TMP_DIR/posts_list.json" \
  "$TMP_DIR/posts_list.stderr"

assert_empty_collection_response "$TMP_DIR/posts_list.json" "getting_started_list_posts"
fetch_and_assert_collection_empty "todos" "$TMP_DIR/todos_list.json"

# inventory-id: getting_started_create_post
run_extracted_bash_block_with_curl_status \
  "getting_started_create_post" \
  "docs-site/guide/getting-started.md" \
  "### Create a record" \
  1 \
  "$TMP_DIR/getting_started_create_post.json" \
  "$TMP_DIR/getting_started_create_post.stderr"
assert_getting_started_post_response "$TMP_DIR/getting_started_create_post.json" "201" "getting_started_create_post"
run_harness_sql "seed_getting_started_excluded_post" \
  "INSERT INTO posts (title, body, published) VALUES ('Excluded Post', 'Not the documented post', false)"
# inventory-id: getting_started_filter_posts
run_extracted_bash_block \
  "getting_started_filter_posts" \
  "docs-site/guide/getting-started.md" \
  "### Filter and sort" \
  1 \
  "$TMP_DIR/getting_started_filter_posts.json" \
  "$TMP_DIR/getting_started_filter_posts.stderr"
assert_getting_started_filter_response "$TMP_DIR/getting_started_filter_posts.json"
# inventory-id: getting_started_get_post
run_extracted_bash_block_with_curl_status \
  "getting_started_get_post" \
  "docs-site/guide/getting-started.md" \
  "### Get a single record" \
  1 \
  "$TMP_DIR/getting_started_get_post.json" \
  "$TMP_DIR/getting_started_get_post.stderr"
assert_getting_started_post_response "$TMP_DIR/getting_started_get_post.json" "200" "getting_started_get_post"
# inventory-id: getting_started_sdk_crud
run_getting_started_sdk_program "$TMP_DIR/getting_started_sdk.stdout" "$TMP_DIR/getting_started_sdk.stderr"
assert_getting_started_sdk_output "$TMP_DIR/getting_started_sdk.stdout"
assert_dashboard_served "$TMP_DIR/admin.html" "$TMP_DIR/admin_asset.js"
# inventory-id: readme_sdk_install
run_documented_sdk_install "readme_sdk_install" "README.md" "## SDK"
# inventory-id: getting_started_sdk_install
run_documented_sdk_install "getting_started_sdk_install" "docs-site/guide/getting-started.md" "## Use the JavaScript SDK"
assert_npm_registry_package_available
run_quickstart_sdk_program "$TMP_DIR/sdk_run.stdout" "$TMP_DIR/sdk_run.stderr"
assert_quickstart_sdk_output "$TMP_DIR/sdk_run.stdout"
run_generated_scaffold_project
assert_generated_scaffold_project "$RUNTIME_WORKDIR/todo-scaffold" "$TMP_DIR/scaffold_init.stdout" "$TMP_DIR/scaffold_run.stdout"
assert_auth_me_disabled "$TMP_DIR/auth_me_disabled.json"

# inventory-id: quickstart_realtime
extract_doc_block "docs-site/guide/quickstart.md" "## 6. Add realtime" js 1 \
  | rewrite_doc_command >"$RUNTIME_WORKDIR/todo-app/realtime.mjs"
node "$RUNTIME_WORKDIR/todo-app/realtime.mjs" >"$TMP_DIR/realtime.stdout" 2>"$TMP_DIR/realtime.stderr" &
REALTIME_PID="$!"
wait_for_realtime_log_entry \
  "Listening for todo changes... (Ctrl-C to stop)" \
  "" \
  "listener readiness"

REALTIME_CREATED_TITLE="Quickstart realtime event"
if ! curl -sS -m 5 -w '\n__AYB_HTTP_CODE:%{http_code}\n' \
  -X POST "$AYB_BASE_URL/api/collections/todos" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"$REALTIME_CREATED_TITLE\"}" >"$TMP_DIR/realtime_create.response"; then
  fail "quickstart_realtime: todo create request failed"
fi
extract_http_json_response \
  "$TMP_DIR/realtime_create.response" \
  "201" \
  "quickstart_realtime" >"$TMP_DIR/realtime_create.json"
python3 - "$TMP_DIR/realtime_create.json" "$REALTIME_CREATED_TITLE" <<'PY'
import json
import sys

body = json.load(open(sys.argv[1], encoding="utf-8"))
expected_title = sys.argv[2]
actual_title = body.get("title")
if actual_title != expected_title:
    raise SystemExit(
        f"quickstart_realtime: created title mismatch: "
        f"got {actual_title!r}, want {expected_title!r}; body={body!r}"
    )
PY
wait_for_realtime_log_entry \
  "[create]" \
  "$REALTIME_CREATED_TITLE" \
  "documented [create] event with inserted title"
kill -0 "$REALTIME_PID" 2>/dev/null \
  || fail "quickstart_realtime: listener exited after receiving the create event"

restart_ayb_with_auth_enabled
register_quickstart_auth_fixtures

if [ -n "${AYB_QUICKSTART_PREREPAIR_SDK_BASELINE_REF:-}" ]; then
  capture_prerepair_readme_sdk_baseline \
    "$AYB_QUICKSTART_PREREPAIR_SDK_BASELINE_REF" \
    "${AYB_QUICKSTART_PREREPAIR_SDK_BASELINE_DIR:-$TMP_DIR/prerepair_readme_sdk_baseline}"
  exit 0
fi

# inventory-id: readme_api_authenticated_crud
run_readme_api_authenticated_crud \
  "$TMP_DIR/readme_authenticated_crud.stdout" \
  "$TMP_DIR/readme_authenticated_crud.stderr"
assert_readme_api_authenticated_crud_response "$TMP_DIR/readme_authenticated_crud.stdout"
assert_readme_api_authenticated_read_and_rejects_unauthorized_create
# inventory-id: readme_sdk_program
run_readme_sdk_program "$TMP_DIR/readme_sdk.stdout" "$TMP_DIR/readme_sdk.stderr"
assert_readme_sdk_output "$TMP_DIR/readme_sdk.stdout"

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
