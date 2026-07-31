#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/bash_assert_helpers.sh"
source "$SCRIPT_DIR/port_helpers.sh"

TMP_DIR="$(mktemp -d)"
BACKGROUND_PIDS=""
BASE_PORT_LEASE_DIR="${AYB_PORT_LEASE_DIR:-$TMP_DIR/port_leases}"

cleanup() {
  for pid in $BACKGROUND_PIDS; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

track_background_pid() {
  BACKGROUND_PIDS="${BACKGROUND_PIDS} $1"
}

lease_case_dir() {
  local case_name="$1"
  local lease_dir="$BASE_PORT_LEASE_DIR/$case_name"

  rm -rf "$lease_dir"
  mkdir -p "$lease_dir"
  printf '%s\n' "$lease_dir"
}

use_lease_case() {
  AYB_PORT_LEASE_DIR="$(lease_case_dir "$1")"
  export AYB_PORT_LEASE_DIR
}

forget_background_pid() {
  local completed_pid="$1"
  local pid
  local remaining=""

  for pid in $BACKGROUND_PIDS; do
    if [ "$pid" != "$completed_pid" ]; then
      remaining="${remaining} ${pid}"
    fi
  done
  BACKGROUND_PIDS="$remaining"
}

start_listener() {
  local requested_port="${1:-0}"
  local ready_file
  ready_file="$(mktemp "$TMP_DIR/port.XXXXXX")" || fail "failed to allocate listener ready file"

  python3 - "$requested_port" "$ready_file" <<'PY' &
import pathlib
import socketserver
import sys

requested_port = int(sys.argv[1])
ready_file = pathlib.Path(sys.argv[2])

with socketserver.TCPServer(("127.0.0.1", requested_port), socketserver.BaseRequestHandler) as server:
    ready_file.write_text(str(server.server_address[1]))
    server.serve_forever()
PY
  local pid="$!"
  track_background_pid "$pid"

  for _ in $(seq 1 50); do
    if [ -s "$ready_file" ]; then
      STARTED_PORT="$(cat "$ready_file")"
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      fail "listener on requested port $requested_port exited before becoming ready"
    fi
    sleep 0.1
  done

  fail "listener on requested port $requested_port did not become ready"
}

pick_unused_port() {
  python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
}

path_mode_octal() {
  python3 - "$1" <<'PY'
import os
import sys

print(oct(os.stat(sys.argv[1]).st_mode & 0o777))
PY
}

dead_numeric_pid() {
  local candidate=999999
  local kill_err

  while [ "$candidate" -gt 1 ]; do
    # A successful kill -0 means a live process owns the PID and can be
    # signalled; skip it. A failure is only proof of death when the errno is
    # ESRCH ("no such process"). EPERM ("operation not permitted") means a
    # live foreign process owns the PID on a multi-user host, so that PID must
    # be rejected -- selecting it would hand the stale-owner test a live
    # specimen. LC_ALL=C pins the strerror text so the match is deterministic.
    if kill_err="$(LC_ALL=C kill -0 "$candidate" 2>&1)"; then
      candidate=$((candidate - 1))
      continue
    fi
    case "$kill_err" in
      *[Nn]o\ such\ process*)
        printf '%s\n' "$candidate"
        return 0
        ;;
    esac
    candidate=$((candidate - 1))
  done

  fail "failed to find a dead numeric PID specimen"
}

install_lsof_barrier_stub() {
  local stub_dir="$1"

  mkdir -p "$stub_dir"
  cat >"$stub_dir/lsof" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

mkdir "$AYB_LSOF_BARRIER_DIR/caller.$$"
for _ in $(seq 1 3000); do
  callers=("$AYB_LSOF_BARRIER_DIR"/caller.*)
  if [ "${#callers[@]}" -ge 2 ] && [ -e "${callers[0]}" ]; then
    exit 1
  fi
  sleep 0.01
done

: >"$AYB_LSOF_BARRIER_DIR/timeout.$$"
echo "lsof barrier timed out before both port selectors inspected a candidate" >&2
exit 2
SH
  chmod +x "$stub_dir/lsof"
}

install_unoccupied_lsof_stub() {
  local stub_dir="$1"

  mkdir -p "$stub_dir"
  cat >"$stub_dir/lsof" <<'SH'
#!/usr/bin/env bash
exit 1
SH
  chmod +x "$stub_dir/lsof"
}

