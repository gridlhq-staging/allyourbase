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
    if [ "${2:-}" = "nonexistent" ]; then
      printf 'unknown demo: nonexistent\n' >&2
      exit 1
    fi
    printf 'demo %s data_dir=%s server_port=%s pg_port=%s home=%s\n' \
      "${2:-}" "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" "${AYB_SERVER_PORT:-}" \
      "${AYB_DATABASE_EMBEDDED_PORT:-}" "${HOME:-}" >> "${AYB_TEST_LOG:?}"
    printf 'Allyourbase Demo\nAccounts:\nCtrl+C\n'
    sleep 0.2
    ;;
esac
SH

cat > "$commands_dir/curl" <<'SH'
#!/usr/bin/env bash
output_file=""
write_status=0
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output_file="$2"
      shift 2
      ;;
    -w)
      write_status=1
      shift 2
      ;;
    http://*)
      url="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

case "$url" in
  */api/schema)
    printf '{"error":"authorization required"}' > "$output_file"
    if [ "$write_status" -eq 1 ]; then
      printf '401'
    fi
    ;;
  *)
    printf '<!doctype html><html></html>'
    ;;
esac
SH

cat > "$commands_dir/lsof" <<'SH'
#!/usr/bin/env bash
for arg in "$@"; do
  if [ "$arg" = ":8090" ]; then
    printf '4242\n'
    exit 0
  fi
done
exit 1
SH

chmod +x "$commands_dir/ayb" "$commands_dir/curl" "$commands_dir/lsof"

output="$tmp_dir/output.log"
if ! PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" bash _dev/manual_smoke_tests/17_demo_launch.test.sh > "$output" 2>&1; then
  cat "$output"
  fail "demo launch runner contract should complete with stubs"
fi

for demo in kanban live-polls movies; do
  demo_line="$(awk -v demo="$demo" '$0 ~ "^demo " demo " " {print; exit}' "$ayb_log")"
  data_dir="$(printf '%s\n' "$demo_line" | sed -E 's/^.* data_dir=([^ ]+) .*$/\1/')"
  server_port="$(printf '%s\n' "$demo_line" | sed -E 's/^.* server_port=([^ ]+) .*$/\1/')"
  pg_port="$(printf '%s\n' "$demo_line" | sed -E 's/^.* pg_port=([^ ]+) .*$/\1/')"
  runtime_home="$(printf '%s\n' "$demo_line" | sed -E 's/^.* home=([^ ]+)$/\1/')"
  case "$data_dir" in
    /tmp/ayb-demoe2e.*) ;;
    *) fail "demo launch runner should launch $demo with a short isolated embedded data dir, got '$data_dir'" ;;
  esac
  if [ -e "$data_dir" ]; then
    fail "demo launch runner should remove $demo isolated embedded data dir during cleanup"
  fi
  case "$server_port" in
    48090|49090|50090|51090|52090) ;;
    *) fail "demo launch runner should isolate $demo AYB server port, got '$server_port'" ;;
  esac
  case "$pg_port" in
    45432|46432|47432|48432|49432) ;;
    *) fail "demo launch runner should isolate $demo embedded Postgres port, got '$pg_port'" ;;
  esac
  case "$runtime_home" in
    /tmp/ayb-demohome.*) ;;
    *) fail "demo launch runner should isolate $demo runtime home, got '$runtime_home'" ;;
  esac
  if [ -e "$runtime_home" ]; then
    fail "demo launch runner should remove $demo isolated runtime home during cleanup"
  fi
done

assert_not_contains "$output" "port 8090 is still occupied" "foreign port 8090 should not block the demo launch runner"

echo "PASS: demo launch runner isolates server, database, app, and runtime state"
