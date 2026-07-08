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

ensure_stopped() {
    "$AYB_BIN" stop > /dev/null 2>&1 || true
    sleep 1
    # Kill anything on demo ports too
    for port in 8090 5173 5175 5177; do
        lsof -ti :"$port" 2>/dev/null | xargs kill 2>/dev/null || true
    done
    sleep 1
}

# Runs a demo, waits for both the AYB server and the demo app to be healthy,
# checks the demo page is served, then kills it.
# Args: demo_name demo_port
test_demo_launch() {
    local name="$1"
    local port="$2"
    local log
    log=$(mktemp /tmp/ayb-demo-test-${name}.XXXXXX)

    echo -e "${CYAN}── Demo: ${name} (port ${port}) ──${NC}"

    # Start demo in background
    "$AYB_BIN" demo "$name" > "$log" 2>&1 &
    local demo_pid=$!

    # Wait for the AYB server (port 8090) to come up
    if wait_for_health 8090 60; then
        pass "${name}: AYB server became healthy"
    else
        fail "${name}: AYB server did not become healthy"
        kill -9 $demo_pid 2>/dev/null || true
        echo "    Log output:"
        head -30 "$log" | sed 's/^/    /'
        rm -f "$log"
        ensure_stopped
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
        kill -INT $demo_pid 2>/dev/null || true
        sleep 2
        kill -9 $demo_pid 2>/dev/null || true
        echo "    Log output:"
        head -30 "$log" | sed 's/^/    /'
        rm -f "$log"
        ensure_stopped
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
    kill -INT $demo_pid 2>/dev/null || true
    local wait_count=0
    while kill -0 $demo_pid 2>/dev/null && [ $wait_count -lt 20 ]; do
        sleep 0.5
        wait_count=$((wait_count + 1))
    done

    if ! kill -0 $demo_pid 2>/dev/null; then
        pass "${name}: demo exited cleanly on SIGINT"
    else
        fail "${name}: demo did not exit on SIGINT"
        kill -9 $demo_pid 2>/dev/null || true
    fi

    rm -f "$log"
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

# Ensure clean state
ensure_stopped

# ── Test each demo ───────────────────────────────────────────────

test_demo_launch "kanban" 5173
ensure_stopped

test_demo_launch "live-polls" 5175
ensure_stopped

test_demo_launch "movies" 5177
ensure_stopped

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