install_reentry_lsof_stub() {
  local stub_dir="$1"

  mkdir -p "$stub_dir"
  cat >"$stub_dir/lsof" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

call_count=0
if [ -f "$AYB_LSOF_CALL_COUNT" ]; then
  call_count="$(cat "$AYB_LSOF_CALL_COUNT")"
fi
call_count=$((call_count + 1))
printf '%s\n' "$call_count" >"$AYB_LSOF_CALL_COUNT"

if [ "$call_count" -eq 2 ] && [ "${2:-}" = ":$AYB_REENTRY_OCCUPIED_PORT" ]; then
  exit 0
fi
exit 1
SH
  chmod +x "$stub_dir/lsof"
}

test_concurrent_port_selection_is_unique() {
  local rounds_run=0
  local collisions_observed=0
  local rounds_required=20
  local candidate_a candidate_b
  local stub_dir="$TMP_DIR/lsof_stub"
  local lease_dir
  local round

  install_lsof_barrier_stub "$stub_dir"
  lease_dir="$(lease_case_dir "collision_leases")"
  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done

  for round in $(seq 1 "$rounds_required"); do
    local barrier_dir="$TMP_DIR/barrier_$round"
    local first_output="$TMP_DIR/round_${round}_first"
    local second_output="$TMP_DIR/round_${round}_second"
    local first_pid second_pid first_port second_port
    local first_status=0
    local second_status=0
    local second_start_delay=0

    mkdir -p "$barrier_dir"
    if [ "$round" -eq 1 ]; then
      # Exercise scheduling delay beyond the old one-second rendezvous budget.
      second_start_delay=2
    fi
    PATH="$stub_dir:$PATH" AYB_LSOF_BARRIER_DIR="$barrier_dir" \
      AYB_PORT_LEASE_DIR="$lease_dir" \
      bash -c 'source "$1"; pick_free_port "$2" "$3"' _ \
      "$SCRIPT_DIR/port_helpers.sh" "$candidate_a" "$candidate_b" >"$first_output" &
    first_pid="$!"
    track_background_pid "$first_pid"
    PATH="$stub_dir:$PATH" AYB_LSOF_BARRIER_DIR="$barrier_dir" \
      AYB_PORT_LEASE_DIR="$lease_dir" \
      bash -c 'sleep "$4"; source "$1"; pick_free_port "$2" "$3"' _ \
      "$SCRIPT_DIR/port_helpers.sh" "$candidate_a" "$candidate_b" "$second_start_delay" >"$second_output" &
    second_pid="$!"
    track_background_pid "$second_pid"

    wait "$first_pid" || first_status=$?
    forget_background_pid "$first_pid"
    wait "$second_pid" || second_status=$?
    forget_background_pid "$second_pid"
    if [ "$first_status" -ne 0 ] || [ "$second_status" -ne 0 ]; then
      echo "FAIL: lsof barrier worker failed in round $round (first_status=$first_status second_status=$second_status)"
      return 1
    fi
    if compgen -G "$barrier_dir/timeout.*" >/dev/null; then
      echo "FAIL: lsof barrier timed out in round $round before both selectors inspected candidate $candidate_a"
      return 1
    fi

    first_port="$(cat "$first_output")"
    second_port="$(cat "$second_output")"
    rounds_run=$((rounds_run + 1))
    if [ "$first_port" = "$second_port" ]; then
      collisions_observed=$((collisions_observed + 1))
    fi
  done

  echo "port_collision_oracle rounds_run=$rounds_run collisions_observed=$collisions_observed"
  if [ "$rounds_run" -eq 0 ]; then
    echo "VACUOUS: concurrent port selection ran zero rounds"
    return 1
  fi
  if [ "$collisions_observed" -ne 0 ]; then
    echo "FAIL: concurrent pick_free_port callers selected the same port in $collisions_observed of $rounds_run rounds"
    return 1
  fi
}

