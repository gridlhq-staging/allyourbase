#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || "$1" != *=* ]]; then
  echo "Usage: $0 <label=path|label=->" >&2
  exit 1
fi

readonly INPUT_SPEC="$1"
readonly LABEL="${INPUT_SPEC%%=*}"
readonly LEDGER_PATH="${INPUT_SPEC#*=}"

if [[ -z "$LABEL" || -z "$LEDGER_PATH" ]]; then
  echo "Follow-up ledger detector failed: label and path must both be non-empty" >&2
  exit 1
fi

if [[ "$LEDGER_PATH" != "-" && ! -f "$LEDGER_PATH" ]]; then
  echo "Follow-up ledger detector failed [$LABEL]: file not found: $LEDGER_PATH" >&2
  exit 1
fi

awk -v label="$LABEL" '
  /^## Open[[:space:]]*$/ {
    section = "open"
    next
  }
  /^## Closed[[:space:]]*$/ {
    section = "closed"
    next
  }
  /^```yaml[[:space:]]*$/ {
    in_yaml = 1
    next
  }
  in_yaml && /^```[[:space:]]*$/ {
    in_yaml = 0
    next
  }
  in_yaml && /^- lane_id:[[:space:]]*/ {
    total++
    current_entry = total
    if (section == "open") {
      open_entries++
    }
    if ($0 ~ /^- lane_id:[[:space:]]*checklists\//) {
      checklist_lane_ids++
      lane_id = $0
      sub(/^- lane_id:[[:space:]]*/, "", lane_id)
      printf("Follow-up ledger violation [%s]: entry=%d line=%d checklist lane_id=%s\n", label, current_entry, NR, lane_id) > "/dev/stderr"
    }
    next
  }
  in_yaml && /^[[:space:]]*ended_at:[[:space:]]*unknown[[:space:]]*$/ {
    unknown_ended_at++
    printf("Follow-up ledger violation [%s]: entry=%d line=%d ended_at=unknown\n", label, current_entry, NR) > "/dev/stderr"
  }
  END {
    if (total == 0) {
      printf("Follow-up ledger summary [%s]: total=0 open=0 checklist_lane_ids=0 unknown_ended_at=0 status=VACUOUS\n", label)
      exit 1
    }
    status = (checklist_lane_ids > 0 || unknown_ended_at > 0) ? "FAIL" : "PASS"
    printf("Follow-up ledger summary [%s]: total=%d open=%d checklist_lane_ids=%d unknown_ended_at=%d status=%s\n", label, total, open_entries, checklist_lane_ids, unknown_ended_at, status)
    if (status == "FAIL") {
      exit 1
    }
  }
' "$LEDGER_PATH"
