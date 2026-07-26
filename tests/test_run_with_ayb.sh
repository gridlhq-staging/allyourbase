#!/usr/bin/env bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
export HOME="${TMP_DIR}/home"
mkdir -p "$HOME"
unset AYB_ADMIN_TOKEN AYB_ADMIN_TOKEN_PATH AYB_BASE_URL AYB_HEALTH_URL AYB_SERVER_PORT
unset AYB_DATABASE_URL AYB_DATABASE_EMBEDDED_PORT PLAYWRIGHT_BASE_URL
HTTP_PID=""
AYB_BINARY_BACKUP_PATH="${TMP_DIR}/ayb.original"
AYB_BINARY_HAD_ORIGINAL=0
if [[ -e ayb ]]; then
  cp ayb "$AYB_BINARY_BACKUP_PATH"
  AYB_BINARY_HAD_ORIGINAL=1
fi

cleanup() {
  if [[ -n "$HTTP_PID" ]]; then
    kill "$HTTP_PID" 2>/dev/null || true
    wait "$HTTP_PID" 2>/dev/null || true
  fi
  if (( AYB_BINARY_HAD_ORIGINAL )); then
    cp "$AYB_BINARY_BACKUP_PATH" ayb
  else
    rm -f ayb
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

printf 'ok\n' > "${TMP_DIR}/health"
PORT_PATH="${TMP_DIR}/health.port"
python3 - "$TMP_DIR" "$PORT_PATH" > "${TMP_DIR}/http.log" 2>&1 <<'PY' &
import functools
import http.server
import pathlib
import socketserver
import sys

directory = pathlib.Path(sys.argv[1])
port_path = pathlib.Path(sys.argv[2])
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(directory))

with socketserver.TCPServer(("127.0.0.1", 0), handler) as server:
    port_path.write_text(str(server.server_address[1]))
    server.serve_forever()
PY
HTTP_PID=$!

HEALTH_PORT=""
for _ in $(seq 1 20); do
  if [[ -z "$HEALTH_PORT" && -s "$PORT_PATH" ]]; then
    HEALTH_PORT="$(cat "$PORT_PATH")"
  fi

  if [[ -n "$HEALTH_PORT" ]] && curl -fsS "http://127.0.0.1:${HEALTH_PORT}/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

if [[ -z "$HEALTH_PORT" ]] || ! curl -fsS "http://127.0.0.1:${HEALTH_PORT}/health" > /dev/null 2>&1; then
  echo "FAIL: test fixture did not start a healthy HTTP listener"
  cat "${TMP_DIR}/http.log"
  exit 1
fi

STDOUT_PATH="${TMP_DIR}/stdout.log"
STDERR_PATH="${TMP_DIR}/stderr.log"
if AYB_START_COMMAND='bash -lc "sleep 1; exit 1"' \
  AYB_HEALTH_URL="http://127.0.0.1:${HEALTH_PORT}/health" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "unexpected success\n"' > "$STDOUT_PATH" 2> "$STDERR_PATH"; then
  echo "FAIL: scripts/run-with-ayb.sh reported success even though AYB never started"
  cat "$STDOUT_PATH"
  cat "$STDERR_PATH"
  exit 1
fi

if ! grep -q 'AYB process exited before health check passed.' "$STDERR_PATH"; then
  echo "FAIL: expected readiness failure message when AYB exits before startup completes"
  cat "$STDERR_PATH"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh rejects unrelated healthy listeners when AYB startup fails"

SUCCESS_DIR="${TMP_DIR}/success"
mkdir -p "$SUCCESS_DIR"
printf 'ok\n' > "${SUCCESS_DIR}/health"
SUCCESS_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
SUCCESS_STDOUT_PATH="${TMP_DIR}/success.stdout.log"
SUCCESS_STDERR_PATH="${TMP_DIR}/success.stderr.log"

if ! AYB_START_COMMAND="python3 -m http.server ${SUCCESS_PORT} --bind 127.0.0.1 --directory \"${SUCCESS_DIR}\"" \
  AYB_HEALTH_URL="http://127.0.0.1:${SUCCESS_PORT}/health" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "post-health-ok\n"' > "$SUCCESS_STDOUT_PATH" 2> "$SUCCESS_STDERR_PATH"; then
  echo "FAIL: scripts/run-with-ayb.sh did not run the post-health command after a healthy startup"
  cat "$SUCCESS_STDOUT_PATH"
  cat "$SUCCESS_STDERR_PATH"
  exit 1
fi

