#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $1"
  exit 1
}

assert_contains() {
  local file_path="$1"
  local needle="$2"
  local message="$3"
  grep -Fq -- "$needle" "$file_path" || fail "$message"
}

toml_value() {
  local file_path="$1"
  local section_name="$2"
  local key_name="$3"

  awk -v section_name="$section_name" -v key_name="$key_name" '
    $0 == "[" section_name "]" { in_section = 1; next }
    /^\[/ { in_section = 0 }
    in_section == 1 && $0 ~ "^[[:space:]]*" key_name "[[:space:]]*=" {
      value = $0
      sub(/^[^=]*=[[:space:]]*"/, "", value)
      sub(/"[[:space:]]*$/, "", value)
      print value
      exit
    }
  ' "$file_path"
}

SCRIPT_PATH="_dev/doc_honesty/ghcr_claims_probe.sh"
[[ -f "$SCRIPT_PATH" ]] || fail "missing ${SCRIPT_PATH}"

repo_prod_ghcr="$(toml_value ".debbie.toml" "identity.prod" "ghcr")"
[[ "$repo_prod_ghcr" == "ghcr.io/griddlehq/allyourbase" ]] || fail "repo .debbie.toml identity.prod.ghcr drifted to ${repo_prod_ghcr}"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

fixture_root="${TMP_DIR}/repo"
mkdir -p "${fixture_root}/.github/workflows" "${fixture_root}/roadmap" "${fixture_root}/_dev" "${fixture_root}/mirrors/dev/.github/workflows" "${fixture_root}/mirrors/staging/.github/workflows" "${fixture_root}/mirrors/prod/.github/workflows"

cat > "${fixture_root}/.debbie.toml" <<'TOML'
[repos.dev]
path = "mirrors/dev"
[repos.staging]
path = "mirrors/staging"
[repos.prod]
path = "mirrors/prod"

[identity.dev]
ghcr = "ghcr.io/gridlhq/allyourbase"
[identity.staging]
ghcr = "ghcr.io/gridlhq-staging/allyourbase"
[identity.prod]
ghcr = "ghcr.io/griddlehq/allyourbase"
TOML

cat > "${fixture_root}/.github/workflows/docker.yml" <<'YML'
- uses: docker/metadata-action@v6
  with:
    images: ghcr.io/gridlhq/allyourbase
YML

cat > "${fixture_root}/mirrors/dev/.github/workflows/docker.yml" <<'YML'
- uses: docker/metadata-action@v6
  with:
    images: ghcr.io/gridlhq/allyourbase
YML
cat > "${fixture_root}/mirrors/staging/.github/workflows/docker.yml" <<'YML'
- uses: docker/metadata-action@v6
  with:
    images: ghcr.io/gridlhq-staging/allyourbase
YML
cat > "${fixture_root}/mirrors/prod/.github/workflows/docker.yml" <<'YML'
- uses: docker/metadata-action@v6
  with:
    images: ghcr.io/griddlehq/allyourbase
YML

cat > "${fixture_root}/PRIORITIES.md" <<'EOF_DOC'
GHCR publication is stable at ghcr.io/griddlehq/allyourbase.
EOF_DOC
cat > "${fixture_root}/roadmap/implemented.md" <<'EOF_DOC'
Resolved push to ghcr.io/griddlehq/allyourbase and dev-cce462e pull verification.
EOF_DOC
cat > "${fixture_root}/_dev/RELEASE_SECRETS_AUDIT.md" <<'EOF_DOC'
Manual pull ghcr.io/griddlehq/allyourbase:manual-link-20260328 and dev-cce462e.
EOF_DOC
cat > "${fixture_root}/_dev/OWNER_ACTIONS.md" <<'EOF_DOC'
GHCR scope approvals completed.
EOF_DOC
cat > "${fixture_root}/RELEASE_NOTES_v0.0.7-beta.md" <<'EOF_DOC'
Docker references should use ghcr.io/griddlehq/allyourbase.
EOF_DOC

stub_dir="${TMP_DIR}/stubs"
mkdir -p "$stub_dir"

cat > "${stub_dir}/docker" <<'EOF_DOCKER'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "pull" ]]; then
  exit 0
fi
image_ref="${2:-}"
if [[ "$image_ref" == *":dev-cce462e" ]]; then
  exit 0
fi
if [[ "$image_ref" == *":v0.0.7-beta" ]]; then
  echo "manifest unknown" >&2
  exit 1
fi
exit 0
EOF_DOCKER
chmod +x "${stub_dir}/docker"

cat > "${stub_dir}/gh" <<'EOF_GH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then
  echo "Logged in to github.com"
  exit 0
fi
if [[ "${1:-}" != "api" ]]; then
  echo "unsupported gh invocation" >&2
  exit 2
fi
endpoint="${2:-}"
case "$endpoint" in
  */packages/container/allyourbase)
    echo '{"name":"allyourbase"}'
    ;;
  */packages/container/allyourbase/versions?per_page=5)
    echo '[{"id":1}]'
    ;;
  */releases/tags/v0.0.7-beta)
    echo '{"tag_name":"v0.0.7-beta"}'
    ;;
  */actions/workflows/docker.yml/runs?per_page=3)
    echo '{"workflow_runs":[]}'
    ;;
  *)
    echo "unsupported endpoint: ${endpoint}" >&2
    exit 3
    ;;
esac
EOF_GH
chmod +x "${stub_dir}/gh"

output_ok="${TMP_DIR}/probe_ok.txt"
if ! PATH="${stub_dir}:$PATH" AYB_GHCR_PROBE_REPO_ROOT="$fixture_root" "$SCRIPT_PATH" > "$output_ok" 2>&1; then
  cat "$output_ok"
  fail "probe helper should exit zero when all required probes execute, even if docker pull findings include failures"
fi

assert_contains "$output_ok" "SUMMARY_PROBE|docker_pull_release_v0_0_7_beta|executed" "summary must include executed release pull probe"
assert_contains "$output_ok" "SUMMARY_FINDING|docker_pull_release_v0_0_7_beta|pull_failure" "release pull failures must be recorded as findings"
assert_contains "$output_ok" "SUMMARY_PROBE|gh_auth_status|executed" "gh auth status probe should execute"
assert_contains "$output_ok" "SUMMARY_NAMESPACE|mirror_prod|ghcr.io/griddlehq/allyourbase" "summary must include mirror prod namespace"

output_missing_gh="${TMP_DIR}/probe_missing_gh.txt"
PATH="${stub_dir}:$PATH" AYB_GHCR_PROBE_REPO_ROOT="$fixture_root" AYB_GHCR_PROBE_GH_COMMAND="gh_missing_for_test" "$SCRIPT_PATH" > "$output_missing_gh" 2>&1 || missing_gh_exit=$?
missing_gh_exit="${missing_gh_exit:-0}"
if [[ "$missing_gh_exit" -eq 0 ]]; then
  cat "$output_missing_gh"
  fail "probe helper should exit non-zero when required probes are not executable"
fi
assert_contains "$output_missing_gh" "SUMMARY_PROBE|gh_auth_status|not_executable" "missing gh should be classified as not_executable"

echo "PASS: ghcr claims probe helper records findings and enforces required-probe execution semantics"
