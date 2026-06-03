#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TRACKER_PATH="${1:-$ROOT_DIR/_dev/competitive-research/SUPABASE-PARITY-TRACKER.md}"
FEATURES_PATH="$ROOT_DIR/_dev/FEATURES.md"
SUPABASE_INVENTORY_PATH="$ROOT_DIR/_dev/competitive-research/competitors/supabase.md"

# Reuse canonical detailed-row parser from the citations contract.
# shellcheck source=tests/contract/parity_tracker_citations.sh
source "$ROOT_DIR/tests/contract/parity_tracker_citations.sh"

TMP_DETAILED="$(mktemp)"
trap 'rm -f "$TMP_DETAILED"' EXIT
extract_detailed_rows "$TRACKER_PATH" >"$TMP_DETAILED"

fail_alignment() {
  echo "$*" >&2
  exit 1
}

tracker_last_updated="$(
  sed -nE 's/^> Last updated: ([0-9]{4}-[0-9]{2}-[0-9]{2}).*/\1/p' "$TRACKER_PATH" | head -n 1
)"

inventory_last_updated="$(
  sed -nE 's/^> Last updated: ([0-9]{4}-[0-9]{2}-[0-9]{2}).*/\1/p' "$SUPABASE_INVENTORY_PATH" | head -n 1
)"

if [ -z "$tracker_last_updated" ]; then
  fail_alignment "missing tracker last-updated stamp: $TRACKER_PATH"
fi
if [ -z "$inventory_last_updated" ]; then
  fail_alignment "missing competitor inventory last-updated stamp: $SUPABASE_INVENTORY_PATH"
fi
if [ "$inventory_last_updated" != "$tracker_last_updated" ]; then
  fail_alignment "supabase inventory last-updated ($inventory_last_updated) does not match tracker ($tracker_last_updated)"
fi

if rg -n "Security advisor dashboard stub|Performance advisor dashboard stub" "$FEATURES_PATH" >/dev/null; then
  fail_alignment "FEATURES advisor rows still marked as dashboard stubs; update wording to match shipped tracker status"
fi

for required_claim in \
  "Edge runtime parity gaps (AYB vs Supabase)" \
  "Observability parity gaps (AYB vs Supabase)" \
  "Vector parity gaps (AYB vs Supabase)" \
  "Compliance parity gaps (AYB vs Supabase)" \
  "WASM support: NOT SHIPPED" \
  "Regional/edge invocation: NOT SHIPPED" \
  "Custom dashboards: NOT SHIPPED" \
  "Vector buckets: NOT SHIPPED" \
  "SOC 2 compliance: NOT SHIPPED" \
  "HIPAA compliance: NOT SHIPPED"; do
  if ! rg -nF "$required_claim" "$SUPABASE_INVENTORY_PATH" >/dev/null; then
    fail_alignment "missing cross-doc parity claim in supabase inventory: $required_claim"
  fi
done

awk -F'\t' '
  BEGIN {
    mismatches = 0
  }
  FNR == NR {
    cat = $1
    status = $3
    d_tracked[cat]++
    if (status == "SHIPPED") d_shipped[cat]++
    else if (status == "PARTIAL") d_partial[cat]++
    else if (status == "NOT SHIPPED") d_missing[cat]++
    else {
      printf("unknown status in detailed table: %s :: %s\n", cat, status)
      mismatches++
    }
    next
  }
  /^## Summary/ { in_summary = 1; next }
  in_summary == 1 && /^---$/ { in_summary = 0 }
  in_summary != 1 { next }
  /^\|[-: ]+\|/ { next }
  /^\|[[:space:]]*Category[[:space:]]*\|/ { next }
  /^\|/ {
    n = split($0, cols, "|")
    if (n < 7) next
    category = cols[2]
    tracked = cols[3]
    shipped = cols[4]
    partial = cols[5]
    missing = cols[6]
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", category)
    gsub(/\*/, "", category)
    gsub(/\*/, "", tracked)
    gsub(/\*/, "", shipped)
    gsub(/\*/, "", partial)
    gsub(/\*/, "", missing)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", tracked)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", shipped)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", partial)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", missing)
    if (category == "") next

    if (category == "TOTAL") {
      saw_total = 1
      s_total_tracked = tracked + 0
      s_total_shipped = shipped + 0
      s_total_partial = partial + 0
      s_total_missing = missing + 0
      next
    }

    s_seen[category] = 1
    s_tracked[category] = tracked + 0
    s_shipped[category] = shipped + 0
    s_partial[category] = partial + 0
    s_missing[category] = missing + 0
  }
  END {
    for (cat in s_seen) {
      if (s_shipped[cat] + s_partial[cat] + s_missing[cat] != s_tracked[cat]) {
        printf("summary arithmetic mismatch: %s has tracked=%d but shipped+partial+missing=%d\n", cat, s_tracked[cat], s_shipped[cat] + s_partial[cat] + s_missing[cat])
        mismatches++
      }

      dt = d_tracked[cat] + 0
      ds = d_shipped[cat] + 0
      dp = d_partial[cat] + 0
      dm = d_missing[cat] + 0
      if (s_tracked[cat] != dt || s_shipped[cat] != ds || s_partial[cat] != dp || s_missing[cat] != dm) {
        printf("summary vs detailed mismatch: %s summary=(%d,%d,%d,%d) detailed=(%d,%d,%d,%d)\n", cat, s_tracked[cat], s_shipped[cat], s_partial[cat], s_missing[cat], dt, ds, dp, dm)
        mismatches++
      }
    }

    d_total_tracked = 0
    d_total_shipped = 0
    d_total_partial = 0
    d_total_missing = 0
    for (cat in d_tracked) {
      d_total_tracked += d_tracked[cat]
      d_total_shipped += d_shipped[cat]
      d_total_partial += d_partial[cat]
      d_total_missing += d_missing[cat]
    }

    if (!saw_total) {
      print "summary TOTAL row missing"
      mismatches++
    } else if (s_total_tracked != d_total_tracked || s_total_shipped != d_total_shipped || s_total_partial != d_total_partial || s_total_missing != d_total_missing) {
      printf("TOTAL row mismatch: summary=(%d,%d,%d,%d) calculated=(%d,%d,%d,%d)\n", s_total_tracked, s_total_shipped, s_total_partial, s_total_missing, d_total_tracked, d_total_shipped, d_total_partial, d_total_missing)
      mismatches++
    }

    if (mismatches == 0) {
      print "PARITY_SUMMARY_OK"
      exit 0
    }
    printf("PARITY_SUMMARY_FAIL mismatches=%d\n", mismatches)
    exit 1
  }
' "$TMP_DETAILED" "$TRACKER_PATH"
