#!/usr/bin/env bash
set -euo pipefail

DOCKER_BIN="${DOCKER_BIN:-docker}"
AYB_DOCKER_IMAGE="${AYB_DOCKER_IMAGE:-ghcr.io/allyourbasehq/allyourbase:latest}"
AYB_DOCKER_CONTAINER="${AYB_DOCKER_CONTAINER:-ayb-docker-runtime-smoke-$$}"
AYB_DOCKER_PORT="${AYB_DOCKER_PORT:-}"
AYB_ADMIN_PASSWORD="${AYB_ADMIN_PASSWORD:-$(dd if=/dev/urandom bs=24 count=1 2>/dev/null | od -An -v -tx1 | tr -d ' \n')}"
AYB_AUTH_JWT_SECRET="${AYB_AUTH_JWT_SECRET:-$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | od -An -v -tx1 | tr -d ' \n')}"
AYB_SMOKE_EMAIL="${AYB_SMOKE_EMAIL:-docker.smoke@example.com}"
AYB_SMOKE_PASSWORD="${AYB_SMOKE_PASSWORD:-$(dd if=/dev/urandom bs=24 count=1 2>/dev/null | od -An -v -tx1 | tr -d ' \n')}"

# Keep all mutable runtime state outside the repo tree by default so smoke runs
# never create Git noise under _dev/release/evidence or local data folders.
RUNTIME_ROOT="${AYB_DOCKER_RUNTIME_ROOT:-$(mktemp -d /tmp/ayb-docker-smoke.XXXXXX)}"
AYB_STATE_ROOT="${AYB_DOCKER_STATE_ROOT:-$RUNTIME_ROOT/ayb_state}"
MANAGED_PG_DATA_DIR="${AYB_DOCKER_PGDATA_DIR:-$AYB_STATE_ROOT/data}"
MANAGED_PG_CACHE_DIR="${AYB_DOCKER_PGCACHE_DIR:-$AYB_STATE_ROOT/pg}"
LOG_DIR="${AYB_DOCKER_LOG_DIR:-$AYB_STATE_ROOT/logs}"
RUN_DIR="${AYB_DOCKER_RUN_DIR:-$AYB_STATE_ROOT/run}"
STORAGE_DIR="${AYB_DOCKER_STORAGE_DIR:-$RUNTIME_ROOT/storage}"
TMP_DIR="${AYB_DOCKER_TMP_DIR:-$RUNTIME_ROOT/tmp}"
HEALTH_FILE="${TMP_DIR}/health.json"
ADMIN_AUTH_FILE="${TMP_DIR}/admin-auth.json"
REGISTER_FILE="${TMP_DIR}/register.json"
LOGIN_FILE="${TMP_DIR}/login.json"
PAYLOAD_FILE="${TMP_DIR}/payload.txt"
UPLOAD_FILE="${TMP_DIR}/upload.json"
STORAGE_LIST_FILE="${TMP_DIR}/storage-list.json"
STORAGE_FILE="${TMP_DIR}/storage-file.txt"
RELOGIN_FILE="${TMP_DIR}/relogin.json"
STORAGE_AFTER_FILE="${TMP_DIR}/storage-file-after.txt"
STORAGE_LIST_AFTER_FILE="${TMP_DIR}/storage-list-after.json"

cleanup() {
  "$DOCKER_BIN" rm -f "$AYB_DOCKER_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$MANAGED_PG_DATA_DIR" "$MANAGED_PG_CACHE_DIR" "$LOG_DIR" "$RUN_DIR" "$STORAGE_DIR" "$TMP_DIR"
chmod 0777 "$MANAGED_PG_DATA_DIR" "$MANAGED_PG_CACHE_DIR" "$LOG_DIR" "$RUN_DIR" "$STORAGE_DIR"

BASE_URL=""

wait_for_health() {
  local attempts="${1:-90}"
  local i
  for i in $(seq 1 "$attempts"); do
    if curl -fsS "${BASE_URL}/health" >"$HEALTH_FILE" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "health check timed out for ${BASE_URL}/health" >&2
  return 1
}

require_http() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  if [[ "$expected" != "$actual" ]]; then
    echo "${label} failed: expected HTTP ${expected}, got ${actual}" >&2
    return 1
  fi
}

resolve_base_url() {
  AYB_DOCKER_PORT="$($DOCKER_BIN port "$AYB_DOCKER_CONTAINER" 8090/tcp | awk -F: 'NR==1 {print $NF}')"
  if [[ -z "$AYB_DOCKER_PORT" ]]; then
    echo "failed to resolve mapped host port for ${AYB_DOCKER_CONTAINER}" >&2
    return 1
  fi
  BASE_URL="http://127.0.0.1:${AYB_DOCKER_PORT}"
}

start_container() {
  local port_args
  "$DOCKER_BIN" rm -f "$AYB_DOCKER_CONTAINER" >/dev/null 2>&1 || true
  if [[ -n "$AYB_DOCKER_PORT" ]]; then
    port_args=(-p "127.0.0.1:${AYB_DOCKER_PORT}:8090")
  else
    port_args=(-p "127.0.0.1::8090")
  fi
  "$DOCKER_BIN" run -d \
    --name "$AYB_DOCKER_CONTAINER" \
    "${port_args[@]}" \
    -e "AYB_ADMIN_PASSWORD=${AYB_ADMIN_PASSWORD}" \
    -e "AYB_AUTH_ENABLED=true" \
    -e "AYB_AUTH_JWT_SECRET=${AYB_AUTH_JWT_SECRET}" \
    -e "AYB_STORAGE_ENABLED=true" \
    -e "AYB_STORAGE_LOCAL_PATH=/ayb_storage" \
    -v "${MANAGED_PG_DATA_DIR}:/home/ayb/.ayb/data" \
    -v "${MANAGED_PG_CACHE_DIR}:/home/ayb/.ayb/pg" \
    -v "${LOG_DIR}:/home/ayb/.ayb/logs" \
    -v "${RUN_DIR}:/home/ayb/.ayb/run" \
    -v "${STORAGE_DIR}:/ayb_storage" \
    "$AYB_DOCKER_IMAGE" >/dev/null
  resolve_base_url
}

