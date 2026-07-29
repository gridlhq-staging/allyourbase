#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 \"<command-to-run-after-ayb-is-healthy>\"" >&2
  exit 1
fi

RUNNER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly RUNNER_DIR
# shellcheck disable=SC1091
source "$RUNNER_DIR/../tests/port_helpers.sh"

readonly POST_HEALTH_COMMAND="$1"
readonly AYB_DEFAULT_START_COMMAND="./ayb start --foreground"
readonly AYB_DEFAULT_SERVER_HOST="localhost"
readonly AYB_DEFAULT_SERVER_PORT="8090"
AUTO_SELECTED_AYB_SERVER_PORT=0
AUTO_SELECTED_AYB_DATABASE_PORT=0
AUTO_DERIVED_PLAYWRIGHT_BASE_URL=0
AUTO_DERIVED_SERVER_SITE_URL=0

if [[ -z "${AYB_BASE_URL:-}" && -z "${AYB_HEALTH_URL:-}" && -z "${AYB_SERVER_PORT:-}" ]]; then
  AUTO_SELECTED_AYB_SERVER_PORT=1
  AYB_SERVER_PORT="$(pick_free_port 48092 49092 50092 51092 52092)" || {
    echo "No free isolated AYB server port available for the local test runtime" >&2
    exit 1
  }
  export AYB_SERVER_PORT
fi

if [[ -z "${AYB_DATABASE_URL:-}" && -z "${AYB_DATABASE_EMBEDDED_PORT:-}" ]]; then
  AUTO_SELECTED_AYB_DATABASE_PORT=1
  AYB_DATABASE_EMBEDDED_PORT="$(pick_free_port 45434 46434 47434 48434 49434)" || {
    echo "No free isolated embedded Postgres port available for the local test runtime" >&2
    exit 1
  }
  export AYB_DATABASE_EMBEDDED_PORT
fi

readonly AYB_START_COMMAND="${AYB_START_COMMAND:-$AYB_DEFAULT_START_COMMAND}"
readonly AYB_START_LOG="${AYB_START_LOG:-/tmp/ayb-e2e.log}"
readonly AYB_HEALTH_TIMEOUT_SECONDS="${AYB_HEALTH_TIMEOUT_SECONDS:-60}"
readonly AYB_HEALTH_POLL_INTERVAL_SECONDS="${AYB_HEALTH_POLL_INTERVAL_SECONDS:-0.5}"
readonly AYB_CANONICAL_ADMIN_TOKEN_PATH="${HOME}/.ayb/admin-token"
readonly AYB_ADMIN_TOKEN_PATH="${AYB_ADMIN_TOKEN_PATH:-$AYB_CANONICAL_ADMIN_TOKEN_PATH}"
CANONICAL_ADMIN_TOKEN_BACKUP_PATH=""
CANONICAL_ADMIN_TOKEN_HAD_ORIGINAL=0
OWNED_EMBEDDED_DATA_DIR=""
HEALTH_ENDPOINT_WAS_READY_BEFORE_START=0

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

AYB_HEALTH_URL="$(derive_ayb_health_url)"
export AYB_HEALTH_URL
if [[ -z "${AYB_BASE_URL:-}" ]]; then
  export AYB_BASE_URL
  AYB_BASE_URL="$(base_url_from_health_url "$AYB_HEALTH_URL")"
fi
if [[ -z "${PLAYWRIGHT_BASE_URL:-}" ]]; then
  AUTO_DERIVED_PLAYWRIGHT_BASE_URL=1
  export PLAYWRIGHT_BASE_URL="$AYB_BASE_URL"
fi
if [[ -z "${AYB_SERVER_SITE_URL:-}" ]]; then
  AUTO_DERIVED_SERVER_SITE_URL=1
  export AYB_SERVER_SITE_URL="$AYB_BASE_URL"
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
# Dashboard browser tests exercise incident and support-ticket persistence
# against the isolated database, so keep those services available by default.
export AYB_STATUS_ENABLED="${AYB_STATUS_ENABLED:-true}"
export AYB_SUPPORT_ENABLED="${AYB_SUPPORT_ENABLED:-true}"
# The isolated embedded-Postgres runtime has no physical standby to exercise.
# An explicitly supplied value, including an empty one, overrides this default.
export AYB_BROWSER_DISABLED_CAPABILITIES="${AYB_BROWSER_DISABLED_CAPABILITIES-replicas}"

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

# The canonical token file holds a live admin bearer token, so it must match the
# 0600 contract writeAdminTokenFile enforces in internal/cli. Shell redirection
# and cp otherwise create it at 0666/source-mode minus umask, which is 0644 under
# the default umask and leaves the token readable by every account on the host.
restrict_canonical_admin_token_permissions() {
  chmod 600 "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
}

