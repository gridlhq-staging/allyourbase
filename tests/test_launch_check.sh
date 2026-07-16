#!/bin/sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CHECKER="$ROOT_DIR/scripts/launch-check.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT INT HUP TERM

FAKE_BIN="$TEST_DIR/bin"
mkdir -p "$FAKE_BIN"
LOG_FILE="$TEST_DIR/commands.log"
: >"$LOG_FILE"

FULL_SHA=0123456789abcdef0123456789abcdef01234567

write_fake_commands() {
  cat >"$FAKE_BIN/gh" <<'GH'
#!/bin/sh
set -eu
printf 'gh %s\n' "$*" >>"$LAUNCH_CHECK_FAKE_LOG"
scenario="${LAUNCH_CHECK_SCENARIO:-success}"
full_sha=0123456789abcdef0123456789abcdef01234567
repo=AllyourbaseHQ/allyourbase

case "$1 $2" in
  "release list")
    [ "$3" = "-R" ] && [ "$4" = "$repo" ] || exit 64
    case "$scenario" in
      no_app_release) printf '%s\n' '[{"tagName":"pg-16","isDraft":false},{"tagName":"v0.0.18-beta","isDraft":true}]' ;;
      release_row_not_object) printf '%s\n' '["not-a-release",{"tagName":"v0.0.17-beta","isDraft":false}]' ;;
      release_trailing_row_not_object) printf '%s\n' '[{"tagName":"v0.0.17-beta","isDraft":false},"not-a-release"]' ;;
      release_missing_tag) printf '%s\n' '[{"isDraft":false},{"tagName":"v0.0.17-beta","isDraft":false}]' ;;
      release_missing_is_draft) printf '%s\n' '[{"tagName":"v0.0.18-beta"}]' ;;
      *) printf '%s\n' '[{"tagName":"v0.0.18-beta","isDraft":true},{"tagName":"pg-16","isDraft":false},{"tagName":"v0.0.17-beta","isDraft":false}]' ;;
    esac
    ;;
  "release view")
    [ "$3" = "v0.0.17-beta" ] && [ "$4" = "-R" ] && [ "$5" = "$repo" ] || exit 64
    case "$scenario" in
      missing_asset)
        printf '%s\n' '{"assets":[{"name":"ayb_0.0.17-beta_darwin_amd64.tar.gz"},{"name":"ayb_0.0.17-beta_darwin_arm64.tar.gz"},{"name":"ayb_0.0.17-beta_linux_amd64.tar.gz"},{"name":"ayb_0.0.17-beta_linux_arm64.tar.gz"},{"name":"ayb_0.0.17-beta_windows_amd64.zip"},{"name":"checksums.txt"}]}'
        ;;
      malformed_release_view) printf '%s\n' '{"assets":' ;;
      *)
        printf '%s\n' '{"assets":[{"name":"ayb_0.0.17-beta_darwin_amd64.tar.gz"},{"name":"ayb_0.0.17-beta_darwin_arm64.tar.gz"},{"name":"ayb_0.0.17-beta_linux_amd64.tar.gz"},{"name":"ayb_0.0.17-beta_linux_arm64.tar.gz"},{"name":"ayb_0.0.17-beta_windows_amd64.zip"},{"name":"ayb_0.0.17-beta_windows_arm64.zip"},{"name":"checksums.txt"}]}'
        ;;
    esac
    ;;
  "api repos/AllyourbaseHQ/allyourbase/commits/main")
    case "$scenario" in
      ci_short_sha) printf '%s\n' 0123456789abcdef ;;
      *) printf '%s\n' "$full_sha" ;;
    esac
    ;;
  "run list")
    case "$scenario" in
      ci_zero) printf '%s\n' '[{"databaseId":101,"workflowName":"Docker","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success"}]' ;;
      ci_malformed) printf '%s\n' '[{"databaseId":' ;;
      ci_empty) printf '%s\n' '[]' ;;
      ci_duplicate) printf '%s\n' '[{"databaseId":201,"workflowName":"CI","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success"},{"databaseId":202,"workflowName":"CI","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success"}]' ;;
      ci_mismatched_head) printf '%s\n' '[{"databaseId":301,"workflowName":"CI","headSha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"completed","conclusion":"success"}]' ;;
      ci_in_progress) printf '%s\n' '[{"databaseId":401,"workflowName":"CI","headSha":"0123456789abcdef0123456789abcdef01234567","status":"in_progress","conclusion":""}]' ;;
      ci_failure) printf '%s\n' '[{"databaseId":501,"workflowName":"CI","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"failure"}]' ;;
      *) printf '%s\n' '[{"databaseId":601,"workflowName":"Docker","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success"},{"databaseId":602,"workflowName":"CI","headSha":"0123456789abcdef0123456789abcdef01234567","status":"completed","conclusion":"success"}]' ;;
    esac
    ;;
  *) exit 64 ;;