start_container
wait_for_health
printf 'health: %s\n' "$(cat "$HEALTH_FILE")"

admin_code="$(
  curl -sS -o "$ADMIN_AUTH_FILE" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/admin/auth" \
    -H 'Content-Type: application/json' \
    --data "$(jq -cn --arg password "$AYB_ADMIN_PASSWORD" '{password:$password}')"
)"
require_http 200 "$admin_code" "admin auth"
admin_token="$(jq -r '.token' "$ADMIN_AUTH_FILE")"
[[ -n "$admin_token" && "$admin_token" != "null" ]]
echo "admin auth: ok"

register_code="$(
  curl -sS -o "$REGISTER_FILE" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/register" \
    -H 'Content-Type: application/json' \
    --data "$(jq -cn --arg email "$AYB_SMOKE_EMAIL" --arg password "$AYB_SMOKE_PASSWORD" '{email:$email,password:$password}')"
)"
require_http 201 "$register_code" "register"
register_token="$(jq -r '.token' "$REGISTER_FILE")"
[[ -n "$register_token" && "$register_token" != "null" ]]
echo "register: ok"

login_code="$(
  curl -sS -o "$LOGIN_FILE" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    --data "$(jq -cn --arg email "$AYB_SMOKE_EMAIL" --arg password "$AYB_SMOKE_PASSWORD" '{email:$email,password:$password}')"
)"
require_http 200 "$login_code" "login"
login_token="$(jq -r '.token' "$LOGIN_FILE")"
[[ -n "$login_token" && "$login_token" != "null" ]]
echo "login: ok"

payload="docker runtime persistence payload $(date +%s)"
printf '%s' "$payload" >"$PAYLOAD_FILE"

upload_code="$(
  curl -sS -o "$UPLOAD_FILE" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/storage/journey" \
    -H "Authorization: Bearer ${login_token}" \
    -F "file=@${PAYLOAD_FILE};type=text/plain"
)"
require_http 201 "$upload_code" "storage upload"
uploaded_name="$(jq -r '.name' "$UPLOAD_FILE")"
[[ -n "$uploaded_name" && "$uploaded_name" != "null" ]]
uploaded_name_path="$(jq -rn --arg name "$uploaded_name" '$name|@uri')"
printf 'storage upload: %s\n' "$uploaded_name"

list_code="$(
  curl -sS -o "$STORAGE_LIST_FILE" -w '%{http_code}' \
    -H "Authorization: Bearer ${login_token}" \
    "${BASE_URL}/api/storage/journey"
)"
require_http 200 "$list_code" "storage list"
jq -e --arg name "$uploaded_name" '.items[] | select(.name == $name)' "$STORAGE_LIST_FILE" >/dev/null
echo "storage list: ok"

fetch_code="$(
  curl -sS -o "$STORAGE_FILE" -w '%{http_code}' \
    -H "Authorization: Bearer ${login_token}" \
    "${BASE_URL}/api/storage/journey/${uploaded_name_path}"
)"
require_http 200 "$fetch_code" "storage fetch before restart"
[[ "$(cat "$STORAGE_FILE")" == "$payload" ]]
echo "storage fetch before restart: ok"

"$DOCKER_BIN" restart "$AYB_DOCKER_CONTAINER" >/dev/null
resolve_base_url
wait_for_health
echo "restart health: ok"

relogin_code="$(
  curl -sS -o "$RELOGIN_FILE" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    --data "$(jq -cn --arg email "$AYB_SMOKE_EMAIL" --arg password "$AYB_SMOKE_PASSWORD" '{email:$email,password:$password}')"
)"
require_http 200 "$relogin_code" "login after restart"
relogin_token="$(jq -r '.token' "$RELOGIN_FILE")"
[[ -n "$relogin_token" && "$relogin_token" != "null" ]]
echo "login after restart: ok"

fetch_after_code="$(
  curl -sS -o "$STORAGE_AFTER_FILE" -w '%{http_code}' \
    -H "Authorization: Bearer ${relogin_token}" \
    "${BASE_URL}/api/storage/journey/${uploaded_name_path}"
)"
require_http 200 "$fetch_after_code" "storage fetch after restart"
[[ "$(cat "$STORAGE_AFTER_FILE")" == "$payload" ]]
echo "storage fetch after restart: ok"

list_after_code="$(
  curl -sS -o "$STORAGE_LIST_AFTER_FILE" -w '%{http_code}' \
    -H "Authorization: Bearer ${relogin_token}" \
    "${BASE_URL}/api/storage/journey"
)"
require_http 200 "$list_after_code" "storage list after restart"
jq -e --arg name "$uploaded_name" '.items[] | select(.name == $name)' "$STORAGE_LIST_AFTER_FILE" >/dev/null
echo "storage list after restart: ok"

printf '\nSMOKE RESULT: PASS\n'
printf 'base_url=%s\n' "$BASE_URL"
printf 'runtime_root=%s\n' "$RUNTIME_ROOT"
