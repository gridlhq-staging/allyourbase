#!/usr/bin/env python3
"""Validate .github/workflows/test-installer.yml trigger configuration."""

import os
import sys

import yaml

WORKFLOW_PATH = os.path.join(
    os.path.dirname(__file__),
    "..",
    "..",
    ".github",
    "workflows",
    "test-installer.yml",
)


def load_triggers():
    with open(WORKFLOW_PATH) as f:
        data = yaml.safe_load(f)
    # PyYAML decodes bare `on:` as Python bool True, not the string "on"
    return data.get("on") or data.get(True)


def main():
    triggers = load_triggers()
    assert triggers is not None, "No 'on' trigger block found"

    assert "workflow_dispatch" in triggers, "Missing workflow_dispatch trigger"

    assert "push" in triggers, "Missing push trigger"
    push = triggers["push"]
    assert push.get("branches") == ["main"], (
        f"push.branches should be ['main'], got {push.get('branches')}"
    )
    expected_paths = {
        "install.sh",
        "tests/test_install.sh",
        ".github/workflows/test-installer.yml",
    }
    actual_paths = set(push.get("paths", []))
    assert expected_paths == actual_paths, (
        f"push.paths mismatch: expected {expected_paths}, got {actual_paths}"
    )

    assert "release" in triggers, "Missing release trigger"
    assert triggers["release"].get("types") == ["published"], (
        "release.types should be ['published'], "
        f"got {triggers['release'].get('types')}"
    )

    print("PASS: all test-installer.yml trigger assertions passed")


if __name__ == "__main__":
    try:
        main()
    except AssertionError as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
