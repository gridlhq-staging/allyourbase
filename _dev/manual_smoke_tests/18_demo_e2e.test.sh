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
source "$REPO_ROOT/tests/port_helpers.sh"

# Use AYB_BIN if set, else try repo-root build, else PATH
if [ -z "${AYB_BIN:-}" ]; then
    if [ -x "$REPO_ROOT/ayb" ]; then
        AYB_BIN="$REPO_ROOT/ayb"
    else
        AYB_BIN="ayb"
    fi
fi
SHARED_AYB_DIR="${HOME}/.ayb"
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

# Isolated demo-app ports selected at preflight (see pick_free_port). The
# release gate must NOT require the universal Vite defaults (5173/5175/5177) to
# be globally free, because those collide with unrelated dev servers on a shared
# host. These are the ports ensure_stopped verifies between runs.
MANAGED_PORTS=()
SERVER_PORT=""
DATABASE_PORT=""
MOVIES_FAKE_OLLAMA_PORT=""

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

ensure_stopped() {
    local wait_count
    sleep 1
    # Every managed port is selected at preflight. Never inspect or stop the
    # universal defaults, which may belong to another worktree on a shared host.
    # ${arr[@]+...} keeps empty-array expansion safe under `set -u` on bash 3.2.
    for port in ${MANAGED_PORTS[@]+"${MANAGED_PORTS[@]}"}; do
        if [ -z "$port" ]; then
            continue
        fi
        # `ayb stop` can return while managed Postgres is still releasing its
        # listener. Give owned teardown a bounded grace period before treating
        # the remaining listener as an unknown process.
        wait_count=0
        while lsof -ti :"$port" >/dev/null 2>&1 && [ "$wait_count" -lt 20 ]; do
            sleep 0.5
            wait_count=$((wait_count + 1))
        done
        if ! require_free_port "$port" "port ${port} is still occupied after ayb stop" "kill"; then
            return 1
        fi
    done
    sleep 1
}

prepare_isolated_home() {
    local runtime_home="$1"
    local cache_name
    mkdir -p "$runtime_home/.ayb"
    # Runtime state stays isolated, while immutable/downloaded Postgres assets
    # reuse the normal cache so each demo does not download or extract binaries.
    for cache_name in pg pgbin; do
        if [ -d "$SHARED_AYB_DIR/$cache_name" ]; then
            ln -s "$SHARED_AYB_DIR/$cache_name" "$runtime_home/.ayb/$cache_name"
        fi
    done
}

validate_port_number() {
    local name="$1"
    local value="$2"
    local normalized_value
    case "$value" in
        ""|*[!0123456789]*)
            echo -e "${RED}ERROR: ${name} must be an ASCII digits-only integer in the range 1..65535${NC}" >&2
            return 1
            ;;
    esac
    normalized_value="${value#"${value%%[!0]*}"}"
    if [ -z "$normalized_value" ]; then
        normalized_value=0
    fi
    if [ "${#normalized_value}" -gt 5 ] ||
        [ "$normalized_value" -lt 1 ] ||
        [ "$normalized_value" -gt 65535 ]; then
        echo -e "${RED}ERROR: ${name} must be in the range 1..65535${NC}" >&2
        return 1
    fi
}

generate_demo_jwt_secret() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
        return
    fi
    if [ -r /dev/urandom ]; then
        od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
        return
    fi
    echo -e "${RED}ERROR: unable to generate a demo JWT secret${NC}" >&2
    return 1
}

resolve_movies_fake_ollama_port() {
    if [ -n "${AYB_MOVIES_FAKE_OLLAMA_PORT:-}" ]; then
        validate_port_number "AYB_MOVIES_FAKE_OLLAMA_PORT" "$AYB_MOVIES_FAKE_OLLAMA_PORT" || return 1
        printf '%s\n' "$AYB_MOVIES_FAKE_OLLAMA_PORT"
        return 0
    fi

    # 11434 is the real Ollama default; requiring it to be free caused false failures on shared hosts.
    pick_free_port 45514 46514 47514 48514 49514
}

