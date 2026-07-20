#!/usr/bin/env bash
# Movies real-provider smoke.
#
# Proves the provider-backed movies path uses real Ollama embeddings:
# - examples/movies/seed.sql:10-17 seeds "inception" with dreams/heist text
#   and embedding [0.91,0.12,0.18].
# - examples/movies/schema.sql:71-100 ranks search_movies by vector distance
#   plus text rank.
# - internal/e2e/demo_smoke_test.go:352-356 and
#   internal/server/admin_sql_movies_integration_test.go:85-90 already pin
#   "dreams heist" -> "inception" for the server-owned SQL contract.
# Frontend text search remains owned by ayb.records.list in
# examples/movies/src/lib/ayb.ts; this smoke intentionally exercises the
# server-owned /api/admin/movies/search route through the demo proxy.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$REPO_ROOT/tests/port_helpers.sh"

OLLAMA_PORT=11434
OLLAMA_URL="http://127.0.0.1:${OLLAMA_PORT}"
OLLAMA_EMBED_MODEL="${OLLAMA_EMBED_MODEL:-nomic-embed-text}"
OLLAMA_EMBED_DIM=768
OLLAMA_INSTALL_VERSION="${OLLAMA_INSTALL_VERSION:-0.31.1}"
MODE="${1:-real}"

if [ -z "${AYB_BIN:-}" ]; then
    if [ -x "$REPO_ROOT/ayb" ]; then
        AYB_BIN="$REPO_ROOT/ayb"
    else
        AYB_BIN="ayb"
    fi
fi

SERVER_PORT=""
DATABASE_PORT=""
MOVIES_PORT=""
RUNTIME_HOME=""
DATA_DIR=""
DEMO_LOG=""
OLLAMA_LOG=""
DEMO_PID=""
OLLAMA_PID=""

die() {
    echo -e "${RED}ERROR:${NC} $*" >&2
    exit 1
}

info() {
    echo -e "${CYAN}...${NC} $*"
}

pass() {
    echo -e "${GREEN}PASS:${NC} $*"
}

require_command() {
    local name="$1"
    command -v "$name" >/dev/null 2>&1 || die "$name not found"
}

prepare_isolated_home() {
    local shared_ayb_dir="${HOME}/.ayb"
    mkdir -p "$RUNTIME_HOME/.ayb"
    for cache_name in pg pgbin; do
        if [ -d "$shared_ayb_dir/$cache_name" ]; then
            ln -s "$shared_ayb_dir/$cache_name" "$RUNTIME_HOME/.ayb/$cache_name"
        fi
    done
}

stop_pid() {
    local pid="${1:-}"
    if [ -z "$pid" ]; then
        return 0
    fi
    kill -INT "$pid" 2>/dev/null || true
    local waits=0
    while kill -0 "$pid" 2>/dev/null && [ "$waits" -lt 20 ]; do
        sleep 0.5
        waits=$((waits + 1))
    done
    kill -9 "$pid" 2>/dev/null || true
}

cleanup() {
    if [ -n "$DEMO_PID" ]; then
        local child_pids=""
        child_pids="$(ps -o pid= --ppid "$DEMO_PID" 2>/dev/null || true)"
        stop_pid "$DEMO_PID"
        for child_pid in $child_pids; do
            stop_pid "$child_pid"
        done
    fi
    if [ -n "$RUNTIME_HOME" ] && [ -n "$SERVER_PORT" ]; then
        HOME="$RUNTIME_HOME" "$AYB_BIN" stop --port "$SERVER_PORT" >/dev/null 2>&1 || true
    fi
    stop_pid "$OLLAMA_PID"
    rm -f "$DEMO_LOG" "$OLLAMA_LOG"
    [ -z "$DATA_DIR" ] || rm -rf "$DATA_DIR"
    [ -z "$RUNTIME_HOME" ] || rm -rf "$RUNTIME_HOME"
}

