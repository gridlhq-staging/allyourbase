#!/bin/bash
# Demo App E2E Tests
# Starts each demo app via `ayb demo`, then runs its Playwright test suite
# against the live server. This catches cross-user sharing, RLS, realtime,
# and other bugs that only appear with a real backend.
#
# Requirements: ayb binary in PATH (or AYB_BIN env var), npm, curl
# WARNING: Starts/stops a real server. Do NOT run while an important AYB instance is active.
#
# Usage:
#   bash 18_demo_e2e.test.sh                  # run all demo E2E suites
#   bash 18_demo_e2e.test.sh kanban           # run only kanban
#   bash 18_demo_e2e.test.sh live-polls       # run only live-polls
#   bash 18_demo_e2e.test.sh movies           # run only movies

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Use AYB_BIN if set, else try repo-root build, else PATH
if [ -z "${AYB_BIN:-}" ]; then
    if [ -x "$REPO_ROOT/ayb" ]; then
        AYB_BIN="$REPO_ROOT/ayb"
    else
        AYB_BIN="ayb"
    fi
fi
# Disable both auth-route and generic anonymous API rate limits for E2E tests.
# Login/register-heavy suites can otherwise exhaust the 30/min anonymous API
# cap before all Playwright workers finish.
export AYB_AUTH_RATE_LIMIT=10000
export AYB_AUTH_RATE_LIMIT_AUTH=10000/min
export AYB_AUTH_ANONYMOUS_RATE_LIMIT=10000
export AYB_RATE_LIMIT_API_ANONYMOUS=10000/min
export AYB_RATE_LIMIT_API=10000/min

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

require_free_port() {
    local port="$1"
    local reason="$2"
    local action="${3:-use}"
    if lsof -ti :"$port" >/dev/null 2>&1; then
        echo -e "${RED}ERROR: ${reason}; refusing to ${action} an unknown process.${NC}" >&2
        return 1
    fi
}

wait_for_url() {
    local url="$1"
    local timeout="${2:-30}"
    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        if curl -sf "$url" > /dev/null 2>&1; then
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
    for port in 8090 5173 5175 5177; do
        if ! require_free_port "$port" "port ${port} is still occupied after ayb stop" "kill"; then
            return 1
        fi
    done
    sleep 1
}

cleanup_demo_e2e_resources() {
    local demo_pid="${1:-}"
    local fake_ollama_pid="${2:-}"
    local log="${3:-}"
    local fake_ollama_log="${4:-}"
    local data_dir="${5:-}"
    local demo_child_pids=""

    if [ -n "$demo_pid" ]; then
        demo_child_pids="$(ps -o pid= --ppid "$demo_pid" 2>/dev/null || true)"
        kill -INT "$demo_pid" 2>/dev/null || true
        for child_pid in $demo_child_pids; do
            kill -INT "$child_pid" 2>/dev/null || true
        done
        local wait_count=0
        while kill -0 "$demo_pid" 2>/dev/null && [ $wait_count -lt 20 ]; do
            sleep 0.5
            wait_count=$((wait_count + 1))
        done
        kill -9 "$demo_pid" 2>/dev/null || true
        for child_pid in $demo_child_pids; do
            if kill -0 "$child_pid" 2>/dev/null; then
                kill -9 "$child_pid" 2>/dev/null || true
            fi
        done
    fi
    if [ -n "$data_dir" ]; then
        AYB_DATABASE_EMBEDDED_DATA_DIR="$data_dir" "$AYB_BIN" stop > /dev/null 2>&1 || true
    fi
    if [ -n "$fake_ollama_pid" ]; then
        kill -9 "$fake_ollama_pid" 2>/dev/null || true
    fi
    rm -f "$fake_ollama_log" "$log"
    if [ -n "$data_dir" ]; then
        rm -rf "$data_dir"
    fi
}