test_live_foreign_lease_is_not_handed_out() {
  local candidate_a candidate_b selected_port live_pid owner_before owner_after
  local stub_dir="$TMP_DIR/unoccupied_lsof_stub"

  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done
  use_lease_case "live_lease"
  install_unoccupied_lsof_stub "$stub_dir"

  sleep 30 &
  live_pid="$!"
  track_background_pid "$live_pid"
  if [ "$live_pid" = "$$" ] || [ "$live_pid" = "1" ]; then
    echo "FAIL: live foreign lease specimen PID $live_pid is not a cleanup-managed child"
    return 1
  fi
  ln -s "$live_pid" "$AYB_PORT_LEASE_DIR/$candidate_a"
  owner_before="$(readlink "$AYB_PORT_LEASE_DIR/$candidate_a")"

  selected_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  if [ "$selected_port" != "$candidate_b" ]; then
    echo "FAIL: live-PID lease handout selected $selected_port; expected unleased candidate $candidate_b instead of leased candidate $candidate_a"
    return 1
  fi
  owner_after="$(readlink "$AYB_PORT_LEASE_DIR/$candidate_a")"
  if [ "$owner_after" != "$owner_before" ] || [ "$owner_after" != "$live_pid" ]; then
    echo "FAIL: live foreign lease owner changed from $owner_before to $owner_after; expected preserved child PID $live_pid"
    return 1
  fi
  echo "live_foreign_lease_oracle protected_port=$candidate_a selected_port=$selected_port live_owner=$owner_after"
  release_port_lease "$candidate_b"
  if [ -h "$AYB_PORT_LEASE_DIR/$candidate_b" ]; then
    echo "FAIL: live-PID lease handout left current-shell lease entry for selected port $candidate_b"
    return 1
  fi
  rm -f -- "$AYB_PORT_LEASE_DIR/$candidate_a"
  kill "$live_pid" 2>/dev/null || true
  wait "$live_pid" 2>/dev/null || true
  forget_background_pid "$live_pid"
}

test_stale_owner_lease_is_reclaimed() {
  local candidate_a candidate_b selected_port lease_owner dead_pid stale_leases_reaped=0
  local stub_dir="$TMP_DIR/stale_owner_lsof_stub"

  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done
  use_lease_case "stale_owner_lease"
  install_unoccupied_lsof_stub "$stub_dir"

  dead_pid="$(dead_numeric_pid)"
  ln -s "$dead_pid" "$AYB_PORT_LEASE_DIR/$candidate_a"
  selected_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  if [ "$selected_port" != "$candidate_a" ]; then
    echo "FAIL: stale lease reclaim selected $selected_port; expected reclaimed candidate $candidate_a"
    return 1
  fi
  lease_owner="$(readlink "$AYB_PORT_LEASE_DIR/$candidate_a")"
  if [ "$lease_owner" != "$$" ]; then
    echo "FAIL: stale lease owner was $lease_owner after reclaim; expected current shell $$"
    return 1
  fi
  if [ "$lease_owner" != "$dead_pid" ]; then
    stale_leases_reaped=1
  fi
  echo "stale_owner_lease_oracle stale_leases_reaped=$stale_leases_reaped reclaimed_port=$selected_port dead_owner=$dead_pid new_owner=$lease_owner"
  if [ "$stale_leases_reaped" -eq 0 ]; then
    echo "VACUOUS: stale lease reclaim did not replace the dead owner"
    return 1
  fi
  release_port_lease "$candidate_a"
  if [ -h "$AYB_PORT_LEASE_DIR/$candidate_a" ]; then
    echo "FAIL: stale lease reclaim left current-shell lease entry for reclaimed port $candidate_a"
    return 1
  fi
}

test_same_pid_reentry_reuses_free_lease() {
  local candidate_a candidate_b first_port second_port lease_owner
  local stub_dir="$TMP_DIR/reentry_free_lsof_stub"

  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done
  use_lease_case "reentry_free_lease"
  install_unoccupied_lsof_stub "$stub_dir"

  first_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  second_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  if [ "$first_port" != "$candidate_a" ] || [ "$second_port" != "$candidate_a" ]; then
    echo "FAIL: same-PID free re-entry selected $first_port then $second_port; expected $candidate_a twice"
    return 1
  fi
  if [ ! -h "$AYB_PORT_LEASE_DIR/$candidate_a" ]; then
    echo "FAIL: same-PID free re-entry did not retain the lease on $candidate_a"
    return 1
  fi
  lease_owner="$(readlink "$AYB_PORT_LEASE_DIR/$candidate_a")"
  if [ "$lease_owner" != "$$" ]; then
    echo "FAIL: same-PID free re-entry lease owner was $lease_owner; expected $$"
    return 1
  fi
  echo "same_pid_reentry_free first_port=$first_port second_port=$second_port retained_owner=$lease_owner"
  release_port_lease "$candidate_a"
  if [ -h "$AYB_PORT_LEASE_DIR/$candidate_a" ]; then
    echo "FAIL: same-PID free re-entry left current-shell lease entry for $candidate_a"
    return 1
  fi
}