trap cleanup EXIT

start_real_ollama() {
    command -v ollama >/dev/null 2>&1 || die "ollama not found; install the validated CLI from the v${OLLAMA_INSTALL_VERSION} release archive after verifying its sha256sum"
    require_free_port "$OLLAMA_PORT" "ollama port ${OLLAMA_PORT} is already occupied" || exit 1
    OLLAMA_LOG=$(mktemp /tmp/ayb-real-ollama.XXXXXX)
    info "starting real Ollama on ${OLLAMA_URL}"
    OLLAMA_HOST="127.0.0.1:${OLLAMA_PORT}" ollama serve >"$OLLAMA_LOG" 2>&1 &
    OLLAMA_PID=$!
    wait_for_url "$OLLAMA_URL/api/tags" 60 || {
        tail -40 "$OLLAMA_LOG" >&2 || true
        die "ollama serve did not become healthy"
    }
    info "pulling ${OLLAMA_EMBED_MODEL}"
    OLLAMA_HOST="127.0.0.1:${OLLAMA_PORT}" ollama pull "$OLLAMA_EMBED_MODEL" || {
        tail -40 "$OLLAMA_LOG" >&2 || true
        die "ollama pull failed for ${OLLAMA_EMBED_MODEL}"
    }
}

start_fake_ollama() {
    require_command node
    require_free_port "$OLLAMA_PORT" "fake ollama port ${OLLAMA_PORT} is already occupied" || exit 1
    OLLAMA_LOG=$(mktemp /tmp/ayb-fake-ollama-real-provider.XXXXXX)
    info "starting fake Ollama on ${OLLAMA_URL}"
    node "$REPO_ROOT/examples/movies/e2e/fake_ollama_server.cjs" >"$OLLAMA_LOG" 2>&1 &
    OLLAMA_PID=$!
    wait_for_url "$OLLAMA_URL/health" 20 || {
        tail -40 "$OLLAMA_LOG" >&2 || true
        die "fake ollama did not become healthy"
    }
}

embedding_json() {
    local input="$1"
    curl -fsS "$OLLAMA_URL/api/embed" \
        -H 'Content-Type: application/json' \
        -d "$(jq -nc --arg model "$OLLAMA_EMBED_MODEL" --arg text "$input" '{model: $model, input: [$text]}')"
}

assert_direct_ollama_embeddings() {
    local first_json
    local second_json
    local first_dim
    local second_dim
    local vectors_equal
    local failures=0

    first_json="$(embedding_json "dreams heist")" || die "direct Ollama embed request failed"
    second_json="$(embedding_json "quiet family drama")" || die "second direct Ollama embed request failed"
    first_dim="$(printf '%s' "$first_json" | jq '.embeddings[0] | length')"
    second_dim="$(printf '%s' "$second_json" | jq '.embeddings[0] | length')"
    vectors_equal="$(jq -n --argjson a "$first_json" --argjson b "$second_json" \
        '$a.embeddings[0] == $b.embeddings[0]')"

    if [ "$first_dim" != "$OLLAMA_EMBED_DIM" ] || [ "$second_dim" != "$OLLAMA_EMBED_DIM" ]; then
        echo -e "${RED}FAIL:${NC} direct Ollama ${OLLAMA_EMBED_MODEL} dimensions: got ${first_dim}/${second_dim}, want ${OLLAMA_EMBED_DIM}/${OLLAMA_EMBED_DIM}" >&2
        failures=$((failures + 1))
    else
        pass "direct Ollama ${OLLAMA_EMBED_MODEL} returns 768-dimensional embeddings"
    fi

    if [ "$vectors_equal" = "true" ]; then
        echo -e "${RED}FAIL:${NC} direct Ollama returned identical vectors for distinct inputs" >&2
        failures=$((failures + 1))
    else
        pass "direct Ollama embeddings differ for distinct inputs"
    fi

    [ "$failures" -eq 0 ] || return 1
}