# SC2174's caveat does not apply: ~/.ayb is the deepest path component, so -m 700
# lands on exactly the directory being created. Existing ~/.ayb keeps its mode,
# which is why the token file is chmod'd separately rather than relying on this.
ensure_canonical_admin_token_dir() {
  # shellcheck disable=SC2174
  mkdir -p -m 700 "$(dirname "$AYB_CANONICAL_ADMIN_TOKEN_PATH")"
}

report_startup_failure() {
  echo "AYB process exited before health check passed." >&2
  print_start_log_excerpt
  return 1
}

ayb_start_command_runs_ayb_binary() {
  case "$AYB_START_COMMAND" in
    "ayb"|"ayb "*|"./ayb"|"./ayb "*|*/ayb|*/ayb\ *) return 0 ;;
    *) return 1 ;;
  esac
}

admin_token_ready() {
  [[ -n "${AYB_ADMIN_TOKEN:-}" ||
     -s "$AYB_ADMIN_TOKEN_PATH" ||
     -s "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]
}

started_runtime_credentials_ready() {
  if admin_token_ready; then
    return 0
  fi

  # Non-AYB fixture/custom launchers may use a supplied password without
  # producing AYB's canonical token artifact.
  [[ -n "${AYB_ADMIN_PASSWORD:-}" &&
     "$CANONICAL_ADMIN_TOKEN_HAD_ORIGINAL" -eq 0 ]] &&
    ! ayb_start_command_runs_ayb_binary
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
    ensure_canonical_admin_token_dir
    cp "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
    restrict_canonical_admin_token_permissions
  else
    remove_canonical_admin_token_file
  fi

  if [[ -n "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" ]]; then
    rm -f "$CANONICAL_ADMIN_TOKEN_BACKUP_PATH" 2>/dev/null || true
  fi
}

prepare_owned_embedded_data_dir() {
  if [[ -n "${AYB_DATABASE_URL:-}" || -n "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" ]]; then
    return 0
  fi

  OWNED_EMBEDDED_DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ayb-runwith-embedded.XXXXXX")"
  export AYB_DATABASE_EMBEDDED_DATA_DIR="$OWNED_EMBEDDED_DATA_DIR"
}

cleanup_owned_embedded_data_dir() {
  if [[ -n "$OWNED_EMBEDDED_DATA_DIR" ]]; then
    rm -rf "$OWNED_EMBEDDED_DATA_DIR"
  fi
}

ayb_start_command_uses_local_binary() {
  case "$AYB_START_COMMAND" in
    "./ayb"|"./ayb "*) return 0 ;;
    *) return 1 ;;
  esac
}

post_health_command_uses_browser_ui() {
  case "$POST_HEALTH_COMMAND" in
    *playwright*|*test:browser*|*browser-tests*) return 0 ;;
    *) return 1 ;;
  esac
}

should_refresh_ui_bundle() {
  case "${AYB_REFRESH_UI_BUNDLE:-}" in
    1|true|TRUE|yes|YES) return 0 ;;
    0|false|FALSE|no|NO) return 1 ;;
  esac

  post_health_command_uses_browser_ui
}

configure_auth_env_for_run() {
  if post_health_command_uses_browser_ui; then
    export AYB_AUTH_ENABLED="${AYB_AUTH_ENABLED:-true}"
  fi

  if [[ "${AYB_AUTH_ENABLED:-}" == "true" && -z "${AYB_AUTH_JWT_SECRET:-}" ]]; then
    export AYB_AUTH_JWT_SECRET
    AYB_AUTH_JWT_SECRET="$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"
  fi
}

ensure_ayb_binary_if_needed() {
  if ! ayb_start_command_uses_local_binary; then
    return 0
  fi

  if should_refresh_ui_bundle; then
    echo "Building current UI bundle because a browser-facing local AYB run needs embedded dashboard assets." >&2
    (cd ui && pnpm build)
  elif [[ -x ./ayb ]]; then
    echo "Using existing ./ayb binary for non-browser local AYB run." >&2
    return 0
  fi

  echo "Building ./ayb because AYB_START_COMMAND uses the local binary." >&2
  go build -o ayb ./cmd/ayb
}

