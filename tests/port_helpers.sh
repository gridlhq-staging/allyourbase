#!/usr/bin/env bash

resolve_port_lease_dir() {
  if [ -z "${AYB_PORT_LEASE_DIR:-}" ]; then
    local temp_base="${TMPDIR:-/tmp}"
    temp_base="${temp_base%/}"
    AYB_PORT_LEASE_DIR="$temp_base/ayb-port-leases-$(id -u)"
  fi

  if [ -L "$AYB_PORT_LEASE_DIR" ]; then
    printf 'ERROR: port lease directory must not be a symlink: %s\n' "$AYB_PORT_LEASE_DIR" >&2
    return 1
  fi
  mkdir -p "$AYB_PORT_LEASE_DIR" || return 1
  if [ -L "$AYB_PORT_LEASE_DIR" ] || [ ! -d "$AYB_PORT_LEASE_DIR" ] || [ ! -O "$AYB_PORT_LEASE_DIR" ]; then
    printf 'ERROR: port lease directory must be an owned directory: %s\n' "$AYB_PORT_LEASE_DIR" >&2
    return 1
  fi
  chmod 700 "$AYB_PORT_LEASE_DIR"
}

valid_port_candidate() {
  local port="$1"

  case "$port" in
    "" | 0 | 0* | *[!0-9]*)
      return 1
      ;;
  esac
  [ "$port" -le 65535 ]
}

port_lease_is_owned() {
  local port="$1"
  local lease_path="$AYB_PORT_LEASE_DIR/$port"
  local lease_owner=""

  valid_port_candidate "$port" || return 1
  [ -h "$lease_path" ] || return 1
  lease_owner="$(readlink "$lease_path" 2>/dev/null)" || return 1
  [ "$lease_owner" = "$$" ]
}

acquire_port_lease() {
  local port="$1"
  local lease_path="$AYB_PORT_LEASE_DIR/$port"
  local lease_owner=""
  local current_owner=""

  valid_port_candidate "$port" || return 1
  if [ -h "$lease_path" ]; then
    if ! lease_owner="$(readlink "$lease_path" 2>/dev/null)"; then
      lease_owner=""
    fi
    if [ "$lease_owner" = "$$" ]; then
      return 0
    fi

    case "$lease_owner" in
      "" | *[!0-9]* | 0)
        ;;
      *)
        # The per-uid lease namespace makes EPERM unreachable in normal use.
        if kill -0 "$lease_owner" 2>/dev/null; then
          return 1
        fi
        ;;
    esac
    if ! current_owner="$(readlink "$lease_path" 2>/dev/null)"; then
      current_owner=""
    fi
    if [ "$current_owner" != "$lease_owner" ]; then
      return 1
    fi
    rm -f -- "$lease_path" || return 1
  fi

  ln -s "$$" "$lease_path" 2>/dev/null
}

release_port_lease() {
  local port="$1"
  local lease_path

  valid_port_candidate "$port" || return 1
  resolve_port_lease_dir || return 1
  lease_path="$AYB_PORT_LEASE_DIR/$port"
  if [ ! -h "$lease_path" ]; then
    return 0
  fi
  if port_lease_is_owned "$port"; then
    rm -f -- "$lease_path"
  fi
}

pick_free_port() {
  local candidate

  resolve_port_lease_dir || return 1
  for candidate in "$@"; do
    if ! valid_port_candidate "$candidate"; then
      continue
    fi
    if ! acquire_port_lease "$candidate"; then
      continue
    fi
    if ! lsof -ti :"$candidate" >/dev/null 2>&1; then
      if port_lease_is_owned "$candidate"; then
        printf '%s\n' "$candidate"
        return 0
      fi
      continue
    fi
    release_port_lease "$candidate" || return 1
  done
  printf 'ERROR: no free port available from candidates: %s\n' "$*" >&2
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
