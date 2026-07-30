#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/bash_assert_helpers.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

commands_dir="$tmp_dir/bin"
mkdir -p "$commands_dir"

ayb_log="$tmp_dir/ayb.log"
npm_log="$tmp_dir/npm.log"
start_log="$tmp_dir/start.log"
lsof_log="$tmp_dir/lsof.log"
curl_log="$tmp_dir/curl.log"

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
    if [ -n "${AYB_TEST_SERVER_READY_MARKER:-}" ]; then
      rm -f "$AYB_TEST_SERVER_READY_MARKER"
    fi
    exit 0
    ;;
  start)
    config_base_url=""
    if [ -f "${3:-}" ]; then
      config_base_url="$(sed -n 's/^[[:space:]]*base_url = "\(http:\/\/127[.]0[.]0[.]1:[0-9][0-9]*\)".*$/\1/p' "$3")"
    fi
    printf 'start config=%s data_dir=%s server_port=%s pg_port=%s home=%s auth=%s jwt=%s anon=%s magic=%s site=%s\n' \
      "${3:-}" "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" "${AYB_SERVER_PORT:-}" \
      "${AYB_DATABASE_EMBEDDED_PORT:-}" "${HOME:-}" "${AYB_AUTH_ENABLED:-}" "${AYB_AUTH_JWT_SECRET:-}" \
      "${AYB_AUTH_ANONYMOUS_AUTH_ENABLED:-}" "${AYB_AUTH_MAGIC_LINK_ENABLED:-}" \
      "${AYB_SERVER_SITE_URL:-}" >> "${AYB_TEST_START_LOG:?}"
    printf 'start_config_base_url=%s\n' "$config_base_url" >> "${AYB_TEST_START_LOG:?}"
    if [ -n "${AYB_TEST_SERVER_READY_MARKER:-}" ]; then
      : > "$AYB_TEST_SERVER_READY_MARKER"
    fi
    ;;
  demo)
    printf 'demo %s data_dir=%s server_port=%s pg_port=%s home=%s jwt=%s\n' \
      "${2:-}" "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" "${AYB_SERVER_PORT:-}" \
      "${AYB_DATABASE_EMBEDDED_PORT:-}" "${HOME:-}" "${AYB_AUTH_JWT_SECRET:-}" >> "${AYB_TEST_LOG:?}"
    if [ "${AYB_TEST_DEMO_EXIT_IMMEDIATELY:-0}" = "1" ]; then
      exit 0
    fi
    sleep 30
    ;;
esac
SH

cat > "$commands_dir/lsof" <<'SH'
#!/usr/bin/env bash
if [ -n "${AYB_TEST_LSOF_LOG:-}" ]; then
  printf '%s\n' "$*" >> "$AYB_TEST_LSOF_LOG"
fi
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
if [ -n "${AYB_TEST_CURL_LOG:-}" ]; then
  printf '%s\n' "$*" >> "$AYB_TEST_CURL_LOG"
fi
if [ -n "${AYB_TEST_NODE_LOG:-}" ]; then
  case "$*" in
    *"http://127.0.0.1:45514/health"*|*"http://127.0.0.1:46514/health"*|*"http://127.0.0.1:47514/health"*|*"http://127.0.0.1:48514/health"*|*"http://127.0.0.1:49514/health"*)
      [ -s "$AYB_TEST_NODE_LOG" ]
      exit
      ;;
  esac
fi
if [ -n "${AYB_TEST_SERVER_READY_MARKER:-}" ]; then
  case "$*" in
    *"http://127.0.0.1:8092/health"*)
      [ -f "$AYB_TEST_SERVER_READY_MARKER" ]
      exit
      ;;
  esac
fi
exit 0
SH

cat > "$commands_dir/npm" <<'SH'
#!/usr/bin/env bash
printf '%s ayb_server_url=%s\n' "$*" "${AYB_SERVER_URL:-}" >> "${AYB_TEST_NPM_LOG:?}"
exit 1
SH

cat > "$commands_dir/node" <<'SH'
#!/usr/bin/env bash
printf '%s fixture_port=%s\n' "$*" "${AYB_MOVIES_FAKE_OLLAMA_PORT:-}" >> "${AYB_TEST_NODE_LOG:?}"
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
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NPM_LOG="$npm_log" AYB_MOVIES_FAKE_OLLAMA_PORT=not-a-port bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "kanban demo E2E runner should fail once fake npm fails"
fi

