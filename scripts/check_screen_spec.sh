#!/usr/bin/env bash
set -euo pipefail

# Reusable screen-spec format gate for docs/reference/screen_specs/*.md.
#
# Validates structure (not loose substring presence): the six mandatory
# top-level sections must appear in canonical order, the "## State contract"
# block must contain the "### Loading" and "### Error" subsections, and each
# optional "## Current implementation gaps" "- Current:" record must carry its
# own following "- Target:" and "- Evidence:" lines.
#
# SCREEN_SPEC_GAP_RECORD_MODE controls only gap-record violations:
#   fatal (default) rejects incomplete gap records.
#   warn prints incomplete gap records while allowing traversal to continue.
# Existing section-order and State contract failures remain fatal in every mode.
#
# Format contract source of truth: docs/reference/screen_specs/_template.md and
# guides/ui_screen_specs.md ("Every screen spec has six core sections").

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <screen-spec-markdown-path-or-directory>" >&2
  exit 1
fi

readonly SPEC_TARGET="$1"
readonly SCREEN_SPEC_GAP_RECORD_MODE="${SCREEN_SPEC_GAP_RECORD_MODE:-fatal}"

case "$SCREEN_SPEC_GAP_RECORD_MODE" in
fatal | warn)
  ;;
*)
  echo "Screen spec format gate failed: invalid SCREEN_SPEC_GAP_RECORD_MODE '$SCREEN_SPEC_GAP_RECORD_MODE' while checking $SPEC_TARGET; expected 'fatal' or 'warn'" >&2
  exit 1
  ;;
esac

is_skipped_spec() {
  case "$1" in
  DIRMAP.md | _template.md)
    return 0
    ;;
  esac
  return 1
}

# Single awk pass holds the entire format contract in one place: the six
# required top-level sections (in canonical order) and the two State contract
# subsections. "## Current implementation gaps" is intentionally not required —
# the template marks it optional. Section names are ";"-delimited (no name
# contains ";").
validate_spec() {
  local spec_path="$1"
  awk -v path="$spec_path" -v gap_mode="$SCREEN_SPEC_GAP_RECORD_MODE" '
  function trim(value) { gsub(/[[:space:]]+$/, "", value); return value }
  function missing_gap_fields() {
    if (!gap_has_target) {
      return gap_has_evidence ? "'\''- Target:'\''" : "'\''- Target:'\'' and '\''- Evidence:'\''"
    }
    if (!gap_has_evidence) {
      return "'\''- Evidence:'\''"
    }
    return ""
  }
  function flush_gap_record(  missing, severity) {
    if (!gap_record_open) {
      return
    }
    missing = missing_gap_fields()
    if (missing != "") {
      severity = (gap_mode == "warn") ? "warning" : "failed"
      printf("Screen spec format gate %s: missing %s in '\''## Current implementation gaps'\'' record starting at line %d in %s\n", severity, missing, gap_record_line, path) > "/dev/stderr"
      gap_failures++
    }
    gap_record_open = gap_has_target = gap_has_evidence = gap_record_line = 0
  }

  BEGIN {
    required_count = split("Task;Layout;State contract;Navigation;Acceptance criteria;Edge cases", required, ";")
    subsection_count = split("Loading;Error", subsections, ";")
    order_pointer = 1
    in_state_contract = 0
    in_gaps = 0
    gap_record_open = 0
    gap_failures = 0
  }

  /^## / {
    flush_gap_record()
    name = trim(substr($0, 4))
    in_state_contract = (name == "State contract")
    in_gaps = (name == "Current implementation gaps")
    seen_section[name] = 1
    if (order_pointer <= required_count && name == required[order_pointer]) {
      order_pointer++
    }
    next
  }

  /^### / {
    if (in_state_contract) {
      seen_subsection[trim(substr($0, 5))] = 1
    }
    next
  }

  in_gaps && /^- Current:/ {
    flush_gap_record()
    gap_record_open = 1
    gap_has_target = gap_has_evidence = 0
    gap_record_line = NR
    next
  }

  in_gaps && gap_record_open && /^- Target:/ {
    gap_has_target = 1
    next
  }

  in_gaps && gap_record_open && /^- Evidence:/ {
    gap_has_evidence = 1
    next
  }

  END {
    flush_gap_record()
    for (i = 1; i <= required_count; i++) {
      if (!(required[i] in seen_section)) {
        printf("Screen spec format gate failed: missing required section '\''## %s'\'' in %s\n", required[i], path) > "/dev/stderr"
        exit 1
      }
    }
    if (order_pointer <= required_count) {
      printf("Screen spec format gate failed: required sections are out of canonical order in %s\n", path) > "/dev/stderr"
      exit 1
    }
    for (i = 1; i <= subsection_count; i++) {
      if (!(subsections[i] in seen_subsection)) {
        printf("Screen spec format gate failed: '\''## State contract'\'' is missing '\''### %s'\'' in %s\n", subsections[i], path) > "/dev/stderr"
        exit 1
      }
    }
    if (gap_failures > 0 && gap_mode == "fatal") {
      exit 1
    }
    printf("Screen spec format gate passed: %s\n", path)
  }
' "$spec_path"
}

check_file() {
  local spec_path="$1"

  if [[ ! -f "$spec_path" ]]; then
    echo "Screen spec file not found: $spec_path" >&2
    exit 1
  fi

  local spec_basename="${spec_path##*/}"
  if is_skipped_spec "$spec_basename"; then
    echo "SKIP (not a spec): $spec_path"
    exit 0
  fi

  validate_spec "$spec_path"
}

check_directory() {
  local spec_dir="$1"

  if [[ ! -d "$spec_dir" ]]; then
    echo "Screen spec directory not found: $spec_dir" >&2
    exit 1
  fi

  local specs=()
  local path
  shopt -s nullglob
  for path in "$spec_dir"/*.md; do
    if ! is_skipped_spec "${path##*/}"; then
      specs+=("$path")
    fi
  done
  shopt -u nullglob

  local total="${#specs[@]}"
  if [[ "$total" -eq 0 ]]; then
    echo "Screen spec format gate failed: no screen specs found in $spec_dir" >&2
    exit 1
  fi

  local failures=0
  for path in "${specs[@]}"; do
    if ! validate_spec "$path"; then
      failures=$((failures + 1))
    fi
  done

  if [[ "$failures" -ne 0 ]]; then
    echo "Screen spec format gate failed: $failures/$total specs failed in $spec_dir" >&2
    exit 1
  fi

  echo "Screen spec format gate passed: $total specs checked in $spec_dir"
}

if [[ -d "$SPEC_TARGET" ]]; then
  check_directory "$SPEC_TARGET"
else
  check_file "$SPEC_TARGET"
fi