start_movies_demo() {
    DATA_DIR=$(mktemp -d /tmp/ayb-movies-real-provider-data.XXXXXX)
    RUNTIME_HOME=$(mktemp -d /tmp/ayb-movies-real-provider-home.XXXXXX)
    DEMO_LOG=$(mktemp /tmp/ayb-movies-real-provider-demo.XXXXXX)
    prepare_isolated_home

    SERVER_PORT=$(pick_free_port 48090 49090 50090 51090 52090) || die "no free port for AYB server"
    DATABASE_PORT=$(pick_free_port 45432 46432 47432 48432 49432) || die "no free port for embedded Postgres"
    MOVIES_PORT=$(pick_free_port 45177 46177 47177 48177 49177) || die "no free port for movies demo"

    info "starting ayb demo movies on app port ${MOVIES_PORT}"
    (cd "$REPO_ROOT/examples/movies" && \
        HOME="$RUNTIME_HOME" \
        AYB_SERVER_PORT="$SERVER_PORT" \
        AYB_DATABASE_EMBEDDED_PORT="$DATABASE_PORT" \
        AYB_DATABASE_EMBEDDED_DATA_DIR="$DATA_DIR" \
        AYB_DEMO_APP_PORT="$MOVIES_PORT" \
        exec "$AYB_BIN" demo movies) >"$DEMO_LOG" 2>&1 &
    DEMO_PID=$!

    wait_for_url "http://127.0.0.1:${SERVER_PORT}/health" 90 || {
        tail -60 "$DEMO_LOG" >&2 || true
        die "AYB server did not become healthy"
    }
    wait_for_url "http://127.0.0.1:${MOVIES_PORT}/" 60 || {
        tail -60 "$DEMO_LOG" >&2 || true
        die "movies demo proxy did not become healthy"
    }
}

assert_movies_search() {
    local response
    local slug
    response="$(curl -fsS "http://127.0.0.1:${MOVIES_PORT}/api/admin/movies/search" \
        -H 'Content-Type: application/json' \
        -d '{"query":"dreams heist","limit":3}')" || die "movies search request failed"
    slug="$(printf '%s' "$response" | jq -r '.rows[0].slug // ""')"
    if [ "$slug" != "inception" ]; then
        printf '%s\n' "$response" >&2
        die "movies search returned first slug ${slug:-<empty>}; want inception"
    fi
    pass "movies demo proxy search returns inception first for dreams heist"
}

note_embedding_json() {
    local note_text="$1"
    curl -fsS "http://127.0.0.1:${MOVIES_PORT}/api/admin/movies/notes/embed" \
        -H 'Content-Type: application/json' \
        -d "$(jq -nc --arg text "$note_text" '{text: $text, movie_slug: "inception"}')"
}

assert_movies_note_embeddings_are_input_sensitive() {
    local first_response
    local second_response
    local first_dim
    local second_dim
    local route_vectors_equal
    first_response="$(note_embedding_json "dream journal bank vault impossible staircase")" || die "first movies note embed request failed"
    second_response="$(note_embedding_json "quiet family dinner neighborhood reconciliation")" || die "second movies note embed request failed"
    first_dim="$(printf '%s' "$first_response" | jq '.embedding | length')"
    second_dim="$(printf '%s' "$second_response" | jq '.embedding | length')"
    route_vectors_equal="$(jq -n --argjson a "$first_response" --argjson b "$second_response" \
        '$a.embedding == $b.embedding')"
    if [ "$first_dim" != "3" ] || [ "$second_dim" != "3" ]; then
        printf '%s\n%s\n' "$first_response" "$second_response" >&2
        echo -e "${RED}FAIL:${NC} server note embeddings dimensions: got ${first_dim}/${second_dim}, want 3/3" >&2
        return 1
    fi
    if [ "$route_vectors_equal" = "true" ]; then
        printf '%s\n%s\n' "$first_response" "$second_response" >&2
        echo -e "${RED}FAIL:${NC} server note embeddings are identical for distinct note texts" >&2
        return 1
    fi
    pass "server note embeddings differ for distinct note texts"
}

