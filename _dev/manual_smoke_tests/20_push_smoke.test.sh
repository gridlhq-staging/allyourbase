#!/usr/bin/env bash
# Push notification runtime smoke.
# Encodes register -> send -> observe through the documented API/CLI surfaces.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/tests/port_helpers.sh"

if [ -z "${AYB_BIN:-}" ]; then
  if [ -x "$REPO_ROOT/ayb" ]; then
    AYB_BIN="$REPO_ROOT/ayb"
  else
    AYB_BIN="ayb"
  fi
fi

SHARED_AYB_DIR="${HOME}/.ayb"
SERVER_PORT=""
DATABASE_PORT=""
SERVER_PID=""
RUNTIME_HOME=""
DATA_DIR=""
SERVER_LOG=""
RESPONSE_DIR=""
BASE_URL=""

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "ERROR: required command not found: $name" >&2
    exit 1
  fi
}

prepare_isolated_home() {
  local runtime_home="$1"
  local cache_name
  mkdir -p "$runtime_home/.ayb"
  for cache_name in pg pgbin; do
    if [ -d "$SHARED_AYB_DIR/$cache_name" ]; then
      ln -s "$SHARED_AYB_DIR/$cache_name" "$runtime_home/.ayb/$cache_name"
    fi
  done
}

cleanup() {
  local original_status="${1:-0}"
  local cleanup_status=0
  local wait_count=0
  trap - EXIT
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill -INT "$SERVER_PID" 2>/dev/null || true
    while kill -0 "$SERVER_PID" 2>/dev/null && [ "$wait_count" -lt 20 ]; do
      sleep 0.5
      wait_count=$((wait_count + 1))
    done
    kill -9 "$SERVER_PID" 2>/dev/null || true
  fi
  if [ -n "$RUNTIME_HOME" ]; then
    HOME="$RUNTIME_HOME" "$AYB_BIN" stop --port "$SERVER_PORT" >/dev/null 2>&1 || true
  fi
  rm -f "$SERVER_LOG"
  rm -rf "$DATA_DIR" "$RUNTIME_HOME" "$RESPONSE_DIR"
  if [ -n "$SERVER_PORT" ]; then
    if ! require_free_port "$SERVER_PORT" "AYB port ${SERVER_PORT} is still occupied after push smoke cleanup" "kill"; then
      cleanup_status=1
    fi
  fi
  if [ -n "$DATABASE_PORT" ]; then
    if ! require_free_port "$DATABASE_PORT" "Postgres port ${DATABASE_PORT} is still occupied after push smoke cleanup" "kill"; then
      cleanup_status=1
    fi
  fi
  if [ "$original_status" -ne 0 ]; then
    return "$original_status"
  fi
  return "$cleanup_status"
}

fail_with_log() {
  local message="$1"
  echo "ERROR: $message" >&2
  if [ -n "$SERVER_LOG" ] && [ -f "$SERVER_LOG" ]; then
    echo "---- ayb start log excerpt ----" >&2
    tail -80 "$SERVER_LOG" >&2 || true
    echo "---- end ayb start log excerpt ----" >&2
  fi
  exit 1
}

wait_for_server_ready() {
  local timeout="${1:-60}"
  local attempts="$timeout"
  local attempt=0
  while [ "$attempt" -lt "$attempts" ]; do
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      fail_with_log "ayb start exited before /health became ready"
    fi
    if wait_for_url "$BASE_URL/health" 1; then
      return 0
    fi
    attempt=$((attempt + 1))
  done
  fail_with_log "timed out waiting for $BASE_URL/health"
}

read_admin_token() {
  local token_path="$RUNTIME_HOME/.ayb/admin-token"
  local attempts=50
  local attempt=0
  while [ "$attempt" -lt "$attempts" ]; do
    if [ -s "$token_path" ]; then
      tr -d '\r\n' < "$token_path"
      return 0
    fi
    sleep 0.2
    attempt=$((attempt + 1))
  done
  fail_with_log "timed out waiting for isolated admin token"
}

