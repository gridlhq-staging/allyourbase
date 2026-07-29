#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/bash_assert_helpers.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

commands_dir="$tmp_dir/bin"
mkdir -p "$commands_dir"

ayb_log="$tmp_dir/ayb.log"
npm_log="$tmp_dir/npm.log"

cat > "$commands_dir/ayb" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${AYB_TEST_LOG:?}"
case "${1:-}" in
  version)
    printf 'test-ayb\n'
    ;;
  stop)
    if [ -n "${AYB_TEST_STOP_MARKER:-}" ]; then
      : > "$AYB_TEST_STOP_MARKER"
    fi
    exit 0
    ;;
  demo)
    printf 'demo %s data_dir=%s server_port=%s pg_port=%s home=%s\n' \
      "${2:-}" "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" "${AYB_SERVER_PORT:-}" \
      "${AYB_DATABASE_EMBEDDED_PORT:-}" "${HOME:-}" >> "${AYB_TEST_LOG:?}"
    sleep 30
    ;;
esac
SH

cat > "$commands_dir/lsof" <<'SH'
#!/usr/bin/env bash
requested_port=""
for arg in "$@"; do
  case "$arg" in
    :*)
      requested_port="${arg#:}"
      ;;
  esac
done
if [ -n "${AYB_TEST_DELAYED_PORT:-}" ] &&
    [ "$requested_port" = "$AYB_TEST_DELAYED_PORT" ] &&
    [ -f "${AYB_TEST_STOP_MARKER:-}" ] &&
    [ ! -f "${AYB_TEST_RELEASE_MARKER:-}" ]; then
  delayed_checks=0
  if [ -f "${AYB_TEST_DELAYED_COUNTER:-}" ]; then
    delayed_checks="$(cat "$AYB_TEST_DELAYED_COUNTER")"
  fi
  delayed_checks=$((delayed_checks + 1))
  printf '%s\n' "$delayed_checks" > "${AYB_TEST_DELAYED_COUNTER:?}"
  printf '4343\n'
  exit 0
fi
case ",${AYB_TEST_OCCUPIED_PORTS:-}," in
  *,"$requested_port",*)
    printf '4242\n'
    exit 0
    ;;
esac
exit 1
SH

cat > "$commands_dir/sleep" <<'SH'
#!/usr/bin/env bash
if [ "${1:-}" = "0.5" ] &&
    [ -n "${AYB_TEST_RELEASE_MARKER:-}" ] &&
    [ -f "${AYB_TEST_STOP_MARKER:-}" ]; then
  : > "$AYB_TEST_RELEASE_MARKER"
  exit 0
fi
exec /bin/sleep "$@"
SH

cat > "$commands_dir/curl" <<'SH'
#!/usr/bin/env bash
exit 0
SH

cat > "$commands_dir/npm" <<'SH'
#!/usr/bin/env bash
printf '%s ayb_server_url=%s\n' "$*" "${AYB_SERVER_URL:-}" >> "${AYB_TEST_NPM_LOG:?}"
exit 1
SH

cat > "$commands_dir/node" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${AYB_TEST_NODE_LOG:?}"
sleep 30
SH

chmod +x "$commands_dir/ayb" "$commands_dir/lsof" "$commands_dir/sleep" "$commands_dir/curl" "$commands_dir/npm" "$commands_dir/node"

output="$tmp_dir/output.log"
node_log="$tmp_dir/node.log"

if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NPM_LOG="$npm_log" AYB_TEST_OCCUPIED_PORTS="8090" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "demo E2E runner should reach the intentional fake npm failure"
fi

assert_not_contains "$output" "port 8090 is still occupied" "foreign port 8090 should not block the demo E2E runner"
assert_contains "$output" "npm ci failed" "runner should progress past occupied 8090 to the fake npm failure"
kanban_line="$(awk '/^demo kanban / {print; exit}' "$ayb_log")"
case "$kanban_line" in
  *"server_port=48090"*|*"server_port=49090"*|*"server_port=50090"*|*"server_port=51090"*|*"server_port=52090"*) ;;
  *) fail "runner should launch kanban with an isolated AYB server port, got '$kanban_line'" ;;