if ! grep -q '^post-health-ok$' "$SUCCESS_STDOUT_PATH"; then
  echo "FAIL: expected post-health command output after helper startup succeeded"
  cat "$SUCCESS_STDOUT_PATH"
  exit 1
fi

if curl -fsS "http://127.0.0.1:${SUCCESS_PORT}/health" > /dev/null 2>&1; then
  echo "FAIL: scripts/run-with-ayb.sh left the started server running after completion"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh runs post-health commands and cleans up the started server"

ISOLATED_DIR="${TMP_DIR}/isolated"
ISOLATED_BIN_DIR="${ISOLATED_DIR}/bin"
ISOLATED_CAPTURE_PATH="${ISOLATED_DIR}/runtime_ports"
mkdir -p "$ISOLATED_BIN_DIR" "${ISOLATED_DIR}/www"
printf 'ok\n' > "${ISOLATED_DIR}/www/health"
cat > "${ISOLATED_BIN_DIR}/lsof" <<'SH'
#!/usr/bin/env bash
case "${*: -1}" in
  :48092|:45434) exit 0 ;;
  *) exit 1 ;;
esac
SH
chmod +x "${ISOLATED_BIN_DIR}/lsof"

if ! PATH="${ISOLATED_BIN_DIR}:$PATH" \
  AYB_START_COMMAND="python3 -m http.server \"\$AYB_SERVER_PORT\" --bind 127.0.0.1 --directory \"${ISOLATED_DIR}/www\"" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  AYB_TEST_RUNTIME_CAPTURE="$ISOLATED_CAPTURE_PATH" \
  bash scripts/run-with-ayb.sh 'printf "%s %s %s %s\n" "$AYB_SERVER_PORT" "$AYB_DATABASE_EMBEDDED_PORT" "$PLAYWRIGHT_BASE_URL" "$AYB_SERVER_SITE_URL" > "$AYB_TEST_RUNTIME_CAPTURE"'; then
  echo "FAIL: scripts/run-with-ayb.sh did not select isolated runtime ports"
  exit 1
fi

if [[ "$(cat "$ISOLATED_CAPTURE_PATH")" != "49092 46434 http://localhost:49092 http://localhost:49092" ]]; then
  echo "FAIL: expected isolated fallback ports plus matching Playwright/WebAuthn public URLs, got $(cat "$ISOLATED_CAPTURE_PATH")"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh isolates default AYB and keeps browser public origins aligned"

BUILD_SCOPE_DIR="${TMP_DIR}/build-scope"
BUILD_SCOPE_BIN_DIR="${BUILD_SCOPE_DIR}/bin"
BUILD_SCOPE_HOME="${BUILD_SCOPE_DIR}/home"
mkdir -p "$BUILD_SCOPE_BIN_DIR" "$BUILD_SCOPE_HOME/.ayb"
printf 'token-for-existing-runtime\n' > "$BUILD_SCOPE_HOME/.ayb/admin-token"
cat > "${BUILD_SCOPE_BIN_DIR}/pnpm" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$AYB_TEST_PNPM_CALL_PATH"
if [[ -n "${AYB_TEST_BUILD_STARTED_PATH:-}" ]]; then
  : > "$AYB_TEST_BUILD_STARTED_PATH"
fi
if [[ "${AYB_TEST_PNPM_SHOULD_FAIL:-}" == "1" ]]; then
  exit 47
fi
SH
cat > "${BUILD_SCOPE_BIN_DIR}/go" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$AYB_TEST_GO_CALL_PATH"
if [[ "${AYB_TEST_GO_SHOULD_FAIL:-}" == "1" ]]; then
  exit 48
fi
if [[ "${AYB_TEST_GO_WRITES_FAKE_AYB:-}" == "1" ]]; then
  output_path=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -o)
        output_path="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done
  if [[ -z "$output_path" ]]; then
    exit 49
  fi
  cat > "$output_path" <<'FAKE_AYB'
#!/usr/bin/env bash
if [[ "$AYB_SERVER_PORT" == "48092" && -e "${AYB_TEST_BUILD_STARTED_PATH:-}" ]]; then
  echo "Error: port $AYB_SERVER_PORT is already in use" >&2
  exit 1
fi
exec python3 -m http.server "$AYB_SERVER_PORT" --bind 127.0.0.1 --directory "$AYB_TEST_FAKE_AYB_WEB_DIR"
FAKE_AYB
  chmod +x "$output_path"
