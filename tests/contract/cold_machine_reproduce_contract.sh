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
  if ! grep -Eq -- "$pattern" "$SCRIPT_PATH"; then
    fail "$description"
  fi
}

require_absent_pattern() {
  local pattern="$1"
  local description="$2"
  if grep -Eq -- "$pattern" "$SCRIPT_PATH"; then
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
require_pattern '-e HOST_HEAD_BINARY_DIR_IN_CONTAINER="\$HOST_HEAD_BINARY_DIR_IN_CONTAINER"[[:space:]]*\\' "docker run HOST_HEAD_BINARY_DIR_IN_CONTAINER env flag missing"
require_pattern '-e HOST_HEAD_BINARY_BASENAME="\$HOST_HEAD_BINARY_BASENAME"[[:space:]]*\\' "docker run HOST_HEAD_BINARY_BASENAME env flag missing"
require_pattern '"\$TARGET_IMAGE"[[:space:]]*\\' "docker run image argument must use TARGET_IMAGE"
require_docker_run_block_order "docker run must place env flags before image and image before command"
require_pattern 'readonly HOST_HEAD_BINARY_PATH="\$\{HOST_HEAD_BINARY_PATH:-\}"' "HOST_HEAD_BINARY_PATH env input missing"
require_pattern 'readonly HOST_HEAD_BINARY_DIR_IN_CONTAINER="/tmp/ayb-head-binary"' "HOST_HEAD_BINARY_DIR_IN_CONTAINER constant missing"
require_pattern 'if \[ -n "\$HOST_HEAD_BINARY_PATH" \]; then' "host head binary conditional missing"
require_pattern 'host_head_binary_input_dir="\$\(dirname \"\$HOST_HEAD_BINARY_PATH\"\)"' "host head binary input directory derivation missing"
require_pattern 'HOST_HEAD_BINARY_DIR="\$\(cd \"\$host_head_binary_input_dir\" && pwd -P\)"' "host head binary directory must be resolved to a physical path before mount"
require_pattern 'HOST_HEAD_BINARY_BASENAME="\$\(basename \"\$HOST_HEAD_BINARY_PATH\"\)"' "host head binary basename derivation missing"
require_pattern 'HOST_HEAD_BINARY_STAGING_DIR="\$\(mktemp -d \"\$\{PWD\}/\.cold_machine_head_overlay\.XXXXXX\"\)"' "host head binary staging directory creation missing"
require_pattern 'cp \"\$HOST_HEAD_BINARY_PATH\" \"\$HOST_HEAD_BINARY_STAGING_DIR/\$HOST_HEAD_BINARY_BASENAME\"' "host head binary staging copy missing"
require_pattern 'chmod \+x \"\$HOST_HEAD_BINARY_STAGING_DIR/\$HOST_HEAD_BINARY_BASENAME\"' "host head binary staging chmod missing"
require_pattern 'HOST_HEAD_BINARY_DIR=\"\$HOST_HEAD_BINARY_STAGING_DIR\"' "docker mount source must switch to staged head binary directory"
require_pattern 'docker_mount_args=\(-v "\$HOST_HEAD_BINARY_DIR:\$HOST_HEAD_BINARY_DIR_IN_CONTAINER:ro"\)' "docker run read-only head binary directory mount missing"
require_pattern 'INSTALL_METADATA="head_binary_overlay"' "head binary overlay metadata marker missing"
require_pattern 'overlay_source_path="\$HOST_HEAD_BINARY_DIR_IN_CONTAINER/\$HOST_HEAD_BINARY_BASENAME"' "overlay source path assembly missing"
require_pattern 'if \[ ! -f "\$overlay_source_path" \]; then' "overlay file existence check missing"
require_pattern 'cp "\$HOME/ayb-head-binary" "\$HOME/\.ayb/bin/ayb"' "head binary overlay copy missing"
require_pattern 'chmod \+x "\$HOME/\.ayb/bin/ayb"' "head binary overlay chmod missing"
require_pattern '"?\$HOME/\.ayb/bin/ayb"? version' "canonical binary version invocation missing"
require_pattern '"?\$HOME/\.ayb/bin/ayb"? start --foreground' "canonical binary start invocation missing"
require_pattern 'useradd -m -s /bin/bash aybuser' "harness must create a non-root container user before starting AYB"
require_pattern 'runuser -u aybuser -- /bin/bash /tmp/ayb-user-run\.sh' "harness must run the install and start path as the non-root container user"
require_pattern 'curl -sS -m 2 -o "\$body_file" -w "%\{http_code\}" "\$health_url"' "bounded /health HTTP poll missing"
require_pattern '\[ "\$http_code" = "200" \]' "health HTTP 200 predicate missing"
require_pattern 'grep -Fq '\''"status":"ok"'\''' "health body status ok predicate missing"

require_absent_pattern 'setup_path' "harness must not call install.sh setup_path"
require_absent_pattern 'export PATH=' "harness must not mutate PATH"

run_with_fake_docker() {
  local fake_root
  local tmp_overlay_dir
  local fake_head_binary
  local run_status=0
  fake_root="$(mktemp -d)"
  tmp_overlay_dir="$(mktemp -d /tmp/ayb-contract-overlay.XXXXXX)"
  mkdir -p "$fake_root"

  cat >"$fake_root/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  version)
    if [ "${2:-}" = "--format" ]; then
      printf '%s\n' "29.2.1"
    else
      printf '%s\n' "fake docker version"
    fi
    ;;
  run)
    shift
    saw_install_url=0
    saw_health_url=0
    saw_overlay_dir_env=0
    saw_overlay_basename_env=0
    saw_overlay_mount=0
    saw_tmp_source_mount=0
    saw_target_image=0
    saw_bash_lc=0
    saw_embedded_script=0
    arg=""

    while [ "$#" -gt 0 ]; do
      arg="${1:-}"
      case "$arg" in
        -e)
          [ "$#" -ge 2 ] || { echo "fake docker run missing value for -e" >&2; exit 2; }
          case "$2" in
            INSTALL_URL=*) saw_install_url=1 ;;
            HEALTH_URL=*) saw_health_url=1 ;;
            HOST_HEAD_BINARY_DIR_IN_CONTAINER=*) saw_overlay_dir_env=1 ;;
            HOST_HEAD_BINARY_BASENAME=*) saw_overlay_basename_env=1 ;;
          esac
          shift 2
          ;;
        -v)
          [ "$#" -ge 2 ] || { echo "fake docker run missing value for -v" >&2; exit 2; }
          case "$2" in
            /tmp/*:/tmp/ayb-head-binary:ro|/tmp:/tmp/ayb-head-binary:ro) saw_tmp_source_mount=1 ;;
            *:/tmp/ayb-head-binary:ro) saw_overlay_mount=1 ;;
          esac
          shift 2
          ;;
        ubuntu:24.04)
          saw_target_image=1
          shift
          ;;
        bash)
          if [ "${2:-}" = "-lc" ]; then
            saw_bash_lc=1
            if [ "${3:-}" ]; then
              if printf '%s' "$3" | grep -Fq 'overlay_source_path="$HOST_HEAD_BINARY_DIR_IN_CONTAINER/$HOST_HEAD_BINARY_BASENAME"' \
                && printf '%s' "$3" | grep -Fq 'cp "$overlay_source_path" /home/aybuser/ayb-head-binary' \
                && printf '%s' "$3" | grep -Fq 'if [ ! -f "$overlay_source_path" ]; then' \
                && printf '%s' "$3" | grep -Fq 'cp "$HOME/ayb-head-binary" "$HOME/.ayb/bin/ayb"' \
                && printf '%s' "$3" | grep -Fq 'chmod +x "$HOME/.ayb/bin/ayb"' \
                && printf '%s' "$3" | grep -Fq '"$HOME/.ayb/bin/ayb" version' \
                && printf '%s' "$3" | grep -Fq '"$HOME/.ayb/bin/ayb" start --foreground'; then
                saw_embedded_script=1
              fi
            fi
            shift 3
          else
            shift
          fi
          ;;
        *)
          shift
          ;;
      esac
    done

    [ "$saw_install_url" -eq 1 ] || { echo "fake docker run missing INSTALL_URL env" >&2; exit 2; }
    [ "$saw_health_url" -eq 1 ] || { echo "fake docker run missing HEALTH_URL env" >&2; exit 2; }
    [ "$saw_overlay_dir_env" -eq 1 ] || { echo "fake docker run missing HOST_HEAD_BINARY_DIR_IN_CONTAINER env" >&2; exit 2; }
    [ "$saw_overlay_basename_env" -eq 1 ] || { echo "fake docker run missing HOST_HEAD_BINARY_BASENAME env" >&2; exit 2; }
    [ "$saw_overlay_mount" -eq 1 ] || { echo "fake docker run missing read-only head overlay mount" >&2; exit 2; }
    [ "$saw_tmp_source_mount" -eq 0 ] || { echo "fake docker run mounted /tmp source path instead of physical host path" >&2; exit 2; }
    [ "$saw_target_image" -eq 1 ] || { echo "fake docker run missing expected target image" >&2; exit 2; }
    [ "$saw_bash_lc" -eq 1 ] || { echo "fake docker run missing bash -lc command" >&2; exit 2; }
    [ "$saw_embedded_script" -eq 1 ] || { echo "fake docker run missing expected head-overlay execution script" >&2; exit 2; }
    printf '%s\n' "PASS: cold-machine install + health contract satisfied"
    ;;
  *)
    printf 'unexpected fake docker invocation: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF
  chmod +x "$fake_root/docker"
  fake_head_binary="$tmp_overlay_dir/ayb-head-binary"
  cat >"$fake_head_binary" <<'EOH'
#!/usr/bin/env bash
exit 0
EOH
  chmod +x "$fake_head_binary"

  PATH="$fake_root:$PATH" HOST_HEAD_BINARY_PATH="$fake_head_binary" bash "$SCRIPT_PATH" || run_status="$?"
  rm -rf "$fake_root" "$tmp_overlay_dir"
  return "$run_status"
}

# Regression test for the Stage 0 harness failure where an empty docker_mount_args
# array tripped nounset before docker run ever reached the container.
fake_output="$(run_with_fake_docker 2>&1)" || fail "harness should succeed with fake docker when no host overlay is configured: $fake_output"
printf '%s\n' "$fake_output" | grep -F 'PASS: cold-machine install + health contract satisfied' >/dev/null \
  || fail "fake docker pass marker missing from harness output"
printf '%s\n' "$fake_output" | grep -E '^STATUS run_outcome=success .*container_exit=0$' >/dev/null \
  || fail "harness status line should report success with fake docker"
printf '%s\n' "$fake_output" | grep -F 'unbound variable' >/dev/null \
  && fail "harness must not trip nounset on an empty docker_mount_args array"

echo "PASS: cold_machine_reproduce contract checks"
