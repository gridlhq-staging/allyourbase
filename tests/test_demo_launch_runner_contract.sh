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
    printf 'demo %s data_dir=%s\n' "${2:-}" "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" >> "${AYB_TEST_LOG:?}"
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
exit 1
SH

chmod +x "$commands_dir/ayb" "$commands_dir/curl" "$commands_dir/lsof"

output="$tmp_dir/output.log"
if ! PATH="$commands_dir:$PATH" AYB_BIN="$commands_dir/ayb" AYB_TEST_LOG="$ayb_log" bash _dev/manual_smoke_tests/17_demo_launch.test.sh > "$output" 2>&1; then
  cat "$output"
  fail "demo launch runner contract should complete with stubs"
fi

for demo in kanban live-polls movies; do
  data_dir="$(awk -v demo="$demo" -F'data_dir=' '$0 ~ "^demo " demo " data_dir=" {print $2; exit}' "$ayb_log")"
  case "$data_dir" in
    /tmp/ayb-demoe2e.*) ;;
    *) fail "demo launch runner should launch $demo with a short isolated embedded data dir, got '$data_dir'" ;;
  esac
  if [ -e "$data_dir" ]; then
    fail "demo launch runner should remove $demo isolated embedded data dir during cleanup"
  fi
done

echo "PASS: demo launch runner isolates embedded data dirs"