assert_contains "$output" "npm ci failed" "kanban-only run should ignore invalid movies fixture port env and reach fake npm"
assert_not_contains "$output" "AYB_MOVIES_FAKE_OLLAMA_PORT must" "kanban-only run should not validate the movies fixture port"

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
: > "$node_log"
before_demo_dirs="$(ls -d /tmp/ayb-demoe2e.* 2>/dev/null | sort || true)"
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_START_LOG="$start_log" AYB_TEST_NPM_LOG="$npm_log" AYB_TEST_NODE_LOG="$node_log" AYB_TEST_OCCUPIED_PORTS="11434" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh movies > "$output" 2>&1; then
  fail "movies demo E2E runner should reach the intentional fake npm failure"
fi
after_demo_dirs="$(ls -d /tmp/ayb-demoe2e.* 2>/dev/null | sort || true)"

assert_not_contains "$output" "movies fake ollama port 11434 is already occupied" "occupied real Ollama default should not block fake movies fixture"
assert_contains "$output" "npm ci failed" "runner should progress past occupied 11434 to the fake npm failure"
if ! grep -q 'fake_ollama_server.cjs' "$node_log"; then
  fail "runner should launch the fake ollama server when only 11434 is occupied"
fi
if ! grep -q '^start config=/tmp/ayb-demoe2e[.]' "$start_log"; then
  fail "runner should pre-start AYB with a temporary movies config"
fi
start_line="$(awk '/^start / {print; exit}' "$start_log")"
case "$start_line" in
  *"auth=true"*"jwt="*"anon=true"*"magic=true"*"site=http://localhost:45177"*|*"auth=true"*"jwt="*"anon=true"*"magic=true"*"site=http://localhost:46177"*|*"auth=true"*"jwt="*"anon=true"*"magic=true"*"site=http://localhost:47177"*|*"auth=true"*"jwt="*"anon=true"*"magic=true"*"site=http://localhost:48177"*|*"auth=true"*"jwt="*"anon=true"*"magic=true"*"site=http://localhost:49177"*) ;;
  *) fail "runner should pre-start movies AYB with auth, anonymous auth, magic link, and matching site URL, got '$start_line'" ;;
esac
start_jwt="$(printf '%s\n' "$start_line" | sed -n 's/^.* jwt=\([^ ]*\) .*$/\1/p')"
if [ -z "$start_jwt" ]; then
  fail "runner should pre-start movies AYB with a non-empty per-run JWT secret, got '$start_line'"
fi
assert_not_contains "$start_log" "movies_demo_super_secret_key_32_chars" "runner should not use a fixed movies JWT secret"
if [ "$after_demo_dirs" != "$before_demo_dirs" ]; then
  fail "runner should remove isolated embedded data dir after occupied-11434 movies run"
fi

: > "$ayb_log"
: > "$node_log"
: > "$start_log"
if PATH="$commands_dir:$PATH" \
    AYB_BIN="$commands_dir/ayb" \
    AYB_TEST_LOG="$ayb_log" \
    AYB_TEST_START_LOG="$start_log" \
    AYB_TEST_NPM_LOG="$npm_log" \
    AYB_TEST_NODE_LOG="$node_log" \
    AYB_MOVIES_FAKE_OLLAMA_PORT=45514 \
    AYB_TEST_OCCUPIED_PORTS="45514" \
    bash _dev/manual_smoke_tests/18_demo_e2e.test.sh movies > "$output" 2>&1; then
  fail "movies demo E2E runner should fail when the selected fake ollama port is occupied"
fi

assert_contains "$output" "movies fake ollama port 45514 is already occupied" "runner should report the selected fake ollama port guard"
if [ -s "$node_log" ]; then
  fail "runner should abort before launching the fake ollama server when the selected fixture port is occupied"
fi
if [ -s "$start_log" ]; then
  fail "runner should abort before pre-starting AYB when the selected fixture port is occupied"
fi
if grep -Fxq "demo movies" "$ayb_log"; then
  fail "runner should abort before launching the movies demo when the selected fixture port is occupied"
fi

: > "$ayb_log"
: > "$node_log"
: > "$start_log"
: > "$lsof_log"
: > "$curl_log"
oversized_port="18446744073709551617"
if PATH="$commands_dir:$PATH" \
    AYB_BIN="$commands_dir/ayb" \
    AYB_TEST_LOG="$ayb_log" \
    AYB_TEST_START_LOG="$start_log" \
    AYB_TEST_NPM_LOG="$npm_log" \
    AYB_TEST_NODE_LOG="$node_log" \
    AYB_TEST_LSOF_LOG="$lsof_log" \
    AYB_TEST_CURL_LOG="$curl_log" \
    AYB_MOVIES_FAKE_OLLAMA_PORT="$oversized_port" \
    bash _dev/manual_smoke_tests/18_demo_e2e.test.sh movies > "$output" 2>&1; then
  fail "movies demo E2E runner should reject an oversized fixture port override"