materialize_movies_config() {
    local data_dir="$1"
    local fixture_port="$2"
    local source_config="$REPO_ROOT/examples/movies/ayb.toml"
    local temp_config="$data_dir/movies-ayb.toml"
    local source_match_count
    source_match_count=$(grep -c 'base_url = "http://127.0.0.1:11434"' "$source_config" || true)
    if [ "$source_match_count" -ne 1 ]; then
        echo "ERROR: expected exactly one movies Ollama base_url in $source_config, found $source_match_count" >&2
        return 1
    fi
    cp "$source_config" "$temp_config"
    sed -i.bak "s|base_url = \"http://127.0.0.1:11434\"|base_url = \"http://127.0.0.1:${fixture_port}\"|" "$temp_config"
    rm -f "$temp_config.bak"
    printf '%s\n' "$temp_config"
}

cleanup_demo_e2e_resources() {
    local demo_pid="${1:-}"
    local fake_ollama_pid="${2:-}"
    local log="${3:-}"
    local fake_ollama_log="${4:-}"
    local data_dir="${5:-}"
    local runtime_home="${6:-}"
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
        HOME="$runtime_home" "$AYB_BIN" stop --port "$SERVER_PORT" > /dev/null 2>&1 || true
    fi
    if [ -n "$fake_ollama_pid" ]; then
        kill -9 "$fake_ollama_pid" 2>/dev/null || true
    fi
    rm -f "$fake_ollama_log" "$log"
    if [ -n "$data_dir" ]; then
        rm -rf "$data_dir"
    fi
    if [ -n "$runtime_home" ]; then
        rm -rf "$runtime_home"
    fi
}

