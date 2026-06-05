#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 \"<command-to-run-after-ayb-is-healthy>\"" >&2
  exit 1
fi

readonly POST_HEALTH_COMMAND="$1"
readonly AYB_DEFAULT_START_COMMAND="./ayb start --foreground"
readonly AYB_DEFAULT_SERVER_HOST="localhost"
readonly AYB_DEFAULT_SERVER_PORT="8090"
readonly AYB_START_COMMAND="${AYB_START_COMMAND:-$AYB_DEFAULT_START_COMMAND}"
readonly AYB_START_LOG="${AYB_START_LOG:-/tmp/ayb-e2e.log}"
readonly AYB_HEALTH_TIMEOUT_SECONDS="${AYB_HEALTH_TIMEOUT_SECONDS:-60}"
readonly AYB_HEALTH_POLL_INTERVAL_SECONDS="${AYB_HEALTH_POLL_INTERVAL_SECONDS:-0.5}"
readonly AYB_CANONICAL_ADMIN_TOKEN_PATH="${HOME}/.ayb/admin-token"
readonly AYB_ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-$AYB_CANONICAL_ADMIN_TOKEN_PATH}"
CANONICAL_ADMIN_TOKEN_BACKUP_PATH=""
CANONICAL_ADMIN_TOKEN_HAD_ORIGINAL=0

derive_ayb_base_url() {
  if [[ -n "${AYB_BASE_URL:-}" ]]; then
    printf '%s\n' "${AYB_BASE_URL%/}"
    return
  fi

  local host="${AYB_SERVER_HOST:-$AYB_DEFAULT_SERVER_HOST}"
  local port="${AYB_SERVER_PORT:-$AYB_DEFAULT_SERVER_PORT}"
  printf 'http://%s:%s\n' "$host" "$port"
}

base_url_from_health_url() {
  local health_url="${1%/}"
  printf '%s\n' "${health_url%/health}"
}

derive_ayb_health_url() {
  if [[ -n "${AYB_HEALTH_URL:-}" ]]; then
    printf '%s\n' "$AYB_HEALTH_URL"
    return
  fi

  printf '%s/health\n' "$(derive_ayb_base_url)"
}

readonly AYB_HEALTH_URL="$(derive_ayb_health_url)"
if [[ -z "${AYB_BASE_URL:-}" ]]; then
  export AYB_BASE_URL
  AYB_BASE_URL="$(base_url_from_health_url "$AYB_HEALTH_URL")"
fi

# Rate-limit overrides prevent load/browser tests from being throttled.
export AYB_AUTH_RATE_LIMIT="${AYB_AUTH_RATE_LIMIT:-10000}"
export AYB_AUTH_ANONYMOUS_RATE_LIMIT="${AYB_AUTH_ANONYMOUS_RATE_LIMIT:-10000}"
export AYB_RATE_LIMIT_API="${AYB_RATE_LIMIT_API:-10000/min}"
export AYB_RATE_LIMIT_API_ANONYMOUS="${AYB_RATE_LIMIT_API_ANONYMOUS:-10000/min}"
# Sensitive auth endpoints like /api/auth/register and WebAuthn login begin/finish
# sit behind auth.rate_limit_auth, so unattended integration reruns must raise
# that limiter too or later test files will trip the default 10/minute cap.
export AYB_AUTH_RATE_LIMIT_AUTH="${AYB_AUTH_RATE_LIMIT_AUTH:-10000/min}"
# The live SDK integration suite covers storage uploads/signing against the
# local backend, so the harness enables storage unless a caller overrides it.
export AYB_STORAGE_ENABLED="${AYB_STORAGE_ENABLED:-true}"

# Auth remains opt-in for baseline load targets, but explicit auth-enabled
# wrapper runs need a local JWT secret before AYB's config validation starts.
if [[ -n "${AYB_AUTH_ENABLED+x}" ]]; then
  export AYB_AUTH_ENABLED
fi
if [[ -n "${AYB_AUTH_JWT_SECRET+x}" ]]; then
  export AYB_AUTH_JWT_SECRET
fi
if [[ "${AYB_AUTH_ENABLED:-}" == "true" && -z "${AYB_AUTH_JWT_SECRET:-}" ]]; then
  export AYB_AUTH_JWT_SECRET
  AYB_AUTH_JWT_SECRET="$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"
fi

