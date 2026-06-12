#!/bin/sh
# install.sh — Single-command installer for Allyourbase (AYB).
#
# Usage:
#   curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh      # latest release
#   sh /tmp/ayb-install.sh
#
#   curl -fsSLo /tmp/ayb-install.sh https://staging.allyourbase.io/install.sh      # pinned version
#   sh /tmp/ayb-install.sh v0.0.12-beta
#
# Environment variables:
#   AYB_INSTALL    - Install directory (default: ~/.ayb)
#   AYB_REPO       - GitHub owner/repo (default: set per distribution)
#   AYB_VERSION    - Version to install (default: latest)
#   GITHUB_TOKEN   - GitHub token for private repos / rate limits
#   NO_MODIFY_PATH - Set to 1 to skip PATH modification

set -eu

# ── Configuration ────────────────────────────────────────────────────────────

REPO="${AYB_REPO:-gridlhq-staging/allyourbase}"
BINARY_NAME="ayb"
INSTALL_DIR="${AYB_INSTALL:-$HOME/.ayb}/bin"

# ── Colors (disabled when piped) ─────────────────────────────────────────────

setup_colors() {
  if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
  else
    RED='' GREEN='' YELLOW='' BLUE='' BOLD='' NC=''
  fi
}

info()  { printf "${BLUE}info${NC}  %s\n" "$1"; }
warn()  { printf "${YELLOW}warn${NC}  %s\n" "$1"; }
error() { printf "${RED}error${NC} %s\n" "$1" >&2; }

# ── Platform Detection ───────────────────────────────────────────────────────

detect_platform() {
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Linux*)   os="linux" ;;
    Darwin*)  os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*)
      error "Windows is not supported by this installer."
      error "Download the .zip from: https://github.com/${REPO}/releases/latest"
      exit 1
      ;;
    *)
      error "Unsupported operating system: $os"
      exit 1
      ;;
  esac

  case "$arch" in
    x86_64|amd64)    arch="amd64" ;;
    aarch64|arm64)   arch="arm64" ;;
    *)
      error "Unsupported architecture: $arch"
      exit 1
      ;;
  esac

  # Detect Rosetta 2 on macOS — if uname reports x86_64 but we're on Apple Silicon,
  # prefer the native ARM64 build
  if [ "$os" = "darwin" ] && [ "$arch" = "amd64" ]; then
    if sysctl -n sysctl.proc_translated 2>/dev/null | grep -q 1; then
      arch="arm64"
      info "Detected Rosetta 2 — installing native Apple Silicon build"
    fi
  fi

  info "Detected platform: ${os}/${arch}"
}

# ── Download Tool Detection ──────────────────────────────────────────────────

detect_downloader() {
  if command -v curl > /dev/null 2>&1; then
    downloader="curl"
  elif command -v wget > /dev/null 2>&1; then
    downloader="wget"
  else
    error "Neither curl nor wget found. Please install one and try again."
    exit 1
  fi
}

# Reject unsafe env overrides before we build GitHub API URLs or write PATH
# snippets into shell profile files.
validate_config() {
  repo_owner=${REPO%%/*}
  repo_name=${REPO#*/}
  if [ "$repo_owner" = "$REPO" ] || [ -z "$repo_owner" ] || [ -z "$repo_name" ]; then
    error "Invalid AYB_REPO: expected GitHub owner/repo slug"
    exit 1
  fi
  case "$REPO" in
    *[!A-Za-z0-9._/-]*|*/*/*)
      error "Invalid AYB_REPO: expected GitHub owner/repo slug"
      exit 1
      ;;
  esac
  case "$repo_owner" in
    [A-Za-z0-9]*)
      ;;
    *)
      error "Invalid AYB_REPO: expected GitHub owner/repo slug"
      exit 1
      ;;
  esac
  case "$repo_name" in
    [A-Za-z0-9]*)
      ;;
    *)
      error "Invalid AYB_REPO: expected GitHub owner/repo slug"
      exit 1
      ;;
  esac

  case "$INSTALL_DIR" in
    /*)
      ;;
    *)
      error "Invalid AYB_INSTALL: use an absolute path with only letters, numbers, /, ., _, +, :, and -"
      exit 1
      ;;
  esac
  case "$INSTALL_DIR" in
    *[!A-Za-z0-9._/+:-]*)
      error "Invalid AYB_INSTALL: use an absolute path with only letters, numbers, /, ., _, +, :, and -"
      exit 1
      ;;
  esac
}

download() {
  url="$1"
  output="$2"

  auth_header=""
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    auth_header="Authorization: token ${GITHUB_TOKEN}"
  fi

  if [ "$downloader" = "curl" ]; then
    if [ -n "$auth_header" ]; then
      curl -fsSL -H "$auth_header" -o "$output" "$url"
    else
      curl -fsSL -o "$output" "$url"
    fi
  else
    if [ -n "$auth_header" ]; then
      wget -q --header="$auth_header" -O "$output" "$url"
    else
      wget -q -O "$output" "$url"
    fi
  fi
}

# Returns recent release JSON for the configured repo.
fetch_releases_json() {
  api_url="https://api.github.com/repos/${REPO}/releases?per_page=20"

  if [ "$downloader" = "curl" ]; then
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "$api_url"
    else
      curl -fsSL "$api_url"
    fi
  else
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      wget -qO- --header="Authorization: token ${GITHUB_TOKEN}" "$api_url"
    else
      wget -qO- "$api_url"
    fi
  fi
}

# Returns the latest AYB app-release tag. Auxiliary repo releases such as
# managed-Postgres `pg-*` tags must not be treated as installer targets.
latest_app_release_tag() {
  releases_json=$(fetch_releases_json)
  printf '%s\n' "$releases_json" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"//;s/".*//' \
    | grep '^v[0-9]' \
    | head -1
}

