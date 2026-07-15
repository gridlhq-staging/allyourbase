#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/bash_assert_helpers.sh"
source "$SCRIPT_DIR/port_helpers.sh"

TMP_DIR="$(mktemp -d)"
LISTENER_PIDS=""

cleanup() {
  for pid in $LISTENER_PIDS; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

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
  LISTENER_PIDS="${LISTENER_PIDS} ${pid}"

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

STARTED_PORT=""
start_listener 0
occupied_port="$STARTED_PORT"
free_port="$(pick_unused_port)"

selected_port="$(pick_free_port "$occupied_port" "$free_port")"
if [ "$selected_port" != "$free_port" ]; then
  fail "expected pick_free_port to skip occupied $occupied_port and choose $free_port, got ${selected_port:-<empty>}"
fi

start_listener "$free_port"
second_occupied_port="$STARTED_PORT"
if pick_free_port "$occupied_port" "$second_occupied_port" >/dev/null; then
  fail "expected pick_free_port to fail when every candidate is occupied"
fi

echo "PASS: port helper contract succeeded"
