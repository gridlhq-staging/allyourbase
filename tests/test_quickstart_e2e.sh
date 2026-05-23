#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=tests/bash_assert_helpers.sh
source "$(dirname "$0")/bash_assert_helpers.sh"

ORIGINAL_HOME="$HOME"
AYB_BIN="$ORIGINAL_HOME/.ayb/bin/ayb"
AYB_BASE_URL="http://localhost:8090"
AYB_HEALTH_URL="${AYB_BASE_URL}/health"
DEMO_URL="http://localhost:5175/"
LOGIN_URL="${AYB_BASE_URL}/api/auth/login"
DEMO_JWT_SECRET="quickstart-e2e-demo-jwt-secret-0123456789abcdef"
MAX_RETRIES=90
RETRY_SLEEP_SECONDS=1

TMP_DIR="$(mktemp -d)"
RUNTIME_HOME="$TMP_DIR/home"
RUNTIME_WORKDIR="$TMP_DIR/workdir"
START_PID=""
DEMO_PID=""

cleanup() {
  HOME="$RUNTIME_HOME" "$AYB_BIN" stop >/dev/null 2>&1 || true

  if [ -n "$START_PID" ]; then
    if kill -0 "$START_PID" 2>/dev/null; then
      kill "$START_PID" 2>/dev/null || true
    fi
    wait "$START_PID" 2>/dev/null || true
  fi

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

if [ ! -x "$AYB_BIN" ]; then
  fail "missing AYB binary at $AYB_BIN. Install via README quickstart: curl -fsSLo /tmp/ayb-install.sh https://install.allyourbase.io/install.sh && sh /tmp/ayb-install.sh"
fi

mkdir -p "$RUNTIME_HOME" "$RUNTIME_WORKDIR"
export HOME="$RUNTIME_HOME"
cd "$RUNTIME_WORKDIR"

"$AYB_BIN" stop >/dev/null 2>&1 || true

AYB_AUTH_ENABLED=true \
AYB_AUTH_JWT_SECRET="$DEMO_JWT_SECRET" \
"$AYB_BIN" start >"$TMP_DIR/ayb_start.stdout" 2>"$TMP_DIR/ayb_start.stderr" &
START_PID="$!"

wait_for_ready_health "$TMP_DIR/health.json" || {
  cat "$TMP_DIR/ayb_start.stdout" >&2 || true
  cat "$TMP_DIR/ayb_start.stderr" >&2 || true
  fail "health check readiness failed"
}

assert_contains "$TMP_DIR/health.json" '"status":"ok"' "health response missing status ok"

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