# ── Version Resolution ───────────────────────────────────────────────────────

get_version() {
  if [ -n "${AYB_VERSION:-}" ]; then
    version="$AYB_VERSION"
    return
  fi

  # Parse version from CLI args (e.g., `| sh -s -- v0.1.0`)
  if [ -n "${1:-}" ]; then
    version="$1"
    return
  fi

  info "Fetching latest AYB release version..."
  version=$(latest_app_release_tag)

  if [ -z "$version" ]; then
    error "Could not determine latest AYB release version. Check https://github.com/${REPO}/releases"
    error "You can also specify a version: curl ... | sh -s -- v0.1.0"
    exit 1
  fi

  info "Latest version: ${version}"
}

# ── GitHub API Asset Download (for private repos) ────────────────────────────

# Resolves a release asset's API download URL and downloads it.
# For public repos, falls back to the direct GitHub download URL.
download_release_asset() {
  asset_name="$1"
  output="$2"

  if [ -n "${GITHUB_TOKEN:-}" ]; then
    # Use GitHub API to find the asset ID, then download via API (works for private repos)
    api_url="https://api.github.com/repos/${REPO}/releases/tags/${version}"
    if [ "$downloader" = "curl" ]; then
      asset_url=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "$api_url" \
        | grep -B 3 "\"name\": \"${asset_name}\"" \
        | grep '"url"' | head -1 \
        | sed 's/.*"url": *"//;s/".*//')
    else
      asset_url=$(wget -qO- --header="Authorization: token ${GITHUB_TOKEN}" "$api_url" \
        | grep -B 3 "\"name\": \"${asset_name}\"" \
        | grep '"url"' | head -1 \
        | sed 's/.*"url": *"//;s/".*//')
    fi

    if [ -n "$asset_url" ]; then
      # Download via API with octet-stream accept header
      if [ "$downloader" = "curl" ]; then
        curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" -H "Accept: application/octet-stream" -o "$output" "$asset_url"
      else
        wget -q --header="Authorization: token ${GITHUB_TOKEN}" --header="Accept: application/octet-stream" -O "$output" "$asset_url"
      fi
      return $?
    fi
  fi

  # Fallback: direct URL (works for public repos)
  base_url="https://github.com/${REPO}/releases/download/${version}"
  download "${base_url}/${asset_name}" "$output"
}

# ── Download & Verify ────────────────────────────────────────────────────────

download_and_verify() {
  # Strip leading 'v' for archive naming (goreleaser uses version without v prefix)
  version_num=$(echo "$version" | sed 's/^v//')
  archive_name="ayb_${version_num}_${os}_${arch}.tar.gz"

  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' EXIT

  info "Downloading ${archive_name}..."
  download_release_asset "$archive_name" "${tmpdir}/${archive_name}"

  info "Downloading checksums..."
  if ! download_release_asset "checksums.txt" "${tmpdir}/checksums.txt" 2>/dev/null; then
    error "Missing checksums.txt for ${version}; refusing unverified install."
    exit 1
  fi

  info "Verifying SHA256 checksum..."
  expected=$(awk -v name="$archive_name" '$2 == name { print $1; exit }' "${tmpdir}/checksums.txt")
  if [ -z "$expected" ]; then
    error "No checksum entry for ${archive_name}; refusing unverified install."
    exit 1
  fi

  if command -v sha256sum > /dev/null 2>&1; then
    actual=$(sha256sum "${tmpdir}/${archive_name}" | awk '{print $1}')
  elif command -v shasum > /dev/null 2>&1; then
    actual=$(shasum -a 256 "${tmpdir}/${archive_name}" | awk '{print $1}')
  else
    error "No SHA256 tool found; cannot verify ${archive_name}."
    exit 1
  fi

  if [ "$actual" = "$expected" ]; then
    printf '  %sChecksum verified%s\n' "$GREEN" "$NC"
  else
    error "Checksum verification FAILED! The download may be corrupted."
    error "  expected: $expected"
    error "  got:      $actual"
    exit 1
  fi
}