test_same_pid_reentry_advances_after_kernel_conflict() {
  local candidate_a candidate_b first_port second_port fallback_owner
  local stub_dir="$TMP_DIR/reentry_occupied_lsof_stub"

  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done
  use_lease_case "reentry_occupied_lease"
  AYB_LSOF_CALL_COUNT="$TMP_DIR/reentry_lsof_call_count"
  AYB_REENTRY_OCCUPIED_PORT="$candidate_a"
  export AYB_LSOF_CALL_COUNT AYB_REENTRY_OCCUPIED_PORT
  install_reentry_lsof_stub "$stub_dir"

  first_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  second_port="$(PATH="$stub_dir:$PATH" pick_free_port "$candidate_a" "$candidate_b")"
  if [ "$first_port" != "$candidate_a" ] || [ "$second_port" != "$candidate_b" ]; then
    echo "FAIL: same-PID occupied re-entry selected $first_port then $second_port; expected $candidate_a then $candidate_b"
    return 1
  fi
  if [ -h "$AYB_PORT_LEASE_DIR/$candidate_a" ]; then
    echo "FAIL: same-PID occupied re-entry retained the stale owned lease on $candidate_a"
    return 1
  fi
  fallback_owner="$(readlink "$AYB_PORT_LEASE_DIR/$candidate_b")"
  if [ "$fallback_owner" != "$$" ]; then
    echo "FAIL: same-PID occupied re-entry fallback lease owner was $fallback_owner; expected $$"
    return 1
  fi
  echo "same_pid_reentry_occupied first_port=$first_port second_port=$second_port removed_stale_port=$candidate_a fallback_owner=$fallback_owner"
  release_port_lease "$candidate_b"
  if [ -h "$AYB_PORT_LEASE_DIR/$candidate_b" ]; then
    echo "FAIL: same-PID occupied re-entry left current-shell lease entry for fallback port $candidate_b"
    return 1
  fi
}