assert_movies_note_embedding_uses_real_provider() {
    local note_text="dream journal bank vault impossible staircase"
    local provider_response
    local response
    local provider_embedding_dim
    local embedding_dim
    local matches_provider_vector
    provider_response="$(embedding_json "$note_text")" || die "direct Ollama note embed request failed"
    provider_embedding_dim="$(printf '%s' "$provider_response" | jq '.embeddings[0] | length')"
    response="$(note_embedding_json "$note_text")" || die "movies note embed request failed"
    embedding_dim="$(printf '%s' "$response" | jq '.embedding | length')"
    matches_provider_vector="$(jq -n \
        --argjson provider "$provider_response" \
        --argjson route "$response" \
        '$route.embedding == $provider.embeddings[0][0:3]')"
    if [ "$provider_embedding_dim" != "$OLLAMA_EMBED_DIM" ]; then
        echo -e "${RED}FAIL:${NC} direct Ollama note embedding has ${provider_embedding_dim} dimensions; want ${OLLAMA_EMBED_DIM}" >&2
        return 1
    fi
    if [ "$embedding_dim" != "3" ]; then
        printf '%s\n' "$response" >&2
        echo -e "${RED}FAIL:${NC} movies note embed returned ${embedding_dim}-dimensional fitted vector; want 3" >&2
        return 1
    fi
    if [ "$matches_provider_vector" != "true" ]; then
        printf '%s\n' "$response" >&2
        echo -e "${RED}FAIL:${NC} movies note embed does not match the first 3 values returned by real Ollama" >&2
        return 1
    fi
    pass "server note embedding matches real Ollama after VECTOR(3) fit"
}

run_fake_provider_red() {
    local server_route_proof_failed=false
    start_fake_ollama
    start_movies_demo

    assert_direct_ollama_embeddings || true
    assert_movies_search || true
    if ! assert_movies_note_embeddings_are_input_sensitive; then
        server_route_proof_failed=true
    fi
    if [ "$server_route_proof_failed" = "true" ]; then
        die "fake provider red mode failed the server note-embed input-sensitivity proof as expected"
    fi
    echo -e "${RED}FAIL:${NC} fake provider unexpectedly passed the server note-embed input-sensitivity proof" >&2
}

main() {
    case "$MODE" in
        real) ;;
        # This script is the single owner of the validated Ollama version; CI reads
        # it from here instead of duplicating the literal in a workflow.
        --print-ollama-install-version)
            printf '%s\n' "$OLLAMA_INSTALL_VERSION"
            return 0
            ;;
        --fake-provider-red) ;;
        *) die "usage: bash _dev/manual_smoke_tests/19_movies_real_provider.test.sh [--fake-provider-red|--print-ollama-install-version]" ;;
    esac

    require_command curl
    require_command jq
    require_command lsof
    command -v "$AYB_BIN" >/dev/null 2>&1 || die "'$AYB_BIN' not found in PATH; set AYB_BIN"

    echo -e "${CYAN}Movies Real-Provider Smoke${NC}"
    echo "AYB_BIN: $AYB_BIN"
    echo "Ollama model: $OLLAMA_EMBED_MODEL"
    # BYOK/chat realism is intentionally out of scope for this provider smoke.

    if [ "$MODE" = "--fake-provider-red" ]; then
        run_fake_provider_red
        return 0
    fi

    start_real_ollama
    assert_direct_ollama_embeddings
    start_movies_demo
    assert_movies_search
    assert_movies_note_embeddings_are_input_sensitive
    assert_movies_note_embedding_uses_real_provider
    pass "real-provider movies smoke completed"
}

main "$@"
