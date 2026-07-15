#!/bin/sh
set -eu

REPO="${ADOPTION_REPOSITORY:-AllyourbaseHQ/allyourbase}"
NPM_PACKAGE="@allyourbase/js"
NPM_DOWNLOADS_URL="https://api.npmjs.org/downloads/point/last-month/@allyourbase/js"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
OUTPUT_PATH="${1:-${ADOPTION_OUTPUT_PATH:-$ROOT_DIR/docs/live-state/$(date -u +%Y%m%dT%H%M%SZ)_adoption.md}}"

TEMP_DIR=""
OUTPUT_TMP=""

. "$SCRIPT_DIR/release_helpers.sh"

cleanup() {
  if [ -n "$OUTPUT_TMP" ]; then
    rm -f "$OUTPUT_TMP"
  fi
  if [ -n "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
  fi
}
trap cleanup EXIT INT HUP TERM

fail_source() {
  source_name="$1"
  shift
  printf 'FAIL source=%s %s\n' "$source_name" "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail_source prerequisites "missing command $1"
}

write_required_source() {
  source_name="$1"
  output_file="$2"
  shift 2
  "$@" >"$output_file" || fail_source "$source_name" "request failed"
}

collect_github_api() {
  source_name="$1"
  endpoint="$2"
  output_file="$3"
  if [ -n "$endpoint" ]; then
    api_path="repos/$REPO/$endpoint"
  else
    api_path="repos/$REPO"
  fi
  case "$endpoint" in
    traffic/*)
      traffic_token="${ADOPTION_TRAFFIC_TOKEN:-${GITHUB_TOKEN:-}}"
      traffic_error_file="$TEMP_DIR/traffic_error.txt"
      if [ -n "$traffic_token" ]; then
        GH_TOKEN="$traffic_token" gh api "$api_path" >"$output_file" 2>"$traffic_error_file" || fail_traffic_request "$source_name" "$traffic_error_file"
      else
        gh api "$api_path" >"$output_file" 2>"$traffic_error_file" || fail_traffic_request "$source_name" "$traffic_error_file"
      fi
      ;;
    *) write_required_source "$source_name" "$output_file" gh api "$api_path" ;;
  esac
}

fail_traffic_request() {
  source_name="$1"
  error_file="$2"
  if grep -Fq 'HTTP 403' "$error_file"; then
    fail_source "$source_name" "request failed; /traffic/* requires ADOPTION_TRAFFIC_TOKEN with push access to the repository; implementation owner: scripts/adoption.sh"
  fi
  fail_source "$source_name" "request failed"
}

collect_release_list() {
  output_file="$1"
  write_required_source "github releases" "$output_file" \
    gh release list -R "$REPO" --json tagName,isDraft,isPrerelease,publishedAt --limit 20
}

select_release_tag() {
  release_file="$1"
  tag=$(select_app_release_tag <"$release_file" 2>&1) || fail_source "github releases" "$tag"
  [ -n "$tag" ] || fail_source "github releases" "empty app release tag"
  printf '%s\n' "$tag"
}

collect_release_view() {
  tag="$1"
  output_file="$2"
  write_required_source "github release view" "$output_file" \
    gh release view "$tag" -R "$REPO" --json tagName,publishedAt,assets
}

collect_npm_downloads() {
  output_file="$1"
  write_required_source "npm downloads" "$output_file" curl -fsS -L "$NPM_DOWNLOADS_URL"
}

render_report() {
  selected_tag="$1"
  output_tmp="$2"
  validation_output=$(ADOPTION_REPOSITORY="$REPO" \
    ADOPTION_NPM_PACKAGE="$NPM_PACKAGE" \
    SELECTED_RELEASE_TAG="$selected_tag" \
    VIEWS_JSON="$TEMP_DIR/views.json" \
    CLONES_JSON="$TEMP_DIR/clones.json" \
    REFERRERS_JSON="$TEMP_DIR/referrers.json" \
    REPOSITORY_JSON="$TEMP_DIR/repository.json" \
    RELEASE_VIEW_JSON="$TEMP_DIR/release_view.json" \
    NPM_JSON="$TEMP_DIR/npm.json" \
    OUTPUT_TMP="$output_tmp" \
    CLOUDFLARE_TOKEN_PRESENT="${CLOUDFLARE_API_TOKEN:-}" \
    CLOUDFLARE_ACCOUNT_PRESENT="${CLOUDFLARE_ACCOUNT_ID:-}" \
    python3 "$SCRIPT_DIR/adoption_report.py" 2>&1) || fail_source validation "$validation_output"
}

main() {
  require_command gh
  require_command curl
  require_command python3

  TEMP_DIR=$(mktemp -d)
  mkdir -p "$(dirname "$OUTPUT_PATH")"
  OUTPUT_TMP=$(mktemp "${OUTPUT_PATH}.tmp.XXXXXX")

  collect_github_api "github views" "traffic/views" "$TEMP_DIR/views.json"
  collect_github_api "github clones" "traffic/clones" "$TEMP_DIR/clones.json"
  collect_github_api "github referrers" "traffic/popular/referrers" "$TEMP_DIR/referrers.json"
  collect_github_api "github repository" "" "$TEMP_DIR/repository.json"
  collect_release_list "$TEMP_DIR/releases.json"
  selected_tag=$(select_release_tag "$TEMP_DIR/releases.json")
  collect_release_view "$selected_tag" "$TEMP_DIR/release_view.json"
  collect_npm_downloads "$TEMP_DIR/npm.json"
  render_report "$selected_tag" "$OUTPUT_TMP"
  mv "$OUTPUT_TMP" "$OUTPUT_PATH"
  OUTPUT_TMP=""
  printf '%s\n' "$OUTPUT_PATH"
}

main "$@"
