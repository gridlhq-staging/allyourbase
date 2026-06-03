#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/bash_assert_helpers.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

commands_dir="$tmp_dir/bin"
mkdir -p "$commands_dir"

ayb_log="$tmp_dir/ayb.log"

cat > "$commands_dir/ayb" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${AYB_TEST_LOG:?}"
case "${1:-}" in
  version)
    printf 'test-ayb\n'
    ;;
  stop)
    exit 0
    ;;
  demo)
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
case ",${AYB_TEST_OCCUPIED_PORTS:-}," in
  *,"$requested_port",*)
    printf '4242\n'
    exit 0
    ;;
esac
exit 1
SH

cat > "$commands_dir/curl" <<'SH'
#!/usr/bin/env bash
exit 0
SH

cat > "$commands_dir/npm" <<'SH'
#!/usr/bin/env bash
exit 1
SH

cat > "$commands_dir/node" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${AYB_TEST_NODE_LOG:?}"
sleep 30
SH

chmod +x "$commands_dir/ayb" "$commands_dir/lsof" "$commands_dir/curl" "$commands_dir/npm" "$commands_dir/node"

output="$tmp_dir/output.log"
node_log="$tmp_dir/node.log"

if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_OCCUPIED_PORTS="8090" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh kanban > "$output" 2>&1; then
  fail "demo E2E runner should fail when a required port is occupied"
fi

assert_contains "$output" "refusing to kill an unknown process" "runner should report the occupied-port guard"
if grep -Fxq "demo kanban" "$ayb_log"; then
  fail "runner should abort before launching a demo when preflight ports are occupied"
fi

: > "$ayb_log"
if PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" AYB_TEST_NODE_LOG="$node_log" AYB_TEST_OCCUPIED_PORTS="11434" bash _dev/manual_smoke_tests/18_demo_e2e.test.sh movies > "$output" 2>&1; then
  fail "movies demo E2E runner should fail when fake ollama port 11434 is occupied"
fi

assert_contains "$output" "movies fake ollama port 11434 is already occupied" "runner should report the movies fake ollama port guard"
if [ -s "$node_log" ]; then
  fail "runner should abort before launching the fake ollama server when port 11434 is occupied"
fi
if grep -Fxq "demo movies" "$ayb_log"; then
  fail "runner should abort before launching the movies demo when fake ollama port 11434 is occupied"
fi

echo "PASS: demo E2E runner aborts on occupied managed ports, including fake ollama"