if ! [[ "$AYB_HEALTH_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || (( AYB_HEALTH_TIMEOUT_SECONDS < 1 )); then
  echo "AYB_HEALTH_TIMEOUT_SECONDS must be a positive integer; got: $AYB_HEALTH_TIMEOUT_SECONDS" >&2
  exit 1
fi

if ! [[ "$AYB_HEALTH_POLL_INTERVAL_SECONDS" =~ ^[0-9]+([.][0-9]+)?$ ]] || ! awk -v value="$AYB_HEALTH_POLL_INTERVAL_SECONDS" 'BEGIN { exit !(value > 0) }'; then
  echo "AYB_HEALTH_POLL_INTERVAL_SECONDS must be a positive number; got: $AYB_HEALTH_POLL_INTERVAL_SECONDS" >&2
  exit 1
fi

print_start_log_excerpt() {
  local line_count=40
  if [[ -f "$AYB_START_LOG" ]]; then
    echo "AYB startup log excerpt ($AYB_START_LOG):" >&2
    tail -n "$line_count" "$AYB_START_LOG" >&2 || true
  fi
}

remove_canonical_admin_token_file() {
  rm -f "$AYB_CANONICAL_ADMIN_TOKEN_PATH" 2>/dev/null || true
}

report_startup_failure() {
  echo "AYB process exited before health check passed." >&2
  print_start_log_excerpt
  return 1
}

admin_token_ready() {
  [[ -n "${AYB_ADMIN_PASSWORD:-}" ||
     -n "${AYB_ADMIN_TOKEN:-}" ||
     -s "$AYB_ADMIN_TOKEN_PATH" ||
     -s "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]
}

stored_admin_token_ready() {
  [[ -n "${AYB_ADMIN_TOKEN:-}" ||
     -s "$AYB_ADMIN_TOKEN_PATH" ||
     -s "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]
}

prepare_canonical_admin_token_file() {
  if [[ -f "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]; then
    CANONICAL_ADMIN_TOKEN_BACKUP_PATH="$(mktemp)"
    cp "$AYB_CANONICAL_ADMIN_TOKEN_PATH" "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH"
    CANONICAL_ADMIN_TOKEN_HAD_ORIGINAL=1
  fi
}

# Restore the canonical token file or remove the generated one; custom
# AYB_ADMIN_TOKEN_PATH files are caller-owned credentials.
restore_canonical_admin_token_if_needed() {
  if (( CANONICAL_ADMIN_TOKEN_HAD_ORIGINAL )); then
    mkdir -p "$(dirname "$AYB_CANONICAL_ADMIN_TOKEN_PATH")"
    cp "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
  else
    remove_canonical_admin_token_file
  fi

  if [[ -n "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" ]]; then
    rm -f "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" 2>/dev/null || true
  fi
}

ensure_ayb_binary_if_needed() {
  case "$AYB_START_COMMAND" in
    "./ayb"|"./ayb "*) ;;
    *) return 0 ;;
  esac

  if [[ -x ./ayb ]]; then
    return 0
  fi

  echo "Building ./ayb because AYB_START_COMMAND uses it and no executable exists." >&2
  go build -o ayb ./cmd/ayb
}

ayb_process_running() {
  local ayb_pid="$1"

  if ! kill -0 "$ayb_pid" 2>/dev/null; then
    return 1
  fi

  local process_state
  process_state="$(ps -o stat= -p "$ayb_pid" 2>/dev/null | tr -d '[:space:]' || true)"
  if [[ -z "$process_state" || "$process_state" == *Z* ]]; then
    return 1
  fi

  return 0
}

# Poll health endpoint + admin-token readiness until both succeed or deadline
# expires. Exits with error if the AYB process dies before becoming healthy.
wait_for_ayb_readiness() {
  local ayb_pid="$1"
  local deadline=$((SECONDS + AYB_HEALTH_TIMEOUT_SECONDS))

  while true; do
    if ! ayb_process_running "$ayb_pid"; then
      report_startup_failure
    fi

    if curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1 && admin_token_ready; then
      if ! ayb_process_running "$ayb_pid"; then
        report_startup_failure
      fi
      return 0
    fi

    if (( SECONDS >= deadline )); then
      echo "Timed out waiting for AYB health check at $AYB_HEALTH_URL after ${AYB_HEALTH_TIMEOUT_SECONDS}s." >&2
      print_start_log_excerpt
      return 1
    fi

    sleep "$AYB_HEALTH_POLL_INTERVAL_SECONDS"
  done
}

existing_ayb_ready() {
  curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1 && stored_admin_token_ready
}

materialize_canonical_admin_token_file() {
  mkdir -p "$(dirname "$AYB_CANONICAL_ADMIN_TOKEN_PATH")"

  if [[ -n "${AYB_ADMIN_TOKEN:-}" ]]; then
    printf '%s\n' "$AYB_ADMIN_TOKEN" > "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
    return 0
  fi

  if [[ -s "$AYB_ADMIN_TOKEN_PATH" && "$AYB_ADMIN_TOKEN_PATH" != "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]; then
    cp "$AYB_ADMIN_TOKEN_PATH" "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
    return 0
  fi

  if [[ -s "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]; then
    return 0
  fi

  echo "Healthy AYB runtime is missing reusable admin-token material." >&2
  return 1
}

# Readiness includes admin-token material so SDK/load commands can authenticate
# immediately after /health turns green.
ensure_ayb_binary_if_needed

# Shared development hosts can already have the requested AYB runtime up from a
# previous wrapper run. Reuse it when it is healthy instead of colliding on the
# same port; unhealthy listeners still fall through to the normal startup path
# so callers get the underlying bind/startup failure. Materialize the canonical
# token file first so reused runtimes preserve the same auth contract as fresh
# wrapper-owned startups.
if existing_ayb_ready; then
  prepare_canonical_admin_token_file
  trap restore_canonical_admin_token_if_needed EXIT
  materialize_canonical_admin_token_file
  bash -lc "$POST_HEALTH_COMMAND"
  exit $?
fi

prepare_canonical_admin_token_file
if [[ -z "${AYB_ADMIN_PASSWORD:-}" ]]; then
  remove_canonical_admin_token_file
fi
bash -lc "$AYB_START_COMMAND" > "$AYB_START_LOG" 2>&1 &
AYB_PID=$!

cleanup() {
  kill "$AYB_PID" 2>/dev/null || true
  wait "$AYB_PID" 2>/dev/null || true
  restore_canonical_admin_token_if_needed
}
trap cleanup EXIT

wait_for_ayb_readiness "$AYB_PID"
bash -lc "$POST_HEALTH_COMMAND"
