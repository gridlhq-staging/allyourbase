#!/bin/sh
set -eu

REPO="AllyourbaseHQ/allyourbase"
WORKFLOW_NAME="CI"
IMAGE_REPO="ghcr.io/allyourbasehq/allyourbase"
INSTALLER_URL="https://install.allyourbase.io/install.sh"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
URLS="
https://allyourbase.io
https://install.allyourbase.io/install.sh
https://demo.allyourbase.io
https://kanban.demo.allyourbase.io
https://polls.demo.allyourbase.io
https://movies.demo.allyourbase.io
https://instantsearch.demo.allyourbase.io
https://api.allyourbase.io/health
"

TEMP_DIR=""

. "$SCRIPT_DIR/release_helpers.sh"

cleanup() {
  if [ -n "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
  fi
}
trap cleanup EXIT INT HUP TERM

info() {
  printf 'ok arm=%s %s\n' "$1" "$2"
}

fail_arm() {
  arm="$1"
  shift
  printf 'FAIL arm=%s %s\n' "$arm" "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail_arm prerequisites "missing command=$1"
}

json_query() {
  python3 -c "$1"
}

resolve_release() {
  releases=$(gh release list -R "$REPO" --json tagName,isDraft,isPrerelease,publishedAt --limit 20) \
    || fail_arm release "release list command failed"
  tag=$(printf '%s' "$releases" | select_app_release_tag 2>&1) || fail_arm release "$tag"
  [ -n "$tag" ] || fail_arm release "empty app release tag"
  version=${tag#v}
  info release "tag=$tag version=$version"
}

require_release_assets() {
  assets_json=$(gh release view "$tag" -R "$REPO" --json assets) \
    || fail_arm release-assets "release view failed tag=$tag"
  missing=$(printf '%s' "$assets_json" | EXPECTED_VERSION="$version" json_query '
import json, os, sys
version = os.environ["EXPECTED_VERSION"]
required = {
    f"ayb_{version}_darwin_amd64.tar.gz",
    f"ayb_{version}_darwin_arm64.tar.gz",
    f"ayb_{version}_linux_amd64.tar.gz",
    f"ayb_{version}_linux_arm64.tar.gz",
    f"ayb_{version}_windows_amd64.zip",
    f"ayb_{version}_windows_arm64.zip",
    "checksums.txt",
}
try:
    payload = json.load(sys.stdin)
    names = {asset.get("name", "") for asset in payload.get("assets", [])}
except Exception as exc:
    raise SystemExit(f"malformed release assets: {exc}")
missing = sorted(required - names)
if missing:
    print(",".join(missing))
') || fail_arm release-assets "$missing"
  [ -z "$missing" ] || fail_arm release-assets "missing=$missing tag=$tag"
  info release-assets "count=7"
}

check_image() {
  TEMP_DIR=$(mktemp -d)
  mkdir -p "$TEMP_DIR/docker-config"
  image_ref="$IMAGE_REPO:$version"
  DOCKER_CONFIG="$TEMP_DIR/docker-config" docker pull "$image_ref" >/dev/null \
    || fail_arm image "pull failed image=$image_ref"
  version_json=$(DOCKER_CONFIG="$TEMP_DIR/docker-config" docker run --rm "$image_ref" ayb version --json) \
    || fail_arm image-version "version command failed image=$image_ref"
  image_version=$(printf '%s' "$version_json" | json_query '
import json, sys
try:
    print(json.load(sys.stdin).get("version", ""))
except Exception as exc:
    raise SystemExit(f"malformed image version json: {exc}")
') || fail_arm image-version "$image_version"
  [ "$image_version" = "$version" ] || fail_arm image-version "expected=$version actual=$image_version"
  info image "image=$image_ref version=$image_version"
}

check_installer() {
  set +e
  installer_output=$(sh tests/test_install.sh --clean-container "$INSTALLER_URL" "$version" 2>&1)
  installer_status=$?
  set -e
  if [ "$installer_status" -ne 0 ]; then
    installer_detail=$(printf '%s' "$installer_output" | tr '\n' ' ' | sed 's/[[:space:]][[:space:]]*/ /g; s/^ //; s/ $//')
    fail_arm installer "clean-container failed detail=${installer_detail:-missing}"
  fi
  installer_version=$(printf '%s\n' "$installer_output" | sed -n 's/.*installer version=\([^[:space:]]*\).*/\1/p' | tail -1)
  [ "$installer_version" = "$version" ] || fail_arm installer "expected=$version actual=${installer_version:-missing}"
  info installer "version=$installer_version"
}

check_url() {
  url="$1"
  body_file=$(mktemp "${TMPDIR:-/tmp}/ayb-launch-body.XXXXXX")
  http_code=$(curl -fsS -L --max-time 25 -o "$body_file" -w "%{http_code} %{url_effective}" "$url") \
    || fail_arm https "curl failed url=$url"
  status=${http_code%% *}
  effective=${http_code#* }
  [ "$status" = "200" ] || fail_arm https "url=$url status=$status effective=$effective"
  info https "url=$url status=$status effective=$effective"
  if [ "$url" = "https://api.allyourbase.io/health" ]; then
    body=$(cat "$body_file")
    [ "$body" = '{"status":"ok","database":"ok"}' ] || fail_arm https-health "body=$body"
    info https-health "json=$body"
  fi
  rm -f "$body_file"
}

check_urls() {
  for url in $URLS; do
    check_url "$url"
  done
}

check_prod_ci() {
  prod_sha=$(gh api "repos/$REPO/commits/main" --jq .sha) \
    || fail_arm prod-ci "main sha query failed"
  printf '%s' "$prod_sha" | grep -Eq '^[0-9a-fA-F]{40}$' \
    || fail_arm prod-ci "invalid sha=$prod_sha"
  runs=$(gh run list -R "$REPO" --commit "$prod_sha" --json databaseId,workflowName,status,conclusion,headSha,createdAt,updatedAt) \
    || fail_arm prod-ci "run list failed sha=$prod_sha"
  ci_summary=$(printf '%s' "$runs" | PROD_SHA="$prod_sha" WORKFLOW_NAME="$WORKFLOW_NAME" json_query '
import json, os, sys
prod_sha = os.environ["PROD_SHA"]
workflow = os.environ["WORKFLOW_NAME"]
try:
    runs = json.load(sys.stdin)
except Exception as exc:
    raise SystemExit(f"malformed runs: {exc}")
ci = [run for run in runs if run.get("workflowName") == workflow]
if len(ci) != 1:
    print(f"n={len(ci)} unfiltered={json.dumps(runs, sort_keys=True)}")
    raise SystemExit(2)
run = ci[0]
if run.get("headSha") != prod_sha:
    print(f"n=1 run={json.dumps(run, sort_keys=True)} unfiltered={json.dumps(runs, sort_keys=True)}")
    raise SystemExit(3)
if run.get("status") != "completed" or run.get("conclusion") != "success":
    print(f"n=1 run={json.dumps(run, sort_keys=True)} unfiltered={json.dumps(runs, sort_keys=True)}")
    raise SystemExit(4)
print("n=1 run id={} status={} conclusion={}".format(run.get("databaseId"), run.get("status"), run.get("conclusion")))
') || fail_arm prod-ci "$ci_summary"
  info prod-ci "sha=$prod_sha $ci_summary"
}

require_command gh
require_command curl
require_command docker
require_command python3

resolve_release
require_release_assets
check_image
check_installer
check_urls
check_prod_ci