# Runs a demo, waits for health, runs its Playwright suite, tears down.
# Args: demo_name demo_port example_dir
run_demo_e2e() {
    local name="$1"
    local port="$2"
    local example_dir="$3"
    local data_dir
    local log
    local demo_pid=""
    local fake_ollama_log=""
    local fake_ollama_pid=""
    log=$(mktemp /tmp/ayb-demo-e2e-${name}.XXXXXX)
    # Isolate only embedded Postgres data for hermetic demo runs. The shared
    # ~/.ayb/pgbin binary cache stays warm, and /tmp keeps Postgres sockets short.
    data_dir=$(mktemp -d /tmp/ayb-demoe2e.XXXXXX)

    echo -e "\n${CYAN}── E2E: ${name} (port ${port}) ──${NC}\n"

    if [ "$name" = "movies" ]; then
        if ! require_free_port 11434 "movies fake ollama port 11434 is already occupied"; then
            fail "${name}: fake ollama port 11434 is not available"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
            return 1
        fi
        fake_ollama_log=$(mktemp /tmp/ayb-fake-ollama-${name}.XXXXXX)
        node "$example_dir/e2e/fake_ollama_server.cjs" > "$fake_ollama_log" 2>&1 &
        fake_ollama_pid=$!
        if ! wait_for_url "http://127.0.0.1:11434/health" 20; then
            fail "${name}: fake ollama did not become healthy"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
            return 1
        fi
    fi

    # ── Start the demo ──
    (cd "$example_dir" && AYB_DATABASE_EMBEDDED_DATA_DIR="$data_dir" exec "$AYB_BIN" demo "$name") > "$log" 2>&1 &
    demo_pid=$!

    # Wait for AYB server
    if ! wait_for_url "http://127.0.0.1:8090/health" 60; then
        fail "${name}: AYB server did not become healthy"
        echo "    Log tail:"
        tail -20 "$log" | sed 's/^/    /'
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
        if ! ensure_stopped; then
            return 1
        fi
        return 1
    fi

    # Wait for demo app
    if ! wait_for_url "http://127.0.0.1:${port}/" 30; then
        fail "${name}: demo app not responding on port ${port}"
        echo "    Log tail:"
        tail -20 "$log" | sed 's/^/    /'
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
        if ! ensure_stopped; then
            return 1
        fi
        return 1
    fi

    pass "${name}: demo is running"

    # ── Install deps + Playwright browser ──
    echo -e "  ${CYAN}…${NC} Installing dependencies (npm ci)..."
    if ! (cd "$example_dir" && npm ci --prefer-offline --no-audit 2>&1 | tail -1); then
        fail "${name}: npm ci failed"
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
        if ! ensure_stopped; then
            return 1
        fi
        return 1
    fi

    # Install Chromium if not already present (idempotent, fast when cached)
    echo -e "  ${CYAN}…${NC} Ensuring Playwright browser..."
    (cd "$example_dir" && npx playwright install chromium 2>&1 | tail -1) || true

    # ── Run the Playwright suite ──
    echo -e "  ${CYAN}…${NC} Running Playwright tests..."
    local pw_log
    pw_log=$(mktemp /tmp/ayb-pw-${name}.XXXXXX)

    if (cd "$example_dir" && npx playwright test --reporter=list 2>&1) | tee "$pw_log"; then
        pass "${name}: Playwright suite passed"
    else
        fail "${name}: Playwright suite FAILED"
        echo ""
        echo "    Last 30 lines of Playwright output:"
        tail -30 "$pw_log" | sed 's/^/    /'
    fi
    rm -f "$pw_log"

    # ── Tear down ──
    cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir"
    if ! ensure_stopped; then
        fail "${name}: cleanup left occupied ports"
        echo ""
        return 1
    fi

    echo ""
}

# ── Pre-flight ───────────────────────────────────────────────────

echo ""
echo -e "${CYAN}═══ Demo App E2E Tests (Playwright) ═══${NC}"
echo ""

if ! command -v "$AYB_BIN" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: '$AYB_BIN' not found in PATH. Set AYB_BIN env var.${NC}"
    exit 1
fi

if ! command -v npm > /dev/null 2>&1; then
    echo -e "${RED}ERROR: npm not found. Install Node.js.${NC}"
    exit 1
fi

echo -e "Binary: $("$AYB_BIN" version 2>/dev/null || echo 'unknown')"
echo -e "AYB_BIN: $AYB_BIN"
echo -e "Rate limit: ${AYB_AUTH_RATE_LIMIT:-default}"
echo ""

# A stale demo process can make the suite pass against the wrong server.
# Treat occupied managed ports as a hard preflight failure.
ensure_stopped || exit 1

# ── Determine which demos to run ─────────────────────────────────

FILTER="${1:-all}"

# ── Run demo E2E suites ──────────────────────────────────────────

if [ "$FILTER" = "all" ] || [ "$FILTER" = "kanban" ]; then
    run_demo_e2e "kanban" 5173 "$REPO_ROOT/examples/kanban"
    ensure_stopped || exit 1
fi

if [ "$FILTER" = "all" ] || [ "$FILTER" = "live-polls" ]; then
    run_demo_e2e "live-polls" 5175 "$REPO_ROOT/examples/live-polls"
    ensure_stopped || exit 1
fi

if [ "$FILTER" = "all" ] || [ "$FILTER" = "movies" ]; then
    run_demo_e2e "movies" 5177 "$REPO_ROOT/examples/movies"
    ensure_stopped || exit 1
fi

# ── Summary ──────────────────────────────────────────────────────

echo -e "${CYAN}═══ Results: ${PASSED} passed, ${FAILED} failed, ${TOTAL} total ═══${NC}"
echo ""

if [ $FAILED -gt 0 ]; then
    echo -e "${RED}FAIL${NC}"
    exit 1
else
    echo -e "${GREEN}ALL DEMO E2E TESTS PASSED${NC}"
    exit 0
fi
