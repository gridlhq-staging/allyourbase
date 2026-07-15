#!/usr/bin/env python3
"""Contract checks for the read-only weekly adoption workflow."""

from pathlib import Path
import re

import yaml


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = ROOT / ".github/workflows/adoption.yml"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def main():
    text = WORKFLOW_PATH.read_text(encoding="utf-8")
    workflow = yaml.load(text, Loader=yaml.BaseLoader)
    triggers = workflow.get("on", workflow.get(True, {}))
    require("workflow_dispatch" in triggers, "workflow_dispatch trigger missing")
    require("schedule" in triggers and triggers["schedule"], "weekly schedule missing")
    cron = triggers["schedule"][0]["cron"]
    require(len(cron.split()) == 5 and cron.split()[4] != "*", "schedule must be weekly")
    require(workflow.get("permissions") == {"contents": "read"}, "permissions must be contents: read only")

    require("actions/checkout@v6" in text, "checkout@v6 missing")
    require("persist-credentials: false" in text, "checkout credentials must not persist")
    require("run: make adoption" in text, "workflow must delegate to make adoption")
    require("ADOPTION_OUTPUT_PATH: ${{ runner.temp }}/adoption.md" in text, "runner-temp output missing")
    require("ADOPTION_REPOSITORY: ${{ github.repository }}" in text, "repository must be identity-agnostic")
    require("ADOPTION_TRAFFIC_TOKEN: ${{ secrets.ADOPTION_TRAFFIC_TOKEN }}" in text, "traffic token input missing")
    require("GITHUB_TOKEN: ${{ github.token }}" in text, "Actions token fallback missing")
    require("cat \"$RUNNER_TEMP/adoption.md\" >> \"$GITHUB_STEP_SUMMARY\"" in text, "step summary append missing")
    require("actions/upload-artifact@v7" in text, "artifact upload missing")
    require("path: ${{ runner.temp }}/adoption.md" in text, "artifact must upload runner-temp report")

    forbidden = r"\bgit\s+(commit|push)\b|\bgh\s+issue\b|debbie\s+sync|contents:\s*write"
    require(not re.search(forbidden, text, re.IGNORECASE), "workflow contains repository-write behavior")
    require("/traffic/" not in text, "workflow must not duplicate traffic token precedence")
    require("docs/live-state/" not in text, "workflow must not write repository live-state")
    print("PASS: adoption workflow contract")


if __name__ == "__main__":
    main()
