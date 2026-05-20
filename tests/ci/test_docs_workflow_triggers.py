#!/usr/bin/env python3
"""Validate .github/workflows/docs.yml trigger configuration."""

import sys
import os
import yaml

WORKFLOW_PATH = os.path.join(
    os.path.dirname(__file__), "..", "..", ".github", "workflows", "docs.yml"
)

def load_triggers():
    with open(WORKFLOW_PATH) as f:
        d = yaml.safe_load(f)
    # PyYAML decodes bare `on:` as Python bool True, not the string "on"
    return d.get("on") or d.get(True)

def main():
    triggers = load_triggers()
    assert triggers is not None, "No 'on' trigger block found"

    # (a) workflow_dispatch must be present
    assert "workflow_dispatch" in triggers, "Missing workflow_dispatch trigger"

    # (b) push trigger with branches and paths
    assert "push" in triggers, "Missing push trigger"
    push = triggers["push"]
    assert push.get("branches") == ["main"], (
        f"push.branches should be ['main'], got {push.get('branches')}"
    )
    expected_paths = {"docs-site/**", "docs/**", "README.md", ".github/workflows/docs.yml"}
    actual_paths = set(push.get("paths", []))
    assert expected_paths == actual_paths, (
        f"push.paths mismatch: expected {expected_paths}, got {actual_paths}"
    )

    # (c) release trigger with types: [published]
    assert "release" in triggers, "Missing release trigger"
    assert triggers["release"].get("types") == ["published"], (
        f"release.types should be ['published'], got {triggers['release'].get('types')}"
    )

    print("PASS: all docs.yml trigger assertions passed")

if __name__ == "__main__":
    try:
        main()
    except AssertionError as e:
        print(f"FAIL: {e}", file=sys.stderr)
        sys.exit(1)