fi
SH
chmod +x "${BUILD_SCOPE_BIN_DIR}/pnpm" "${BUILD_SCOPE_BIN_DIR}/go"

NON_BROWSER_PNPM_CALL_PATH="${BUILD_SCOPE_DIR}/non_browser_pnpm"
NON_BROWSER_GO_CALL_PATH="${BUILD_SCOPE_DIR}/non_browser_go"
NON_BROWSER_STDOUT_PATH="${BUILD_SCOPE_DIR}/non_browser.stdout.log"
NON_BROWSER_STDERR_PATH="${BUILD_SCOPE_DIR}/non_browser.stderr.log"
if ! HOME="$BUILD_SCOPE_HOME" \
  PATH="${BUILD_SCOPE_BIN_DIR}:$PATH" \
  AYB_START_COMMAND='./ayb start --foreground' \
  AYB_HEALTH_URL="http://127.0.0.1:${HEALTH_PORT}/health" \
  AYB_TEST_PNPM_SHOULD_FAIL=1 \
  AYB_TEST_GO_SHOULD_FAIL=1 \
  AYB_TEST_PNPM_CALL_PATH="$NON_BROWSER_PNPM_CALL_PATH" \
  AYB_TEST_GO_CALL_PATH="$NON_BROWSER_GO_CALL_PATH" \
  bash scripts/run-with-ayb.sh 'printf "non-browser-ok\n"' > "$NON_BROWSER_STDOUT_PATH" 2> "$NON_BROWSER_STDERR_PATH"; then
  echo "FAIL: non-browser local AYB runs should not require a UI build before startup"
  cat "$NON_BROWSER_STDOUT_PATH"
  cat "$NON_BROWSER_STDERR_PATH"
  exit 1
fi

if [[ -e "$NON_BROWSER_PNPM_CALL_PATH" ]]; then
  echo "FAIL: non-browser local AYB runs should not invoke pnpm; got $(cat "$NON_BROWSER_PNPM_CALL_PATH")"
  exit 1
fi

if [[ -e "$NON_BROWSER_GO_CALL_PATH" ]]; then
  echo "FAIL: non-browser local AYB runs should not invoke go before reusing a healthy runtime; got $(cat "$NON_BROWSER_GO_CALL_PATH")"
  exit 1
fi

if ! grep -q '^non-browser-ok$' "$NON_BROWSER_STDOUT_PATH"; then
  echo "FAIL: expected non-browser post-health command to run against existing healthy AYB"
  cat "$NON_BROWSER_STDOUT_PATH"
  exit 1
fi

FRESH_PREBUILT_GO_CALL_PATH="${BUILD_SCOPE_DIR}/fresh_prebuilt_go"
FRESH_PREBUILT_STDOUT_PATH="${BUILD_SCOPE_DIR}/fresh_prebuilt.stdout.log"
FRESH_PREBUILT_STDERR_PATH="${BUILD_SCOPE_DIR}/fresh_prebuilt.stderr.log"
FRESH_PREBUILT_WEB_DIR="${BUILD_SCOPE_DIR}/fresh_prebuilt_www"
FRESH_PREBUILT_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
mkdir -p "$FRESH_PREBUILT_WEB_DIR"
printf 'ok\n' > "${FRESH_PREBUILT_WEB_DIR}/health"
cat > ayb <<'SH'
#!/usr/bin/env bash
exec python3 -m http.server "$AYB_SERVER_PORT" --bind 127.0.0.1 --directory "$AYB_TEST_FAKE_AYB_WEB_DIR"
SH
chmod +x ayb

if ! HOME="$BUILD_SCOPE_HOME" \
  PATH="${BUILD_SCOPE_BIN_DIR}:$PATH" \
  AYB_START_COMMAND='./ayb start --foreground' \
  AYB_SERVER_PORT="$FRESH_PREBUILT_PORT" \
  AYB_HEALTH_URL="http://127.0.0.1:${FRESH_PREBUILT_PORT}/health" \
  AYB_TEST_GO_SHOULD_FAIL=1 \
  AYB_TEST_GO_CALL_PATH="$FRESH_PREBUILT_GO_CALL_PATH" \
  AYB_TEST_FAKE_AYB_WEB_DIR="$FRESH_PREBUILT_WEB_DIR" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "fresh-prebuilt-ok\n"' > "$FRESH_PREBUILT_STDOUT_PATH" 2> "$FRESH_PREBUILT_STDERR_PATH"; then
  echo "FAIL: fresh non-browser local AYB runs should start an existing ./ayb without rebuilding"
  cat "$FRESH_PREBUILT_STDOUT_PATH"
  cat "$FRESH_PREBUILT_STDERR_PATH"
  exit 1
