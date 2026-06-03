#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TRACKER_PATH="${1:-$ROOT_DIR/_dev/competitive-research/SUPABASE-PARITY-TRACKER.md}"

fail() {
  echo "$*" >&2
  exit 1
}

trim() {
  sed -E "s/^[[:space:]]+//; s/[[:space:]]+$//"
}

extract_detailed_rows() {
  local input_path="$1"

  # Parse markdown tables in Detailed Breakdown only.
  # Field 3 is the exact AYB status column and is matched literally.
  awk -F'|' '
    /^## Detailed Breakdown/ { in_details=1; next }
    /^## Priority Gap Analysis/ { in_details=0 }
    in_details != 1 { next }
    /^### / { category=$0; sub(/^### /, "", category); sub(/ \([0-9]+% parity\)$/, "", category); next }
    /^\|[-: ]+\|/ { next }
    /^\|[[:space:]]*(Supabase Feature|Feature)[[:space:]]*\|/ { next }
    /^\|/ {
      feature=$2; status=$3; notes=$4
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", feature)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", status)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", notes)
      if (category == "") category="(uncategorized)"
      if (feature != "" && status != "") print category "\t" feature "\t" status "\t" notes
    }
  ' "$input_path"
}

extract_shipped_rows() {
  local input_path="$1"
  extract_detailed_rows "$input_path" | awk -F'\t' '$3 == "SHIPPED" { print $2 "\t" $4 }'
}

validate_file() {
  local input_path="$1"
  local missing=0

  while IFS=$'\t' read -r feature notes; do
    [ -n "$feature" ] || continue
    local token
    token="$(printf '%s\n' "$notes" | sed -nE 's/.*`evidence:[[:space:]]*([^`]+)`/\1/p')"
    if [ -z "$token" ]; then
      echo "missing evidence token: $feature"
      missing=$((missing + 1))
      continue
    fi

    local rel_path="$token"
    local line_no=""
    if [[ "$token" =~ ^([^:]+):([0-9]+)$ ]]; then
      rel_path="${BASH_REMATCH[1]}"
      line_no="${BASH_REMATCH[2]}"
    fi

    rel_path="$(printf '%s' "$rel_path" | trim)"
    local abs_path="$ROOT_DIR/$rel_path"
    if [ ! -e "$abs_path" ]; then
      echo "missing evidence path: $feature -> $rel_path"
      missing=$((missing + 1))
      continue
    fi

    if [ -n "$line_no" ]; then
      if ! sed -n "${line_no}p" "$abs_path" | grep -q '.'; then
        echo "missing evidence line: $feature -> $rel_path:$line_no"
        missing=$((missing + 1))
        continue
      fi
    fi

    # When a row names a concrete function symbol, verify it is present.
    local symbol
    symbol="$(printf '%s\n' "$feature" | sed -nE 's/.*\(([A-Z][A-Za-z0-9_]*)\).*/\1/p')"
    if [ -n "$symbol" ] && ! grep -q "$symbol" "$abs_path"; then
      echo "missing evidence symbol: $feature -> $rel_path :: $symbol"
      missing=$((missing + 1))
      continue
    fi
  done < <(extract_shipped_rows "$input_path")

  if [ "$missing" -eq 0 ]; then
    echo "PARITY_CITATIONS_OK"
    return 0
  fi

  echo "PARITY_CITATIONS_FAIL missing=$missing"
  return 1
}

run_self_test() {
  local fixture_bad fixture_good
  fixture_bad="$(mktemp)"
  fixture_good="$(mktemp)"

  cat >"$fixture_bad" <<'FIX'
## Detailed Breakdown
| Supabase Feature | AYB Status | Notes |
|-----------------|:----------:|-------|
| Good fixture row | SHIPPED | `evidence: tests/contract/parity_tracker_citations.sh:1` |
| Bad fixture row | SHIPPED | `evidence: tests/contract/does_not_exist_fixture.sh` |
## Priority Gap Analysis
FIX

  cat >"$fixture_good" <<'FIX'
## Detailed Breakdown
| Supabase Feature | AYB Status | Notes |
|-----------------|:----------:|-------|
| Good fixture row | SHIPPED | `evidence: tests/contract/parity_tracker_citations.sh:1` |
## Priority Gap Analysis
FIX

  if ! validate_file "$fixture_good" >/dev/null; then
    rm -f "$fixture_bad" "$fixture_good"
    fail "self-test failed: expected good fixture to pass"
  fi

  if validate_file "$fixture_bad" >/dev/null 2>&1; then
    rm -f "$fixture_bad" "$fixture_good"
    fail "self-test failed: expected bad fixture to fail"
  fi

  rm -f "$fixture_bad" "$fixture_good"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  run_self_test
  validate_file "$TRACKER_PATH"
fi