curl_json_expect() {
  local method="$1"
  local url="$2"
  local expected_status="$3"
  local auth_header="$4"
  local payload="$5"
  local body_path="$6"
  local status
  if [ -n "$auth_header" ]; then
    status="$(curl -sS -o "$body_path" -w '%{http_code}' -X "$method" "$url" \
      -H "$auth_header" \
      -H "Content-Type: application/json" \
      --data "$payload")"
  else
    status="$(curl -sS -o "$body_path" -w '%{http_code}' -X "$method" "$url" \
      -H "Content-Type: application/json" \
      --data "$payload")"
  fi
  if [ "$status" != "$expected_status" ]; then
    echo "ERROR: $method $url returned HTTP $status, want $expected_status" >&2
    cat "$body_path" >&2 || true
    exit 1
  fi
}

poll_delivery_sent() {
  local admin_token="$1"
  local delivery_id="$2"
  local output_path="$3"
  local attempt=0
  while [ "$attempt" -lt 60 ]; do
    curl -fsS -o "$output_path" \
      -H "Authorization: Bearer $admin_token" \
      "$BASE_URL/api/admin/push/deliveries/$delivery_id"
    if jq -e '.status == "sent"' "$output_path" >/dev/null; then
      return 0
    fi
    sleep 0.5
    attempt=$((attempt + 1))
  done
  echo "ERROR: delivery $delivery_id did not reach sent status" >&2
  cat "$output_path" >&2 || true
  fail_with_log "delivery $delivery_id did not reach sent status"
}

require_prerequisites() {
  require_command curl
  require_command jq
  require_command lsof
  require_command "$AYB_BIN"
}

select_runtime_ports() {
  SERVER_PORT="$(pick_free_port 19090 19091 19092 19093 19094 19095)" || {
    echo "ERROR: no free AYB server port in candidate pool" >&2
    exit 1
  }
  DATABASE_PORT="$(pick_free_port 19432 19433 19434 19435 19436 19437)" || {
    echo "ERROR: no free embedded Postgres port in candidate pool" >&2
    exit 1
  }
  require_free_port "$SERVER_PORT" "selected AYB server port ${SERVER_PORT} is occupied"
  require_free_port "$DATABASE_PORT" "selected Postgres port ${DATABASE_PORT} is occupied"
}

prepare_runtime_paths() {
  RUNTIME_HOME="$(mktemp -d /tmp/ayb-push-home.XXXXXX)"
  DATA_DIR="$(mktemp -d /tmp/ayb-push-pg.XXXXXX)"
  SERVER_LOG="$(mktemp /tmp/ayb-push-smoke.XXXXXX.log)"
  RESPONSE_DIR="$(mktemp -d /tmp/ayb-push-responses.XXXXXX)"
  BASE_URL="http://127.0.0.1:${SERVER_PORT}"
  prepare_isolated_home "$RUNTIME_HOME"
  trap 'cleanup "$?"' EXIT
}

start_server() {
  local jwt_secret
  jwt_secret="$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')"

  HOME="$RUNTIME_HOME" \
    AYB_SERVER_PORT="$SERVER_PORT" \
    AYB_DATABASE_EMBEDDED_PORT="$DATABASE_PORT" \
    AYB_DATABASE_EMBEDDED_DATA_DIR="$DATA_DIR" \
    AYB_AUTH_ENABLED=true \
    AYB_AUTH_JWT_SECRET="$jwt_secret" \
    AYB_JOBS_ENABLED=true \
    AYB_PUSH_ENABLED=true \
    AYB_PUSH_USE_LOG_PROVIDER=true \
    "$AYB_BIN" start --foreground --host 127.0.0.1 --port "$SERVER_PORT" >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!

  wait_for_server_ready 60
}

register_user_and_app() {
  local admin_token
  local suffix
  local email
  local password
  local auth_body
  local user_token
  local user_id
  local app_id

  admin_token="$(read_admin_token)"
  suffix="$(date +%s)-$$"
  email="push-smoke-${suffix}@example.test"
  password="PushSmoke-${suffix}-password"
  auth_body="$(mktemp "$RESPONSE_DIR/auth.XXXXXX.json")"
  app_body="$(mktemp "$RESPONSE_DIR/app.XXXXXX.json")"

  curl_json_expect POST "$BASE_URL/api/auth/register" 201 "" \
    "$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password}')" \
    "$auth_body"
  user_token="$(jq -er '.token | select(length > 20)' "$auth_body")"
  user_id="$(jq -er '.user.id | select(length > 0)' "$auth_body")"

  HOME="$RUNTIME_HOME" "$AYB_BIN" apps create "push-smoke-${suffix}" \
    --owner-id "$user_id" \
    --url "$BASE_URL" \
    --admin-token "$admin_token" \
    --json >"$app_body"
  app_id="$(jq -er '.id | select(length > 0)' "$app_body")"
  jq -e --arg owner "$user_id" '.ownerUserId == $owner' "$app_body" >/dev/null

  printf '%s\n%s\n%s\n%s\n%s\n' "$admin_token" "$suffix" "$user_token" "$user_id" "$app_id"
}

