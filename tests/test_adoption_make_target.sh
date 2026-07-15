#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
source "$ROOT_DIR/tests/bash_assert_helpers.sh"

DRY_RUN=$(mktemp)
trap 'rm -f "$DRY_RUN"' EXIT INT HUP TERM

make -C "$ROOT_DIR" -n adoption >"$DRY_RUN"
assert_contains "$DRY_RUN" 'sh scripts/adoption.sh' 'adoption target must delegate to the collector'
assert_not_contains "$ROOT_DIR/Makefile" '/traffic/views' 'Makefile must not embed GitHub traffic endpoints'
assert_not_contains "$ROOT_DIR/Makefile" '/traffic/clones' 'Makefile must not embed GitHub traffic endpoints'
assert_not_contains "$ROOT_DIR/Makefile" 'api.npmjs.org' 'Makefile must not embed npm URLs'
assert_not_contains "$ROOT_DIR/Makefile" '## Human signals' 'Makefile must not embed report Markdown'
assert_not_contains "$ROOT_DIR/Makefile" 'debbie sync' 'Makefile must not invoke Debbie'

printf 'PASS: adoption Make target contract\n'