# Runs a demo, waits for health, runs its Playwright suite, tears down.
# Args: demo_name demo_port example_dir
run_demo_e2e() {
    local name="$1"
    local port="$2"
    local example_dir="$3"
    local data_dir
    local pg_data_dir
    local runtime_home
    local log
    local demo_pid=""
    local fake_ollama_log=""
    local fake_ollama_pid=""
    local movies_config=""
    local movies_jwt_secret=""
    log=$(mktemp /tmp/ayb-demo-e2e-${name}.XXXXXX)
    # Isolate mutable runtime and Postgres data. The shared binary cache stays
    # warm, and /tmp keeps Postgres sockets short.
    data_dir=$(mktemp -d /tmp/ayb-demoe2e.XXXXXX)
    pg_data_dir="$data_dir/pgdata"
    runtime_home=$(mktemp -d /tmp/ayb-demohome.XXXXXX)
    prepare_isolated_home "$runtime_home"

    # Serve this demo on the isolated port and point Playwright's config at the
    # same port. Keep both the launched demo surface and any Playwright-managed
    # fallback dev server off universal defaults.
    export AYB_DEMO_APP_PORT="$port"
    export AYB_SERVER_URL="http://127.0.0.1:${SERVER_PORT}"

    echo -e "\n${CYAN}── E2E: ${name} (port ${port}) ──${NC}\n"

    if [ "$name" = "movies" ]; then
        movies_jwt_secret="$(generate_demo_jwt_secret)" || {
            fail "${name}: could not generate a per-run JWT secret"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            return 1
        }
        if ! require_free_port "$MOVIES_FAKE_OLLAMA_PORT" "movies fake ollama port ${MOVIES_FAKE_OLLAMA_PORT} is already occupied"; then
            fail "${name}: fake ollama port ${MOVIES_FAKE_OLLAMA_PORT} is not available"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            return 1
        fi
        fake_ollama_log=$(mktemp /tmp/ayb-fake-ollama-${name}.XXXXXX)
        node "$example_dir/e2e/fake_ollama_server.cjs" > "$fake_ollama_log" 2>&1 &
        fake_ollama_pid=$!
        if ! wait_for_url "http://127.0.0.1:${MOVIES_FAKE_OLLAMA_PORT}/health" 20; then
            fail "${name}: fake ollama did not become healthy"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            return 1
        fi
        movies_config=$(materialize_movies_config "$data_dir" "$MOVIES_FAKE_OLLAMA_PORT") || {
            fail "${name}: temporary config materialization failed"
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            return 1
        }
        if ! (
            cd "$example_dir" || exit 1
            HOME="$runtime_home" \
            AYB_SERVER_PORT="$SERVER_PORT" \
            AYB_DATABASE_EMBEDDED_PORT="$DATABASE_PORT" \
            AYB_DATABASE_EMBEDDED_DATA_DIR="$pg_data_dir" \
            AYB_AUTH_ENABLED=true \
            AYB_AUTH_JWT_SECRET="$movies_jwt_secret" \
            AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true \
            AYB_AUTH_MAGIC_LINK_ENABLED=true \
            AYB_SERVER_SITE_URL="http://localhost:${port}" \
            "$AYB_BIN" start --config "$movies_config"
        ) >> "$log" 2>&1; then
            fail "${name}: AYB server failed to start with temporary config"
            echo "    Log tail:"
            tail -20 "$log" | sed 's/^/    /'
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            return 1
        fi
        if ! wait_for_url "http://127.0.0.1:${SERVER_PORT}/health" 60; then
            fail "${name}: AYB server did not become healthy"
            echo "    Log tail:"
            tail -20 "$log" | sed 's/^/    /'
            cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
            if ! ensure_stopped; then
                return 1
            fi
            return 1
        fi
    fi

    # ── Start the demo ──
    (
        cd "$example_dir" || exit 1
        export HOME="$runtime_home"
        export AYB_SERVER_PORT="$SERVER_PORT"
        export AYB_DATABASE_EMBEDDED_PORT="$DATABASE_PORT"
        export AYB_DATABASE_EMBEDDED_DATA_DIR="$pg_data_dir"
        if [ "$name" = "movies" ]; then \
            AYB_AUTH_ENABLED=true \
            AYB_AUTH_JWT_SECRET="$movies_jwt_secret" \
            AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true \
            AYB_AUTH_MAGIC_LINK_ENABLED=true \
            AYB_SERVER_SITE_URL="http://localhost:${port}" \
            exec "$AYB_BIN" demo "$name"; \
        else \
            exec "$AYB_BIN" demo "$name"; \
        fi
    ) > "$log" 2>&1 &
    demo_pid=$!

    # Wait for AYB server
    if ! wait_for_url "http://127.0.0.1:${SERVER_PORT}/health" 60; then
        fail "${name}: AYB server did not become healthy"
        echo "    Log tail:"
        tail -20 "$log" | sed 's/^/    /'
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
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
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
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
        cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
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
    local playwright_status=0
    local previous_dir
    local guard_status=0
    pw_log=$(mktemp /tmp/ayb-pw-${name}.XXXXXX)

    (cd "$example_dir" && AYB_DEMO_EXTERNAL_SERVER=1 npx playwright test 2>&1) | tee "$pw_log"
    playwright_status=${PIPESTATUS[0]}

    previous_dir="$(pwd)"
    cd "$REPO_ROOT" || return 1
    bash scripts/check-playwright-executed.sh "$example_dir/playwright-report/results.json" "$name"
    guard_status=$?
    cd "$previous_dir" || return 1

    if [ "$playwright_status" -eq 0 ] && [ "$guard_status" -eq 0 ]; then
        pass "${name}: Playwright suite passed"
    else
        if [ "$playwright_status" -ne 0 ]; then
            fail "${name}: Playwright suite FAILED"
            echo ""
            echo "    Last 30 lines of Playwright output:"
            tail -30 "$pw_log" | sed 's/^/    /'
        fi
        if [ "$guard_status" -ne 0 ]; then
            fail "${name}: Playwright execution guard FAILED"
        fi
        playwright_status=1
    fi
    rm -f "$pw_log"

    # ── Tear down ──
    cleanup_demo_e2e_resources "$demo_pid" "$fake_ollama_pid" "$log" "$fake_ollama_log" "$data_dir" "$runtime_home"
    if ! ensure_stopped; then
        fail "${name}: cleanup left occupied ports"
        echo ""
        return 1
    fi

    echo ""
    return "$playwright_status"
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

# ── Determine which demos to run ─────────────────────────────────

FILTER="${1:-all}"

if [ "$FILTER" = "all" ] || [ "$FILTER" = "movies" ]; then
    if [ -n "${AYB_MOVIES_FAKE_OLLAMA_PORT:-}" ]; then
        validate_port_number "AYB_MOVIES_FAKE_OLLAMA_PORT" "$AYB_MOVIES_FAKE_OLLAMA_PORT" || exit 1
    fi
fi

# Select isolated, currently-free app ports before any ensure_stopped call so
# the gate never depends on the universal Vite defaults (5173/5175/5177) being
# globally free on a shared host. Candidates stay in the high-port range.
SERVER_PORT=$(pick_free_port 48090 49090 50090 51090 52090) || { echo -e "${RED}ERROR: no free port for AYB demo server${NC}"; exit 1; }
DATABASE_PORT=$(pick_free_port 45432 46432 47432 48432 49432) || { echo -e "${RED}ERROR: no free port for embedded Postgres${NC}"; exit 1; }
KANBAN_PORT=$(pick_free_port 45173 46173 47173 48173 49173) || { echo -e "${RED}ERROR: no free port for kanban demo${NC}"; exit 1; }
POLLS_PORT=$(pick_free_port 45175 46175 47175 48175 49175) || { echo -e "${RED}ERROR: no free port for live-polls demo${NC}"; exit 1; }
MOVIES_PORT=$(pick_free_port 45177 46177 47177 48177 49177) || { echo -e "${RED}ERROR: no free port for movies demo${NC}"; exit 1; }
MANAGED_PORTS=("$SERVER_PORT" "$DATABASE_PORT" "$KANBAN_PORT" "$POLLS_PORT" "$MOVIES_PORT")

if [ "$FILTER" = "all" ] || [ "$FILTER" = "movies" ]; then
    MOVIES_FAKE_OLLAMA_PORT=$(resolve_movies_fake_ollama_port) || { echo -e "${RED}ERROR: no free port for movies fake Ollama fixture${NC}"; exit 1; }
    export AYB_MOVIES_FAKE_OLLAMA_PORT="$MOVIES_FAKE_OLLAMA_PORT"
fi

# A stale demo process can make the suite pass against the wrong server.
# Treat occupied managed ports as a hard preflight failure.
ensure_stopped || exit 1

if [ -n "$MOVIES_FAKE_OLLAMA_PORT" ]; then
    MANAGED_PORTS=("${MANAGED_PORTS[@]}" "$MOVIES_FAKE_OLLAMA_PORT")
fi

# ── Run demo E2E suites ──────────────────────────────────────────

if [ "$FILTER" = "all" ] || [ "$FILTER" = "kanban" ]; then
    run_demo_e2e "kanban" "$KANBAN_PORT" "$REPO_ROOT/examples/kanban"
    ensure_stopped || exit 1
fi

if [ "$FILTER" = "all" ] || [ "$FILTER" = "live-polls" ]; then
    run_demo_e2e "live-polls" "$POLLS_PORT" "$REPO_ROOT/examples/live-polls"
    ensure_stopped || exit 1
fi

if [ "$FILTER" = "all" ] || [ "$FILTER" = "movies" ]; then
    run_demo_e2e "movies" "$MOVIES_PORT" "$REPO_ROOT/examples/movies"
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
