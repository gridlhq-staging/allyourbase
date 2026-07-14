#!/bin/bash
# Demo Launch Integration Tests
# Verifies that `ayb demo kanban`, `ayb demo live-polls`, and `ayb demo movies` can actually
# launch from the command line: start server, apply schema, serve pages.
#
# This catches the class of bugs where the demo fails with cryptic errors
# (missing admin token, port conflicts, schema apply failures, etc.)
#
# Requirements: ayb binary in PATH (or pass AYB_BIN env var), curl
# WARNING: Starts/stops a real server. Do NOT run while an important AYB instance is active.

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

AYB_BIN="${AYB_BIN:-ayb}"
PASSED=0
FAILED=0
TOTAL=0

# Isolated demo-app ports selected at preflight (see pick_free_demo_port). The
# release gate must NOT require the universal Vite defaults (5173/5175/5177) to
# be globally free, because those collide with unrelated dev servers on a shared
# host. These are the ports ensure_stopped verifies between runs.
DEMO_APP_PORTS=()

# ── Helpers ──────────────────────────────────────────────────────

pass() {
    PASSED=$((PASSED + 1))
    TOTAL=$((TOTAL + 1))
    echo -e "  ${GREEN}✓${NC} $1"
}

fail() {
    FAILED=$((FAILED + 1))
    TOTAL=$((TOTAL + 1))
    echo -e "  ${RED}✗${NC} $1"
}

assert_contains() {
    local text="$1"
    local pattern="$2"
    local label="$3"
    if echo "$text" | grep -qi "$pattern"; then
        pass "$label"
    else
        fail "$label (expected to find '$pattern')"
    fi
}

wait_for_health() {
    local port="${1:-8090}"
    local timeout="${2:-30}"
    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        if curl -sf "http://127.0.0.1:${port}/health" > /dev/null 2>&1; then
            return 0
        fi
        sleep 0.5
        elapsed=$((elapsed + 1))
    done
    return 1
}

wait_for_no_health() {
    local port="${1:-8090}"
    local timeout="${2:-15}"
    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        if ! curl -sf "http://127.0.0.1:${port}/health" > /dev/null 2>&1; then
            return 0
        fi
        sleep 0.5
        elapsed=$((elapsed + 1))
    done
    return 1
}

require_free_port() {
    local port="$1"
    local reason="$2"
    local action="${3:-use}"
    if lsof -ti :"$port" >/dev/null 2>&1; then
        echo -e "${RED}ERROR: ${reason}; refusing to ${action} an unknown process.${NC}" >&2
        return 1
    fi
}

# pick_free_demo_port returns the first currently-free port from its candidate
# list. Demos are served on these isolated ports (via AYB_DEMO_APP_PORT) so the
# gate does not depend on the well-known Vite defaults being globally free.
pick_free_demo_port() {
    local candidate
    for candidate in "$@"; do
        if ! lsof -ti :"$candidate" >/dev/null 2>&1; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}

ensure_stopped() {
    "$AYB_BIN" stop > /dev/null 2>&1 || true
    sleep 1
    # 8090 is AYB's own server port; DEMO_APP_PORTS are the isolated app ports
    # this run selected. We never require the universal Vite defaults free.
    # ${arr[@]+...} keeps empty-array expansion safe under `set -u` on bash 3.2.
    for port in 8090 ${DEMO_APP_PORTS[@]+"${DEMO_APP_PORTS[@]}"}; do
        if ! require_free_port "$port" "port ${port} is still occupied after ayb stop" "kill"; then
            return 1
        fi
    done
    sleep 1
}

cleanup_demo_launch_resources() {
    local demo_pid="${1:-}"
    local log="${2:-}"
    local data_dir="${3:-}"

    if [ -n "$demo_pid" ]; then
        kill -INT "$demo_pid" 2>/dev/null || true
        local wait_count=0
        while kill -0 "$demo_pid" 2>/dev/null && [ $wait_count -lt 20 ]; do
            sleep 0.5
            wait_count=$((wait_count + 1))
        done
        kill -9 "$demo_pid" 2>/dev/null || true
    fi
    rm -f "$log"
    if [ -n "$data_dir" ]; then
        rm -rf "$data_dir"
    fi
}