fi

if [[ -e "$FRESH_PREBUILT_GO_CALL_PATH" ]]; then
  echo "FAIL: fresh non-browser local AYB runs should not invoke go when ./ayb exists; got $(cat "$FRESH_PREBUILT_GO_CALL_PATH")"
  exit 1
fi

if ! grep -q '^fresh-prebuilt-ok$' "$FRESH_PREBUILT_STDOUT_PATH"; then
  echo "FAIL: expected fresh non-browser post-health command to run against existing ./ayb"
  cat "$FRESH_PREBUILT_STDOUT_PATH"
  exit 1
fi

STALE_BROWSER_PNPM_CALL_PATH="${BUILD_SCOPE_DIR}/stale_browser_pnpm"
STALE_BROWSER_GO_CALL_PATH="${BUILD_SCOPE_DIR}/stale_browser_go"
STALE_BROWSER_STDOUT_PATH="${BUILD_SCOPE_DIR}/stale_browser.stdout.log"
STALE_BROWSER_STDERR_PATH="${BUILD_SCOPE_DIR}/stale_browser.stderr.log"
if HOME="$BUILD_SCOPE_HOME" \
  PATH="${BUILD_SCOPE_BIN_DIR}:$PATH" \
  AYB_START_COMMAND='./ayb start --foreground --host 127.0.0.1' \
  AYB_HEALTH_URL="http://127.0.0.1:${HEALTH_PORT}/health" \
  AYB_TEST_PNPM_CALL_PATH="$STALE_BROWSER_PNPM_CALL_PATH" \
  AYB_TEST_GO_CALL_PATH="$STALE_BROWSER_GO_CALL_PATH" \
  bash scripts/run-with-ayb.sh 'printf "playwright stale-browser-ok\n"' > "$STALE_BROWSER_STDOUT_PATH" 2> "$STALE_BROWSER_STDERR_PATH"; then
  echo "FAIL: browser-facing local AYB runs must not reuse an already-healthy runtime after refreshing embedded UI assets"
  cat "$STALE_BROWSER_STDOUT_PATH"
  cat "$STALE_BROWSER_STDERR_PATH"
  exit 1
fi

if grep -q '^playwright stale-browser-ok$' "$STALE_BROWSER_STDOUT_PATH"; then
  echo "FAIL: browser-facing local AYB run executed against an already-healthy stale runtime"
  cat "$STALE_BROWSER_STDOUT_PATH"
  exit 1
fi

if ! grep -q 'Refusing to reuse an already-healthy local AYB runtime for a browser-facing run that needs freshly embedded dashboard assets.' "$STALE_BROWSER_STDERR_PATH"; then
  echo "FAIL: expected stale browser-runtime reuse refusal"
  cat "$STALE_BROWSER_STDERR_PATH"
  exit 1
fi

BROWSER_PNPM_CALL_PATH="${BUILD_SCOPE_DIR}/browser_pnpm"
BROWSER_GO_CALL_PATH="${BUILD_SCOPE_DIR}/browser_go"
BROWSER_STDOUT_PATH="${BUILD_SCOPE_DIR}/browser.stdout.log"
BROWSER_STDERR_PATH="${BUILD_SCOPE_DIR}/browser.stderr.log"
BROWSER_WEB_DIR="${BUILD_SCOPE_DIR}/browser_www"
BROWSER_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
mkdir -p "$BROWSER_WEB_DIR"
printf 'ok\n' > "${BROWSER_WEB_DIR}/health"
if ! HOME="$BUILD_SCOPE_HOME" \
  PATH="${BUILD_SCOPE_BIN_DIR}:$PATH" \
  AYB_START_COMMAND='./ayb start --foreground --host 127.0.0.1' \
  AYB_SERVER_PORT="$BROWSER_PORT" \
  AYB_HEALTH_URL="http://127.0.0.1:${BROWSER_PORT}/health" \
  AYB_TEST_PNPM_CALL_PATH="$BROWSER_PNPM_CALL_PATH" \
  AYB_TEST_GO_CALL_PATH="$BROWSER_GO_CALL_PATH" \
  AYB_TEST_GO_WRITES_FAKE_AYB=1 \
  AYB_TEST_FAKE_AYB_WEB_DIR="$BROWSER_WEB_DIR" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "playwright browser-ok\n"' > "$BROWSER_STDOUT_PATH" 2> "$BROWSER_STDERR_PATH"; then
  echo "FAIL: browser local AYB runs should still build the UI bundle before startup"
  cat "$BROWSER_STDOUT_PATH"
  cat "$BROWSER_STDERR_PATH"
  exit 1
