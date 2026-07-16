#!/usr/bin/env bash
set -euo pipefail

DIST="${CHECK_UI_BUNDLE_DIST:-ui/dist}"

# Stage 4 production budget: 752,776-byte immutable Stage 3 entry measurement
# (`ui/dist/assets/index-Cy91MTnC.js`, rebuilt at HEAD e8d6410e... with
# `cd ui && pnpm install --frozen-lockfile && pnpm build`) + 0 bytes headroom.
# Lane `jul14_pm_7` union artifacts were not present in this worktree; using
# zero headroom is biased toward catching any routing-union growth and requires
# a measured re-baseline through the merge queue if that later lane exceeds it.
DEFAULT_BUDGET=752776
BUDGET="${CHECK_UI_BUNDLE_BUDGET:-$DEFAULT_BUDGET}"

if [[ ! "$BUDGET" =~ ^[0-9]+$ ]]; then
	printf 'UI bundle size budget must be digits only, got %s.\n' "$BUDGET"
	exit 1
fi

python3 - "$DIST" "$BUDGET" <<'PY'
from __future__ import annotations

import os
import sys
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit


class ModuleScriptParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.module_srcs: list[str | None] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag.lower() != "script":
            return
        attr_map = {name.lower(): value for name, value in attrs if value is not None}
        if attr_map.get("type", "").lower() == "module":
            self.module_srcs.append(attr_map.get("src"))


def fail(message: str) -> None:
    print(message)
    sys.exit(1)


def resolved_inside(candidate: Path, dist: Path) -> Path | None:
    try:
        resolved = candidate.resolve(strict=True)
        resolved.relative_to(dist)
    except (OSError, ValueError):
        return None
    if not resolved.is_file():
        return None
    return resolved


def candidate_paths(dist: Path, entry_url: str) -> list[Path]:
    parsed = urlsplit(entry_url)
    if parsed.scheme or parsed.netloc:
        return []
    url_path = unquote(parsed.path)
    relative_path = Path(url_path.lstrip("/"))
    candidates = [dist / relative_path]
    parts = relative_path.parts
    if url_path.startswith("/") and len(parts) > 1:
        candidates.append(dist.joinpath(*parts[1:]))
    return candidates


def resolve_entry(dist: Path, entry_url: str) -> Path | None:
    for candidate in candidate_paths(dist, entry_url):
        resolved = resolved_inside(candidate, dist)
        if resolved is not None:
            return resolved
    return None


def main() -> None:
    dist = Path(sys.argv[1]).resolve(strict=False)
    budget = int(sys.argv[2])
    index_path = dist / "index.html"

    try:
        index_html = index_path.read_text(encoding="utf-8")
    except OSError as exc:
        fail(f"Unable to read {index_path}: {exc}.")

    parser = ModuleScriptParser()
    parser.feed(index_html)
    if len(parser.module_srcs) != 1:
        fail(f"Expected exactly one module script in {index_path}, found {len(parser.module_srcs)}.")

    entry_url = parser.module_srcs[0]
    if not entry_url:
        fail(f"Expected the sole module script in {index_path} to have a src attribute.")
    entry_path = resolve_entry(dist, entry_url)
    if entry_path is None:
        fail(f"Module entry {entry_url} did not resolve to a regular file under {dist}.")

    actual = os.path.getsize(entry_path)
    if actual > budget:
        print("UI bundle size guardrail failed.")
        print(f"Entry: {entry_path}")
        print(f"Actual bytes: {actual}")
        print(f"Budget bytes: {budget}")
        sys.exit(1)

    print("UI bundle size guardrail passed.")
    print(f"Entry: {entry_path}")
    print(f"Actual bytes: {actual}")
    print(f"Budget bytes: {budget}")


if __name__ == "__main__":
    main()
PY