test_shell_release_port_leases() {
  local candidate_a candidate_b candidate_c port remaining
  local stub_dir="$TMP_DIR/release_lsof_stub"

  candidate_a="$(pick_unused_port)"
  candidate_b="$(pick_unused_port)"
  while [ "$candidate_b" = "$candidate_a" ]; do
    candidate_b="$(pick_unused_port)"
  done
  candidate_c="$(pick_unused_port)"
  while [ "$candidate_c" = "$candidate_a" ] || [ "$candidate_c" = "$candidate_b" ]; do
    candidate_c="$(pick_unused_port)"
  done
  use_lease_case "shell_release_lease"
  install_unoccupied_lsof_stub "$stub_dir"

  for port in "$candidate_a" "$candidate_b" "$candidate_c"; do
    if [ "$(PATH="$stub_dir:$PATH" pick_free_port "$port")" != "$port" ]; then
      echo "FAIL: shell release setup did not acquire lease for $port"
      return 1
    fi
    if [ "$(readlink "$AYB_PORT_LEASE_DIR/$port")" != "$$" ]; then
      echo "FAIL: shell release setup lease for $port was not owned by $$"
      return 1
    fi
  done

  for port in "$candidate_a" "$candidate_b" "$candidate_c"; do
    release_port_lease "$port"
    if [ -h "$AYB_PORT_LEASE_DIR/$port" ]; then
      echo "FAIL: shell release left lease entry for $port"
      return 1
    fi
  done

  remaining=0
  for lease_path in "$AYB_PORT_LEASE_DIR"/*; do
    if [ -h "$lease_path" ]; then
      remaining=$((remaining + 1))
    fi
  done
  echo "shell_release_cleanup acquired_leases=3 leases_remaining_after_shell_release=$remaining"
  if [ "$remaining" -ne 0 ]; then
    return 1
  fi
}

STARTED_PORT=""

symlink_target="$TMP_DIR/symlink_target"
symlink_lease_dir="$TMP_DIR/symlink_lease_dir"
mkdir -m 0755 "$symlink_target"
ln -s "$symlink_target" "$symlink_lease_dir"
AYB_PORT_LEASE_DIR="$symlink_lease_dir"
export AYB_PORT_LEASE_DIR
if resolve_port_lease_dir; then
  fail "expected resolve_port_lease_dir to reject a symlink"
fi
symlink_target_mode="$(path_mode_octal "$symlink_target")"
if [ "$symlink_target_mode" != "0o755" ]; then
  fail "symlink lease directory changed target mode to $symlink_target_mode"
fi

use_lease_case "path_traversal"
outside_lease="$BASE_PORT_LEASE_DIR/outside_lease"
ln -s "$$" "$outside_lease"
if release_port_lease "../outside_lease"; then
  fail "expected release_port_lease to reject a non-numeric candidate"
fi
if [ ! -h "$outside_lease" ]; then
  fail "path-like port candidate removed a lease outside AYB_PORT_LEASE_DIR"
fi
rm -f -- "$outside_lease"

use_lease_case "top_level_permissions"
resolve_port_lease_dir
initial_mode="$(path_mode_octal "$AYB_PORT_LEASE_DIR")"
if [ "$initial_mode" != "0o700" ]; then
  fail "expected resolve_port_lease_dir to create a private lease directory, got mode $initial_mode"
fi
chmod 0755 "$AYB_PORT_LEASE_DIR"
resolve_port_lease_dir
restricted_mode="$(path_mode_octal "$AYB_PORT_LEASE_DIR")"
if [ "$restricted_mode" != "0o700" ]; then
  fail "expected resolve_port_lease_dir to restrict an existing lease directory, got mode $restricted_mode"
fi

use_lease_case "top_level_occupied_free"
start_listener 0
occupied_port="$STARTED_PORT"
free_port="$(pick_unused_port)"

selected_port="$(pick_free_port "$occupied_port" "$free_port")"
if [ "$selected_port" != "$free_port" ]; then
  fail "expected pick_free_port to skip occupied $occupied_port and choose $free_port, got ${selected_port:-<empty>}"
fi
release_port_lease "$free_port"
if [ -h "$AYB_PORT_LEASE_DIR/$free_port" ]; then
  fail "top-level occupied/free check left current-shell lease entry for selected port $free_port"
fi

use_lease_case "top_level_exhausted"
start_listener "$free_port"
second_occupied_port="$STARTED_PORT"
sleep 30 &
exhausted_live_pid="$!"
track_background_pid "$exhausted_live_pid"
leased_exhausted_port="$(pick_unused_port)"
ln -s "$exhausted_live_pid" "$AYB_PORT_LEASE_DIR/$leased_exhausted_port"
exhausted_stderr="$TMP_DIR/exhausted_candidates_stderr"
if pick_free_port "$occupied_port" "$second_occupied_port" "$leased_exhausted_port" 2>"$exhausted_stderr" >/dev/null; then
  fail "expected pick_free_port to fail when every candidate is bound or leased"
fi
exhausted_stderr_bytes="$(wc -c <"$exhausted_stderr" | tr -d ' ')"
echo "exhausted_candidates_oracle bound_candidates=2 leased_candidates=1 stderr_bytes=$exhausted_stderr_bytes candidates=$occupied_port,$second_occupied_port,$leased_exhausted_port"
if [ "$exhausted_stderr_bytes" -eq 0 ]; then
  fail "expected pick_free_port exhaustion to emit a diagnostic"
fi
exhausted_diagnostic="$(cat "$exhausted_stderr")"
expected_exhausted_diagnostic="ERROR: no free port available from candidates: $occupied_port $second_occupied_port $leased_exhausted_port"
if [ "$exhausted_diagnostic" != "$expected_exhausted_diagnostic" ]; then
  fail "unexpected pick_free_port exhaustion diagnostic: ${exhausted_diagnostic:-<empty>}"
fi
rm -f -- "$AYB_PORT_LEASE_DIR/$leased_exhausted_port"
kill "$exhausted_live_pid" 2>/dev/null || true
wait "$exhausted_live_pid" 2>/dev/null || true
forget_background_pid "$exhausted_live_pid"

red_oracle_failures=0
if ! test_concurrent_port_selection_is_unique; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if ! test_live_foreign_lease_is_not_handed_out; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if ! test_stale_owner_lease_is_reclaimed; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if ! test_same_pid_reentry_reuses_free_lease; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if ! test_same_pid_reentry_advances_after_kernel_conflict; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if ! test_shell_release_port_leases; then
  red_oracle_failures=$((red_oracle_failures + 1))
fi
if [ "$red_oracle_failures" -ne 0 ]; then
  fail "$red_oracle_failures port lease red oracle(s) exposed unprotected handouts"
fi

remaining_live_pid_leases=0
while IFS= read -r lease_path; do
  lease_owner="$(readlink "$lease_path" 2>/dev/null || true)"
  if [[ "$lease_owner" =~ ^[0-9]+$ ]] && kill -0 "$lease_owner" 2>/dev/null; then
    remaining_live_pid_leases=$((remaining_live_pid_leases + 1))
  fi
done < <(find "$BASE_PORT_LEASE_DIR" -type l -print)
echo "port_helper_cleanup leases_remaining_after_run=$remaining_live_pid_leases"
if [ "$remaining_live_pid_leases" -ne 0 ]; then
  fail "port helper contract left $remaining_live_pid_leases live-PID lease(s) after the run"
fi

echo "PASS: port helper contract succeeded"
