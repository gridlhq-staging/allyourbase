#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=tests/bash_assert_helpers.sh
source "$(dirname "$0")/bash_assert_helpers.sh"

AYB_BASE_URL="http://localhost:8101"
AYB_HEALTH_URL="${AYB_BASE_URL}/health"
AUTH_ME_URL="${AYB_BASE_URL}/api/auth/me"
DEMO_URL="http://localhost:5175/"
LOGIN_URL="${AYB_BASE_URL}/api/auth/login"
DEMO_JWT_SECRET="quickstart-e2e-demo-jwt-secret-0123456789abcdef"
MAX_RETRIES=90
RETRY_SLEEP_SECONDS=1

TMP_DIR="$(mktemp -d)"
RUNTIME_HOME="$TMP_DIR/home"
RUNTIME_WORKDIR="$TMP_DIR/workdir"
AYB_BIN="$RUNTIME_HOME/.ayb/bin/ayb"
DEMO_PID=""

cleanup() {
  HOME="$RUNTIME_HOME" AYB_SERVER_PORT=8101 "$AYB_BIN" stop >/dev/null 2>&1 || true

  if [ -n "$DEMO_PID" ]; then
    if kill -0 "$DEMO_PID" 2>/dev/null; then
      kill "$DEMO_PID" 2>/dev/null || true
    fi
    wait "$DEMO_PID" 2>/dev/null || true
  fi

  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

wait_for_ready_health() {
  local body_file="$1"
  local http_code=""
  local attempt=1

  while [ "$attempt" -le "$MAX_RETRIES" ]; do
    http_code="$(curl -sS -m 2 -o "$body_file" -w "%{http_code}" "$AYB_HEALTH_URL" || true)"
    # The published quickstart contract only guarantees a healthy status field.
    # Some released binaries omit the database field even when the server is ready.
    if [ "$http_code" = "200" ] \
      && grep -Fq '"status":"ok"' "$body_file"; then
      return 0
    fi
    sleep "$RETRY_SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done

  echo "Health endpoint did not reach ready state after ${MAX_RETRIES} attempts." >&2
  echo "Last HTTP status: ${http_code:-<none>}" >&2
  [ -f "$body_file" ] && cat "$body_file" >&2
  return 1
}

wait_for_demo_http_200_with_body() {
  local body_file="$1"
  local http_code=""
  local attempt=1

  while [ "$attempt" -le "$MAX_RETRIES" ]; do
    http_code="$(curl -sS -m 2 -o "$body_file" -w "%{http_code}" "$DEMO_URL" || true)"
    if [ "$http_code" = "200" ] \
      && [ -s "$body_file" ] \
      && grep -Fq '<title>Live Polls</title>' "$body_file"; then
      return 0
    fi
    sleep "$RETRY_SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done

  echo "Demo app did not become ready after ${MAX_RETRIES} attempts." >&2
  echo "Last HTTP status: ${http_code:-<none>}" >&2
  [ -f "$body_file" ] && cat "$body_file" >&2
  return 1
}

assert_auth_me_disabled() {
  local body_file="$1"
  local http_code=""

  http_code="$(curl -sS -m 5 -o "$body_file" -w "%{http_code}" "$AUTH_ME_URL" || true)"
  if [ "$http_code" != "404" ]; then
    echo "expected auth-disabled /api/auth/me to return HTTP 404, got ${http_code:-<none>}" >&2
    [ -f "$body_file" ] && cat "$body_file" >&2
    fail "pre-demo auth-disabled check failed"
  fi
}

mkdir -p "$(dirname "$AYB_BIN")" "$RUNTIME_WORKDIR"
go build -o "$AYB_BIN" ./cmd/ayb
export HOME="$RUNTIME_HOME"
cd "$RUNTIME_WORKDIR"

AYB_SERVER_PORT=8101 "$AYB_BIN" stop >/dev/null 2>&1 || true

AYB_SERVER_PORT=8101 \
AYB_AUTH_ENABLED=false \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" start >"$TMP_DIR/ayb_start.stdout" 2>"$TMP_DIR/ayb_start.stderr" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "documented ayb start command failed"
}

wait_for_ready_health "$TMP_DIR/health.json" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "health check readiness failed"
}

assert_contains "$TMP_DIR/health.json" '"status":"ok"' "health response missing status ok"
assert_auth_me_disabled "$TMP_DIR/auth_me_disabled.json"

AYB_SERVER_PORT=8101 \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" demo live-polls >"$TMP_DIR/demo.stdout" 2>"$TMP_DIR/demo.stderr" &
DEMO_PID="$!"

sleep 1
if ! kill -0 "$DEMO_PID" 2>/dev/null; then
  cat "$TMP_DIR/demo.stdout" >&2 || true
  cat "$TMP_DIR/demo.stderr" >&2 || true
  fail "demo process exited before readiness"
fi

wait_for_demo_http_200_with_body "$TMP_DIR/demo_index.html" || {
  cat "$TMP_DIR/demo.stdout" >&2 || true
  cat "$TMP_DIR/demo.stderr" >&2 || true
  fail "demo app serving check failed"
}

login_http_code="$(curl -sS -m 5 -o "$TMP_DIR/login.json" -w "%{http_code}" \
  -X POST "$LOGIN_URL" \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@demo.test","password":"password123"}' || true)"

if [ "$login_http_code" != "200" ]; then
  echo "login request failed with HTTP ${login_http_code:-<none>}" >&2
  [ -f "$TMP_DIR/login.json" ] && cat "$TMP_DIR/login.json" >&2
  fail "seeded auth login check failed"
fi

if ! grep -Eq '"token"[[:space:]]*:[[:space:]]*"[^"]+"' "$TMP_DIR/login.json"; then
  cat "$TMP_DIR/login.json" >&2 || true
  fail "login response missing non-empty token field"
fi

echo "PASS: quickstart e2e probe succeeded"