esac
GH

  cat >"$FAKE_BIN/curl" <<'CURL'
#!/bin/sh
set -eu
printf 'curl %s\n' "$*" >>"$LAUNCH_CHECK_FAKE_LOG"
scenario="${LAUNCH_CHECK_SCENARIO:-success}"
out_file=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out_file="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done

case "$scenario:$url" in
  url_docs:https://allyourbase.io|url_installer:https://install.allyourbase.io/install.sh|url_demo:https://demo.allyourbase.io|url_kanban:https://kanban.demo.allyourbase.io|url_polls:https://polls.demo.allyourbase.io|url_movies:https://movies.demo.allyourbase.io|url_instantsearch:https://instantsearch.demo.allyourbase.io|url_health:https://api.allyourbase.io/health)
    [ -n "$out_file" ] && printf 'broken' >"$out_file"
    printf '500 %s\n' "$url"
    exit 0
    ;;
  health_bad_json:https://api.allyourbase.io/health)
    [ -n "$out_file" ] && printf '{"status":"ok","database":"down"}' >"$out_file"
    printf '200 %s\n' "$url"
    exit 0
    ;;
  *:https://api.allyourbase.io/health)
    [ -n "$out_file" ] && printf '{"status":"ok","database":"ok"}' >"$out_file"
    printf '200 %s\n' "$url"
    exit 0
    ;;
  *)
    [ -n "$out_file" ] && printf 'ok' >"$out_file"
    printf '200 %s/\n' "$url"
    exit 0
    ;;
esac
CURL

  cat >"$FAKE_BIN/docker" <<'DOCKER'
#!/bin/sh
set -eu
printf 'docker %s DOCKER_CONFIG=%s DOCKER_HOST=%s\n' "$*" "${DOCKER_CONFIG:-}" "${DOCKER_HOST:-}" >>"$LAUNCH_CHECK_FAKE_LOG"
scenario="${LAUNCH_CHECK_SCENARIO:-success}"
case "$1" in
  pull)
    [ "$2" = "ghcr.io/allyourbasehq/allyourbase:0.0.17-beta" ] || exit 65
    if [ "$scenario" = "docker_pull" ]; then
      exit 66
    fi
    ;;
  run)
    if [ "$2" = "--rm" ] && [ "$3" = "ghcr.io/allyourbasehq/allyourbase:0.0.17-beta" ]; then
      if [ "$scenario" = "docker_version_bad" ]; then
        printf '%s\n' '{"version":"0.0.16-beta"}'
        exit 0
      fi
      if [ "$scenario" = "docker_version_malformed" ]; then
        printf '%s\n' '{"version":'
        exit 0
      fi
      printf '%s\n' '{"commit":"0123456789abcdef0123456789abcdef01234567","date":"2026-07-15T08:42:41Z","version":"0.0.17-beta"}'
      exit 0
    fi
    if [ "$scenario" = "install_fail" ]; then
      exit 67
    fi
    printf '%s\n' 'clean-container installer version=0.0.17-beta'
    ;;
  *) exit 64 ;;
esac
DOCKER
  chmod +x "$FAKE_BIN/gh" "$FAKE_BIN/curl" "$FAKE_BIN/docker"
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  file_path="$1"
  needle="$2"
  grep -Fq "$needle" "$file_path" || fail "missing '$needle' in $file_path"
}