# Runs a demo, waits for both the AYB server and the demo app to be healthy,
# checks the demo page is served, then kills it.
# Args: demo_name demo_port
test_demo_launch() {
    local name="$1"
    local port="$2"
    local data_dir
    local demo_pid=""
    local log
    log=$(mktemp /tmp/ayb-demo-test-${name}.XXXXXX)
    # Isolate only embedded Postgres data for hermetic demo runs. The shared
    # ~/.ayb/pgbin binary cache stays warm, and /tmp keeps Postgres sockets short.
    data_dir=$(mktemp -d /tmp/ayb-demoe2e.XXXXXX)

    echo -e "${CYAN}── Demo: ${name} (port ${port}) ──${NC}"

    # Start demo in background on an isolated port so the gate does not require
    # the universal Vite default for this demo to be globally free.
    AYB_DATABASE_EMBEDDED_DATA_DIR="$data_dir" AYB_DEMO_APP_PORT="$port" "$AYB_BIN" demo "$name" > "$log" 2>&1 &
    demo_pid=$!

    # Wait for the AYB server (port 8090) to come up
    if wait_for_health 8090 60; then
        pass "${name}: AYB server became healthy"
    else
        fail "${name}: AYB server did not become healthy"
        echo "    Log output:"
        head -30 "$log" | sed 's/^/    /'
        cleanup_demo_launch_resources "$demo_pid" "$log" "$data_dir"
        if ! ensure_stopped; then
            return 1
        fi
        echo ""
        return 1
    fi

    # Wait for the demo app to serve on its port
    local demo_ready=0
    local elapsed=0
    while [ $elapsed -lt 30 ]; do
        if curl -sf "http://127.0.0.1:${port}/" > /dev/null 2>&1; then
            demo_ready=1
            break
        fi
        sleep 0.5
        elapsed=$((elapsed + 1))
    done

    if [ $demo_ready -eq 1 ]; then
        pass "${name}: demo app serves on port ${port}"
    else
        fail "${name}: demo app not responding on port ${port}"
        echo "    Log output:"
        head -30 "$log" | sed 's/^/    /'
        cleanup_demo_launch_resources "$demo_pid" "$log" "$data_dir"
        if ! ensure_stopped; then
            return 1
        fi
        echo ""
        return 1
    fi

    # Fetch the demo page and verify it's HTML (not an error page)
    local page_content
    page_content=$(curl -sf "http://127.0.0.1:${port}/" 2>/dev/null || echo "")
    if echo "$page_content" | grep -qi "<html\|<!doctype"; then
        pass "${name}: demo page serves valid HTML"
    else
        fail "${name}: demo page does not look like HTML"
    fi

    # Check the demo banner output
    local banner
    banner=$(cat "$log")
    assert_contains "$banner" "Allyourbase Demo" "${name}: banner shows demo header"
    assert_contains "$banner" "Accounts:" "${name}: banner shows demo accounts"
    assert_contains "$banner" "Ctrl+C" "${name}: banner shows stop hint"

    # Verify API proxy works (demo app proxies /api to AYB server)
    local api_body api_status
    api_body=$(mktemp /tmp/ayb-demo-api-${name}.XXXXXX)
    api_status=$(curl -sS -o "$api_body" -w "%{http_code}" "http://127.0.0.1:${port}/api/schema" 2>/dev/null || echo "000")
    if [ "$api_status" = "401" ] && grep -qi "authorization" "$api_body"; then
        pass "${name}: /api proxy works (schema endpoint reaches AYB auth)"
    else
        fail "${name}: /api proxy not working"
        echo "    /api/schema status: ${api_status}"
        head -5 "$api_body" | sed 's/^/    /'
    fi
    rm -f "$api_body"

    # Clean shutdown
    kill -INT "$demo_pid" 2>/dev/null || true
    local wait_count=0
    while kill -0 "$demo_pid" 2>/dev/null && [ $wait_count -lt 20 ]; do
        sleep 0.5
        wait_count=$((wait_count + 1))
    done

    if ! kill -0 "$demo_pid" 2>/dev/null; then
        pass "${name}: demo exited cleanly on SIGINT"
    else
        fail "${name}: demo did not exit on SIGINT"
        kill -9 "$demo_pid" 2>/dev/null || true
    fi

    cleanup_demo_launch_resources "" "$log" "$data_dir"
    echo ""
    return 0
}

# ── Pre-flight ───────────────────────────────────────────────────

echo ""
echo -e "${CYAN}═══ Demo Launch Integration Tests ═══${NC}"
echo ""

if ! command -v "$AYB_BIN" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: '$AYB_BIN' not found in PATH. Set AYB_BIN env var.${NC}"
    exit 1
fi

echo -e "Binary: $("$AYB_BIN" version 2>/dev/null || echo 'unknown')"
echo ""

# Select isolated, currently-free app ports before any ensure_stopped call so
# the gate never depends on the universal Vite defaults (5173/5175/5177) being
# globally free on a shared host. Candidates stay in the high-port range to
# avoid clashing with common dev servers.
KANBAN_PORT=$(pick_free_demo_port 45173 46173 47173 48173 49173) || { echo -e "${RED}ERROR: no free port for kanban demo${NC}"; exit 1; }
POLLS_PORT=$(pick_free_demo_port 45175 46175 47175 48175 49175) || { echo -e "${RED}ERROR: no free port for live-polls demo${NC}"; exit 1; }
MOVIES_PORT=$(pick_free_demo_port 45177 46177 47177 48177 49177) || { echo -e "${RED}ERROR: no free port for movies demo${NC}"; exit 1; }
DEMO_APP_PORTS=("$KANBAN_PORT" "$POLLS_PORT" "$MOVIES_PORT")

# Ensure clean state
ensure_stopped || exit 1

# ── Test each demo ───────────────────────────────────────────────

test_demo_launch "kanban" "$KANBAN_PORT"
ensure_stopped || exit 1

test_demo_launch "live-polls" "$POLLS_PORT"
ensure_stopped || exit 1

test_demo_launch "movies" "$MOVIES_PORT"
ensure_stopped || exit 1

# ── Test: unknown demo name gives helpful error ──────────────────

echo -e "${CYAN}── Demo: unknown name ──${NC}"
UNKNOWN_OUTPUT=$("$AYB_BIN" demo nonexistent 2>&1 || true)
assert_contains "$UNKNOWN_OUTPUT" "unknown demo" "unknown demo gives error"
echo ""

# ── Summary ──────────────────────────────────────────────────────

echo -e "${CYAN}═══ Results: ${PASSED} passed, ${FAILED} failed, ${TOTAL} total ═══${NC}"
echo ""

if [ $FAILED -gt 0 ]; then
    echo -e "${RED}FAIL${NC}"
    exit 1
else
    echo -e "${GREEN}ALL TESTS PASSED${NC}"
    exit 0
fi