fi

assert_contains "$output" "AYB_MOVIES_FAKE_OLLAMA_PORT must be in the range 1..65535" "main runner should clearly reject an oversized fixture port"
assert_not_contains "$output" "integer expression expected" "main runner should reject an oversized fixture port before Bash numeric comparison"
assert_not_contains "$output" "integer expected" "main runner should reject an oversized fixture port without integer diagnostics"
if [ -s "$node_log" ] || [ -s "$start_log" ]; then
  fail "main runner should reject an oversized fixture port before launching Node or AYB"
fi
if [ -s "$lsof_log" ] || [ -s "$curl_log" ]; then
  fail "main runner should reject an oversized fixture port before calling lsof or curl"
fi

: > "$node_log"
: > "$lsof_log"
: > "$curl_log"
if PATH="$commands_dir:$PATH" \
    AYB_TEST_NODE_LOG="$node_log" \
    AYB_TEST_LSOF_LOG="$lsof_log" \
    AYB_TEST_CURL_LOG="$curl_log" \
    AYB_MOVIES_FAKE_OLLAMA_PORT="$oversized_port" \
    bash examples/movies/e2e/run_demo_with_fake_ollama.sh > "$output" 2>&1; then
  fail "standalone movies runner should reject an oversized fixture port override"
fi

assert_contains "$output" "AYB_MOVIES_FAKE_OLLAMA_PORT must be in the range 1..65535" "standalone runner should clearly reject an oversized fixture port"
assert_not_contains "$output" "integer expression expected" "standalone runner should reject an oversized fixture port before Bash numeric comparison"
assert_not_contains "$output" "integer expected" "standalone runner should reject an oversized fixture port without integer diagnostics"
if [ -s "$node_log" ]; then
  fail "standalone runner should reject an oversized fixture port before launching Node"
fi
if [ -s "$lsof_log" ] || [ -s "$curl_log" ]; then
  fail "standalone runner should reject an oversized fixture port before calling lsof or curl"
fi

: > "$ayb_log"
: > "$node_log"
: > "$start_log"
server_ready_marker="$tmp_dir/server.ready"
if ! PATH="$commands_dir:$PATH" \
    AYB_BIN="$commands_dir/ayb" \
    AYB_TEST_LOG="$ayb_log" \
    AYB_TEST_START_LOG="$start_log" \
    AYB_TEST_NODE_LOG="$node_log" \
    AYB_TEST_DEMO_EXIT_IMMEDIATELY=1 \
    AYB_TEST_SERVER_READY_MARKER="$server_ready_marker" \
    bash examples/movies/e2e/run_demo_with_fake_ollama.sh > "$output" 2>&1; then
  fail "standalone movies runner should complete its mocked success path: $(cat "$output")"
fi

standalone_fixture_port="$(sed -n 's/^.* fixture_port=\([0-9][0-9]*\)$/\1/p' "$node_log")"
case "$standalone_fixture_port" in
  45514|46514|47514|48514|49514) ;;
  *) fail "standalone runner should launch the fixture on an isolated selected port, got '$standalone_fixture_port'" ;;
esac
assert_contains "$start_log" "start_config_base_url=http://127.0.0.1:${standalone_fixture_port}" "standalone runner should point its temporary config at the selected fixture port"
assert_not_contains "$start_log" "movies_demo_super_secret_key_32_chars" "standalone runner should not use a fixed movies JWT secret"
standalone_start_line="$(awk '/^start / {print; exit}' "$start_log")"
standalone_start_jwt="$(printf '%s\n' "$standalone_start_line" | sed -n 's/^.* jwt=\([^ ]*\) .*$/\1/p')"
if [ -z "$standalone_start_jwt" ]; then
  fail "standalone runner should pre-start AYB with a non-empty per-run JWT secret, got '$standalone_start_line'"
fi
standalone_config="$(sed -n 's/^start config=\([^ ]*\).*$/\1/p' "$start_log")"
if [ -z "$standalone_config" ] || [ -e "$standalone_config" ]; then
  fail "standalone runner should clean up its temporary config, got '$standalone_config'"
fi

: > "$ayb_log"
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NPM_LOG="$npm_log" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "kanban demo E2E runner should fail once fake npm fails"
fi

kanban_line="$(awk '/^demo kanban / {print; exit}' "$ayb_log")"
data_dir="$(printf '%s\n' "$kanban_line" | sed -E 's/^.* data_dir=([^ ]+) .*$/\1/')"
runtime_home="$(printf '%s\n' "$kanban_line" | sed -E 's/^.* home=([^ ]+) jwt=.*$/\1/')"
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