run_case() {
  scenario="$1"
  expected_status="$2"
  output="$TEST_DIR/${scenario}.out"
  : >"$LOG_FILE"
  set +e
  PATH="$FAKE_BIN:$PATH" LAUNCH_CHECK_FAKE_LOG="$LOG_FILE" LAUNCH_CHECK_SCENARIO="$scenario" \
    sh "$CHECKER" >"$output" 2>&1
  status=$?
  set -e
  if [ "$expected_status" = pass ] && [ "$status" -ne 0 ]; then
    cat "$output" >&2
    fail "$scenario should have passed"
  fi
  if [ "$expected_status" = fail ] && [ "$status" -eq 0 ]; then
    cat "$output" >&2
    fail "$scenario should have failed"
  fi
}

write_fake_commands

[ -x "$CHECKER" ] || fail 'launch-check script must remain executable'

run_case success pass
success_out="$TEST_DIR/success.out"
assert_contains "$success_out" 'release tag=v0.0.17-beta version=0.0.17-beta'
assert_contains "$success_out" 'image=ghcr.io/allyourbasehq/allyourbase:0.0.17-beta version=0.0.17-beta'
assert_contains "$success_out" 'installer version=0.0.17-beta'
assert_contains "$success_out" 'url=https://allyourbase.io status=200'
assert_contains "$success_out" 'health json={"status":"ok","database":"ok"}'
assert_contains "$success_out" 'sha=0123456789abcdef0123456789abcdef01234567'
assert_contains "$success_out" 'run id=602 status=completed conclusion=success'
assert_contains "$LOG_FILE" 'gh release view v0.0.17-beta -R AllyourbaseHQ/allyourbase --json assets'
assert_contains "$LOG_FILE" 'docker pull ghcr.io/allyourbasehq/allyourbase:0.0.17-beta'
assert_contains "$LOG_FILE" 'docker run --rm ghcr.io/allyourbasehq/allyourbase:0.0.17-beta ayb version --json'

for case_name in \
  no_app_release release_row_not_object release_trailing_row_not_object release_missing_tag release_missing_is_draft \
  missing_asset malformed_release_view docker_pull docker_version_bad \
  docker_version_malformed install_fail url_docs url_installer url_demo url_kanban \
  url_polls url_movies url_instantsearch url_health health_bad_json ci_short_sha \
  ci_zero ci_malformed ci_empty ci_duplicate ci_mismatched_head ci_in_progress ci_failure
do
  run_case "$case_name" fail
done

assert_contains "$TEST_DIR/ci_zero.out" 'arm=prod-ci'
assert_contains "$TEST_DIR/ci_zero.out" 'n=0'
assert_contains "$TEST_DIR/ci_zero.out" 'Docker'
assert_contains "$TEST_DIR/ci_short_sha.out" 'arm=prod-ci'
assert_contains "$TEST_DIR/no_app_release.out" 'arm=release'
assert_contains "$TEST_DIR/release_row_not_object.out" 'arm=release'
assert_contains "$TEST_DIR/release_trailing_row_not_object.out" 'arm=release'
assert_contains "$TEST_DIR/release_missing_tag.out" 'arm=release'
assert_contains "$TEST_DIR/release_missing_is_draft.out" 'arm=release'
assert_contains "$TEST_DIR/missing_asset.out" 'arm=release-assets'
assert_contains "$TEST_DIR/missing_asset.out" 'ayb_0.0.17-beta_windows_arm64.zip'
assert_contains "$TEST_DIR/docker_pull.out" 'arm=image'
assert_contains "$TEST_DIR/docker_version_bad.out" 'arm=image-version'
assert_contains "$TEST_DIR/install_fail.out" 'arm=installer'
assert_contains "$TEST_DIR/url_docs.out" 'arm=https'
assert_contains "$TEST_DIR/health_bad_json.out" 'arm=https-health'

printf 'PASS: launch-check deterministic contract\n'
