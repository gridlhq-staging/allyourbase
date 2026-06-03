#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/tests/contract/pg_arm64_asset_arch.sh"
source "$ROOT_DIR/tests/bash_assert_helpers.sh"

[[ -f "$SCRIPT_PATH" ]] || fail "missing $SCRIPT_PATH"

# Keep Stage 4 probe decoupled from unrelated pgmanager test-target health.
# Reject any full-package target token (bare, trailing slash, or Go recursive
# wildcard `/...`) regardless of flag order or shell quoting.
forbidden_pkg_target_pattern='(^|[[:space:]\"'"'"';|&()<>])\./internal/pgmanager(/(\.\.\.)?)?([[:space:]\"'"'"';|&()<>]|$)'
if grep -Eq "$forbidden_pkg_target_pattern" "$SCRIPT_PATH"; then
  fail "contract probe must not target bare ./internal/pgmanager"
fi

# Guard the guard: quoted and unquoted full-package forms must match as forbidden.
if ! printf '%s\n' 'go test ./internal/pgmanager -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch unquoted ./internal/pgmanager"
fi
if ! printf '%s\n' 'go test "./internal/pgmanager" -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch quoted ./internal/pgmanager"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager/ -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch unquoted ./internal/pgmanager/ with trailing slash"
fi
if ! printf '%s\n' 'go test "./internal/pgmanager/" -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch quoted ./internal/pgmanager/ with trailing slash"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager/... -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch unquoted ./internal/pgmanager/... recursive wildcard"
fi
if ! printf '%s\n' 'go test "./internal/pgmanager/..." -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must catch quoted ./internal/pgmanager/... recursive wildcard"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager; echo fallback' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must treat semicolon as token boundary for ./internal/pgmanager"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager/...; echo fallback' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must treat semicolon as token boundary for ./internal/pgmanager/..."
fi
if ! printf '%s\n' 'echo prep;go test ./internal/pgmanager -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must treat semicolon as prefix token boundary for ./internal/pgmanager"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager>/tmp/guard.log' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must treat redirection as token boundary for ./internal/pgmanager"
fi
if ! printf '%s\n' 'go test ./internal/pgmanager/...<input.txt' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must treat redirection as token boundary for ./internal/pgmanager/..."
fi
if printf '%s\n' 'go test ./internal/pgmanager/platform.go ./internal/pgmanager/platform_contract_probe_test.go -run "$CONTRACT_PROBE_TEST"' | grep -Eq "$forbidden_pkg_target_pattern"; then
  fail "forbidden package-target matcher must allow file-scoped owner inputs"
fi

# Require the owner-bounded file-scoped probe inputs.
assert_contains "$SCRIPT_PATH" "./internal/pgmanager/platform.go" "contract probe must include platform.go owner input"
assert_contains "$SCRIPT_PATH" "./internal/pgmanager/platform_contract_probe_test.go" "contract probe must include contract probe test input"

# Keep deterministic test-name gate without requiring one exact command spelling.
assert_contains "$SCRIPT_PATH" "-run \"\$CONTRACT_PROBE_TEST\"" "contract probe must keep test-name gate for deterministic output"

echo "PASS: pg_arm64_asset_arch guard checks"
