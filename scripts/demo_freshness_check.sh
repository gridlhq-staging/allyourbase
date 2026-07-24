#!/usr/bin/env bash
set -euo pipefail

if ! gh_output="$(gh run list -R AllyourbaseHQ/allyourbase --workflow=cross_demo_live.yml --status completed --limit 1 --json conclusion,updatedAt 2>&1)"; then
  printf 'gh run list failed for cross_demo_live.yml freshness check: %s\n' "$gh_output" >&2
  exit 1
fi

python3 - "$gh_output" <<'PY'
import json
import sys
from datetime import datetime, timezone

max_age_seconds = 26 * 60 * 60

try:
    runs = json.loads(sys.argv[1])
except json.JSONDecodeError as error:
    print(f"malformed gh run list JSON: {error}", file=sys.stderr)
    sys.exit(1)

if not isinstance(runs, list) or not runs:
    print("no completed cross_demo_live.yml runs found", file=sys.stderr)
    sys.exit(1)

run = runs[0]
if not isinstance(run, dict):
    print("malformed cross_demo_live.yml run entry", file=sys.stderr)
    sys.exit(1)

conclusion = run.get("conclusion")
if not conclusion:
    print("missing conclusion on latest completed cross_demo_live.yml run", file=sys.stderr)
    sys.exit(1)
if conclusion != "success":
    print(f"latest completed cross_demo_live.yml run conclusion was {conclusion}", file=sys.stderr)
    sys.exit(1)

updated_at = run.get("updatedAt")
if not updated_at:
    print("missing updatedAt on latest completed cross_demo_live.yml run", file=sys.stderr)
    sys.exit(1)

try:
    parsed_updated_at = datetime.fromisoformat(updated_at.replace("Z", "+00:00"))
except ValueError:
    print(f"malformed updatedAt on latest completed cross_demo_live.yml run: {updated_at}", file=sys.stderr)
    sys.exit(1)

if parsed_updated_at.tzinfo is None:
    print(f"malformed updatedAt on latest completed cross_demo_live.yml run: {updated_at}", file=sys.stderr)
    sys.exit(1)

age_seconds = (datetime.now(timezone.utc) - parsed_updated_at.astimezone(timezone.utc)).total_seconds()
if age_seconds > max_age_seconds:
    print(
        f"stale successful cross_demo_live.yml run: updatedAt {updated_at} is older than 26 hours",
        file=sys.stderr,
    )
    sys.exit(1)

print(f"fresh successful cross_demo_live.yml run: updatedAt {updated_at}")
PY
