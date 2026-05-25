#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/tools/cold_machine_reproduce.sh"

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

require_pattern() {
  local pattern="$1"
  local description="$2"
  if ! grep -Eq "$pattern" "$SCRIPT_PATH"; then
    fail "$description"
  fi
}

require_absent_pattern() {
  local pattern="$1"
  local description="$2"
  if grep -Eq "$pattern" "$SCRIPT_PATH"; then
    fail "$description"
  fi
}

require_line_order() {
  local first_pattern="$1"
  local second_pattern="$2"
  local description="$3"
  local first_line second_line
  first_line="$(grep -nE -- "$first_pattern" "$SCRIPT_PATH" | head -n 1 | cut -d: -f1)"
  second_line="$(grep -nE -- "$second_pattern" "$SCRIPT_PATH" | head -n 1 | cut -d: -f1)"
  if [ -z "$first_line" ] || [ -z "$second_line" ] || [ "$first_line" -ge "$second_line" ]; then
    fail "$description"
  fi
}

require_docker_run_block_order() {
  local description="$1"
  if ! awk '
    /docker run --rm/ { docker_line = NR }
    /^[[:space:]]+-e INSTALL_URL=/ { install_env_line = NR }
    /^[[:space:]]+-e HEALTH_URL=/ { health_env_line = NR }
    /^[[:space:]]+"\$TARGET_IMAGE"/ { image_line = NR }
    /^[[:space:]]+bash -lc / { command_line = NR }
    END {
      exit !(docker_line && install_env_line && health_env_line && image_line && command_line \
        && docker_line < install_env_line \
        && install_env_line < health_env_line \
        && health_env_line < image_line \
        && image_line < command_line)
    }
  ' "$SCRIPT_PATH"; then
    fail "$description"
  fi
}

[ -f "$SCRIPT_PATH" ] || fail "missing harness script at tools/cold_machine_reproduce.sh"

require_pattern '^resolve_docker_host\(\)' "resolve_docker_host() function missing"
require_pattern 'readonly TARGET_IMAGE="\$\{1:-ubuntu:24\.04\}"' "TARGET_IMAGE must default from the optional first argument"
require_pattern 'FAIL_C0' "FAIL_C0 marker missing"
require_pattern 'FAIL_C1A' "FAIL_C1A marker missing"
require_pattern 'FAIL_C1B' "FAIL_C1B marker missing"
require_pattern 'FAIL_C2' "FAIL_C2 marker missing"
require_pattern 'FAIL_H0' "FAIL_H0 marker missing"
require_pattern 'docker run --rm[[:space:]]*\\' "docker run invocation missing"
require_pattern '-e INSTALL_URL="\$INSTALL_URL"[[:space:]]*\\' "docker run INSTALL_URL env flag missing"
require_pattern '-e HEALTH_URL="\$HEALTH_URL"[[:space:]]*\\' "docker run HEALTH_URL env flag missing"
require_pattern '"\$TARGET_IMAGE"[[:space:]]*\\' "docker run image argument must use TARGET_IMAGE"
require_docker_run_block_order "docker run must place env flags before image and image before command"
require_pattern '"?\$HOME/\.ayb/bin/ayb"? version' "canonical binary version invocation missing"
require_pattern '"?\$HOME/\.ayb/bin/ayb"? start --foreground' "canonical binary start invocation missing"
require_pattern 'curl -sS -m 2 -o "\$body_file" -w "%\{http_code\}" "\$health_url"' "bounded /health HTTP poll missing"
require_pattern '\[ "\$http_code" = "200" \]' "health HTTP 200 predicate missing"
require_pattern 'grep -Fq '\''"status":"ok"'\''' "health body status ok predicate missing"

require_absent_pattern 'setup_path' "harness must not call install.sh setup_path"
require_absent_pattern 'export PATH=' "harness must not mutate PATH"

echo "PASS: cold_machine_reproduce contract checks"
