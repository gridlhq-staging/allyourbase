#!/usr/bin/env bash

pick_free_port() {
  local candidate
  for candidate in "$@"; do
    if ! lsof -ti :"$candidate" >/dev/null 2>&1; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

require_free_port() {
  local port="$1"
  local reason="$2"
  local action="${3:-use}"
  if lsof -ti :"$port" >/dev/null 2>&1; then
    echo "ERROR: ${reason}; refusing to ${action} an unknown process." >&2
    return 1
  fi
}

wait_for_url() {
  local url="$1"
  local timeout="${2:-30}"
  local attempts=$((timeout * 2))
  local attempt=0
  while [ "$attempt" -lt "$attempts" ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
    attempt=$((attempt + 1))
  done
  return 1
}
