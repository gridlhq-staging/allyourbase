#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/bash_assert_helpers.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

target_recipe="$tmp_dir/target_recipe.txt"
awk '
  /^test-sdk-integration:/ { in_target = 1 }
  in_target { print }
  in_target && /^$/ { exit }
' Makefile > "$target_recipe"

assert_contains "$target_recipe" "source tests/port_helpers.sh" \
  "SDK integration target should reuse the shared free-port selector"
assert_contains "$target_recipe" "pick_free_port 48091 49091 50091 51091 52091" \
  "SDK integration target should select from isolated high ports"
assert_contains "$target_recipe" 'export AYB_SERVER_PORT' \
  "SDK integration target should export its selected port to AYB"
assert_contains "$target_recipe" "pick_free_port 45433 46433 47433 48433 49433" \
  "SDK integration target should select an isolated embedded Postgres port"
assert_contains "$target_recipe" 'export AYB_DATABASE_EMBEDDED_PORT' \
  "SDK integration target should export its selected embedded Postgres port"
assert_contains "$target_recipe" 'bash scripts/run-with-ayb.sh' \
  "SDK integration target should start AYB through the shared runner"

commands_dir="$tmp_dir/bin"
mkdir -p "$commands_dir"
cat > "$commands_dir/lsof" <<'SH'
#!/usr/bin/env bash
for arg in "$@"; do
  case "$arg" in
    :8090|:15432|:48091|:45433)
      printf '4242\n'
      exit 0
      ;;
  esac
done
exit 1
SH
chmod +x "$commands_dir/lsof"

# The probes below call pick_free_port for real, which writes lease markers.
# Point them at a lease directory under tmp_dir so this contract never leaves
# markers in the host-shared default namespace, where they would be visible to
# every other process running as this uid. The EXIT trap removes it.
export AYB_PORT_LEASE_DIR="$tmp_dir/port_leases"

selected_port="$({ PATH="$commands_dir:$PATH"; source tests/port_helpers.sh; pick_free_port 48091 49091 50091 51091 52091; })"
if [[ "$selected_port" != "49091" ]]; then
  fail "SDK integration port selection should bypass occupied ports; got '$selected_port'"
fi

selected_database_port="$({ PATH="$commands_dir:$PATH"; source tests/port_helpers.sh; pick_free_port 45433 46433 47433 48433 49433; })"
if [[ "$selected_database_port" != "46433" ]]; then
  fail "SDK integration database port selection should bypass occupied ports; got '$selected_database_port'"
fi

echo "PASS: SDK integration target isolates its AYB server and embedded Postgres ports"
