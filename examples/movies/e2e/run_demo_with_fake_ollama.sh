#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
LOG_FILE="${TMPDIR:-/tmp}/ayb-fake-ollama.log"

node "$SCRIPT_DIR/fake_ollama_server.cjs" >"$LOG_FILE" 2>&1 &
OLLAMA_PID=$!

cleanup() {
  kill "$OLLAMA_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:11434/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

"$REPO_ROOT/ayb" stop >/dev/null 2>&1 || true

exec "$REPO_ROOT/ayb" demo movies
