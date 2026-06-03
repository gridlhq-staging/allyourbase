#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly PG_VERSION="${PG_VERSION:-16}"
readonly PLATFORM="linux-arm64"
readonly ARCHIVE_NAME="ayb-postgres-${PG_VERSION}-${PLATFORM}.tar.xz"
readonly EXPECTED_ARCH="aarch64"
readonly CONTRACT_PROBE_TEST='^TestContractProbeDownloadURL$'

derive_download_url() {
  local probe_output=""
  local derived_url=""

  probe_output="$(
    cd "$ROOT_DIR"
    AYB_CONTRACT_PG_VERSION="$PG_VERSION" \
      AYB_CONTRACT_PG_PLATFORM="$PLATFORM" \
      go test ./internal/pgmanager/platform.go ./internal/pgmanager/platform_contract_probe_test.go -run "$CONTRACT_PROBE_TEST" -count=1 -v
  )"

  derived_url="$(printf '%s\n' "$probe_output" | sed -n 's/^CONTRACT_DOWNLOAD_URL=//p' | tail -n 1)"

  if [[ -z "$derived_url" ]]; then
    printf '%s\n' "failed to derive managed release URL from pgmanager contract probe" >&2
    printf '%s\n' "$probe_output" >&2
    exit 1
  fi

  printf '%s' "$derived_url"
}

readonly DOWNLOAD_URL="$(derive_download_url)"

work_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

archive_path="${work_dir}/${ARCHIVE_NAME}"
extract_dir="${work_dir}/extract"

mkdir -p "$extract_dir"
curl -fsSL "$DOWNLOAD_URL" -o "$archive_path"
tar -xJf "$archive_path" -C "$extract_dir"

postgres_bin="${extract_dir}/ayb-postgres-${PG_VERSION}/bin/postgres"
arch_output="$(file -b "$postgres_bin")"

observed_arch="unknown"
if printf '%s\n' "$arch_output" | grep -iq 'aarch64\|arm64'; then
  observed_arch="aarch64"
elif printf '%s\n' "$arch_output" | grep -iq 'x86-64\|x86_64\|amd64'; then
  observed_arch="x86-64"
fi

if [[ "$observed_arch" == "$EXPECTED_ARCH" ]]; then
  printf '%s\n' "PG_ARM64_ARCH_OK"
  exit 0
fi

printf '%s\n' "PG_ARM64_ARCH_FAIL observed=${observed_arch}"
exit 1
