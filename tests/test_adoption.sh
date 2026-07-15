#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
COLLECTOR="$ROOT_DIR/scripts/adoption.sh"

source "$ROOT_DIR/tests/bash_assert_helpers.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT INT HUP TERM

FAKE_BIN="$TEST_DIR/bin"
LOG_FILE="$TEST_DIR/commands.log"
mkdir -p "$FAKE_BIN"
: >"$LOG_FILE"

write_fake_commands() {
  cat >"$FAKE_BIN/gh" <<'GH'
#!/bin/sh
set -eu
printf 'gh %s\n' "$*" >>"$ADOPTION_FAKE_LOG"
scenario="${ADOPTION_TEST_SCENARIO:-success}"
repo="${ADOPTION_REPOSITORY:-AllyourbaseHQ/allyourbase}"

case "$scenario:${2:-}" in
  traffic_token:repos/*/traffic/*) [ "${GH_TOKEN:-}" = "traffic-secret" ] || exit 81 ;;
  github_token:repos/*/traffic/*) [ "${GH_TOKEN:-}" = "github-secret" ] || exit 82 ;;
  local_gh_auth:repos/*/traffic/*) [ -z "${GH_TOKEN:-}" ] || exit 83 ;;
  traffic_403:repos/*/traffic/*) printf '%s\n' 'HTTP 403: Resource not accessible by integration' >&2; exit 1 ;;
esac

case "$1" in
  api)
    endpoint="$2"
    case "$scenario:$endpoint" in
      gh_failure:repos/*/traffic/views) exit 29 ;;
      malformed_views:repos/*/traffic/views) printf '%s\n' '{"count":' ;;
      missing_views_count:repos/*/traffic/views) printf '%s\n' '{"uniques":1,"views":[]}' ;;
      non_numeric_views_count:repos/*/traffic/views) printf '%s\n' '{"count":"one","uniques":1,"views":[]}' ;;
      missing_views_period:repos/*/traffic/views) printf '%s\n' '{"count":1,"uniques":1}' ;;
      malformed_views_timestamp:repos/*/traffic/views) printf '%s\n' '{"count":1,"uniques":1,"views":[{"timestamp":"yesterday","count":1,"uniques":1}]}' ;;
      *:repos/*/traffic/views) printf '%s\n' '{"count":1,"uniques":1,"views":[{"timestamp":"2026-07-14T00:00:00Z","count":1,"uniques":1},{"timestamp":"2026-07-01T00:00:00Z","count":0,"uniques":0}]}' ;;
      malformed_clones:repos/*/traffic/clones) printf '%s\n' '{"count":' ;;
      missing_clones_period:repos/*/traffic/clones) printf '%s\n' '{"count":428,"uniques":76,"clones":[]}' ;;
      *:repos/*/traffic/clones) printf '%s\n' '{"count":428,"uniques":76,"clones":[{"timestamp":"2026-07-01T00:00:00Z","count":100,"uniques":20},{"timestamp":"2026-07-14T00:00:00Z","count":328,"uniques":56}]}' ;;
      referrers_nonempty:repos/*/traffic/popular/referrers) printf '%s\n' '[{"referrer":"github.com","count":7,"uniques":4},{"referrer":"news.example","count":2,"uniques":1}]' ;;
      malformed_referrers:repos/*/traffic/popular/referrers) printf '%s\n' '[{"referrer":' ;;
      *:repos/*/traffic/popular/referrers) printf '%s\n' '[]' ;;
      malformed_repo:repos/*) printf '%s\n' '{"stargazers_count":' ;;
      *:repos/*) printf '%s\n' '{"stargazers_count":0,"forks_count":0,"open_issues_count":0}' ;;
      *) exit 64 ;;
    esac
    ;;
  release)
    case "$2" in
      list)
        case "$scenario" in
          no_app_release) printf '%s\n' '[{"tagName":"v0.0.19-beta","isDraft":true},{"tagName":"pg-16","isDraft":false}]' ;;
          malformed_releases) printf '%s\n' '[{"tagName":' ;;
          release_row_not_object) printf '%s\n' '["not-a-release",{"tagName":"v0.0.17-beta","isDraft":false,"publishedAt":"2026-07-15T02:19:43Z"}]' ;;
          release_trailing_row_not_object) printf '%s\n' '[{"tagName":"v0.0.17-beta","isDraft":false,"publishedAt":"2026-07-15T02:19:43Z"},"not-a-release"]' ;;
          release_missing_tag) printf '%s\n' '[{"isDraft":false,"publishedAt":"2026-07-15T02:19:43Z"},{"tagName":"v0.0.17-beta","isDraft":false,"publishedAt":"2026-07-15T02:19:43Z"}]' ;;
          release_missing_is_draft) printf '%s\n' '[{"tagName":"v0.0.18-beta","publishedAt":"2026-07-16T02:19:43Z"}]' ;;
          *) printf '%s\n' '[{"tagName":"v0.0.19-beta","isDraft":true},{"tagName":"pg-16","isDraft":false},{"tagName":"v0.0.17-beta","isDraft":false,"publishedAt":"2026-07-15T02:19:43Z"}]' ;;
        esac
        ;;
      view)
        [ "$3" = "v0.0.17-beta" ] || exit 65
        case "$scenario" in
          release_view_failure) exit 30 ;;
          malformed_release_view) printf '%s\n' '{"tagName":' ;;
          bad_release_tag) printf '%s\n' '{"tagName":"v0.0.16-beta","publishedAt":"2026-07-15T02:19:43Z","assets":[]}' ;;
          malformed_release_published_at) printf '%s\n' '{"tagName":"v0.0.17-beta","publishedAt":"yesterday","assets":[]}' ;;
          noncanonical_release_published_at) printf '%s\n' '{"tagName":"v0.0.17-beta","publishedAt":"2026-7-15T2:19:43Z","assets":[]}' ;;
          missing_asset_download) printf '%s\n' '{"tagName":"v0.0.17-beta","publishedAt":"2026-07-15T02:19:43Z","assets":[{"name":"ayb.tar.gz"}]}' ;;
          *)
            printf '%s\n' '{"tagName":"v0.0.17-beta","publishedAt":"2026-07-15T02:19:43Z","assets":[{"name":"ayb_0.0.17-beta_darwin_amd64.tar.gz","downloadCount":2},{"name":"ayb_0.0.17-beta_darwin_arm64.tar.gz","downloadCount":5},{"name":"ayb_0.0.17-beta_linux_amd64.tar.gz","downloadCount":11},{"name":"checksums.txt","downloadCount":3}]}'
            ;;
        esac
        ;;
      *) exit 64 ;;
    esac
    ;;
  *) exit 64 ;;
esac
GH

  cat >"$FAKE_BIN/curl" <<'CURL'
#!/bin/sh
set -eu
printf 'curl %s\n' "$*" >>"$ADOPTION_FAKE_LOG"
scenario="${ADOPTION_TEST_SCENARIO:-success}"
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
[ "$url" = "https://api.npmjs.org/downloads/point/last-month/@allyourbase/js" ] || exit 64
case "$scenario" in
  npm_failure) exit 31 ;;
  malformed_npm) printf '%s\n' '{"downloads":' ;;
  missing_npm_downloads) printf '%s\n' '{"start":"2026-06-15","end":"2026-07-14","package":"@allyourbase/js"}' ;;
  bad_npm_package) printf '%s\n' '{"downloads":402,"start":"2026-06-15","end":"2026-07-14","package":"@wrong/pkg"}' ;;
  malformed_npm_start) printf '%s\n' '{"downloads":402,"start":"yesterday","end":"2026-07-14","package":"@allyourbase/js"}' ;;
  malformed_npm_end) printf '%s\n' '{"downloads":402,"start":"2026-06-15","end":"2026-02-30","package":"@allyourbase/js"}' ;;
  reversed_npm_period) printf '%s\n' '{"downloads":402,"start":"2026-07-15","end":"2026-07-14","package":"@allyourbase/js"}' ;;
  *) printf '%s\n' '{"downloads":402,"start":"2026-06-15","end":"2026-07-14","package":"@allyourbase/js"}' ;;
esac
CURL

  chmod +x "$FAKE_BIN/gh" "$FAKE_BIN/curl"
}

run_case() {
  local scenario="$1"
  local expected_status="$2"
  local output="$TEST_DIR/${scenario}.out"
  local report="$TEST_DIR/${scenario}.md"
  local command_path="$FAKE_BIN:$PATH"
  local cloudflare_token=
  local cloudflare_account=
  case "$scenario" in
    missing_gh) command_path="$TEST_DIR/missing-gh-bin:/usr/bin:/bin" ;;
    missing_curl) command_path="$TEST_DIR/missing-curl-bin" ;;
    cloudflare_configured)
      cloudflare_token=fixture-token-should-not-render
      cloudflare_account=fixture-account-should-not-render
      ;;
  esac
  : >"$LOG_FILE"
  rm -f "$report"
  set +e
  if [[ "$scenario" == cloudflare_configured || "$scenario" == cloudflare_empty ]]; then
    PATH="$command_path" \
      ADOPTION_FAKE_LOG="$LOG_FILE" \
      ADOPTION_TEST_SCENARIO="$scenario" \
      ADOPTION_REPOSITORY="AllyourbaseHQ/allyourbase" \
      CLOUDFLARE_API_TOKEN="$cloudflare_token" \
      CLOUDFLARE_ACCOUNT_ID="$cloudflare_account" \
      /bin/sh "$COLLECTOR" "$report" >"$output" 2>&1
  else
    PATH="$command_path" \
      ADOPTION_FAKE_LOG="$LOG_FILE" \
      ADOPTION_TEST_SCENARIO="$scenario" \
      ADOPTION_REPOSITORY="AllyourbaseHQ/allyourbase" \
      /usr/bin/env -u CLOUDFLARE_API_TOKEN -u CLOUDFLARE_ACCOUNT_ID \
      /bin/sh "$COLLECTOR" "$report" >"$output" 2>&1
  fi
  local status=$?
  set -e

  if [ "$expected_status" = pass ] && [ "$status" -ne 0 ]; then
    cat "$output" >&2
    fail "$scenario should have passed"
  fi
  if [ "$expected_status" = fail ] && [ "$status" -eq 0 ]; then
    cat "$output" >&2
    fail "$scenario should have failed"
  fi
  if [ "$expected_status" = fail ] && [ -e "$report" ]; then
    cat "$report" >&2
    fail "$scenario should not leave a completed output file"
  fi
}

assert_report_success() {
  local report="$TEST_DIR/success.md"
  assert_contains "$report" 'Summary: 1 human view, 0 stars, 0 referrers; 428 clones are our own CI.' 'honest summary missing'
  assert_contains "$report" '## Human signals' 'human section missing'
  assert_contains "$report" 'GitHub views: total=1 unique=1' 'view totals missing'
  assert_contains "$report" 'GitHub views period: 2026-07-01T00:00:00Z through 2026-07-14T00:00:00Z' 'view source period missing'
  assert_contains "$report" 'Stars: 0' 'stars missing'
  assert_contains "$report" 'Forks: 0' 'forks missing'
  assert_contains "$report" 'Open issues: 0' 'open issues missing'
  assert_contains "$report" 'Referrers: none' 'empty referrers missing'
  assert_contains "$report" '## CI-dominated / not adoption' 'ci classification section missing'
  assert_contains "$report" 'GitHub clones: total=428 unique=76 (project CI; not human adoption)' 'clone classification missing'
  assert_contains "$report" 'GitHub clones period: 2026-07-01T00:00:00Z through 2026-07-14T00:00:00Z' 'clone source period missing'
  assert_contains "$report" 'Selected app release: v0.0.17-beta published=2026-07-15T02:19:43Z' 'release tag missing'
  assert_contains "$report" 'Release asset downloads total: 21' 'release total missing'
  assert_contains "$report" 'ayb_0.0.17-beta_darwin_amd64.tar.gz | 2' 'asset detail missing'
  assert_contains "$report" 'ayb_0.0.17-beta_darwin_arm64.tar.gz | 5' 'asset detail missing'
  assert_contains "$report" 'ayb_0.0.17-beta_linux_amd64.tar.gz | 11' 'asset detail missing'
  assert_contains "$report" 'checksums.txt | 3' 'asset detail missing'
  assert_contains "$report" '## Ambiguous npm activity' 'npm section missing'
  assert_contains "$report" 'npm @allyourbase/js downloads: 402 from 2026-06-15 through 2026-07-14' 'npm values missing'
  assert_contains "$report" 'Project CI installs the SDK, so these downloads are ambiguous and not counted as human adoption.' 'npm ambiguity missing'
  assert_contains "$report" '## Optional Cloudflare analytics' 'cloudflare section missing'
  assert_contains "$report" 'POST https://api.cloudflare.com/client/v4/graphql' 'cloudflare endpoint gap missing'
  assert_contains "$report" 'CLOUDFLARE_API_TOKEN with Account > Account Analytics > Read on the relevant account' 'cloudflare token scope gap missing'
  assert_contains "$report" 'CLOUDFLARE_ACCOUNT_ID' 'cloudflare account gap missing'
  assert_contains "$report" 'Implementation owner: scripts/adoption.sh' 'cloudflare owner gap missing'
  assert_not_contains "$report" 'total adoption' 'combined adoption total must not be rendered'
  assert_not_contains "$report" 'adoption score' 'adoption score must not be rendered'
  assert_not_contains "$report" 'clones are human adoption' 'clones must not be labeled human adoption'
  assert_not_contains "$report" 'release downloads are human adoption' 'release downloads must not be labeled human adoption'
  assert_not_contains "$report" 'npm downloads are human adoption' 'npm downloads must not be labeled human adoption'
}

assert_referrers_preserved() {
  local report="$TEST_DIR/referrers_nonempty.md"
  assert_contains "$report" 'github.com | 7 | 4' 'referrer source/count/unique fields missing'
  assert_contains "$report" 'news.example | 2 | 1' 'second referrer source/count/unique fields missing'
}

assert_cloudflare_configured_gap() {
  local report="$TEST_DIR/cloudflare_configured.md"
  assert_contains "$report" 'Cloudflare credentials are configured, but Cloudflare analytics are not collected by this stage.' 'configured cloudflare gap summary missing'
  assert_contains "$report" 'POST https://api.cloudflare.com/client/v4/graphql' 'configured cloudflare endpoint gap missing'
  assert_contains "$report" 'CLOUDFLARE_API_TOKEN with Account > Account Analytics > Read on the relevant account' 'configured cloudflare token scope gap missing'
  assert_contains "$report" 'CLOUDFLARE_ACCOUNT_ID' 'configured cloudflare account gap missing'
  assert_contains "$report" 'Implementation owner: scripts/adoption.sh' 'configured cloudflare owner gap missing'
  assert_not_contains "$report" 'fixture-token-should-not-render' 'cloudflare token value must not render'
  assert_not_contains "$report" 'fixture-account-should-not-render' 'cloudflare account value must not render'
}

assert_cloudflare_empty_is_unavailable() {
  local report="$TEST_DIR/cloudflare_empty.md"
  assert_contains "$report" 'Cloudflare analytics were not collected because optional credentials are unavailable.' 'empty cloudflare credentials must be unavailable'
  assert_not_contains "$report" 'Cloudflare credentials are configured' 'empty cloudflare credentials must not be configured'
}

assert_failure_names_source() {
  local scenario="$1"
  local source_name="$2"
  assert_contains "$TEST_DIR/${scenario}.out" "$source_name" "$scenario should name $source_name"
}

run_output_path_case() {
  local environment_report="$TEST_DIR/environment-output.md"
  local positional_report="$TEST_DIR/positional-output.md"
  PATH="$FAKE_BIN:$PATH" ADOPTION_FAKE_LOG="$LOG_FILE" ADOPTION_OUTPUT_PATH="$environment_report" \
    /bin/sh "$COLLECTOR" >"$TEST_DIR/environment-output.out" 2>&1
  [ -f "$environment_report" ] || fail 'ADOPTION_OUTPUT_PATH should select the report destination'
  PATH="$FAKE_BIN:$PATH" ADOPTION_FAKE_LOG="$LOG_FILE" ADOPTION_OUTPUT_PATH="$environment_report" \
    /bin/sh "$COLLECTOR" "$positional_report" >"$TEST_DIR/positional-output.out" 2>&1
  [ -f "$positional_report" ] || fail 'positional output should remain compatible and take precedence'
}

run_token_case() {
  local scenario="$1"
  local traffic_token="$2"
  local github_token="$3"
  local report="$TEST_DIR/${scenario}.md"
  PATH="$FAKE_BIN:$PATH" ADOPTION_FAKE_LOG="$LOG_FILE" ADOPTION_TEST_SCENARIO="$scenario" \
    ADOPTION_TRAFFIC_TOKEN="$traffic_token" GITHUB_TOKEN="$github_token" \
    /bin/sh "$COLLECTOR" "$report" >"$TEST_DIR/${scenario}.out" 2>&1
  if [ -n "$traffic_token" ]; then
    assert_not_contains "$TEST_DIR/${scenario}.out" "$traffic_token" 'traffic token must not be printed'
    assert_not_contains "$LOG_FILE" "$traffic_token" 'traffic token must not appear in command logs'
  fi
  if [ -n "$github_token" ]; then
    assert_not_contains "$TEST_DIR/${scenario}.out" "$github_token" 'GitHub token must not be printed'
    assert_not_contains "$LOG_FILE" "$github_token" 'GitHub token must not appear in command logs'
  fi
}

write_fake_commands
mkdir -p "$TEST_DIR/missing-gh-bin" "$TEST_DIR/missing-curl-bin"
cp "$FAKE_BIN/curl" "$TEST_DIR/missing-gh-bin/curl"
cp "$FAKE_BIN/gh" "$TEST_DIR/missing-curl-bin/gh"
ln -s "$(command -v python3)" "$TEST_DIR/missing-gh-bin/python3"
ln -s "$(command -v python3)" "$TEST_DIR/missing-curl-bin/python3"
ln -s /usr/bin/dirname "$TEST_DIR/missing-curl-bin/dirname"

run_case success pass
assert_report_success
assert_contains "$TEST_DIR/success.out" "$TEST_DIR/success.md" 'collector should print final report path'
assert_contains "$LOG_FILE" 'gh api repos/AllyourbaseHQ/allyourbase/traffic/views' 'views endpoint not called'
assert_contains "$LOG_FILE" 'gh api repos/AllyourbaseHQ/allyourbase/traffic/clones' 'clones endpoint not called'
assert_contains "$LOG_FILE" 'gh api repos/AllyourbaseHQ/allyourbase/traffic/popular/referrers' 'referrers endpoint not called'
assert_contains "$LOG_FILE" 'gh api repos/AllyourbaseHQ/allyourbase' 'repository endpoint not called'
if grep -Fxq 'gh api repos/AllyourbaseHQ/allyourbase/' "$LOG_FILE"; then
  fail 'repository endpoint must not include a trailing slash'
fi
assert_contains "$LOG_FILE" 'curl' 'npm endpoint not called'

run_case referrers_nonempty pass
assert_referrers_preserved

run_case cloudflare_configured pass
assert_cloudflare_configured_gap

run_case cloudflare_empty pass
assert_cloudflare_empty_is_unavailable

for case_name in \
  gh_failure malformed_views missing_views_count non_numeric_views_count missing_views_period \
  malformed_views_timestamp malformed_clones missing_clones_period \
  malformed_referrers malformed_repo malformed_releases release_row_not_object \
  release_trailing_row_not_object release_missing_tag release_missing_is_draft no_app_release release_view_failure \
  malformed_release_view bad_release_tag malformed_release_published_at noncanonical_release_published_at \
  missing_asset_download npm_failure malformed_npm missing_npm_downloads bad_npm_package malformed_npm_start \
  malformed_npm_end reversed_npm_period
do
  run_case "$case_name" fail
done

assert_failure_names_source gh_failure 'github views'
assert_contains "$TEST_DIR/gh_failure.out" 'request failed' 'generic traffic failure should report request failure'
assert_not_contains "$TEST_DIR/gh_failure.out" 'ADOPTION_TRAFFIC_TOKEN' 'generic traffic failure must not report traffic token gap'
assert_not_contains "$TEST_DIR/gh_failure.out" 'push access' 'generic traffic failure must not report push-access gap'
assert_failure_names_source malformed_views 'github views'
assert_failure_names_source missing_views_count 'github views'
assert_failure_names_source missing_views_period 'github views'
assert_failure_names_source malformed_views_timestamp 'github views'
assert_failure_names_source malformed_clones 'github clones'
assert_failure_names_source missing_clones_period 'github clones'
assert_failure_names_source malformed_referrers 'github referrers'
assert_failure_names_source malformed_repo 'github repository'
assert_failure_names_source malformed_releases 'github releases'
assert_failure_names_source release_row_not_object 'github releases'
assert_failure_names_source release_trailing_row_not_object 'github releases'
assert_failure_names_source release_missing_tag 'github releases'
assert_failure_names_source release_missing_is_draft 'github releases'
assert_failure_names_source no_app_release 'github releases'
assert_failure_names_source release_view_failure 'github release view'
assert_failure_names_source malformed_release_view 'github release view'
assert_failure_names_source bad_release_tag 'github release view'
assert_failure_names_source malformed_release_published_at 'github release view: publishedAt must be a UTC timestamp'
assert_failure_names_source noncanonical_release_published_at 'github release view: publishedAt must be a UTC timestamp'
assert_failure_names_source missing_asset_download 'github release view'
assert_failure_names_source npm_failure 'npm downloads'
assert_failure_names_source malformed_npm 'npm downloads'
assert_failure_names_source missing_npm_downloads 'npm downloads'
assert_failure_names_source bad_npm_package 'npm downloads'
assert_failure_names_source malformed_npm_start 'npm downloads: start must be a valid YYYY-MM-DD date'
assert_failure_names_source malformed_npm_end 'npm downloads: end must be a valid YYYY-MM-DD date'
assert_failure_names_source reversed_npm_period 'npm downloads: start must not be after end'

run_case missing_gh fail
assert_failure_names_source missing_gh 'missing command gh'

run_case missing_curl fail
assert_failure_names_source missing_curl 'missing command curl'

run_output_path_case
run_token_case traffic_token traffic-secret github-secret
run_token_case github_token '' github-secret
run_token_case local_gh_auth '' ''

run_case traffic_403 fail
assert_contains "$TEST_DIR/traffic_403.out" 'ADOPTION_TRAFFIC_TOKEN' 'traffic 403 gap should name token input'
assert_contains "$TEST_DIR/traffic_403.out" '/traffic/*' 'traffic 403 gap should name endpoint family'
assert_contains "$TEST_DIR/traffic_403.out" 'push access' 'traffic 403 gap should name required token scope'
assert_contains "$TEST_DIR/traffic_403.out" 'scripts/adoption.sh' 'traffic 403 gap should name implementation owner'

printf 'PASS: adoption deterministic contract\n'