register_device() {
  local suffix="$1"
  local user_token="$2"
  local user_id="$3"
  local app_id="$4"
  local device_body
  device_token="fcm-token-${suffix}"
  device_body="$(mktemp "$RESPONSE_DIR/device.XXXXXX.json")"
  curl_json_expect POST "$BASE_URL/api/push/devices" 201 "Authorization: Bearer $user_token" \
    "$(jq -n --arg app_id "$app_id" --arg token "$device_token" '{app_id:$app_id,provider:"fcm",platform:"android",token:$token,device_name:"Push Smoke Android"}')" \
    "$device_body"
  jq -e \
    --arg app_id "$app_id" \
    --arg user_id "$user_id" \
    --arg token "$device_token" \
    '.app_id == $app_id and .user_id == $user_id and .provider == "fcm" and .platform == "android" and .token == $token and .is_active == true and (.id | length > 0)' \
    "$device_body" >/dev/null
  jq -er '.id' "$device_body"
}

send_and_assert_delivery() {
  local admin_token="$1"
  local suffix="$2"
  local user_id="$3"
  local app_id="$4"
  local device_token_id="$5"
  local send_body
  local delivery_body
  local title
  local body
  local data_kind
  local delivery_id
  title="Push smoke title ${suffix}"
  body="Push smoke body ${suffix}"
  data_kind="push-smoke-${suffix}"
  send_body="$(mktemp "$RESPONSE_DIR/send.XXXXXX.json")"
  delivery_body="$(mktemp "$RESPONSE_DIR/delivery.XXXXXX.json")"
  HOME="$RUNTIME_HOME" "$AYB_BIN" push send \
    --app-id "$app_id" \
    --user-id "$user_id" \
    --title "$title" \
    --body "$body" \
    --data "$(jq -cn --arg kind "$data_kind" --arg token_id "$device_token_id" '{kind:$kind,token_id:$token_id}')" \
    --url "$BASE_URL" \
    --admin-token "$admin_token" \
    --json >"$send_body"
  jq -e '.deliveries | length == 1' "$send_body" >/dev/null
  delivery_id="$(jq -er '.deliveries[0].id | select(length > 0)' "$send_body")"

  poll_delivery_sent "$admin_token" "$delivery_id" "$delivery_body"
  jq -e \
    --arg app_id "$app_id" \
    --arg user_id "$user_id" \
    --arg device_token_id "$device_token_id" \
    --arg title "$title" \
    --arg body "$body" \
    --arg data_kind "$data_kind" \
    '.app_id == $app_id
      and .user_id == $user_id
      and .device_token_id == $device_token_id
      and .provider == "fcm"
      and .title == $title
      and .body == $body
      and .status == "sent"
      and .data_payload.kind == $data_kind
      and .data_payload.token_id == $device_token_id
      and (.provider_message_id | startswith("log-"))' \
    "$delivery_body" >/dev/null

  rm -f "$send_body" "$delivery_body"
}

main() {
  local registration
  local admin_token
  local suffix
  local user_token
  local user_id
  local app_id
  local device_token_id

  require_prerequisites
  select_runtime_ports
  prepare_runtime_paths
  start_server

  registration="$(register_user_and_app)"
  admin_token="$(printf '%s\n' "$registration" | sed -n '1p')"
  suffix="$(printf '%s\n' "$registration" | sed -n '2p')"
  user_token="$(printf '%s\n' "$registration" | sed -n '3p')"
  user_id="$(printf '%s\n' "$registration" | sed -n '4p')"
  app_id="$(printf '%s\n' "$registration" | sed -n '5p')"

  device_token_id="$(register_device "$suffix" "$user_token" "$user_id" "$app_id")"
  send_and_assert_delivery "$admin_token" "$suffix" "$user_id" "$app_id" "$device_token_id"

  echo "Push smoke passed."
}

main "$@"
