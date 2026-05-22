#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
LOG_FILE="${TMPDIR:-/tmp}/ayb-fake-ollama.log"

node "$SCRIPT_DIR/fake_ollama_server.cjs" >"$LOG_FILE" 2>&1 &
OLLAMA_PID=$!
HEALTHY=0

cleanup() {
  "$REPO_ROOT/ayb" stop >/dev/null 2>&1 || true
  kill "$OLLAMA_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:11434/health" >/dev/null 2>&1; then
    HEALTHY=1
    break
  fi
  sleep 0.1
done

if [ "${HEALTHY:-0}" -ne 1 ]; then
  echo "fake ollama health check failed at http://127.0.0.1:11434/health" >&2
  exit 1
fi

"$REPO_ROOT/ayb" stop >/dev/null 2>&1 || true

SERVER_STOPPED=0
for _ in $(seq 1 40); do
  if ! curl -sf "http://127.0.0.1:8090/health" >/dev/null 2>&1; then
    SERVER_STOPPED=1
    break
  fi
  sleep 0.25
done

if [ "$SERVER_STOPPED" -ne 1 ]; then
  echo "ayb server is still responding on 127.0.0.1:8090 after stop; refusing to reuse stale runtime" >&2
  exit 1
fi

cd "$SCRIPT_DIR/.."

AYB_AUTH_ENABLED=true \
AYB_AUTH_JWT_SECRET="movies-e2e-jwt-secret-minimum-32-bytes" \
  "$REPO_ROOT/ayb" start --config "$SCRIPT_DIR/../ayb.toml" >/dev/null

SERVER_READY=0
for _ in $(seq 1 40); do
  if curl -sf "http://127.0.0.1:8090/health" >/dev/null 2>&1; then
    SERVER_READY=1
    break
  fi
  sleep 0.25
done

if [ "$SERVER_READY" -ne 1 ]; then
  echo "ayb server failed to become healthy on 127.0.0.1:8090" >&2
  exit 1
fi

"$REPO_ROOT/ayb" demo movies
