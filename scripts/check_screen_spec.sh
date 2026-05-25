#!/usr/bin/env bash
set -euo pipefail

# Reusable screen-spec format gate for docs/screen_specs/*.md.
#
# Validates structure (not loose substring presence): the six mandatory
# top-level sections must appear in canonical order, and the "## State contract"
# block must contain the "### Loading" and "### Error" subsections. This lets
# downstream stages fail on structure before debating spec content.
#
# Format contract source of truth: docs/screen_specs/_template.md and
# guides/ui_screen_specs.md ("Every screen spec has six core sections").

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <screen-spec-markdown-path>" >&2
  exit 1
fi

readonly SPEC_PATH="$1"

if [[ ! -f "$SPEC_PATH" ]]; then
  echo "Screen spec file not found: $SPEC_PATH" >&2
  exit 1
fi

# Single awk pass holds the entire format contract in one place: the six
# required top-level sections (in canonical order) and the two State contract
# subsections. "## Current implementation gaps" is intentionally not required —
# the template marks it optional. Section names are ";"-delimited (no name
# contains ";").
awk -v path="$SPEC_PATH" '
  function trim(value) { gsub(/[[:space:]]+$/, "", value); return value }

  BEGIN {
    required_count = split("Task;Layout;State contract;Navigation;Acceptance criteria;Edge cases", required, ";")
    subsection_count = split("Loading;Error", subsections, ";")
    # Pointer into the canonical sequence; advances only on in-order matches so
    # a reordered spec leaves it short of the end.
    order_pointer = 1
    in_state_contract = 0
  }

  # Top-level ("## ") headers. The trailing space in the pattern excludes the
  # deeper "### " headers, which are handled separately below.
  /^## / {
    name = trim(substr($0, 4))
    in_state_contract = (name == "State contract")
    seen_section[name] = 1
    if (order_pointer <= required_count && name == required[order_pointer]) {
      order_pointer++
    }
    next
  }

  # Subsection headers, only counted while inside the State contract block.
  /^### / {
    if (in_state_contract) {
      seen_subsection[trim(substr($0, 5))] = 1
    }
    next
  }

  END {
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
    printf("Screen spec format gate passed: %s\n", path)
  }
' "$SPEC_PATH"