# ── Install ──────────────────────────────────────────────────────────────────

install_binary() {
  info "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."

  mkdir -p "$INSTALL_DIR"

  tar xzf "${tmpdir}/${archive_name}" -C "$tmpdir"
  mv "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"
}

# ── PATH Setup ───────────────────────────────────────────────────────────────

setup_path() {
  if [ "${NO_MODIFY_PATH:-0}" = "1" ]; then
    return
  fi

  # Check if already in PATH
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*)
      return
      ;;
  esac

  export_line="export PATH=\"${INSTALL_DIR}:\$PATH\""
  profile_updated=false

  # Update all detected shell configs, not just $SHELL.
  # Users often have $SHELL set to one thing but use another interactively.

  # bash
  for rc in "$HOME/.bashrc" "$HOME/.bash_profile"; do
    if [ -f "$rc" ]; then
      if ! grep -qF "$INSTALL_DIR" "$rc" 2>/dev/null; then
        if printf '\n# Allyourbase\n%s\n' "$export_line" >> "$rc" 2>/dev/null; then
          profile_updated=true
          info "Added to ${rc}"
        else
          warn "Could not write to ${rc} (permission denied)"
        fi
      fi
      break
    fi
  done

  # zsh
  rc="$HOME/.zshrc"
  if [ -f "$rc" ]; then
    if ! grep -qF "$INSTALL_DIR" "$rc" 2>/dev/null; then
      if printf '\n# Allyourbase\n%s\n' "$export_line" >> "$rc" 2>/dev/null; then
        profile_updated=true
        info "Added to ${rc}"
      else
        warn "Could not write to ${rc} (permission denied)"
      fi
    fi
  fi

  # fish
  fish_conf="${HOME}/.config/fish/config.fish"
  fish_line="set -gx PATH ${INSTALL_DIR} \$PATH"
  if [ -d "$(dirname "$fish_conf")" ]; then
    if ! grep -qF "$INSTALL_DIR" "$fish_conf" 2>/dev/null; then
      if printf '\n# Allyourbase\n%s\n' "$fish_line" >> "$fish_conf" 2>/dev/null; then
        profile_updated=true
        info "Added to ${fish_conf}"
      else
        warn "Could not write to ${fish_conf} (permission denied)"
      fi
    fi
  fi

  if [ "$profile_updated" = "false" ]; then
    warn "Could not auto-update PATH. Add this to your shell profile:"
    printf "  %s\n" "$export_line"
  fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  setup_colors
  validate_config

  printf "\n"
  printf '  %sAllyourbase Installer%s\n' "$BOLD" "$NC"
  printf '  %shttps://github.com/%s%s\n' "$BLUE" "$REPO" "$NC"
  printf "\n"

  detect_platform
  detect_downloader
  get_version "${1:-}"
  download_and_verify
  install_binary
  setup_path

  printf "\n"
  printf '  %s%sayb %s installed successfully!%s\n' "$GREEN" "$BOLD" "$version" "$NC"
  printf "\n"
  printf '  Binary:  %s/%s\n' "$INSTALL_DIR" "$BINARY_NAME"
  printf "\n"
  printf "  Get started:\n"
  start_command="ayb start"
  database_command="ayb start --database-url URL"
  if command -v "$BINARY_NAME" > /dev/null 2>&1; then
    :
  else
    case ":${PATH}:" in
      *":${INSTALL_DIR}:"*)
        :
        ;;
      *)
        start_command="${INSTALL_DIR}/${BINARY_NAME} start"
        database_command="${INSTALL_DIR}/${BINARY_NAME} start --database-url URL"
        ;;
    esac
  fi
  printf '    %s%s%s                     # managed Postgres, zero config\n' "$BOLD" "$start_command" "$NC"
  printf '    %s%s%s  # external Postgres\n' "$BOLD" "$database_command" "$NC"

  # Check if we need to remind about PATH
  if ! command -v "$BINARY_NAME" > /dev/null 2>&1; then
    case ":${PATH}:" in
      *":${INSTALL_DIR}:"*)
        ;;
      *)
        printf "\n"
        printf '  %sRestart your terminal to update your PATH.%s\n' "$YELLOW" "$NC"
        ;;
    esac
  fi

  printf "\n"
}

main "$@"