esac
case "$kanban_line" in
  *"pg_port=45432"*|*"pg_port=46432"*|*"pg_port=47432"*|*"pg_port=48432"*|*"pg_port=49432"*) ;;
  *) fail "runner should launch kanban with an isolated embedded Postgres port, got '$kanban_line'" ;;
esac
case "$(cat "$npm_log")" in
  *"ayb_server_url=http://127.0.0.1:48090"*|*"ayb_server_url=http://127.0.0.1:49090"*|*"ayb_server_url=http://127.0.0.1:50090"*|*"ayb_server_url=http://127.0.0.1:51090"*|*"ayb_server_url=http://127.0.0.1:52090"*) ;;
  *) fail "runner should expose the isolated AYB server URL to the kanban Vite process" ;;
esac

: > "$ayb_log"
stop_marker="$tmp_dir/stop.marker"
delayed_counter="$tmp_dir/delayed.counter"
release_marker="$tmp_dir/release.marker"
if PATH="$commands_dir:$PATH" \
    AYB_BIN="$commands_dir/ayb" \
    AYB_TEST_LOG="$ayb_log" \
    AYB_TEST_NPM_LOG="$npm_log" \
    AYB_TEST_STOP_MARKER="$stop_marker" \
    AYB_TEST_DELAYED_COUNTER="$delayed_counter" \
    AYB_TEST_DELAYED_PORT="45432" \
    AYB_TEST_RELEASE_MARKER="$release_marker" \
    bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "kanban demo E2E runner should reach the intentional fake npm failure"
fi

assert_contains "$output" "npm ci failed" "runner should reach the fake npm failure before delayed cleanup"
assert_not_contains "$output" "port 45432 is still occupied after ayb stop" "runner should wait for managed Postgres to release its port"
if [ ! -f "$release_marker" ] || [ "$(cat "$delayed_counter")" -lt 1 ]; then
  fail "runner should retry the delayed managed Postgres port probe"
fi

: > "$ayb_log"
before_demo_dirs="$(ls -d /tmp/ayb-demoe2e.* 2>/dev/null | sort || true)"
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NPM_LOG="$npm_log" AYB_TEST_NODE_LOG="$node_log" AYB_TEST_OCCUPIED_PORTS="11434" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh movies > "$output" 2>&1; then
  fail "movies demo E2E runner should fail when fake ollama port 11434 is occupied"
fi
after_demo_dirs="$(ls -d /tmp/ayb-demoe2e.* 2>/dev/null | sort || true)"

assert_contains "$output" "movies fake ollama port 11434 is already occupied" "runner should report the movies fake ollama port guard"
if [ -s "$node_log" ]; then
  fail "runner should abort before launching the fake ollama server when port 11434 is occupied"
fi
if grep -Fxq "demo movies" "$ayb_log"; then
  fail "runner should abort before launching the movies demo when fake ollama port 11434 is occupied"
fi
if [ "$after_demo_dirs" != "$before_demo_dirs" ]; then
  fail "runner should remove isolated embedded data dir when fake ollama port is occupied"
fi

: > "$ayb_log"
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NPM_LOG="$npm_log" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "kanban demo E2E runner should fail once fake npm fails"
fi

kanban_line="$(awk '/^demo kanban / {print; exit}' "$ayb_log")"
data_dir="$(printf '%s\n' "$kanban_line" | sed -E 's/^.* data_dir=([^ ]+) .*$/\1/')"
runtime_home="$(printf '%s\n' "$kanban_line" | sed -E 's/^.* home=([^ ]+)$/\1/')"
case "$data_dir" in
  /tmp/ayb-demoe2e.*) ;;
  *) fail "runner should launch demos with a short isolated embedded data dir, got '$data_dir'" ;;
esac
if [ -e "$data_dir" ]; then
  fail "runner should remove isolated embedded data dir during failure cleanup"
fi
case "$runtime_home" in
  /tmp/ayb-demohome.*) ;;
  *) fail "runner should launch demos with an isolated runtime home, got '$runtime_home'" ;;
esac
if [ -e "$runtime_home" ]; then
  fail "runner should remove isolated runtime home during failure cleanup"
fi

echo "PASS: demo E2E runner isolates shared runtime ports and guards fake ollama"
