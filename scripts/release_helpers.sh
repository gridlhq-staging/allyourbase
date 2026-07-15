#!/bin/sh

# select_app_release_tag reads GitHub release-list JSON on stdin and prints the
# newest eligible app release tag. Eligibility intentionally matches launch
# checks: non-draft tags beginning with v followed by a digit.
# TODO: Document select_app_release_tag.
# Shared by scripts/adoption.sh and scripts/launch-check.sh so both stages use
# the same release-selection contract.
select_app_release_tag() {
  python3 -c '
import json, re, sys
try:
    releases = json.load(sys.stdin)
except Exception as exc:
    raise SystemExit(f"malformed release list: {exc}")
if not isinstance(releases, list):
    raise SystemExit("release list must be an array")
selected_tag = None
for index, release in enumerate(releases):
    if not isinstance(release, dict):
        raise SystemExit(f"release list member {index} must be an object")
    tag = release.get("tagName")
    if not isinstance(tag, str) or not tag:
        raise SystemExit(f"release list member {index}.tagName must be a non-empty string")
    is_draft = release.get("isDraft")
    if not isinstance(is_draft, bool):
        raise SystemExit(f"release list member {index}.isDraft must be a boolean")
    if selected_tag is None and not is_draft and re.match(r"^v[0-9]", tag):
        selected_tag = tag
if selected_tag is None:
    raise SystemExit("no non-draft app release")
print(selected_tag)
'
}
