#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

readonly FILE_LINE_LIMIT=500
readonly TS_FILE_LINE_LIMIT=800
readonly TSX_FILE_LINE_LIMIT=600
readonly SCAN_ROOT="${CHECK_FILE_SIZES_ROOT:-$REPO_ROOT}"
readonly ALLOWLIST_PATH="${CHECK_FILE_SIZES_ALLOWLIST:-$REPO_ROOT/scripts/allowlist-oversized.txt}"
readonly TS_ALLOWLIST_PATH="${CHECK_FILE_SIZES_TS_ALLOWLIST:-$REPO_ROOT/scripts/allowlist-oversized-typescript.txt}"

normalize_path() {
  local path="$1"
  path="${path#./}"
  echo "$path"
}

line_limit_for_path() {
  local path="$1"
  case "$path" in
    *.tsx) echo "$TSX_FILE_LINE_LIMIT" ;;
    *.ts) echo "$TS_FILE_LINE_LIMIT" ;;
    *) echo "$FILE_LINE_LIMIT" ;;
  esac
}

validate_allowlist_format() {
  local allowlist_path="$1"
  [[ -f "$allowlist_path" ]] || return 0

  while IFS= read -r entry; do
    entry="$(echo "$entry" | sed 's/[[:space:]]*$//')"
    [[ -z "$entry" || "$entry" =~ ^# ]] && continue

    local local_path="${entry%%:*}"
    local line_count="${entry##*:}"

    if [[ -z "$local_path" || -z "$line_count" || "$local_path" == "$entry" ]]; then
      echo "Invalid allowlist entry in $allowlist_path: $entry" >&2
      exit 1
    fi
    if ! [[ "$line_count" =~ ^[0-9]+$ ]]; then
      echo "Invalid line count in allowlist entry $entry" >&2
      exit 1
    fi

    local normalized_path
    normalized_path="$(normalize_path "$local_path")"
    if [[ "$normalized_path" != "$local_path" ]]; then
      echo "Invalid allowlist path (must not start with ./): $entry" >&2
      exit 1
    fi
  done < "$allowlist_path"
}

allowlisted_count_for_path() {
  local allowlist_path="$1"
  local relative_path="$2"
  [[ -f "$allowlist_path" ]] || return 0

  awk -F: -v path="$relative_path" '
    $0 !~ /^[[:space:]]*#/ && NF == 2 && $1 == path { print $2; exit }
  ' "$allowlist_path"
}

validate_typescript_allowlist_entries() {
  local allowlist_path="$1"
  [[ -f "$allowlist_path" ]] || return 0

  local failures=()
  while IFS= read -r entry; do
    entry="$(echo "$entry" | sed 's/[[:space:]]*$//')"
    [[ -z "$entry" || "$entry" =~ ^# ]] && continue

    local local_path="${entry%%:*}"
    local allowlisted_count="${entry##*:}"
    local absolute_path="$SCAN_ROOT/$local_path"
    local limit
    limit="$(line_limit_for_path "$local_path")"

    if [[ ! -f "$absolute_path" ]]; then
      failures+=("Stale allowlist entry in $allowlist_path: $entry (file missing)")
      continue
    fi

    local line_count
    line_count="$(wc -l < "$absolute_path" | tr -d '[:space:]')"
    if (( line_count <= limit )); then
      failures+=("Stale allowlist entry in $allowlist_path: $entry (file has $line_count lines)")
    fi
  done < "$allowlist_path"

  if (( ${#failures[@]} > 0 )); then
    printf '%s\n' "${failures[@]}" >&2
    exit 1
  fi
}

scan_file_size_violations() {
  local allowlist_path="$1"
  shift
  local violations=()

  while IFS= read -r -d '' file; do
    local relative_path
    relative_path="$(normalize_path "${file#$SCAN_ROOT/}")"
    local line_count
    line_count="$(wc -l < "$file" | tr -d '[:space:]')"
    local line_limit
    line_limit="$(line_limit_for_path "$relative_path")"

    if (( line_count <= line_limit )); then
      continue
    fi

    local allowlisted_count
    allowlisted_count="$(allowlisted_count_for_path "$allowlist_path" "$relative_path")"
    if [[ -z "$allowlisted_count" ]]; then
      violations+=("$relative_path:$line_count (missing from allowlist)")
      continue
    fi

    if (( line_count > allowlisted_count )); then
      violations+=("$relative_path:$line_count (allowlist has $allowlisted_count)")
    fi
  done < <("$@")

  if (( ${#violations[@]} > 0 )); then
    printf '%s\n' "${violations[@]}" | LC_ALL=C sort
  fi
}

find_go_sources() {
  find "$SCAN_ROOT" -name '*.go' -type f ! -name '*_test.go' ! -path "$SCAN_ROOT/vendor/*" ! -path "$SCAN_ROOT/_dev/*" -print0
}

find_typescript_sources() {
  # Test-file exclusions intentionally mirror Go's source-only guardrail.
  find "$SCAN_ROOT" \( -path '*/node_modules' -o -path '*/dist' -o -path '*/__tests__' \) -prune -o \
    \( -name '*.ts' -o -name '*.tsx' \) -type f \
    ! -name '*.d.ts' \
    ! -name '*.test.ts' \
    ! -name '*.test.tsx' \
    ! -name '*.spec.ts' \
    ! -name '*.spec.tsx' \
    -print0
}

validate_allowlist_format "$ALLOWLIST_PATH"
validate_allowlist_format "$TS_ALLOWLIST_PATH"
validate_typescript_allowlist_entries "$TS_ALLOWLIST_PATH"

go_violations="$(scan_file_size_violations "$ALLOWLIST_PATH" find_go_sources)"
if [[ -n "$go_violations" ]]; then
  echo "Go source-size guardrail failed (limit ${FILE_LINE_LIMIT} lines)."
  echo "Oversized files not covered by allowlist or with stale counts:"
  printf '%s\n' "$go_violations"
  exit 1
fi

echo "Go source-size guardrail passed."

typescript_violations="$(scan_file_size_violations "$TS_ALLOWLIST_PATH" find_typescript_sources)"
if [[ -n "$typescript_violations" ]]; then
  echo "TypeScript source-size guardrail failed (limits: .ts ${TS_FILE_LINE_LIMIT} lines, .tsx ${TSX_FILE_LINE_LIMIT} lines)."
  echo "Oversized TypeScript files not covered by allowlist or with stale counts:"
  printf '%s\n' "$typescript_violations"
  exit 1
fi

echo "TypeScript source-size guardrail passed."
