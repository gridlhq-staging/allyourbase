#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
source "$REPO_ROOT/tests/port_helpers.sh"
AYB_BIN="${AYB_BIN:-$REPO_ROOT/ayb}"
LOG_FILE="${TMPDIR:-/tmp}/ayb-fake-ollama.log"
TEMP_CONFIG_DIR=""
OLLAMA_PID=""

validate_port_number() {
  local name="$1"
  local value="$2"
  local normalized_value
  case "$value" in
    ""|*[!0123456789]*)
      echo "ERROR: ${name} must be an ASCII digits-only integer in the range 1..65535" >&2
      return 1
      ;;
  esac
  normalized_value="${value#"${value%%[!0]*}"}"
  if [ -z "$normalized_value" ]; then
    normalized_value=0
  fi
  if [ "${#normalized_value}" -gt 5 ] ||
      [ "$normalized_value" -lt 1 ] ||
      [ "$normalized_value" -gt 65535 ]; then
    echo "ERROR: ${name} must be in the range 1..65535" >&2
    return 1
  fi
}

generate_demo_jwt_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  if [ -r /dev/urandom ]; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
    return
  fi
  echo "ERROR: unable to generate a demo JWT secret" >&2
  return 1
}

resolve_fake_ollama_port() {
  if [ -n "${AYB_MOVIES_FAKE_OLLAMA_PORT:-}" ]; then
    validate_port_number "AYB_MOVIES_FAKE_OLLAMA_PORT" "$AYB_MOVIES_FAKE_OLLAMA_PORT" || return 1
    printf '%s\n' "$AYB_MOVIES_FAKE_OLLAMA_PORT"
    return 0
  fi
  pick_free_port 45514 46514 47514 48514 49514
}

materialize_temp_config() {
  local fixture_port="$1"
  local source_config="$SCRIPT_DIR/../ayb.toml"
  local temp_config="$TEMP_CONFIG_DIR/ayb.toml"
  local source_match_count
  source_match_count=$(grep -c 'base_url = "http://127.0.0.1:11434"' "$source_config" || true)
  if [ "$source_match_count" -ne 1 ]; then
    echo "ERROR: expected exactly one movies Ollama base_url in $source_config, found $source_match_count" >&2
    return 1
  fi
  cp "$source_config" "$temp_config"
  sed -i.bak "s|base_url = \"http://127.0.0.1:11434\"|base_url = \"http://127.0.0.1:${fixture_port}\"|" "$temp_config"
  rm -f "$temp_config.bak"
  printf '%s\n' "$temp_config"
}

AYB_MOVIES_FAKE_OLLAMA_PORT="$(resolve_fake_ollama_port)"
export AYB_MOVIES_FAKE_OLLAMA_PORT

require_free_port "$AYB_MOVIES_FAKE_OLLAMA_PORT" "movies fake ollama port ${AYB_MOVIES_FAKE_OLLAMA_PORT} is already occupied"
node "$SCRIPT_DIR/fake_ollama_server.cjs" >"$LOG_FILE" 2>&1 &
OLLAMA_PID=$!
HEALTHY=0

cleanup() {
  "$AYB_BIN" stop >/dev/null 2>&1 || true
  if [ -n "$OLLAMA_PID" ]; then
    kill "$OLLAMA_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$TEMP_CONFIG_DIR" ]; then
    rm -rf "$TEMP_CONFIG_DIR"
  fi
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 20); do
  if wait_for_url "http://127.0.0.1:${AYB_MOVIES_FAKE_OLLAMA_PORT}/health" 1; then
    HEALTHY=1
    break
  fi
done

if [ "${HEALTHY:-0}" -ne 1 ]; then
  echo "fake ollama health check failed at http://127.0.0.1:${AYB_MOVIES_FAKE_OLLAMA_PORT}/health" >&2
  exit 1
fi

"$AYB_BIN" stop >/dev/null 2>&1 || true

SERVER_STOPPED=0
for _ in $(seq 1 40); do
  if ! curl -sf "http://127.0.0.1:8092/health" >/dev/null 2>&1; then
    SERVER_STOPPED=1
    break
  fi
  sleep 0.25
done

if [ "$SERVER_STOPPED" -ne 1 ]; then
  echo "ayb server is still responding on 127.0.0.1:8092 after stop; refusing to reuse stale runtime" >&2
  exit 1
fi

cd "$SCRIPT_DIR/.."

TEMP_CONFIG_DIR="$(mktemp -d)"
TEMP_CONFIG="$(materialize_temp_config "$AYB_MOVIES_FAKE_OLLAMA_PORT")"
MOVIES_JWT_SECRET="$(generate_demo_jwt_secret)"

AYB_AUTH_ENABLED=true \
AYB_AUTH_JWT_SECRET="$MOVIES_JWT_SECRET" \
AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true \
AYB_AUTH_MAGIC_LINK_ENABLED=true \
AYB_SERVER_SITE_URL="${AYB_SERVER_SITE_URL:-http://localhost:5177}" \
"$AYB_BIN" start --config "$TEMP_CONFIG" >/dev/null

SERVER_READY=0
for _ in $(seq 1 40); do
  if curl -sf "http://127.0.0.1:8092/health" >/dev/null 2>&1; then
    SERVER_READY=1
    break
  fi
  sleep 0.25
done

if [ "$SERVER_READY" -ne 1 ]; then
  echo "ayb server failed to become healthy on 127.0.0.1:8092" >&2
  exit 1
fi

"$AYB_BIN" demo movies