fi

if [[ "$(cat "$BROWSER_PNPM_CALL_PATH" 2>/dev/null || true)" != "build" ]]; then
  echo "FAIL: browser local AYB runs should enter ui and invoke pnpm build"
  cat "$BROWSER_STDOUT_PATH"
  cat "$BROWSER_STDERR_PATH"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh scopes UI bundle refresh to browser-facing local AYB runs"

PORT_REFRESH_MARKER_PATH="${BUILD_SCOPE_DIR}/port_refresh_build_started"
PORT_REFRESH_CAPTURE_PATH="${BUILD_SCOPE_DIR}/port_refresh_runtime"
PORT_REFRESH_STDOUT_PATH="${BUILD_SCOPE_DIR}/port_refresh.stdout.log"
PORT_REFRESH_STDERR_PATH="${BUILD_SCOPE_DIR}/port_refresh.stderr.log"
PORT_REFRESH_PNPM_CALL_PATH="${BUILD_SCOPE_DIR}/port_refresh_pnpm"
PORT_REFRESH_GO_CALL_PATH="${BUILD_SCOPE_DIR}/port_refresh_go"
PORT_REFRESH_WEB_DIR="${BUILD_SCOPE_DIR}/port_refresh_www"
mkdir -p "$PORT_REFRESH_WEB_DIR"
printf 'ok\n' > "${PORT_REFRESH_WEB_DIR}/health"
cat > "${BUILD_SCOPE_BIN_DIR}/lsof" <<'SH'
#!/usr/bin/env bash
if [[ "${*: -1}" == ":48092" && -e "$AYB_TEST_BUILD_STARTED_PATH" ]]; then
  exit 0
fi
exit 1
SH
chmod +x "${BUILD_SCOPE_BIN_DIR}/lsof"

unset AYB_BASE_URL AYB_HEALTH_URL AYB_SERVER_PORT AYB_DATABASE_EMBEDDED_PORT
unset PLAYWRIGHT_BASE_URL AYB_SERVER_SITE_URL
for _ in $(seq 1 40); do
  if ! curl -fsS "http://localhost:48092/health" > /dev/null 2>&1; then
    break
  fi
  sleep 0.05
done
if ! HOME="$BUILD_SCOPE_HOME" \
  PATH="${BUILD_SCOPE_BIN_DIR}:$PATH" \
  AYB_START_COMMAND='./ayb start --foreground --host 127.0.0.1' \
  AYB_REFRESH_UI_BUNDLE=true \
  AYB_TEST_BUILD_STARTED_PATH="$PORT_REFRESH_MARKER_PATH" \
  AYB_TEST_PNPM_CALL_PATH="$PORT_REFRESH_PNPM_CALL_PATH" \
  AYB_TEST_GO_CALL_PATH="$PORT_REFRESH_GO_CALL_PATH" \
  AYB_TEST_GO_WRITES_FAKE_AYB=1 \
  AYB_TEST_FAKE_AYB_WEB_DIR="$PORT_REFRESH_WEB_DIR" \
  AYB_TEST_RUNTIME_CAPTURE="$PORT_REFRESH_CAPTURE_PATH" \
  AYB_ADMIN_PASSWORD='unused-for-test' \
  bash scripts/run-with-ayb.sh 'printf "%s %s %s\n" "$AYB_SERVER_PORT" "$PLAYWRIGHT_BASE_URL" "$AYB_SERVER_SITE_URL" > "$AYB_TEST_RUNTIME_CAPTURE"' > "$PORT_REFRESH_STDOUT_PATH" 2> "$PORT_REFRESH_STDERR_PATH"; then
  echo "FAIL: wrapper should refresh automatic ports after the browser build"
  cat "$PORT_REFRESH_STDOUT_PATH"
  cat "$PORT_REFRESH_STDERR_PATH"
  exit 1
fi

if [[ "$(cat "$PORT_REFRESH_CAPTURE_PATH")" != "49092 http://localhost:49092 http://localhost:49092" ]]; then
  echo "FAIL: expected post-build port refresh and aligned public URLs, got $(cat "$PORT_REFRESH_CAPTURE_PATH")"
  exit 1
fi

echo "PASS: scripts/run-with-ayb.sh refreshes automatic ports after slow browser builds"