# The browser-facing build can take long enough for another process to claim a
# port selected at wrapper startup. Recheck only wrapper-owned ports at the
# immediate pre-start boundary and keep every derived public URL aligned.
refresh_auto_selected_runtime_ports() {
  if (( AUTO_SELECTED_AYB_SERVER_PORT )); then
    AYB_SERVER_PORT="$(pick_free_port 48092 49092 50092 51092 52092)" || {
      echo "No free isolated AYB server port available for the local test runtime" >&2
      return 1
    }
    export AYB_SERVER_PORT
    local host="${AYB_SERVER_HOST:-$AYB_DEFAULT_SERVER_HOST}"
    AYB_BASE_URL="http://${host}:${AYB_SERVER_PORT}"
    AYB_HEALTH_URL="${AYB_BASE_URL}/health"
    export AYB_BASE_URL AYB_HEALTH_URL
    if (( AUTO_DERIVED_PLAYWRIGHT_BASE_URL )); then
      export PLAYWRIGHT_BASE_URL="$AYB_BASE_URL"
    fi
    if (( AUTO_DERIVED_SERVER_SITE_URL )); then
      export AYB_SERVER_SITE_URL="$AYB_BASE_URL"
    fi
  fi

  if (( AUTO_SELECTED_AYB_DATABASE_PORT )); then
    AYB_DATABASE_EMBEDDED_PORT="$(pick_free_port 45434 46434 47434 48434 49434)" || {
      echo "No free isolated embedded Postgres port available for the local test runtime" >&2
      return 1
    }
    export AYB_DATABASE_EMBEDDED_PORT
  fi
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

# Poll health and credential readiness until both succeed or the deadline
# expires. Exits with error if the AYB process dies before becoming healthy.
wait_for_ayb_readiness() {
  local ayb_pid="$1"
  local deadline=$((SECONDS + AYB_HEALTH_TIMEOUT_SECONDS))
  local observed_health_transition=1
  if (( HEALTH_ENDPOINT_WAS_READY_BEFORE_START )); then
    observed_health_transition=0
  fi

  while true; do
    if ! ayb_process_running "$ayb_pid"; then
      report_startup_failure
    fi

    if (( ! observed_health_transition )); then
      if ! curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1; then
        observed_health_transition=1
      fi
    elif curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1 && started_runtime_credentials_ready; then
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
  curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1 && admin_token_ready
}

refuse_stale_browser_runtime_reuse() {
  echo "Refusing to reuse an already-healthy local AYB runtime for a browser-facing run that needs freshly embedded dashboard assets." >&2
  echo "Use a free AYB_SERVER_PORT/AYB_HEALTH_URL or stop the existing runtime so ./ayb can serve the rebuilt dashboard bundle." >&2
  return 1
}

materialize_canonical_admin_token_file() {
  ensure_canonical_admin_token_dir

  if [[ -n "${AYB_ADMIN_TOKEN:-}" ]]; then
    printf '%s\n' "$AYB_ADMIN_TOKEN" > "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
    restrict_canonical_admin_token_permissions
    return 0
  fi

  if [[ -s "$AYB_ADMIN_TOKEN_PATH" && "$AYB_ADMIN_TOKEN_PATH" != "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]; then
    cp "$AYB_ADMIN_TOKEN_PATH" "$AYB_CANONICAL_ADMIN_TOKEN_PATH"
    restrict_canonical_admin_token_permissions
    return 0
  fi

  if [[ -s "$AYB_CANONICAL_ADMIN_TOKEN_PATH" ]]; then
    return 0
  fi

  echo "Healthy AYB runtime is missing reusable admin-token material." >&2
  return 1
}

# Real AYB readiness includes admin-token material so SDK/load commands can
# authenticate immediately after /health turns green.
# Shared development hosts can already have the requested AYB runtime up from a
# previous wrapper run. Reuse it when it is healthy instead of colliding on the
# same port; unhealthy listeners still fall through to the normal startup path
# so callers get the underlying bind/startup failure. Materialize the canonical
# token file first so reused runtimes preserve the same auth contract as fresh
# wrapper-owned startups.
configure_auth_env_for_run

if existing_ayb_ready; then
  if ayb_start_command_uses_local_binary && should_refresh_ui_bundle; then
    refuse_stale_browser_runtime_reuse
  fi

  prepare_canonical_admin_token_file
  trap restore_canonical_admin_token_if_needed EXIT
  materialize_canonical_admin_token_file
  bash -lc "$POST_HEALTH_COMMAND"
  exit $?
fi

ensure_ayb_binary_if_needed
refresh_auto_selected_runtime_ports

if curl -fsS "$AYB_HEALTH_URL" > /dev/null 2>&1; then
  HEALTH_ENDPOINT_WAS_READY_BEFORE_START=1
fi
prepare_canonical_admin_token_file
remove_canonical_admin_token_file
prepare_owned_embedded_data_dir
bash -lc "$AYB_START_COMMAND" > "$AYB_START_LOG" 2>&1 &
AYB_PID=$!

cleanup() {
  kill "$AYB_PID" 2>/dev/null || true
  wait "$AYB_PID" 2>/dev/null || true
  restore_canonical_admin_token_if_needed
  cleanup_owned_embedded_data_dir
}
trap cleanup EXIT

wait_for_ayb_readiness "$AYB_PID"
bash -lc "$POST_HEALTH_COMMAND"
